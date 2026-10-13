// Package mqttclient connects the service to the MQTT broker.
package mqttclient

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/raihan/fleet-management/internal/config"
	"github.com/raihan/fleet-management/internal/model"
)

const (
	subscribeQoS   byte = 1
	handlerTimeout      = 5 * time.Second
)

// Ingestor consumes validated locations.
type Ingestor interface {
	Ingest(ctx context.Context, loc model.Location) error
}

// Subscriber listens to vehicle location topics and forwards valid payloads to an Ingestor.
type Subscriber struct {
	client paho.Client
	topic  string
	ingest Ingestor
	log    *slog.Logger
}

// NewSubscriber builds a subscriber. Call Start to connect.
func NewSubscriber(cfg config.MQTT, ingest Ingestor, log *slog.Logger) *Subscriber {
	s := &Subscriber{topic: cfg.Topic, ingest: ingest, log: log}

	opts := paho.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(cfg.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(3 * time.Second).
		SetOrderMatters(false).
		SetOnConnectHandler(s.onConnect).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Warn("mqtt connection lost", "error", err)
		})

	s.client = paho.NewClient(opts)
	return s
}

// Start begins connecting in the background; it retries until the broker is reachable.
func (s *Subscriber) Start() {
	s.log.Info("mqtt connecting")
	s.client.Connect()
}

// Stop disconnects from the broker, letting in-flight work finish for up to 250 ms.
func (s *Subscriber) Stop() {
	s.client.Disconnect(250)
}

// onConnect (re)subscribes on every successful connection, including reconnects.
func (s *Subscriber) onConnect(c paho.Client) {
	s.log.Info("mqtt connected", "topic", s.topic)
	token := c.Subscribe(s.topic, subscribeQoS, s.handleMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		s.log.Error("mqtt subscribe failed", "topic", s.topic, "error", err)
	}
}

func (s *Subscriber) handleMessage(_ paho.Client, msg paho.Message) {
	topicVehicleID, err := vehicleIDFromTopic(msg.Topic())
	if err != nil {
		s.log.Warn("mqtt message dropped", "topic", msg.Topic(), "reason", err)
		return
	}

	loc, err := model.ParseLocation(msg.Payload())
	if err != nil {
		s.log.Warn("mqtt message dropped", "topic", msg.Topic(), "reason", err)
		return
	}

	if loc.VehicleID != topicVehicleID {
		s.log.Warn("mqtt message dropped",
			"topic", msg.Topic(), "reason", "vehicle_id in payload does not match topic", "payload_vehicle_id", loc.VehicleID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()
	if err := s.ingest.Ingest(ctx, loc); err != nil {
		s.log.Error("ingest location failed", "vehicle_id", loc.VehicleID, "error", err)
	}
}

// vehicleIDFromTopic extracts {vehicle_id} from /fleet/vehicle/{vehicle_id}/location.
func vehicleIDFromTopic(topic string) (string, error) {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) != 4 || parts[0] != "fleet" || parts[1] != "vehicle" || parts[3] != "location" || parts[2] == "" {
		return "", errors.New("unexpected topic format")
	}
	return parts[2], nil
}
