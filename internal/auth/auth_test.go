package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/raihan/fleet-management/internal/config"
)

func newTestService() *Service {
	return NewService(config.Auth{
		ClientID:     "client",
		ClientSecret: "secret",
		JWTSecret:    "jwt-secret",
		TokenTTL:     time.Hour,
	})
}

func TestAuthenticate(t *testing.T) {
	s := newTestService()
	if err := s.Authenticate("client", "secret"); err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}
	for _, c := range [][2]string{{"client", "bad"}, {"bad", "secret"}, {"", ""}} {
		if err := s.Authenticate(c[0], c[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Authenticate(%q,%q) = %v, want ErrInvalidCredentials", c[0], c[1], err)
		}
	}
}

func TestIssueAndValidate(t *testing.T) {
	s := newTestService()
	token, ttl, err := s.Issue("client")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != time.Hour {
		t.Fatalf("ttl = %v, want 1h", ttl)
	}
	sub, err := s.Validate(token)
	if err != nil || sub != "client" {
		t.Fatalf("Validate = (%q, %v), want (client, nil)", sub, err)
	}
}

func TestValidateRejects(t *testing.T) {
	s := newTestService()
	token, _, _ := s.Issue("client")

	other := NewService(config.Auth{ClientID: "client", ClientSecret: "secret", JWTSecret: "different", TokenTTL: time.Hour})
	if _, err := other.Validate(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token signed with another secret accepted: %v", err)
	}

	if _, err := s.Validate("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("garbage token accepted: %v", err)
	}

	// Token validated far in the future must be expired.
	future := newTestService()
	future.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := future.Validate(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}
