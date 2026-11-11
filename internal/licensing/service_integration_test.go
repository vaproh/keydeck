package licensing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

func TestActivateTrialWithPostgres(t *testing.T) {
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

	slug := fmt.Sprintf("itest-%d", time.Now().UnixNano())
	product, err := store.New(pool).CreateProduct(ctx, store.CreateProductParams{Slug: slug, Name: "Integration test", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, product.ID) })

	if _, err := store.New(pool).UpsertProductKey(ctx, store.UpsertProductKeyParams{
		ProductID: product.ID, LicensePublicKey: "test", UpdatePublicKey: "test", KeyPrefix: "IT",
	}); err != nil {
		t.Fatal(err)
	}

	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LICENSE_SIGNING_KEY_"+strings.ToUpper(strings.ReplaceAll(slug, "-", "_")), base64.StdEncoding.EncodeToString(seed))
	t.Setenv("UPDATE_SIGNING_KEY_"+strings.ToUpper(strings.ReplaceAll(slug, "-", "_")), base64.StdEncoding.EncodeToString(seed))

	raw, hash, last4 := NewKey("IT")
	minutes := int32(15)
	expires := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}
	if _, err := store.New(pool).CreateLicense(ctx, store.CreateLicenseParams{
		ProductID: product.ID, KeyHash: hash, KeyLast4: last4, PlanName: "Trial",
		KeyType: "trial", TrialMinutes: &minutes, StartsOnActivation: true, OneTimeActivation: true,
		GraceSeconds: 0, ConvertFrom: []byte(`{"max_accounts":3}`), ConvertFrom_2: []byte(`["warmup"]`),
		ExpiresAt: expires, MaxActivations: 1,
	}); err != nil {
		t.Fatal(err)
	}

	service := New(pool, config.Config{})
	installation := strings.Repeat("a", 64)
	first, err := service.Activate(ctx, ActivateRequest{
		Key: raw, InstallationHash: installation, Product: slug, AppVersion: "test",
	}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Lease == "" || first.KeyType != "trial" || first.GraceSeconds != 0 || first.RemainingSeconds == nil {
		t.Fatalf("unexpected activation response: %+v", first)
	}
	if *first.RemainingSeconds > 15*60 || *first.RemainingSeconds < 14*60 {
		t.Fatalf("trial clock not server-authoritative: %d", *first.RemainingSeconds)
	}

	if _, err := service.Activate(ctx, ActivateRequest{
		Key: raw, InstallationHash: strings.Repeat("b", 64), Product: slug,
	}, ""); !errors.Is(err, ErrUsed) {
		t.Fatalf("expected ErrUsed for a second installation, got %v", err)
	}

	if err := service.Deactivate(ctx, ActivateRequest{Key: raw, InstallationHash: installation, Product: slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Heartbeat(ctx, ActivateRequest{
		Key: raw, InstallationHash: installation, Product: slug,
	}, ""); !errors.Is(err, ErrNoActivation) {
		t.Fatalf("expected ErrNoActivation after deactivate, got %v", err)
	}
}
