// Package auth implements an OAuth2 client-credentials flow that issues HS256 JWT access tokens.
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/raihan/fleet-management/internal/config"
)

const issuer = "fleet-management"

var (
	// ErrInvalidCredentials is returned when the client id/secret pair is wrong.
	ErrInvalidCredentials = errors.New("invalid client credentials")
	// ErrInvalidToken is returned when an access token is malformed, expired or badly signed.
	ErrInvalidToken = errors.New("invalid token")
)

// Service authenticates clients and issues/validates access tokens.
type Service struct {
	clientID     string
	clientSecret string
	jwtSecret    []byte
	ttl          time.Duration
	now          func() time.Time
}

// NewService builds an auth service from configuration.
func NewService(cfg config.Auth) *Service {
	return &Service{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		jwtSecret:    []byte(cfg.JWTSecret),
		ttl:          cfg.TokenTTL,
		now:          time.Now,
	}
}

// Authenticate verifies a client id/secret pair using constant-time comparison.
func (s *Service) Authenticate(clientID, clientSecret string) error {
	idOK := subtle.ConstantTimeCompare([]byte(clientID), []byte(s.clientID)) == 1
	secretOK := subtle.ConstantTimeCompare([]byte(clientSecret), []byte(s.clientSecret)) == 1
	if !idOK || !secretOK {
		return ErrInvalidCredentials
	}
	return nil
}

// Issue creates a signed access token for the client and returns it with its lifetime.
func (s *Service) Issue(clientID string) (string, time.Duration, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   clientID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", 0, fmt.Errorf("sign token: %w", err)
	}
	return signed, s.ttl, nil
}

// Validate checks signature, issuer and expiry, and returns the token subject (client id).
func (s *Service) Validate(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return s.jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil || !parsed.Valid {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
