package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/raihan/fleet-management/internal/auth"
)

const grantClientCredentials = "client_credentials"

type authHandler struct {
	tokens TokenService
	log    *slog.Logger
}

// tokenRequest accepts both application/x-www-form-urlencoded (OAuth2 standard) and JSON bodies.
type tokenRequest struct {
	GrantType    string `form:"grant_type" json:"grant_type" binding:"required"`
	ClientID     string `form:"client_id" json:"client_id" binding:"required"`
	ClientSecret string `form:"client_secret" json:"client_secret" binding:"required"`
}

// token implements the OAuth2 client_credentials grant (RFC 6749 section 4.4).
func (h *authHandler) token(c *gin.Context) {
	var req tokenRequest
	if err := c.ShouldBind(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "invalid_request", "grant_type, client_id and client_secret are required")
		return
	}
	if req.GrantType != grantClientCredentials {
		errorJSON(c, http.StatusBadRequest, "unsupported_grant_type", "only client_credentials is supported")
		return
	}
	if err := h.tokens.Authenticate(req.ClientID, req.ClientSecret); err != nil {
		errorJSON(c, http.StatusUnauthorized, "invalid_client", "invalid client credentials")
		return
	}

	token, ttl, err := h.tokens.Issue(req.ClientID)
	if err != nil {
		h.log.Error("issue token failed", "error", err)
		errorJSON(c, http.StatusInternalServerError, "server_error", "could not issue token")
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(ttl.Seconds()),
	})
}

const clientIDKey = "client_id"

// requireAuth rejects requests without a valid "Authorization: Bearer <jwt>" header.
func requireAuth(tokens TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
		token = strings.TrimSpace(token)
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			unauthorized(c, "missing or malformed bearer token")
			return
		}
		clientID, err := tokens.Validate(token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				unauthorized(c, "invalid or expired token")
				return
			}
			unauthorized(c, "token validation failed")
			return
		}
		c.Set(clientIDKey, clientID)
		c.Next()
	}
}

func unauthorized(c *gin.Context, message string) {
	c.Header("WWW-Authenticate", `Bearer realm="fleet"`)
	errorJSON(c, http.StatusUnauthorized, "unauthorized", message)
}
