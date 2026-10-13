// Command publisher simulates vehicles and publishes mock GPS data to MQTT every 2 seconds.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/raihan/fleet-management/internal/geofence"
	"github.com/raihan/fleet-management/internal/model"
)

type point struct{ lat, lon float64 }

// route is a ~1.3 km closed loop (≈330 m square) whose first corner is the default geofence
// center (Monas), so every vehicle enters the 50 m geofence roughly once per minute at 20 m/s.
var route = []point{
	{-6.1754, 106.8272},
	{-6.1754, 106.8302},
	{-6.1784, 106.8302},
	{-6.1784, 106.8272},
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	brokerURL := getEnv("MQTT_BROKER_URL", "tcp://localhost:1883")
	vehicleIDs := splitNonEmpty(getEnv("VEHICLE_IDS", "B1234XYZ,B5678ABC"))
	interval, err := time.ParseDuration(getEnv("PUBLISH_INTERVAL", "2s"))
	if err != nil || interval <= 0 {
		return fmt.Errorf("invalid PUBLISH_INTERVAL: %v", getEnv("PUBLISH_INTERVAL", "2s"))
	}
	speedMPS, err := strconv.ParseFloat(getEnv("SPEED_MPS", "20"), 64)
	if err != nil || speedMPS <= 0 {
		return fmt.Errorf("invalid SPEED_MPS: %v", getEnv("SPEED_MPS", "20"))
	}
	if len(vehicleIDs) == 0 {
		return fmt.Errorf("VEHICLE_IDS must contain at least one vehicle id")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := paho.NewClient(paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("fleet-publisher").
		SetCleanSession(true).
		SetAutoReconnect(true))

	if err := connect(ctx, client, log); err != nil {
		return err
	}
	defer client.Disconnect(250)

	vehicles := make([]*vehicle, len(vehicleIDs))
	for i, id := range vehicleIDs {
		vehicles[i] = newVehicle(id)
		vehicles[i].advance(float64(i) * 400) // spread vehicles along the route
	}

	stepM := speedMPS * interval.Seconds()
	log.Info("publishing mock locations", "vehicles", vehicleIDs, "interval", interval, "step_m", stepM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("publisher stopped")
			return nil
		case <-ticker.C:
			for _, v := range vehicles {
				v.advance(stepM)
				publish(client, v, log)
			}
		}
	}
}

func connect(ctx context.Context, client paho.Client, log *slog.Logger) error {
	for {
		token := client.Connect()
		if token.WaitTimeout(5*time.Second) && token.Error() == nil {
			log.Info("mqtt connected")
			return nil
		}
		log.Warn("mqtt connect failed, retrying", "error", token.Error())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func publish(client paho.Client, v *vehicle, log *slog.Logger) {
	pos := v.position()
	loc := model.Location{
		VehicleID: v.id,
		Latitude:  round6(pos.lat),
		Longitude: round6(pos.lon),
		Timestamp: time.Now().Unix(),
	}
	payload, err := json.Marshal(loc)
	if err != nil {
		log.Error("marshal location", "error", err)
		return
	}

	topic := fmt.Sprintf("/fleet/vehicle/%s/location", v.id)
	token := client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		log.Error("publish failed", "topic", topic, "error", token.Error())
		return
	}
	log.Info("published", "topic", topic, "payload", string(payload))
}

// vehicle moves along the route at a constant speed.
type vehicle struct {
	id       string
	segment  int
	progress float64 // meters travelled within the current segment
}

func newVehicle(id string) *vehicle { return &vehicle{id: id} }

func (v *vehicle) advance(meters float64) {
	for {
		segLen := distance(route[v.segment], route[(v.segment+1)%len(route)])
		remaining := segLen - v.progress
		if meters < remaining {
			v.progress += meters
			return
		}
		meters -= remaining
		v.segment = (v.segment + 1) % len(route)
		v.progress = 0
	}
}

func (v *vehicle) position() point {
	from := route[v.segment]
	to := route[(v.segment+1)%len(route)]
	frac := v.progress / distance(from, to)
	return point{
		lat: from.lat + (to.lat-from.lat)*frac,
		lon: from.lon + (to.lon-from.lon)*frac,
	}
}

// distance returns the haversine distance in meters.
func distance(a, b point) float64 {
	return geofence.DistanceMeters(a.lat, a.lon, b.lat, b.lon)
}

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
