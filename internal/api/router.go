// Package api exposes the REST interface of the fleet backend (Gin).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/raihan/fleet-management/internal/geofence"
	"github.com/raihan/fleet-management/internal/model"
	"github.com/raihan/fleet-management/web"
)

// LocationService is what the HTTP layer needs from the business layer.
type LocationService interface {
	Latest(ctx context.Context, vehicleID string) (model.Location, error)
	History(ctx context.Context, vehicleID string, start, end int64) ([]model.Location, error)
	Vehicles(ctx context.Context) ([]string, error)
}

// GeofenceService is what the HTTP layer needs to show the geofence and its events.
type GeofenceService interface {
	Zone() geofence.Zone
	RecentEvents(ctx context.Context, limit int) ([]model.GeofenceEventRecord, error)
}

// TokenService is what the HTTP layer needs from the auth layer.
type TokenService interface {
	Authenticate(clientID, clientSecret string) error
	Issue(clientID string) (token string, ttl time.Duration, err error)
	Validate(token string) (clientID string, err error)
}

// NewRouter wires all routes.
//
//	GET  /                                     (public)  web dashboard
//	POST /oauth/token                          (public)  client_credentials -> access token
//	GET  /healthz                              (public)  liveness
//	GET  /vehicles                             (Bearer)  ids of vehicles with data
//	GET  /vehicles/:vehicle_id/location       (Bearer)  last known location
//	GET  /vehicles/:vehicle_id/history        (Bearer)  locations between ?start= and ?end=
//	GET  /geofence                             (Bearer)  configured geofence
//	GET  /geofence/events                      (Bearer)  latest events consumed by the worker
func NewRouter(locations LocationService, geofences GeofenceService, tokens TokenService, log *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger(log))

	r.GET("/", func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", web.Index) })
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	authH := &authHandler{tokens: tokens, log: log}
	r.POST("/oauth/token", authH.token)

	v := &vehicleHandler{locations: locations, log: log}
	protected := r.Group("/", requireAuth(tokens))
	protected.GET("/vehicles", v.list)
	protected.GET("/vehicles/:vehicle_id/location", v.latest)
	protected.GET("/vehicles/:vehicle_id/history", v.history)

	g := &geofenceHandler{geofences: geofences, log: log}
	protected.GET("/geofence", g.zone)
	protected.GET("/geofence/events", g.events)

	return r
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start).String())
	}
}

func errorJSON(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}
