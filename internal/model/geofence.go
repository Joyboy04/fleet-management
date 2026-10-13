package model

import "time"

// EventGeofenceEntry is the event name published when a vehicle enters a geofence.
const EventGeofenceEntry = "geofence_entry"

// Coordinates is a latitude/longitude pair.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GeofenceEvent is the message sent to RabbitMQ when a vehicle enters a geofence.
type GeofenceEvent struct {
	VehicleID string      `json:"vehicle_id"`
	Event     string      `json:"event"`
	Location  Coordinates `json:"location"`
	Timestamp int64       `json:"timestamp"` // unix seconds, taken from the triggering reading
}

// NewGeofenceEntry builds the entry event for the reading that triggered it.
func NewGeofenceEntry(loc Location) GeofenceEvent {
	return GeofenceEvent{
		VehicleID: loc.VehicleID,
		Event:     EventGeofenceEntry,
		Location:  Coordinates{Latitude: loc.Latitude, Longitude: loc.Longitude},
		Timestamp: loc.Timestamp,
	}
}

// GeofenceEventRecord is a geofence event as stored by the worker after consuming it from RabbitMQ.
type GeofenceEventRecord struct {
	ID         int64       `json:"id"`
	VehicleID  string      `json:"vehicle_id"`
	Event      string      `json:"event"`
	Location   Coordinates `json:"location"`
	Timestamp  int64       `json:"timestamp"`
	ReceivedAt time.Time   `json:"received_at"`
}
