package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
)

func TestLicenseJSONIncludesHolderAndLimits(t *testing.T) {
	row := store.License{ID: 7, PlanName: "Monthly", HolderName: "Acme Media", LimitsJson: []byte(`{"max_accounts":50}`)}
	got := licenseJSON(row)
	if got["holder_name"] != "Acme Media" || got["plan_name"] != "Monthly" {
		t.Fatalf("unexpected license json: %+v", got)
	}
	raw, _ := json.Marshal(got["limits"])
	if string(raw) != `{"max_accounts":50}` {
		t.Fatalf("limits changed: %s", raw)
	}
}

func TestNormalizeVersion(t *testing.T) {
	for input, want := range map[string]string{"1.2.3": "v1.2.3", "v1.2.3": "v1.2.3", "1.2.3-beta.1": "v1.2.3-beta.1"} {
		if got := normalizeVersion(input); got != want {
			t.Fatalf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReleaseFilename(t *testing.T) {
	if got := releaseFilename("1.2.3", "windows-amd64"); got != "1.2.3-windows-amd64.zip" {
		t.Fatalf("unexpected filename: %s", got)
	}
}

func TestAuthConfigIncludesVersion(t *testing.T) {
	app := &App{cfg: config.Config{TurnstileSiteKey: "site", Version: "1.2.3"}, auth: auth.New(config.Config{}, nil)}
	router := chi.NewRouter()
	router.Mount("/", app.AuthRouter())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != "1.2.3" || body["site_key"] != "site" {
		t.Fatalf("unexpected config: %+v", body)
	}
}
