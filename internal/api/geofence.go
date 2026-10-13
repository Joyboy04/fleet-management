package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultEventsLimit = 20
	maxEventsLimit     = 100
)

type geofenceHandler struct {
	geofences GeofenceService
	log       *slog.Logger
}

// zone handles GET /geofence.
func (h *geofenceHandler) zone(c *gin.Context) {
	z := h.geofences.Zone()
	c.JSON(http.StatusOK, gin.H{
		"name":      z.Name,
		"latitude":  z.Latitude,
		"longitude": z.Longitude,
		"radius_m":  z.RadiusM,
	})
}

// events handles GET /geofence/events?limit=<1..100>.
func (h *geofenceHandler) events(c *gin.Context) {
	limit := defaultEventsLimit
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxEventsLimit {
			errorJSON(c, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 100")
			return
		}
		limit = n
	}

	events, err := h.geofences.RecentEvents(c.Request.Context(), limit)
	if err != nil {
		h.log.Error("request failed", "path", c.Request.URL.Path, "error", err)
		errorJSON(c, http.StatusInternalServerError, "server_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, events)
}
