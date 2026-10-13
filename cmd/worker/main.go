// Command worker consumes geofence events from the geofence_alerts queue and records them in PostgreSQL.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/raihan/fleet-management/internal/config"
	"github.com/raihan/fleet-management/internal/messaging"
	"github.com/raihan/fleet-management/internal/model"
	"github.com/raihan/fleet-management/internal/repository"
)

const reconnectDelay = 3 * time.Second

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := repository.WaitForPostgres(ctx, pool, log); err != nil {
		return err
	}
	events := repository.NewEventRepository(pool)

	consumer := messaging.NewConsumer(cfg.RabbitMQ, log)
	for {
		err := consumer.Run(ctx, handleEvent(events, log))
		if ctx.Err() != nil {
			log.Info("worker stopped")
			return nil
		}
		log.Warn("consumer stopped, reconnecting", "error", err, "delay", reconnectDelay)
		select {
		case <-ctx.Done():
			log.Info("worker stopped")
			return nil
		case <-time.After(reconnectDelay):
		}
	}
}

// handleEvent logs each alert and records it so the dashboard can show it. A failed insert
// returns an error, which makes the consumer requeue the message.
func handleEvent(events *repository.EventRepository, log *slog.Logger) messaging.Handler {
	return func(ctx context.Context, evt model.GeofenceEvent) error {
		log.Info("geofence alert",
			"vehicle_id", evt.VehicleID,
			"event", evt.Event,
			"latitude", evt.Location.Latitude,
			"longitude", evt.Location.Longitude,
			"timestamp", evt.Timestamp)

		insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return events.Insert(insertCtx, evt)
	}
}
