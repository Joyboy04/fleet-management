package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/raihan/fleet-management/internal/api"
	"github.com/raihan/fleet-management/internal/auth"
	"github.com/raihan/fleet-management/internal/config"
	"github.com/raihan/fleet-management/internal/geofence"
	"github.com/raihan/fleet-management/internal/messaging"
	"github.com/raihan/fleet-management/internal/mqttclient"
	"github.com/raihan/fleet-management/internal/repository"
	"github.com/raihan/fleet-management/internal/service"
	"github.com/raihan/fleet-management/migrations"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
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
	log.Info("postgres connected")

	if err := repository.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}
	log.Info("database schema ready")

	locationRepo := repository.NewLocationRepository(pool)
	locationSvc := service.NewLocationService(locationRepo, log)
	authSvc := auth.NewService(cfg.Auth)

	eventPublisher := messaging.NewPublisher(cfg.RabbitMQ, log)
	defer eventPublisher.Close()

	zone := geofence.Zone{
		Name:      cfg.Geofence.Name,
		Latitude:  cfg.Geofence.Latitude,
		Longitude: cfg.Geofence.Longitude,
		RadiusM:   cfg.Geofence.RadiusM,
	}
	detector := geofence.NewDetector(zone)
	geofenceSvc := service.NewGeofenceService(zone, repository.NewEventRepository(pool))
	log.Info("geofence active",
		"name", cfg.Geofence.Name, "latitude", cfg.Geofence.Latitude,
		"longitude", cfg.Geofence.Longitude, "radius_m", cfg.Geofence.RadiusM)
	ingestSvc := service.NewIngestService(locationSvc, detector, eventPublisher, log)

	subscriber := mqttclient.NewSubscriber(cfg.MQTT, ingestSvc, log)
	subscriber.Start()
	defer subscriber.Stop()

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           api.NewRouter(locationSvc, geofenceSvc, authSvc, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	return srv.Shutdown(shutdownCtx)
}
