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

func TestProductLicenseAndReleaseWithPostgres(t *testing.T) {
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
	slug := fmt.Sprintf("store-itest-%d", time.Now().UnixNano())
	product, err := q.CreateProduct(ctx, store.CreateProductParams{Slug: slug, Name: "Store integration", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, product.ID) })

	if _, err := q.UpsertProductKey(ctx, store.UpsertProductKeyParams{
		ProductID: product.ID, LicensePublicKey: "license", UpdatePublicKey: "update", KeyPrefix: "ST",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AddLimit(ctx, store.AddLimitParams{ProductID: product.ID, Key: "max_accounts", Label: "Accounts"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AddFeature(ctx, store.AddFeatureParams{ProductID: product.ID, Key: "warmup", Label: "Warmup"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePreset(ctx, store.CreatePresetParams{
		ProductID: product.ID, Name: "Trial", ConvertFrom: []byte(`{"max_accounts":3}`),
		ConvertFrom_2: []byte(`["warmup"]`), ValidityDays: ptrInt32(0), MaxActivations: 1,
	}); err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	license, err := q.CreateLicense(ctx, store.CreateLicenseParams{
		ProductID: product.ID, KeyHash: hex.EncodeToString(random), KeyLast4: "TEST",
		PlanName: "Trial", HolderName: "Store holder", KeyType: "trial",
		TrialMinutes: ptrInt32(15), StartsOnActivation: true, OneTimeActivation: true,
		GraceSeconds: 0, ConvertFrom: []byte(`{"max_accounts":3}`),
		ConvertFrom_2: []byte(`["warmup"]`), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		MaxActivations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if license.HolderName != "Store holder" {
		t.Fatalf("holder not returned: %+v", license)
	}
	rows, err := q.ListLicenses(ctx, store.ListLicensesParams{Column1: product.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].HolderName != "Store holder" {
		t.Fatalf("unexpected licenses: %+v", rows)
	}
	if _, err := q.CreateRelease(ctx, store.CreateReleaseParams{
		ProductID: product.ID, Version: "1.0.0", Channel: "stable", Platform: "windows-amd64",
		MinVersion: "0.9.0", RolloutPercent: 100, ArtifactName: "test.zip",
		ArtifactPath: "/tmp/test.zip", Sha256: "abc", SizeBytes: 10,
		Manifest: `{"version":"1.0.0"}`, ManifestSignature: "sig", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	releases, err := q.ListReleases(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].Version != "1.0.0" {
		t.Fatalf("unexpected releases: %+v", releases)
	}
}

func ptrInt32(v int32) *int32 { return &v }
