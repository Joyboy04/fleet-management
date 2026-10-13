// Package messaging connects the fleet services to RabbitMQ.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/raihan/fleet-management/internal/config"
	"github.com/raihan/fleet-management/internal/model"
)

const (
	contentTypeJSON = "application/json"
	dialTimeout     = 5 * time.Second
	heartbeat       = 10 * time.Second
	prefetchCount   = 10
	consumerTag     = "fleet-worker"
)

func dial(url string) (*amqp.Connection, error) {
	return amqp.DialConfig(url, amqp.Config{
		Heartbeat: heartbeat,
		Dial:      amqp.DefaultDial(dialTimeout),
	})
}

// declareTopology idempotently creates the exchange, queue and binding, so that either
// the API or the worker can start first.
func declareTopology(ch *amqp.Channel, cfg config.RabbitMQ) error {
	if err := ch.ExchangeDeclare(cfg.Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", cfg.Exchange, err)
	}
	if _, err := ch.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", cfg.Queue, err)
	}
	if err := ch.QueueBind(cfg.Queue, cfg.RoutingKey, cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %q: %w", cfg.Queue, err)
	}
	return nil
}

// Publisher publishes geofence events to RabbitMQ with publisher confirms.
// It connects lazily and reconnects automatically after a failure.
type Publisher struct {
	cfg  config.RabbitMQ
	log  *slog.Logger
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewPublisher creates a publisher. No connection is made until the first publish.
func NewPublisher(cfg config.RabbitMQ, log *slog.Logger) *Publisher {
	return &Publisher{cfg: cfg, log: log}
}

// PublishGeofenceEntry sends the event to the geofence exchange and waits for the broker's confirm.
func (p *Publisher) PublishGeofenceEntry(ctx context.Context, evt model.GeofenceEvent) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal geofence event: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureChannel(); err != nil {
		return err
	}

	confirm, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, p.cfg.Exchange, p.cfg.RoutingKey, false, false, amqp.Publishing{
		ContentType:  contentTypeJSON,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
	if err != nil {
		p.reset()
		return fmt.Errorf("publish geofence event: %w", err)
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		p.reset()
		return fmt.Errorf("wait for publish confirm: %w", err)
	}
	if !acked {
		return errors.New("broker rejected geofence event")
	}
	return nil
}

// Close releases the connection.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
}

// ensureChannel (re)establishes the connection and confirm-mode channel if needed. Caller holds p.mu.
func (p *Publisher) ensureChannel() error {
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	p.reset()

	conn, err := dial(p.cfg.URL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if err := declareTopology(ch, p.cfg); err != nil {
		_ = conn.Close()
		return err
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	p.conn, p.ch = conn, ch
	p.log.Info("rabbitmq publisher connected", "exchange", p.cfg.Exchange, "queue", p.cfg.Queue)
	return nil
}

// reset closes and forgets the current connection. Caller holds p.mu.
func (p *Publisher) reset() {
	if p.conn != nil && !p.conn.IsClosed() {
		_ = p.conn.Close()
	}
	p.conn, p.ch = nil, nil
}

// Handler processes one geofence event. Returning an error requeues the message.
type Handler func(ctx context.Context, evt model.GeofenceEvent) error

// Consumer reads geofence events from the geofence queue.
type Consumer struct {
	cfg config.RabbitMQ
	log *slog.Logger
}

// NewConsumer creates a consumer.
func NewConsumer(cfg config.RabbitMQ, log *slog.Logger) *Consumer {
	return &Consumer{cfg: cfg, log: log}
}

// Run connects and consumes until ctx is cancelled (returns nil) or the connection is lost
// (returns an error, so the caller can reconnect).
func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	conn, err := dial(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer ch.Close()

	if err := declareTopology(ch, c.cfg); err != nil {
		return err
	}
	if err := ch.Qos(prefetchCount, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}
	deliveries, err := ch.Consume(c.cfg.Queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %q: %w", c.cfg.Queue, err)
	}
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))

	c.log.Info("rabbitmq consumer ready", "queue", c.cfg.Queue)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-closed:
			return fmt.Errorf("rabbitmq connection closed: %v", err)
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			c.dispatch(ctx, d, handle)
		}
	}
}

func (c *Consumer) dispatch(ctx context.Context, d amqp.Delivery, handle Handler) {
	var evt model.GeofenceEvent
	if err := json.Unmarshal(d.Body, &evt); err != nil {
		c.log.Warn("dropping malformed geofence message", "error", err)
		_ = d.Nack(false, false)
		return
	}
	if err := handle(ctx, evt); err != nil {
		c.log.Error("handler failed, requeueing", "vehicle_id", evt.VehicleID, "error", err)
		_ = d.Nack(false, true)
		return
	}
	_ = d.Ack(false)
}
