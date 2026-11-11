package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vaproh/keydeck/internal/config"
)

func turnstileTestServer(t *testing.T, status int, body string, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("secret") != "secret" || r.Form.Get("response") != "token" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func newTurnstile(endpoint string) *Turnstile {
	value := NewTurnstile("secret", "admin.example.com")
	value.endpoint = endpoint
	return value
}

func TestTurnstileSuccess(t *testing.T) {
	server := turnstileTestServer(t, http.StatusOK, `{"success":true,"hostname":"admin.example.com","action":"admin_login"}`, 0)
	defer server.Close()
	if err := newTurnstile(server.URL).Verify(context.Background(), "token", "203.0.113.9"); err != nil {
		t.Fatal(err)
	}
}

func TestTurnstileRejectsInvalidResponses(t *testing.T) {
	cases := map[string]string{
		"failure":         `{"success":false,"hostname":"admin.example.com","action":"admin_login","error-codes":["invalid-input-response"]}`,
		"hostname":        `{"success":true,"hostname":"evil.example.com","action":"admin_login"}`,
		"action":          `{"success":true,"hostname":"admin.example.com","action":"other"}`,
		"missing-action":  `{"success":true,"hostname":"admin.example.com"}`,
		"malformed":       `{`,
		"error-code-only": `{"success":true,"hostname":"admin.example.com","action":"admin_login","error-codes":["timeout-or-duplicate"]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := turnstileTestServer(t, http.StatusOK, body, 0)
			defer server.Close()
			if err := newTurnstile(server.URL).Verify(context.Background(), "token", ""); !errors.Is(err, ErrTurnstile) {
				t.Fatalf("expected ErrTurnstile, got %v", err)
			}
		})
	}
}

func TestTurnstileRejectsMissingTokenAndHTTPFailure(t *testing.T) {
	server := turnstileTestServer(t, http.StatusInternalServerError, `{}`, 0)
	defer server.Close()
	client := newTurnstile(server.URL)
	if err := client.Verify(context.Background(), "", ""); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("empty token: %v", err)
	}
	if err := client.Verify(context.Background(), strings.Repeat("x", 2049), ""); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("oversized token: %v", err)
	}
	if err := client.Verify(context.Background(), "token", ""); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("http failure: %v", err)
	}
}

func TestTurnstileRejectsDuplicateToken(t *testing.T) {
	server := turnstileTestServer(t, http.StatusOK, `{"success":true,"hostname":"admin.example.com","action":"admin_login"}`, 0)
	defer server.Close()
	client := newTurnstile(server.URL)
	if err := client.Verify(context.Background(), "token", ""); err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(context.Background(), "token", ""); !errors.Is(err, ErrTurnstile) {
		t.Fatal("replayed token was accepted")
	}
}

func TestTurnstileTimeout(t *testing.T) {
	server := turnstileTestServer(t, http.StatusOK, `{"success":true,"hostname":"admin.example.com","action":"admin_login"}`, 100*time.Millisecond)
	defer server.Close()
	client := newTurnstile(server.URL)
	client.client.Timeout = 10 * time.Millisecond
	if err := client.Verify(context.Background(), "token", ""); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("expected timeout failure, got %v", err)
	}
}

func TestLoginFailsClosedWithoutVerifier(t *testing.T) {
	service := NewWithTurnstile(config.Config{}, nil, nil)
	if _, err := service.Login(context.Background(), "203.0.113.1", "password", "123456", "token"); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("expected ErrTurnstile, got %v", err)
	}
}

func TestLoginStillRequiresPasswordAndRateLimit(t *testing.T) {
	service := NewWithTurnstile(config.Config{}, nil, allowVerifier{})
	for i := 0; i < 5; i++ {
		if _, err := service.Login(context.Background(), "203.0.113.2", "wrong", "123456", "token"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("attempt %d: expected unauthorized, got %v", i, err)
		}
	}
	if _, err := service.Login(context.Background(), "203.0.113.2", "wrong", "123456", "token"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
}

type allowVerifier struct{}

func (allowVerifier) Verify(context.Context, string, string) error { return nil }
