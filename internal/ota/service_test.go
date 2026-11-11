package ota

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
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

func TestExpiredTokenAndArtifactBoundary(t *testing.T) {
	dir := t.TempDir()
	s := &Service{cfg: config.Config{DownloadSecret: "secret", ArtifactsDir: dir}, now: func() time.Time { return time.Unix(3000, 0) }}
	token := s.token(7, time.Unix(2000, 0))
	if _, ok := s.verifyToken(token); ok {
		t.Fatal("expired token accepted")
	}
	release := storeRelease(filepath.Join(dir, "release.zip"))
	if _, err := s.ArtifactPath(release); err != nil {
		t.Fatalf("artifact inside root rejected: %v", err)
	}
	release = storeRelease(filepath.Join(dir, "..", "escape.zip"))
	if _, err := s.ArtifactPath(release); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func storeRelease(path string) store.Release { return store.Release{ArtifactPath: path} }
