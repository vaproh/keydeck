package licensing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vaproh/license-desk/internal/config"
	"github.com/vaproh/license-desk/internal/store"
)

var (
	ErrInvalid      = errors.New("invalid license")
	ErrExpired      = errors.New("license expired")
	ErrRevoked      = errors.New("license revoked")
	ErrActivation   = errors.New("activation limit reached")
	ErrUsed         = errors.New("license already used")
	ErrProduct      = errors.New("license is for another product")
	ErrProductOff   = errors.New("product is disabled")
	ErrNoActivation = errors.New("activation not found")
	ErrMalformed    = errors.New("malformed license request")
)

type Service struct {
	pool  *pgxpool.Pool
	q     *store.Queries
	cfg   config.Config
	clock func() time.Time
}

func New(pool *pgxpool.Pool, cfg config.Config) *Service {
	return &Service{pool: pool, q: store.New(pool), cfg: cfg, clock: func() time.Time { return time.Now().UTC() }}
}

type ActivateRequest struct {
	Key              string `json:"key"`
	InstallationHash string `json:"installation_hash"`
	MachineHash      string `json:"machine_hash"`
	AppVersion       string `json:"app_version"`
	Product          string `json:"product"`
}

type Response struct {
	Lease            string          `json:"lease"`
	PlanName         string          `json:"plan_name"`
	KeyType          string          `json:"key_type"`
	Lifetime         bool            `json:"lifetime"`
	Limits           json.RawMessage `json:"limits"`
	Features         json.RawMessage `json:"features"`
	ServerNow        int64           `json:"server_now"`
	ExpiresAt        *int64          `json:"expires_at"`
	RemainingSeconds *int64          `json:"remaining_seconds"`
	GraceSeconds     int32           `json:"grace_seconds"`
}

func (s *Service) Activate(ctx context.Context, req ActivateRequest, ip string) (Response, error) {
	return s.run(ctx, req, ip, true)
}

func (s *Service) Heartbeat(ctx context.Context, req ActivateRequest, ip string) (Response, error) {
	return s.run(ctx, req, ip, false)
}

func (s *Service) Deactivate(ctx context.Context, req ActivateRequest) error {
	row, _, err := s.license(ctx, req)
	if err != nil {
		return err
	}
	return s.q.RevokeActivation(ctx, store.RevokeActivationParams{LicenseID: row.ID, InstallationHash: req.InstallationHash})
}

func (s *Service) run(ctx context.Context, req ActivateRequest, ip string, allowCreate bool) (Response, error) {
	if len(req.Key) < 8 || len(req.InstallationHash) != 64 || req.Product == "" {
		return Response{}, ErrMalformed
	}
	row, product, err := s.license(ctx, req)
	if err != nil {
		return Response{}, err
	}
	now := s.clock()
	expiry, hasExpiry, lifetime := effectiveExpiry(row)
	if hasExpiry && !now.Before(expiry) {
		return Response{}, ErrExpired
	}

	activation, err := s.q.GetActivation(ctx, store.GetActivationParams{LicenseID: row.ID, InstallationHash: req.InstallationHash})
	hasActivation := err == nil && !activation.RevokedAt.Valid
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Response{}, err
	}
	if !hasActivation && !allowCreate {
		return Response{}, ErrNoActivation
	}
	if !hasActivation && row.OneTimeActivation && row.UsedAt.Valid {
		return Response{}, ErrUsed
	}
	if !hasActivation {
		count, err := s.q.CountActiveActivations(ctx, row.ID)
		if err != nil {
			return Response{}, err
		}
		if count >= int64(row.MaxActivations) {
			return Response{}, ErrActivation
		}
		if _, err := s.q.CreateActivation(ctx, store.CreateActivationParams{
			LicenseID:        row.ID,
			InstallationHash: req.InstallationHash,
			MachineHash:      req.MachineHash,
			AppVersion:       req.AppVersion,
			Ip:               ip,
		}); err != nil {
			return Response{}, err
		}
	} else {
		if err := s.q.TouchActivation(ctx, store.TouchActivationParams{
			LicenseID: row.ID, InstallationHash: req.InstallationHash, AppVersion: req.AppVersion,
		}); err != nil {
			return Response{}, err
		}
	}

	if row.StartsOnActivation && !row.FirstActivatedAt.Valid {
		var duration time.Duration
		switch {
		case row.KeyType == "trial" && row.TrialMinutes != nil:
			duration = time.Duration(*row.TrialMinutes) * time.Minute
		case row.ValidityDays != nil:
			duration = time.Duration(*row.ValidityDays) * 24 * time.Hour
		case row.ExpiresAt.Valid:
			duration = row.ExpiresAt.Time.Sub(now)
		}
		if duration <= 0 {
			return Response{}, ErrInvalid
		}
		row, err = s.q.ActivateLicense(ctx, store.ActivateLicenseParams{
			ID:                  row.ID,
			FirstActivatedAt:    ts(now),
			ActivationExpiresAt: ts(now.Add(duration)),
		})
		if err != nil {
			return Response{}, err
		}
		expiry, hasExpiry, lifetime = effectiveExpiry(row)
	} else if row.KeyType == "trial" && !row.UsedAt.Valid {
		row, err = s.q.ActivateLicense(ctx, store.ActivateLicenseParams{
			ID: row.ID, FirstActivatedAt: ts(now), ActivationExpiresAt: row.ActivationExpiresAt,
		})
		if err != nil {
			return Response{}, err
		}
		expiry, hasExpiry, lifetime = effectiveExpiry(row)
	}

	claims, err := s.claims(row, product, req.InstallationHash, now, expiry, hasExpiry, lifetime)
	if err != nil {
		return Response{}, err
	}
	lease, err := s.signClaims(claims, product.Slug)
	if err != nil {
		return Response{}, err
	}

	resp := Response{
		Lease:        lease,
		PlanName:     row.PlanName,
		KeyType:      row.KeyType,
		Lifetime:     lifetime,
		Limits:       rawJSON(row.LimitsJson, "{}"),
		Features:     rawJSON(row.FeaturesJson, "[]"),
		ServerNow:    now.Unix(),
		GraceSeconds: row.GraceSeconds,
	}
	if row.KeyType == "trial" {
		resp.GraceSeconds = 0
	}
	if hasExpiry {
		unix := expiry.Unix()
		remaining := int64(expiry.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		resp.ExpiresAt = &unix
		resp.RemainingSeconds = &remaining
	}
	return resp, nil
}

func (s *Service) license(ctx context.Context, req ActivateRequest) (store.License, store.Product, error) {
	row, err := s.q.GetLicenseByHash(ctx, hashKey(req.Key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.License{}, store.Product{}, ErrInvalid
		}
		return store.License{}, store.Product{}, err
	}
	if row.Status != "active" {
		return store.License{}, store.Product{}, ErrRevoked
	}
	product, err := s.q.GetProduct(ctx, row.ProductID)
	if err != nil {
		return store.License{}, store.Product{}, ErrProduct
	}
	if product.Slug != req.Product {
		return store.License{}, store.Product{}, ErrProduct
	}
	if product.Status != "active" {
		return store.License{}, store.Product{}, ErrProductOff
	}
	return row, product, nil
}

func (s *Service) claims(row store.License, product store.Product, installation string, now, expiry time.Time, hasExpiry, lifetime bool) (map[string]any, error) {
	leaseExpiry := now.Add(24 * time.Hour)
	var licenseExpiry *int64
	var remaining *int64
	var graceUntil *int64
	if lifetime {
		licenseExpiry = nil
	} else if hasExpiry {
		unix := expiry.Unix()
		licenseExpiry = &unix
		rem := int64(expiry.Sub(now).Seconds())
		if rem < 0 {
			rem = 0
		}
		remaining = &rem
		grace := int64(row.GraceSeconds)
		if row.KeyType == "trial" {
			grace = 0
		}
		g := expiry.Add(time.Duration(grace) * time.Second).Unix()
		graceUntil = &g
		if expiry.Before(leaseExpiry) {
			leaseExpiry = expiry
		}
	}
	var limits, features any
	_ = json.Unmarshal(rawJSON(row.LimitsJson, "{}"), &limits)
	_ = json.Unmarshal(rawJSON(row.FeaturesJson, "[]"), &features)
	return map[string]any{
		"product":           product.Slug,
		"license_id":        row.ID,
		"plan_name":         row.PlanName,
		"key_type":          row.KeyType,
		"lifetime":          lifetime,
		"limits":            limits,
		"features":          features,
		"installation_hash": installation,
		"server_now":        now.Unix(),
		"issued_at":         now.Unix(),
		"lease_expires_at":  leaseExpiry.Unix(),
		"expires_at":        licenseExpiry,
		"remaining_seconds": remaining,
		"grace_until":       graceUntil,
	}, nil
}

func (s *Service) signClaims(claims map[string]any, product string) (string, error) {
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	key, err := config.SigningKey(product, "LICENSE")
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(key), raw)
	return base64.StdEncoding.EncodeToString(raw) + "." + base64.StdEncoding.EncodeToString(sig), nil
}

func effectiveExpiry(row store.License) (time.Time, bool, bool) {
	if row.KeyType == "lifetime" {
		return time.Time{}, false, true
	}
	if row.StartsOnActivation {
		if !row.ActivationExpiresAt.Valid {
			return time.Time{}, false, false
		}
		return row.ActivationExpiresAt.Time, true, false
	}
	if !row.ExpiresAt.Valid {
		return time.Time{}, false, false
	}
	return row.ExpiresAt.Time, true, false
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func rawJSON(b []byte, fallback string) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(b)
}

func NewKey(prefix string) (string, string, string) {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	raw := fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), raw[len(raw)-4:]
}
