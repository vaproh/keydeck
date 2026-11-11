package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

func TestBearerMiddlewareWithPostgres(t *testing.T) {
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
	slug := fmt.Sprintf("auth-itest-%d", time.Now().UnixNano())
	product, err := q.CreateProduct(ctx, store.CreateProductParams{Slug: slug, Name: "Auth integration", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, product.ID) })

	raw, hashed, hint := NewAdminToken()
	if _, err := q.CreateAdminToken(ctx, store.CreateAdminTokenParams{
		Name: "agent-test", TokenHash: hashed, TokenHint: hint,
		ConvertFrom: []byte(`["products:write"]`), ProductID: product.ID,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	service := New(config.Config{}, q)
	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent, ok := AgentFrom(r.Context())
		if !ok {
			t.Fatal("agent missing from context")
		}
		if agent.ProductID != product.ID || !agent.Has("products:write") {
			t.Fatalf("unexpected agent: %+v", agent)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/1/limits", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/products/1/limits", nil)
	req.Header.Set("Authorization", "Bearer "+raw+"x")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d", rec.Code)
	}
}
