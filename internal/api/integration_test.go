package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

func TestAgentTokenHTTPFlowWithPostgres(t *testing.T) {
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
	slug := fmt.Sprintf("api-itest-%d", time.Now().UnixNano())
	product, err := q.CreateProduct(ctx, store.CreateProductParams{Slug: slug, Name: "API integration", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CreateProduct(ctx, store.CreateProductParams{Slug: slug + "-other", Name: "Other", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id IN ($1,$2)`, product.ID, other.ID) })

	rawSession := "session-" + slug
	if err := q.CreateSession(ctx, store.CreateSessionParams{
		ID: auth.HashToken(rawSession), Actor: "operator", CsrfToken: "csrf",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM admin_sessions WHERE id = $1`, auth.HashToken(rawSession)) })

	t.Setenv("SIGNING_KEYS_DIR", t.TempDir())
	app := New(config.Config{AdminActor: "operator"}, pool)
	router := app.AdminRouter()
	do := func(method, path, csrf string, body any, bearer string) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("content-type", "application/json")
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		} else {
			req.AddCookie(&http.Cookie{Name: "desk_session", Value: rawSession})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodGet, "/admin/products", "", nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("list products: %d", rec.Code)
	}
	autoSlug := slug + "-auto"
	recAuto := do(http.MethodPost, "/admin/products", "csrf", map[string]string{"slug": autoSlug, "name": "Auto keys"}, "")
	if recAuto.Code != http.StatusCreated {
		t.Fatalf("auto product create: %d %s", recAuto.Code, recAuto.Body.String())
	}
	var autoProduct struct {
		Product struct {
			ID int64 `json:"id"`
		} `json:"product"`
	}
	if err := json.Unmarshal(recAuto.Body.Bytes(), &autoProduct); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, autoProduct.Product.ID) })
	recKey := do(http.MethodGet, fmt.Sprintf("/admin/products/%d", autoProduct.Product.ID), "", nil, "")
	if recKey.Code != http.StatusOK {
		t.Fatalf("auto product detail: %d", recKey.Code)
	}
	rec := do(http.MethodPost, "/admin/tokens", "csrf", map[string]any{
		"name": "agent-test", "scopes": []string{"products:write"}, "product_id": product.ID, "expires_days": 1,
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Token == "" {
		t.Fatalf("token response: %v %s", err, rec.Body.String())
	}
	allRec := do(http.MethodPost, "/admin/tokens", "csrf", map[string]any{
		"name": "all-products-agent", "scopes": []string{"products:read"}, "product_id": 0, "expires_days": 1,
	}, "")
	if allRec.Code != http.StatusCreated {
		t.Fatalf("create all-products token: %d %s", allRec.Code, allRec.Body.String())
	}
	if rec := do(http.MethodPost, fmt.Sprintf("/admin/products/%d/limits", product.ID), "", map[string]string{"key": "max_test", "label": "Test"}, created.Token); rec.Code != http.StatusCreated {
		t.Fatalf("agent limit add: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, fmt.Sprintf("/admin/products/%d/limits", other.ID), "", map[string]string{"key": "max_other"}, created.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("agent escaped product scope: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/admin/tokens", "", nil, created.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("agent could list tokens: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/admin/tokens", "", nil, created.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("agent could create token: %d", rec.Code)
	}
}
