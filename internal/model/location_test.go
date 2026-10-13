package model

import (
	"errors"
	"testing"
)

func TestParseLocation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{"valid spec example", `{"vehicle_id":"B1234XYZ","latitude":-6.2088,"longitude":106.8456,"timestamp":1715003456}`, false},
		{"zero lat/lon is valid", `{"vehicle_id":"B1","latitude":0,"longitude":0,"timestamp":1}`, false},
		{"malformed json", `{nope`, true},
		{"missing vehicle_id", `{"latitude":1,"longitude":1,"timestamp":1}`, true},
		{"missing latitude", `{"vehicle_id":"B1","longitude":1,"timestamp":1}`, true},
		{"missing longitude", `{"vehicle_id":"B1","latitude":1,"timestamp":1}`, true},
		{"missing timestamp", `{"vehicle_id":"B1","latitude":1,"longitude":1}`, true},
		{"latitude out of range", `{"vehicle_id":"B1","latitude":91,"longitude":1,"timestamp":1}`, true},
		{"longitude out of range", `{"vehicle_id":"B1","latitude":1,"longitude":181,"timestamp":1}`, true},
		{"bad vehicle id chars", `{"vehicle_id":"B 1/2","latitude":1,"longitude":1,"timestamp":1}`, true},
		{"empty vehicle id", `{"vehicle_id":"","latitude":1,"longitude":1,"timestamp":1}`, true},
		{"timestamp zero", `{"vehicle_id":"B1","latitude":1,"longitude":1,"timestamp":0}`, true},
		{"timestamp wrong type", `{"vehicle_id":"B1","latitude":1,"longitude":1,"timestamp":"abc"}`, true},
		{"latitude string", `{"vehicle_id":"B1","latitude":"x","longitude":1,"timestamp":1}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseLocation([]byte(tt.payload))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidLocation) {
				t.Fatalf("error should wrap ErrInvalidLocation: %v", err)
			}
			if err == nil && loc.VehicleID == "" {
				t.Fatal("expected populated location")
			}
		})
	}
}
