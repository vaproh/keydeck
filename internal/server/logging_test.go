package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggerDoesNotLogQueryOrHeaders(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/activate?key=super-secret", nil)
	req.Header.Set("Authorization", "Bearer super-secret")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	line := buf.String()
	if !strings.Contains(line, "path=/v1/activate") || !strings.Contains(line, "status=204") {
		t.Fatalf("missing request fields: %s", line)
	}
	if strings.Contains(line, "super-secret") || strings.Contains(line, "key=") {
		t.Fatalf("sensitive value leaked: %s", line)
	}
}

func TestRecovererLogsPanic(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := RequestLogger(log)(Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/activate", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(buf.String(), "panic") || !strings.Contains(buf.String(), "path=/v1/activate") {
		t.Fatalf("panic was not logged: %s", buf.String())
	}
}
