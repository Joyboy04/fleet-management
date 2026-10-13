package service

import (
	"context"

	"github.com/raihan/fleet-management/internal/geofence"
	"github.com/raihan/fleet-management/internal/model"
)

// EventStore reads geofence events persisted by the worker.
type EventStore interface {
	Recent(ctx context.Context, limit int) ([]model.GeofenceEventRecord, error)
}

// GeofenceService exposes the configured geofence and its recent events.
type GeofenceService struct {
	zone  geofence.Zone
	store EventStore
}

// NewGeofenceService wires the service with its dependencies.
func NewGeofenceService(zone geofence.Zone, store EventStore) *GeofenceService {
	return &GeofenceService{zone: zone, store: store}
}

// Zone returns the configured geofence.
func (s *GeofenceService) Zone() geofence.Zone { return s.zone }

// RecentEvents returns up to limit events, newest first.
func (s *GeofenceService) RecentEvents(ctx context.Context, limit int) ([]model.GeofenceEventRecord, error) {
	return s.store.Recent(ctx, limit)
}
