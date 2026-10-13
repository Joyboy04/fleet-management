// Package service contains the business logic of the fleet backend.
package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/raihan/fleet-management/internal/model"
)

var (
	// ErrInvalidVehicleID is returned when a vehicle id has an unsupported format.
	ErrInvalidVehicleID = errors.New("invalid vehicle_id")
	// ErrInvalidRange is returned when a history time range is malformed.
	ErrInvalidRange = errors.New("invalid time range: start and end must be positive unix seconds with start <= end")
)

// LocationStore is the persistence contract needed by LocationService.
type LocationStore interface {
	Insert(ctx context.Context, loc model.Location) error
	Latest(ctx context.Context, vehicleID string) (model.Location, error)
	History(ctx context.Context, vehicleID string, start, end int64) ([]model.Location, error)
	Vehicles(ctx context.Context) ([]string, error)
}

// LocationService handles incoming and queried vehicle locations.
type LocationService struct {
	store LocationStore
	log   *slog.Logger
}

// NewLocationService wires the service with its dependencies.
func NewLocationService(store LocationStore, log *slog.Logger) *LocationService {
	return &LocationService{store: store, log: log}
}

// Ingest stores a validated location reading.
func (s *LocationService) Ingest(ctx context.Context, loc model.Location) error {
	if err := s.store.Insert(ctx, loc); err != nil {
		return err
	}
	s.log.Debug("location stored", "vehicle_id", loc.VehicleID, "timestamp", loc.Timestamp)
	return nil
}

// Latest returns the last known location of a vehicle.
func (s *LocationService) Latest(ctx context.Context, vehicleID string) (model.Location, error) {
	if !model.ValidVehicleID(vehicleID) {
		return model.Location{}, ErrInvalidVehicleID
	}
	return s.store.Latest(ctx, vehicleID)
}

// Vehicles returns the ids of all vehicles that have reported a location.
func (s *LocationService) Vehicles(ctx context.Context) ([]string, error) {
	return s.store.Vehicles(ctx)
}

// History returns a vehicle's locations between start and end (unix seconds, inclusive).
func (s *LocationService) History(ctx context.Context, vehicleID string, start, end int64) ([]model.Location, error) {
	if !model.ValidVehicleID(vehicleID) {
		return nil, ErrInvalidVehicleID
	}
	if start <= 0 || end <= 0 || start > end {
		return nil, ErrInvalidRange
	}
	return s.store.History(ctx, vehicleID, start, end)
}
