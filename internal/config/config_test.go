package config

import (
	"strings"
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/keydeck")
	t.Setenv("ADMIN_PASSWORD_HASH", "hash")
	t.Setenv("ADMIN_TOTP_SECRET", "secret")
	t.Setenv("DOWNLOAD_SECRET", "download")
	t.Setenv("PUBLIC_BASE_URL", "https://ota.example.com")
	t.Setenv("TURNSTILE_SITE_KEY", "site")
	t.Setenv("TURNSTILE_SECRET_KEY", "secret-key")
	t.Setenv("TURNSTILE_EXPECTED_HOSTNAME", "admin.example.com")
}

func TestLoadIncludesTurnstile(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TurnstileSiteKey != "site" || cfg.TurnstileExpectedHost != "admin.example.com" {
		t.Fatalf("turnstile config missing: %+v", cfg)
	}
}

func TestLoadFailsClosedWithoutTurnstile(t *testing.T) {
	setValidEnv(t)
	t.Setenv("TURNSTILE_SECRET_KEY", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TURNSTILE_SECRET_KEY") {
		t.Fatalf("expected missing turnstile error, got %v", err)
	}
}
