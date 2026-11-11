package licensing

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/license-desk/internal/store"
)

func TestEffectiveExpiry(t *testing.T) {
	now := time.Now().UTC()
	row := store.License{KeyType: "timed", ExpiresAt: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}}
	expiry, has, lifetime := effectiveExpiry(row)
	if !has || lifetime || !expiry.Equal(row.ExpiresAt.Time) {
		t.Fatal("timed key should use expires_at")
	}

	row.KeyType = "lifetime"
	if _, has, lifetime = effectiveExpiry(row); has || !lifetime {
		t.Fatal("lifetime key must not expire")
	}

	row.KeyType = "trial"
	row.StartsOnActivation = true
	if _, has, _ = effectiveExpiry(row); has {
		t.Fatal("unactivated trial must wait for activation")
	}
	row.ActivationExpiresAt = pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true}
	if expiry, has, _ = effectiveExpiry(row); !has || !expiry.Equal(row.ActivationExpiresAt.Time) {
		t.Fatal("trial should use activation expiry")
	}
}

func TestNewKeyIsHashed(t *testing.T) {
	raw, hash, last4 := NewKey("REO")
	if len(raw) < 12 || len(hash) != 64 || len(last4) != 4 {
		t.Fatalf("unexpected key shape: %q %q %q", raw, hash, last4)
	}
	if hashKey(raw) != hash {
		t.Fatal("hash mismatch")
	}
}
