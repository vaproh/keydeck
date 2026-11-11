package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/keydeck")
	t.Setenv("ADMIN_PASSWORD_HASH", "hash")
	t.Setenv("ADMIN_TOTP_SECRET", "secret")
	t.Setenv("DOWNLOAD_SECRET", "download")
	t.Setenv("PUBLIC_BASE_URL", "https://ota.example.com/")
	t.Setenv("COOKIE_SECURE", "false")
}

func TestLoad(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://ota.example.com" {
		t.Fatalf("base URL not trimmed: %q", cfg.PublicBaseURL)
	}
	if cfg.CookieSecure {
		t.Fatal("COOKIE_SECURE was not read")
	}
}

func TestLoadRejectsMissingSecretsAndHTTP(t *testing.T) {
	setValidEnv(t)
	t.Setenv("ADMIN_PASSWORD_HASH", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD_HASH") {
		t.Fatalf("expected missing password hash error, got %v", err)
	}
	setValidEnv(t)
	t.Setenv("PUBLIC_BASE_URL", "http://ota.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https error, got %v", err)
	}
}

func TestSigningKey(t *testing.T) {
	seed := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("LICENSE_SIGNING_KEY_RE_ORCHESTRA", seed)
	key, err := SigningKey("re-orchestra", "license")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("unexpected key length: %d", len(key))
	}
	t.Setenv("UPDATE_SIGNING_KEY_RE_ORCHESTRA", "not-base64")
	if _, err := SigningKey("re-orchestra", "update"); err == nil {
		t.Fatal("expected invalid base64 error")
	}
}
