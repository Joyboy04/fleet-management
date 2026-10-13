// Package geofence implements circular geofences and entry detection.
package geofence

import (
	"math"
	"sync"
)

const earthRadiusM = 6371000.0

// DistanceMeters returns the haversine distance in meters between two coordinates.
func DistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// Zone is a circular geofence.
type Zone struct {
	Name      string
	Latitude  float64
	Longitude float64
	RadiusM   float64
}

// Contains reports whether the coordinate lies inside (or on the edge of) the zone.
func (z Zone) Contains(lat, lon float64) bool {
	return DistanceMeters(z.Latitude, z.Longitude, lat, lon) <= z.RadiusM
}

// Detector tracks, per vehicle, whether it is inside a zone and reports entry transitions.
// It is safe for concurrent use.
type Detector struct {
	zone   Zone
	mu     sync.Mutex
	inside map[string]bool
}

// NewDetector creates a detector for a single zone.
func NewDetector(zone Zone) *Detector {
	return &Detector{zone: zone, inside: make(map[string]bool)}
}

// Entered records the vehicle's latest position and returns true only on the reading
// where it transitions from outside (or unknown) to inside the zone.
func (d *Detector) Entered(vehicleID string, lat, lon float64) bool {
	now := d.zone.Contains(lat, lon)

	d.mu.Lock()
	defer d.mu.Unlock()
	was := d.inside[vehicleID]
	d.inside[vehicleID] = now
	return now && !was
}
