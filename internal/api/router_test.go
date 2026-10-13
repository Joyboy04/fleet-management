package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/raihan/fleet-management/internal/auth"
	"github.com/raihan/fleet-management/internal/config"
	"github.com/raihan/fleet-management/internal/geofence"
	"github.com/raihan/fleet-management/internal/model"
	"github.com/raihan/fleet-management/internal/service"
)

type fakeStore struct{ locs []model.Location }

func (f *fakeStore) Insert(context.Context, model.Location) error { return nil }

func (f *fakeStore) Latest(_ context.Context, id string) (model.Location, error) {
	var best *model.Location
	for i := range f.locs {
		if f.locs[i].VehicleID == id && (best == nil || f.locs[i].Timestamp > best.Timestamp) {
			best = &f.locs[i]
		}
	}
	if best == nil {
		return model.Location{}, model.ErrNotFound
	}
	return *best, nil
}

func (f *fakeStore) History(_ context.Context, id string, start, end int64) ([]model.Location, error) {
	out := make([]model.Location, 0)
	for _, l := range f.locs {
		if l.VehicleID == id && l.Timestamp >= start && l.Timestamp <= end {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeStore) Vehicles(context.Context) ([]string, error) { return []string{"B1234XYZ"}, nil }

type fakeEvents struct{}

func (fakeEvents) Recent(context.Context, int) ([]model.GeofenceEventRecord, error) {
	return []model.GeofenceEventRecord{{ID: 1, VehicleID: "B1234XYZ", Event: model.EventGeofenceEntry}}, nil
}

func newTestServer() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &fakeStore{locs: []model.Location{
		{VehicleID: "B1234XYZ", Latitude: -6.2088, Longitude: 106.8456, Timestamp: 1715003456},
		{VehicleID: "B1234XYZ", Latitude: -6.2090, Longitude: 106.8460, Timestamp: 1715003458},
		{VehicleID: "B1234XYZ", Latitude: -6.2100, Longitude: 106.8470, Timestamp: 1715009000},
	}}
	tokens := auth.NewService(config.Auth{ClientID: "c", ClientSecret: "s", JWTSecret: "j", TokenTTL: time.Hour})
	zone := geofence.Zone{Name: "Monas", Latitude: -6.1754, Longitude: 106.8272, RadiusM: 50}
	return NewRouter(service.NewLocationService(store, log), service.NewGeofenceService(zone, fakeEvents{}), tokens, log)
}

func do(h http.Handler, method, path, bearer string, form url.Values) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func getToken(t *testing.T, h http.Handler) string {
	t.Helper()
	w := do(h, http.MethodPost, "/oauth/token", "", url.Values{
		"grant_type": {"client_credentials"}, "client_id": {"c"}, "client_secret": {"s"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("token status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken == "" || resp.TokenType != "Bearer" || resp.ExpiresIn != 3600 {
		t.Fatalf("unexpected token response: %+v", resp)
	}
	return resp.AccessToken
}

func TestTokenEndpoint(t *testing.T) {
	h := newTestServer()
	_ = getToken(t, h)

	tests := []struct {
		name string
		form url.Values
		want int
	}{
		{"wrong secret", url.Values{"grant_type": {"client_credentials"}, "client_id": {"c"}, "client_secret": {"x"}}, http.StatusUnauthorized},
		{"wrong grant", url.Values{"grant_type": {"password"}, "client_id": {"c"}, "client_secret": {"s"}}, http.StatusBadRequest},
		{"missing fields", url.Values{"grant_type": {"client_credentials"}}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if w := do(h, http.MethodPost, "/oauth/token", "", tt.form); w.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tt.want, w.Body)
			}
		})
	}
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	h := newTestServer()
	for _, path := range []string{"/vehicles/B1234XYZ/location", "/vehicles/B1234XYZ/history?start=1&end=2"} {
		if w := do(h, http.MethodGet, path, "", nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token = %d, want 401", path, w.Code)
		}
		if w := do(h, http.MethodGet, path, "garbage", nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s with bad token = %d, want 401", path, w.Code)
		}
	}
}

func TestLatestLocation(t *testing.T) {
	h := newTestServer()
	token := getToken(t, h)

	w := do(h, http.MethodGet, "/vehicles/B1234XYZ/location", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", w.Code, w.Body)
	}
	var got model.Location
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Timestamp != 1715009000 || got.VehicleID != "B1234XYZ" {
		t.Fatalf("unexpected latest location: %+v", got)
	}

	if w := do(h, http.MethodGet, "/vehicles/UNKNOWN/location", token, nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown vehicle = %d, want 404", w.Code)
	}
}

func TestHistory(t *testing.T) {
	h := newTestServer()
	token := getToken(t, h)

	w := do(h, http.MethodGet, "/vehicles/B1234XYZ/history?start=1715000000&end=1715004000", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", w.Code, w.Body)
	}
	var got []model.Location
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(got), got)
	}

	// Empty range must be [] not null.
	w = do(h, http.MethodGet, "/vehicles/B1234XYZ/history?start=1&end=2", token, nil)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty history = %d %q, want 200 []", w.Code, w.Body)
	}

	for _, q := range []string{"", "?start=abc&end=2", "?start=1", "?start=10&end=5"} {
		if w := do(h, http.MethodGet, "/vehicles/B1234XYZ/history"+q, token, nil); w.Code != http.StatusBadRequest {
			t.Fatalf("history%s = %d, want 400", q, w.Code)
		}
	}
}

func TestDashboardAndGeofenceEndpoints(t *testing.T) {
	h := newTestServer()
	token := getToken(t, h)

	if w := do(h, http.MethodGet, "/", "", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Fleet Monitor") {
		t.Fatalf("dashboard = %d, want 200 with page", w.Code)
	}

	w := do(h, http.MethodGet, "/vehicles", token, nil)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `["B1234XYZ"]` {
		t.Fatalf("vehicles = %d %s", w.Code, w.Body)
	}

	w = do(h, http.MethodGet, "/geofence", token, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"radius_m":50`) {
		t.Fatalf("geofence = %d %s", w.Code, w.Body)
	}

	w = do(h, http.MethodGet, "/geofence/events?limit=5", token, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"event":"geofence_entry"`) {
		t.Fatalf("events = %d %s", w.Code, w.Body)
	}
	for _, q := range []string{"0", "101", "abc"} {
		if w := do(h, http.MethodGet, "/geofence/events?limit="+q, token, nil); w.Code != http.StatusBadRequest {
			t.Fatalf("events limit=%s = %d, want 400", q, w.Code)
		}
	}
	for _, p := range []string{"/vehicles", "/geofence", "/geofence/events"} {
		if w := do(h, http.MethodGet, p, "", nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token = %d, want 401", p, w.Code)
		}
	}
}
