// Package repository contains PostgreSQL data access.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/raihan/fleet-management/internal/model"
)

// historyLimit caps the number of rows returned by a single history query.
const historyLimit = 10000

// LocationRepository persists vehicle locations in the vehicle_loctions table.
type LocationRepository struct {
	pool *pgxpool.Pool
}

// NewLocationRepository creates a repository backed by the given pool.
func NewLocationRepository(pool *pgxpool.Pool) *LocationRepository {
	return &LocationRepository{pool: pool}
}

// Insert stores one location reading.
func (r *LocationRepository) Insert(ctx context.Context, loc model.Location) error {
	const query = `
		INSERT INTO vehicle_loctions (vehicle_id, latitude, longitude, "timestamp")
		VALUES ($1, $2, $3, $4)`

	if _, err := r.pool.Exec(ctx, query, loc.VehicleID, loc.Latitude, loc.Longitude, loc.Timestamp); err != nil {
		return fmt.Errorf("insert vehicle location: %w", err)
	}
	return nil
}

// Latest returns the most recent reading of a vehicle, or model.ErrNotFound.
func (r *LocationRepository) Latest(ctx context.Context, vehicleID string) (model.Location, error) {
	const query = `
		SELECT vehicle_id, latitude, longitude, "timestamp"
		FROM vehicle_loctions
		WHERE vehicle_id = $1
		ORDER BY "timestamp" DESC, id DESC
		LIMIT 1`

	var loc model.Location
	err := r.pool.QueryRow(ctx, query, vehicleID).Scan(&loc.VehicleID, &loc.Latitude, &loc.Longitude, &loc.Timestamp)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Location{}, model.ErrNotFound
	}
	if err != nil {
		return model.Location{}, fmt.Errorf("query latest location: %w", err)
	}
	return loc, nil
}

// History returns readings with start <= timestamp <= end, oldest first.
func (r *LocationRepository) History(ctx context.Context, vehicleID string, start, end int64) ([]model.Location, error) {
	const query = `
		SELECT vehicle_id, latitude, longitude, "timestamp"
		FROM vehicle_loctions
		WHERE vehicle_id = $1 AND "timestamp" BETWEEN $2 AND $3
		ORDER BY "timestamp" ASC, id ASC
		LIMIT $4`

	rows, err := r.pool.Query(ctx, query, vehicleID, start, end, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("query location history: %w", err)
	}
	defer rows.Close()

	locs := make([]model.Location, 0)
	for rows.Next() {
		var loc model.Location
		if err := rows.Scan(&loc.VehicleID, &loc.Latitude, &loc.Longitude, &loc.Timestamp); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		locs = append(locs, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate locations: %w", err)
	}
	return locs, nil
}

// Vehicles returns the distinct ids of vehicles that have reported at least one location.
func (r *LocationRepository) Vehicles(ctx context.Context) ([]string, error) {
	const query = `SELECT DISTINCT vehicle_id FROM vehicle_loctions ORDER BY vehicle_id`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query vehicles: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan vehicle id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vehicles: %w", err)
	}
	return ids, nil
}
