package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLicenseListenerDoesNotServeAdminOrOTA(t *testing.T) {
	for _, path := range []string{"/admin", "/v1/update-check"} {
		rec := httptest.NewRecorder()
		License(logger()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("license %s status = %d, want 404", path, rec.Code)
		}
	}
}

func TestOTAListenerDoesNotServeAdmin(t *testing.T) {
	rec := httptest.NewRecorder()
	OTA(logger()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ota /admin status = %d, want 404", rec.Code)
	}
}

func TestAdminListenerDoesNotServeLicenseOrOTA(t *testing.T) {
	for _, path := range []string{"/v1/activate", "/v1/update-check"} {
		rec := httptest.NewRecorder()
		Admin(logger()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("admin %s status = %d, want 404", path, rec.Code)
		}
	}
}

func TestAdminListenerServesAdmin(t *testing.T) {
	rec := httptest.NewRecorder()
	Admin(logger()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin /admin status = %d, want 200", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	for name, handler := range map[string]http.Handler{
		"license": License(logger()),
		"ota":     OTA(logger()),
		"admin":   Admin(logger()),
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
			t.Fatalf("%s health = %d %q", name, rec.Code, rec.Body.String())
		}
	}
}
