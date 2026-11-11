package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/store"
)

type denyVerifier struct{}

func (denyVerifier) Verify(context.Context, string, string) error { return ErrTurnstile }

func TestLoginFailureNeverCreatesSessionWithPostgres(t *testing.T) {
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
	passwordHash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		AdminPasswordHash: passwordHash,
		AdminTOTPSecret:   "JBSWY3DPEHPK3PXP",
		AdminActor:        "operator",
	}
	q := store.New(pool)
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_sessions`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	service := NewWithTurnstile(cfg, q, denyVerifier{})
	if _, err := service.Login(ctx, "203.0.113.50", "correct horse battery staple", "123456", "token"); !errors.Is(err, ErrTurnstile) {
		t.Fatalf("expected ErrTurnstile, got %v", err)
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_sessions`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("session created after failed verification: %d -> %d", before, after)
	}
}

func TestLoginSuccessStillNeedsPasswordAndTOTPWithPostgres(t *testing.T) {
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
	passwordHash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	secret := "JBSWY3DPEHPK3PXP"
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminPasswordHash: passwordHash, AdminTOTPSecret: secret, AdminActor: "operator"}
	service := NewWithTurnstile(cfg, store.New(pool), allowVerifier{})
	session, err := service.Login(ctx, "203.0.113.51", "correct horse battery staple", code, "token")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.CSRF == "" {
		t.Fatalf("unexpected session: %+v", session)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM admin_sessions WHERE id = $1`, hash(session.ID)) })
	if _, err := service.Login(ctx, "203.0.113.51", "wrong", code, "token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected wrong password to fail, got %v", err)
	}
}
