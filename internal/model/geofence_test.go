package model

import (
	"encoding/json"
	"testing"
)

func TestGeofenceEventJSONMatchesSpec(t *testing.T) {
	evt := NewGeofenceEntry(Location{VehicleID: "B1234XYZ", Latitude: -6.2088, Longitude: 106.8456, Timestamp: 1715003456})
	got, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"vehicle_id":"B1234XYZ","event":"geofence_entry","location":{"latitude":-6.2088,"longitude":106.8456},"timestamp":1715003456}`
	if string(got) != want {
		t.Fatalf("event json = %s, want %s", got, want)
	}
}
