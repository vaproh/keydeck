package licensing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	license, err := store.New(pool).CreateLicense(ctx, store.CreateLicenseParams{
		ProductID: product.ID, KeyHash: hash, KeyLast4: last4, PlanName: "Trial", HolderName: "Test holder",
		KeyType: "trial", TrialMinutes: &minutes, StartsOnActivation: true, OneTimeActivation: true,
		GraceSeconds: 0, LeaseTtlSeconds: 60, ConvertFrom: []byte(`{"max_accounts":3}`), ConvertFrom_2: []byte(`["warmup"]`),
		ExpiresAt: expires, MaxActivations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if license.HolderName != "Test holder" {
		t.Fatalf("holder name was not persisted: %q", license.HolderName)
	}

	service := New(pool, config.Config{})
	installation := strings.Repeat("a", 64)
	first, err := service.Activate(ctx, ActivateRequest{
		Key: raw, InstallationHash: installation, Product: slug, AppVersion: "test",
		DeviceName: "QA machine", OS: "Windows", OSVersion: "11", Arch: "amd64", UserAgent: "keydeck-test",
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
	activationLogs, err := store.New(pool).ListLicenseEvents(ctx, store.ListLicenseEventsParams{LicenseID: license.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(activationLogs) != 1 || activationLogs[0].EventType != "activation" || activationLogs[0].ActivationID == nil {
		t.Fatalf("activation event missing: %+v", activationLogs)
	}
	if activationLogs[0].DeviceName != "QA machine" || activationLogs[0].Os != "Windows" {
		t.Fatalf("device data missing: %+v", activationLogs[0])
	}
	parts := strings.Split(first.Lease, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected lease format: %q", first.Lease)
	}
	claimsRaw, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatal(err)
	}
	leaseExpiry, _ := claims["lease_expires_at"].(float64)
	serverNow, _ := claims["server_now"].(float64)
	if int64(leaseExpiry-serverNow) != 60 {
		t.Fatalf("lease TTL not applied: expiry=%v now=%v", leaseExpiry, serverNow)
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
	beats, err := store.New(pool).ListLicenseHeartbeats(ctx, store.ListLicenseHeartbeatsParams{LicenseID: license.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 1 || beats[0].ActivationID != nil {
		t.Fatalf("unknown heartbeat was not recorded: %+v", beats)
	}
	if _, err := pool.Exec(ctx, `UPDATE license_events SET created_at = now() - interval '8 days' WHERE license_id = $1 AND event_type = 'heartbeat'`, license.ID); err != nil {
		t.Fatal(err)
	}
	removed, err := service.CleanupHeartbeats(ctx)
	if err != nil || removed < 1 {
		t.Fatalf("cleanup removed %d: %v", removed, err)
	}
	logs, _ := store.New(pool).ListLicenseEvents(ctx, store.ListLicenseEventsParams{LicenseID: license.ID, Limit: 10})
	if len(logs) < 2 {
		t.Fatalf("activation logs were removed: %+v", logs)
	}
}
