// Package model holds the domain types shared across the fleet services.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidLocation is returned (wrapped) whenever a location payload fails validation.
var ErrInvalidLocation = errors.New("invalid location")

// ErrNotFound is returned when no location exists for the requested vehicle.
var ErrNotFound = errors.New("not found")

var vehicleIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ValidVehicleID reports whether id is an acceptable vehicle identifier.
func ValidVehicleID(id string) bool {
	return vehicleIDPattern.MatchString(id)
}

// Location is a single GPS reading reported by a vehicle.
type Location struct {
	VehicleID string  `json:"vehicle_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp int64   `json:"timestamp"` // unix seconds
}

// locationPayload mirrors the wire format. Pointers let us tell "missing" apart from a zero value.
type locationPayload struct {
	VehicleID *string  `json:"vehicle_id"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Timestamp *int64   `json:"timestamp"`
}

// ParseLocation decodes and validates a raw MQTT payload.
func ParseLocation(data []byte) (Location, error) {
	var p locationPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return Location{}, fmt.Errorf("%w: malformed json: %v", ErrInvalidLocation, err)
	}

	switch {
	case p.VehicleID == nil:
		return Location{}, fmt.Errorf("%w: vehicle_id is required", ErrInvalidLocation)
	case p.Latitude == nil:
		return Location{}, fmt.Errorf("%w: latitude is required", ErrInvalidLocation)
	case p.Longitude == nil:
		return Location{}, fmt.Errorf("%w: longitude is required", ErrInvalidLocation)
	case p.Timestamp == nil:
		return Location{}, fmt.Errorf("%w: timestamp is required", ErrInvalidLocation)
	}

	loc := Location{
		VehicleID: *p.VehicleID,
		Latitude:  *p.Latitude,
		Longitude: *p.Longitude,
		Timestamp: *p.Timestamp,
	}
	if err := loc.Validate(); err != nil {
		return Location{}, err
	}
	return loc, nil
}

// Validate checks value ranges.
func (l Location) Validate() error {
	if !ValidVehicleID(l.VehicleID) {
		return fmt.Errorf("%w: vehicle_id must match %s", ErrInvalidLocation, vehicleIDPattern)
	}
	if l.Latitude < -90 || l.Latitude > 90 {
		return fmt.Errorf("%w: latitude must be between -90 and 90", ErrInvalidLocation)
	}
	if l.Longitude < -180 || l.Longitude > 180 {
		return fmt.Errorf("%w: longitude must be between -180 and 180", ErrInvalidLocation)
	}
	if l.Timestamp <= 0 {
		return fmt.Errorf("%w: timestamp must be a positive unix time", ErrInvalidLocation)
	}
	return nil
}
