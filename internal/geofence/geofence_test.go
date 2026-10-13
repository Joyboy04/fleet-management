package geofence

import (
	"math"
	"testing"
)

var monas = Zone{Name: "Monas", Latitude: -6.1754, Longitude: 106.8272, RadiusM: 50}

func TestDistanceMeters(t *testing.T) {
	if d := DistanceMeters(-6.1754, 106.8272, -6.1754, 106.8272); d != 0 {
		t.Fatalf("same point distance = %v, want 0", d)
	}
	// 0.001 degree of latitude is ~111.2 m.
	d := DistanceMeters(0, 0, 0.001, 0)
	if math.Abs(d-111.19) > 0.5 {
		t.Fatalf("distance = %v, want ~111.19", d)
	}
}

func TestZoneContains(t *testing.T) {
	if !monas.Contains(-6.1754, 106.8272) {
		t.Fatal("center must be inside")
	}
	if !monas.Contains(-6.17565, 106.8272) { // ~28 m south
		t.Fatal("point ~28 m away must be inside")
	}
	if monas.Contains(-6.1760, 106.8272) { // ~67 m south
		t.Fatal("point ~67 m away must be outside")
	}
}

func TestDetectorEntersOncePerVisit(t *testing.T) {
	d := NewDetector(monas)
	steps := []struct {
		lat, lon float64
		want     bool
	}{
		{-6.1770, 106.8272, false}, // outside
		{-6.1757, 106.8272, true},  // enters
		{-6.1755, 106.8272, false}, // still inside
		{-6.1790, 106.8272, false}, // leaves
		{-6.1756, 106.8272, true},  // re-enters
	}
	for i, s := range steps {
		if got := d.Entered("B1", s.lat, s.lon); got != s.want {
			t.Fatalf("step %d: Entered = %v, want %v", i, got, s.want)
		}
	}
}

func TestDetectorFirstReadingInsideCountsAsEntry(t *testing.T) {
	d := NewDetector(monas)
	if !d.Entered("B1", -6.1754, 106.8272) {
		t.Fatal("first reading inside the zone must count as an entry")
	}
}

func TestDetectorTracksVehiclesIndependently(t *testing.T) {
	d := NewDetector(monas)
	if !d.Entered("A", -6.1754, 106.8272) {
		t.Fatal("A should enter")
	}
	if !d.Entered("B", -6.1754, 106.8272) {
		t.Fatal("B should enter independently of A")
	}
}
