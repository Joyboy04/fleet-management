package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/raihan/fleet-management/internal/model"
	"github.com/raihan/fleet-management/internal/service"
)

type vehicleHandler struct {
	locations LocationService
	log       *slog.Logger
}

// list handles GET /vehicles.
func (h *vehicleHandler) list(c *gin.Context) {
	ids, err := h.locations.Vehicles(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ids)
}

// latest handles GET /vehicles/:vehicle_id/location.
func (h *vehicleHandler) latest(c *gin.Context) {
	loc, err := h.locations.Latest(c.Request.Context(), c.Param("vehicle_id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, loc)
}

// history handles GET /vehicles/:vehicle_id/history?start=<unix>&end=<unix>.
func (h *vehicleHandler) history(c *gin.Context) {
	start, err := parseUnix(c.Query("start"))
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "invalid_request", "start must be a unix timestamp in seconds")
		return
	}
	end, err := parseUnix(c.Query("end"))
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "invalid_request", "end must be a unix timestamp in seconds")
		return
	}

	locs, err := h.locations.History(c.Request.Context(), c.Param("vehicle_id"), start, end)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, locs)
}

func (h *vehicleHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidVehicleID), errors.Is(err, service.ErrInvalidRange):
		errorJSON(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, model.ErrNotFound):
		errorJSON(c, http.StatusNotFound, "not_found", "no location found for this vehicle")
	default:
		h.log.Error("request failed", "path", c.Request.URL.Path, "error", err)
		errorJSON(c, http.StatusInternalServerError, "server_error", "internal server error")
	}
}

func parseUnix(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
