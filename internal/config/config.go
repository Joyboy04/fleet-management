// Package config loads service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// MQTT holds broker connection settings.
type MQTT struct {
	BrokerURL string
	ClientID  string
	Topic     string
}

// HTTP holds REST server settings.
type HTTP struct {
	Addr string
}

// Auth holds OAuth2 (client credentials) and JWT settings.
type Auth struct {
	ClientID     string
	ClientSecret string
	JWTSecret    string
	TokenTTL     time.Duration
}

// RabbitMQ holds broker connection and topology settings shared by the API and the worker.
type RabbitMQ struct {
	URL        string
	Exchange   string
	Queue      string
	RoutingKey string
}

// Geofence describes the circular area that triggers geofence_entry events.
type Geofence struct {
	Name      string
	Latitude  float64
	Longitude float64
	RadiusM   float64
}

// Config is the full runtime configuration of the API service.
type Config struct {
	DatabaseURL string
	MQTT        MQTT
	HTTP        HTTP
	Auth        Auth
	RabbitMQ    RabbitMQ
	Geofence    Geofence
}

// Worker is the runtime configuration of the geofence worker service.
type Worker struct {
	DatabaseURL string
	RabbitMQ    RabbitMQ
}

// Load reads the API configuration from the environment, applying defaults for local development.
func Load() (Config, error) {
	ttl, err := time.ParseDuration(getEnv("TOKEN_TTL", "1h"))
	if err != nil || ttl <= 0 {
		return Config{}, fmt.Errorf("invalid TOKEN_TTL: %q", getEnv("TOKEN_TTL", "1h"))
	}

	geo, err := loadGeofence()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		MQTT: MQTT{
			BrokerURL: getEnv("MQTT_BROKER_URL", "tcp://localhost:1883"),
			ClientID:  getEnv("MQTT_CLIENT_ID", "fleet-api"),
			Topic:     getEnv("MQTT_TOPIC", "/fleet/vehicle/+/location"),
		},
		HTTP: HTTP{
			Addr: getEnv("HTTP_ADDR", ":8080"),
		},
		Auth: Auth{
			ClientID:     getEnv("OAUTH_CLIENT_ID", ""),
			ClientSecret: getEnv("OAUTH_CLIENT_SECRET", ""),
			JWTSecret:    getEnv("JWT_SECRET", ""),
			TokenTTL:     ttl,
		},
		RabbitMQ: loadRabbitMQ(),
		Geofence: geo,
	}

	switch {
	case cfg.DatabaseURL == "":
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	case cfg.Auth.ClientID == "":
		return Config{}, fmt.Errorf("OAUTH_CLIENT_ID is required")
	case cfg.Auth.ClientSecret == "":
		return Config{}, fmt.Errorf("OAUTH_CLIENT_SECRET is required")
	case cfg.Auth.JWTSecret == "":
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}

// LoadWorker reads the worker configuration from the environment.
func LoadWorker() (Worker, error) {
	cfg := Worker{DatabaseURL: getEnv("DATABASE_URL", ""), RabbitMQ: loadRabbitMQ()}
	if cfg.DatabaseURL == "" {
		return Worker{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func loadRabbitMQ() RabbitMQ {
	return RabbitMQ{
		URL:        getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		Exchange:   getEnv("RABBITMQ_EXCHANGE", "fleet.events"),
		Queue:      getEnv("RABBITMQ_QUEUE", "geofence_alerts"),
		RoutingKey: getEnv("RABBITMQ_ROUTING_KEY", "geofence.entry"),
	}
}

// loadGeofence defaults to a 50 m radius around Monas, which lies on the mock publisher's route.
func loadGeofence() (Geofence, error) {
	lat, err := getFloat("GEOFENCE_LAT", -6.1754)
	if err != nil {
		return Geofence{}, err
	}
	lon, err := getFloat("GEOFENCE_LON", 106.8272)
	if err != nil {
		return Geofence{}, err
	}
	radius, err := getFloat("GEOFENCE_RADIUS_M", 50)
	if err != nil {
		return Geofence{}, err
	}

	switch {
	case lat < -90 || lat > 90:
		return Geofence{}, fmt.Errorf("GEOFENCE_LAT must be between -90 and 90")
	case lon < -180 || lon > 180:
		return Geofence{}, fmt.Errorf("GEOFENCE_LON must be between -180 and 180")
	case radius <= 0:
		return Geofence{}, fmt.Errorf("GEOFENCE_RADIUS_M must be greater than 0")
	}
	return Geofence{
		Name:      getEnv("GEOFENCE_NAME", "Monas"),
		Latitude:  lat,
		Longitude: lon,
		RadiusM:   radius,
	}, nil
}

func getFloat(key string, fallback float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", key, v)
	}
	return f, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
