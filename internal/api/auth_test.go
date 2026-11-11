package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vaproh/keydeck/internal/config"
)

func TestAuthConfigExposesOnlySiteKey(t *testing.T) {
	app := &App{cfg: config.Config{TurnstileSiteKey: "public-site", TurnstileSecretKey: "must-not-leak"}}
	rec := httptest.NewRecorder()
	app.authConfig(rec, httptest.NewRequest(http.MethodGet, "/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["site_key"] != "public-site" || len(body) != 1 {
		t.Fatalf("unexpected config: %+v", body)
	}
}

func TestLoginRejectsMalformedJSON(t *testing.T) {
	app := &App{}
	rec := httptest.NewRecorder()
	app.login(rec, httptest.NewRequest(http.MethodPost, "/login", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
