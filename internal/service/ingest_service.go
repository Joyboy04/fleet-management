package service

import (
	"context"
	"log/slog"

	"github.com/raihan/fleet-management/internal/model"
)

// EntryDetector reports whether a reading is a vehicle's entry into the geofence.
type EntryDetector interface {
	Entered(vehicleID string, lat, lon float64) bool
}

// EventPublisher sends geofence events to the message broker.
type EventPublisher interface {
	PublishGeofenceEntry(ctx context.Context, evt model.GeofenceEvent) error
}

// IngestService is the MQTT ingestion pipeline: store the reading, then evaluate the geofence.
type IngestService struct {
	locations *LocationService
	detector  EntryDetector
	events    EventPublisher
	log       *slog.Logger
}

// NewIngestService wires the ingestion pipeline.
func NewIngestService(locations *LocationService, detector EntryDetector, events EventPublisher, log *slog.Logger) *IngestService {
	return &IngestService{locations: locations, detector: detector, events: events, log: log}
}

// Ingest persists the reading and publishes a geofence_entry event if the vehicle just entered the zone.
// A failed event publish is logged but does not fail ingestion: the location is already safely stored.
func (s *IngestService) Ingest(ctx context.Context, loc model.Location) error {
	if err := s.locations.Ingest(ctx, loc); err != nil {
		return err
	}
	if !s.detector.Entered(loc.VehicleID, loc.Latitude, loc.Longitude) {
		return nil
	}

	evt := model.NewGeofenceEntry(loc)
	if err := s.events.PublishGeofenceEntry(ctx, evt); err != nil {
		s.log.Error("publish geofence event failed", "vehicle_id", loc.VehicleID, "error", err)
		return nil
	}
	s.log.Info("geofence entry published", "vehicle_id", loc.VehicleID, "latitude", loc.Latitude, "longitude", loc.Longitude)
	return nil
}
