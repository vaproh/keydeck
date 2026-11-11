package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

func TestAdminProductLicenseAndReleaseFlowWithPostgres(t *testing.T) {
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
	slug := fmt.Sprintf("admin-itest-%d", time.Now().UnixNano())
	envSlug := strings.ToUpper(strings.ReplaceAll(slug, "-", "_"))
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LICENSE_SIGNING_KEY_"+envSlug, base64.StdEncoding.EncodeToString(seed))
	t.Setenv("UPDATE_SIGNING_KEY_"+envSlug, base64.StdEncoding.EncodeToString(seed))

	dir := t.TempDir()
	app := New(config.Config{AdminActor: "operator", ArtifactsDir: dir, DownloadSecret: "secret", PublicBaseURL: "https://ota.test"}, pool)
	router := app.AdminRouter()
	rawSession := "session-" + slug
	if err := store.New(pool).CreateSession(ctx, store.CreateSessionParams{
		ID: auth.HashToken(rawSession), Actor: "operator", CsrfToken: "csrf",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM products WHERE slug = $1`, slug)
		_, _ = pool.Exec(ctx, `DELETE FROM admin_sessions WHERE id = $1`, auth.HashToken(rawSession))
	})
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("content-type", "application/json")
		req.Header.Set("X-CSRF-Token", "csrf")
		req.AddCookie(&http.Cookie{Name: "desk_session", Value: rawSession})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPost, "/admin/products", map[string]any{
		"slug": slug, "name": "Admin integration", "license_public_key": "license",
		"update_public_key": "update", "key_prefix": "AD",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Product struct {
			ID int64 `json:"id"`
		} `json:"product"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created.Product.ID
	if id == 0 {
		t.Fatalf("missing product id: %s", rec.Body.String())
	}

	if rec := do(http.MethodGet, fmt.Sprintf("/admin/products/%d", id), nil); rec.Code != http.StatusOK {
		t.Fatalf("get product: %d", rec.Code)
	}
	if rec := do(http.MethodPatch, fmt.Sprintf("/admin/products/%d", id), map[string]string{"name": "Renamed", "status": "active"}); rec.Code != http.StatusOK {
		t.Fatalf("update product: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, fmt.Sprintf("/admin/products/%d/limits", id), map[string]string{"key": "max_accounts", "label": "Accounts"}); rec.Code != http.StatusCreated {
		t.Fatalf("add limit: %d", rec.Code)
	}
	if rec := do(http.MethodPost, fmt.Sprintf("/admin/products/%d/features", id), map[string]string{"key": "warmup", "label": "Warmup"}); rec.Code != http.StatusCreated {
		t.Fatalf("add feature: %d", rec.Code)
	}
	if rec := do(http.MethodPost, fmt.Sprintf("/admin/products/%d/presets", id), map[string]any{"name": "Trial", "limits": map[string]int{"max_accounts": 3}, "features": []string{"warmup"}, "max_activations": 1}); rec.Code != http.StatusCreated {
		t.Fatalf("add preset: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodPost, fmt.Sprintf("/admin/products/%d/licenses", id), map[string]any{
		"plan_name": "Trial", "holder_name": "Admin holder", "key_type": "trial", "trial_minutes": 15,
		"limits": map[string]int{"max_accounts": 3}, "features": []string{"warmup"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create license: %d %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		License struct {
			ID int64 `json:"id"`
		} `json:"license"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &issued)
	if issued.License.ID == 0 {
		t.Fatalf("missing license id: %s", rec.Body.String())
	}
	if rec := do(http.MethodGet, "/admin/licenses", nil); rec.Code != http.StatusOK {
		t.Fatalf("list licenses: %d", rec.Code)
	}
	if rec := do(http.MethodGet, fmt.Sprintf("/admin/licenses/%d", issued.License.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("get license: %d", rec.Code)
	}
	if rec := do(http.MethodPatch, fmt.Sprintf("/admin/licenses/%d", issued.License.ID), map[string]any{"holder_name": "Renamed holder", "notes": "note"}); rec.Code != http.StatusOK {
		t.Fatalf("update license: %d %s", rec.Code, rec.Body.String())
	}
	for _, action := range []string{"extend", "reset", "revoke"} {
		if rec := do(http.MethodPost, fmt.Sprintf("/admin/licenses/%d/%s", issued.License.ID, action), map[string]int{"days": 30}); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", action, rec.Code, rec.Body.String())
		}
	}

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	_ = writer.WriteField("product_id", fmt.Sprint(id))
	_ = writer.WriteField("version", "2.0.0")
	_ = writer.WriteField("channel", "stable")
	_ = writer.WriteField("platform", "windows-amd64")
	part, _ := writer.CreateFormFile("artifact", "release.zip")
	_, _ = part.Write([]byte("zip"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/releases", &upload)
	req.Header.Set("content-type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", "csrf")
	req.AddCookie(&http.Cookie{Name: "desk_session", Value: rawSession})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload release: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/admin/releases", nil); rec.Code != http.StatusOK {
		t.Fatalf("list releases: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/admin/audit", nil); rec.Code != http.StatusOK {
		t.Fatalf("audit: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/admin/stats", nil); rec.Code != http.StatusOK {
		t.Fatalf("stats: %d", rec.Code)
	}
}
