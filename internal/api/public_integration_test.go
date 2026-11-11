package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/licensing"
	"github.com/vaproh/keydeck/internal/store"
)

func TestLicenseAndOTAHTTPWithPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	slug := fmt.Sprintf("public-itest-%d", time.Now().UnixNano())
	product, err := q.CreateProduct(ctx, store.CreateProductParams{Slug: slug, Name: "Public API", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, product.ID) })
	if _, err := q.UpsertProductKey(ctx, store.UpsertProductKeyParams{
		ProductID: product.ID, LicensePublicKey: "license", UpdatePublicKey: "update", KeyPrefix: "PB",
	}); err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	envSlug := strings.ToUpper(strings.ReplaceAll(slug, "-", "_"))
	t.Setenv("LICENSE_SIGNING_KEY_"+envSlug, base64.StdEncoding.EncodeToString(seed))
	t.Setenv("UPDATE_SIGNING_KEY_"+envSlug, base64.StdEncoding.EncodeToString(seed))

	rawKey, hash, last4 := licensing.NewKey("PB")
	minutes := int32(15)
	if _, err := q.CreateLicense(ctx, store.CreateLicenseParams{
		ProductID: product.ID, KeyHash: hash, KeyLast4: last4, PlanName: "Trial",
		HolderName: "Public holder", KeyType: "trial", TrialMinutes: &minutes,
		StartsOnActivation: true, OneTimeActivation: true, GraceSeconds: 0,
		ConvertFrom: []byte(`{"max_accounts":3}`), ConvertFrom_2: []byte(`["warmup"]`),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}, MaxActivations: 1,
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	artifact := filepath.Join(dir, "release.zip")
	if err := os.WriteFile(artifact, []byte("zip-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateRelease(ctx, store.CreateReleaseParams{
		ProductID: product.ID, Version: "1.2.0", Channel: "stable", Platform: "windows-amd64",
		MinVersion: "1.0.0", RolloutPercent: 100, Notes: "notes", ArtifactName: "release.zip",
		ArtifactPath: artifact, Sha256: "abc", SizeBytes: 9, Manifest: `{"version":"1.2.0"}`,
		ManifestSignature: "sig", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		AdminActor: "operator", PublicBaseURL: "https://ota.example.test",
		ArtifactsDir: dir, DownloadSecret: "download-secret",
	}
	app := New(cfg, pool)
	licenseRouter := app.LicenseRouter()
	otaRouter := app.OTARouter()

	install := strings.Repeat("a", 64)
	body, _ := json.Marshal(map[string]string{
		"key": rawKey, "installation_hash": install, "product": slug, "app_version": "test",
	})
	rec := httptest.NewRecorder()
	licenseRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/activate", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("activate: %d %s", rec.Code, rec.Body.String())
	}
	var activated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &activated); err != nil || activated["lease"] == "" {
		t.Fatalf("activation response: %v %s", err, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	licenseRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/heartbeat", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	licenseRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/deactivate", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	otaRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/update-check?product="+slug+"&channel=stable&platform=windows-amd64&current=1.0.0", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("update check: %d %s", rec.Code, rec.Body.String())
	}
	var update struct {
		Available   bool   `json:"available"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &update); err != nil || !update.Available || update.DownloadURL == "" {
		t.Fatalf("update response: %v %s", err, rec.Body.String())
	}
	token := update.DownloadURL[strings.LastIndex(update.DownloadURL, "/")+1:]
	rec = httptest.NewRecorder()
	otaRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/"+token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
	}
	got, _ := io.ReadAll(rec.Body)
	if string(got) != "zip-bytes" {
		t.Fatalf("unexpected artifact: %q", got)
	}
}
