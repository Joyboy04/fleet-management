package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/raihan/fleet-management/internal/model"
)

// EventRepository persists geofence events received by the worker.
type EventRepository struct {
	pool *pgxpool.Pool
}

// NewEventRepository creates a repository backed by the given pool.
func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// Insert stores one geofence event.
func (r *EventRepository) Insert(ctx context.Context, evt model.GeofenceEvent) error {
	const query = `
		INSERT INTO geofence_events (vehicle_id, event, latitude, longitude, "timestamp")
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := r.pool.Exec(ctx, query, evt.VehicleID, evt.Event, evt.Location.Latitude, evt.Location.Longitude, evt.Timestamp); err != nil {
		return fmt.Errorf("insert geofence event: %w", err)
	}
	return nil
}

// Recent returns the latest events, newest first.
func (r *EventRepository) Recent(ctx context.Context, limit int) ([]model.GeofenceEventRecord, error) {
	const query = `
		SELECT id, vehicle_id, event, latitude, longitude, "timestamp", received_at
		FROM geofence_events
		ORDER BY id DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query geofence events: %w", err)
	}
	defer rows.Close()

	events := make([]model.GeofenceEventRecord, 0)
	for rows.Next() {
		var e model.GeofenceEventRecord
		if err := rows.Scan(&e.ID, &e.VehicleID, &e.Event, &e.Location.Latitude, &e.Location.Longitude, &e.Timestamp, &e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan geofence event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate geofence events: %w", err)
	}
	return events, nil
}
