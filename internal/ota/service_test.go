package ota

import (
	"testing"
	"time"

	"github.com/vaproh/license-desk/internal/config"
)

func TestDownloadToken(t *testing.T) {
	s := &Service{cfg: configWithSecret("test-secret"), now: func() time.Time { return time.Unix(1000, 0) }}
	token := s.token(42, time.Unix(2000, 0))
	id, ok := s.verifyToken(token)
	if !ok || id != 42 {
		t.Fatalf("token rejected: %d %v", id, ok)
	}
	if _, ok := s.verifyToken(token + "x"); ok {
		t.Fatal("tampered token accepted")
	}
}

func TestSemver(t *testing.T) {
	if compareVersions("1.2.3", "1.2.2") <= 0 {
		t.Fatal("expected 1.2.3 > 1.2.2")
	}
	if compareVersions("v1.2.3", "1.2.3") != 0 {
		t.Fatal("v prefix should not change version")
	}
}

func configWithSecret(secret string) (c config.Config) { c.DownloadSecret = secret; return }
