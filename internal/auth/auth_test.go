package auth

import (
	"github.com/vaproh/keydeck/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyArgon2(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if verifyArgon2(hash, "wrong") {
		t.Fatal("invalid password accepted")
	}
}

func TestNewAdminTokenIsOpaqueAndVerifiable(t *testing.T) {
	raw, hashed, hint := NewAdminToken()
	if len(raw) < 20 || raw[:9] != "kd_admin_" {
		t.Fatalf("unexpected token prefix: %q", raw)
	}
	if hint != raw[len(raw)-4:] {
		t.Fatalf("hint %q does not match token suffix", hint)
	}
	if hashed != HashToken(raw) {
		t.Fatal("token hash is not stable")
	}
	if HashToken(raw) == HashToken(raw+"x") {
		t.Fatal("different tokens produced the same hash")
	}
}

func TestAgentScopeCheck(t *testing.T) {
	agent := Agent{Name: "agent", Scopes: map[string]bool{"products:read": true}}
	if !agent.Has("products:read") {
		t.Fatal("expected scope to be present")
	}
	if agent.Has("products:write") {
		t.Fatal("unexpected scope")
	}
}

func TestSessionLifetimeIsSevenDays(t *testing.T) {
	if sessionTTL != 7*24*time.Hour {
		t.Fatalf("expected 7 day session, got %s", sessionTTL)
	}
}

func TestBearerHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer kd_admin_test")
	if got := bearer(r); got != "kd_admin_test" {
		t.Fatalf("unexpected bearer: %q", got)
	}
	r.Header.Set("Authorization", "Basic x")
	if got := bearer(r); got != "" {
		t.Fatalf("unexpected bearer: %q", got)
	}
}

func TestBootstrapTokenGetsAllScopes(t *testing.T) {
	service := New(config.Config{BootstrapAdminToken: "bootstrap-secret"}, nil)
	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent, ok := AgentFrom(r.Context())
		if !ok || !agent.Bootstrap || !agent.Has("releases:write") || agent.ProductID != 0 {
			t.Fatalf("unexpected bootstrap agent: %+v", agent)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer bootstrap-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}
