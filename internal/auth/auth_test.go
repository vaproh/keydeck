package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vaproh/keydeck/internal/config"
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

func TestCaptchaIsOneUse(t *testing.T) {
	s := New(config.Config{}, nil)
	id, question, ttl := s.Captcha()
	if ttl != 300 || question == "" {
		t.Fatalf("unexpected captcha: %q %d", question, ttl)
	}
	// 4 + 4 is the only way this test can be deterministic enough; use the
	// question instead of guessing the answer.
	parts := strings.Split(question, " + ")
	if len(parts) != 2 {
		t.Fatalf("unexpected question: %q", question)
	}
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[1])
	if !s.checkCaptcha(id, strconv.Itoa(a+b)) {
		t.Fatal("correct captcha rejected")
	}
	if s.checkCaptcha(id, strconv.Itoa(a+b)) {
		t.Fatal("captcha reused")
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
