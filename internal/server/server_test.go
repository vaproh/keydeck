package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicListenerDoesNotServeAdmin(t *testing.T) {
	rec := httptest.NewRecorder()
	Public().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public /admin status = %d, want 404", rec.Code)
	}
}

func TestAdminListenerServesAdmin(t *testing.T) {
	rec := httptest.NewRecorder()
	Admin().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin /admin status = %d, want 200", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	Public().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("health = %d %q", rec.Code, rec.Body.String())
	}
}
