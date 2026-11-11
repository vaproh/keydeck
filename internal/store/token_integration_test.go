package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

func TestAdminTokenLifecycleWithPostgres(t *testing.T) {
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

	slug := fmt.Sprintf("token-itest-%d", time.Now().UnixNano())
	product, err := store.New(pool).CreateProduct(ctx, store.CreateProductParams{
		Slug: slug, Name: "Token integration", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, product.ID) })

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	hash := hex.EncodeToString(random)
	row, err := store.New(pool).CreateAdminToken(ctx, store.CreateAdminTokenParams{
		Name: "agent-test", TokenHash: hash, TokenHint: "abcd",
		ConvertFrom: []byte(`["products:read","products:write"]`),
		ProductID:   product.ID,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	found, err := store.New(pool).GetAdminTokenByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != row.ID || found.ProductID != product.ID {
		t.Fatalf("unexpected token row: %+v", found)
	}
	if string(found.ScopesJson) != `["products:read", "products:write"]` && string(found.ScopesJson) != `["products:read","products:write"]` {
		t.Fatalf("unexpected scopes: %s", found.ScopesJson)
	}
	if err := store.New(pool).TouchAdminToken(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.New(pool).RevokeAdminToken(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	found, err = store.New(pool).GetAdminTokenByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !found.RevokedAt.Valid {
		t.Fatal("token was not revoked")
	}
}
