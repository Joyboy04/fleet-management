package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/raihan/fleet-management/internal/model"
)

type stubStore struct {
	inserted []model.Location
	err      error
}

func (s *stubStore) Insert(_ context.Context, loc model.Location) error {
	if s.err != nil {
		return s.err
	}
	s.inserted = append(s.inserted, loc)
	return nil
}
func (s *stubStore) Latest(context.Context, string) (model.Location, error) {
	return model.Location{}, model.ErrNotFound
}
func (s *stubStore) History(context.Context, string, int64, int64) ([]model.Location, error) {
	return nil, nil
}
func (s *stubStore) Vehicles(context.Context) ([]string, error) { return nil, nil }

type stubDetector struct{ entered bool }

func (d stubDetector) Entered(string, float64, float64) bool { return d.entered }

type stubPublisher struct {
	events []model.GeofenceEvent
	err    error
}

func (p *stubPublisher) PublishGeofenceEntry(_ context.Context, evt model.GeofenceEvent) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, evt)
	return nil
}

func newIngest(store *stubStore, entered bool, pub *stubPublisher) *IngestService {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewIngestService(NewLocationService(store, log), stubDetector{entered: entered}, pub, log)
}

var sampleLoc = model.Location{VehicleID: "B1234XYZ", Latitude: -6.2088, Longitude: 106.8456, Timestamp: 1715003456}

func TestIngestStoresAndPublishesOnEntry(t *testing.T) {
	store, pub := &stubStore{}, &stubPublisher{}
	if err := newIngest(store, true, pub).Ingest(context.Background(), sampleLoc); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("stored %d readings, want 1", len(store.inserted))
	}
	if len(pub.events) != 1 || pub.events[0] != model.NewGeofenceEntry(sampleLoc) {
		t.Fatalf("unexpected events: %+v", pub.events)
	}
}

func TestIngestDoesNotPublishWithoutEntry(t *testing.T) {
	store, pub := &stubStore{}, &stubPublisher{}
	if err := newIngest(store, false, pub).Ingest(context.Background(), sampleLoc); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 1 || len(pub.events) != 0 {
		t.Fatalf("stored=%d events=%d, want 1 and 0", len(store.inserted), len(pub.events))
	}
}

func TestIngestSurvivesPublishFailure(t *testing.T) {
	store, pub := &stubStore{}, &stubPublisher{err: errors.New("broker down")}
	if err := newIngest(store, true, pub).Ingest(context.Background(), sampleLoc); err != nil {
		t.Fatalf("publish failure must not fail ingestion: %v", err)
	}
	if len(store.inserted) != 1 {
		t.Fatal("reading must still be stored")
	}
}

func TestIngestStoreFailureSkipsGeofence(t *testing.T) {
	store, pub := &stubStore{err: errors.New("db down")}, &stubPublisher{}
	if err := newIngest(store, true, pub).Ingest(context.Background(), sampleLoc); err == nil {
		t.Fatal("expected store error")
	}
	if len(pub.events) != 0 {
		t.Fatal("no event may be published when the reading was not stored")
	}
}
