package api

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
)

func tsPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC().Format(time.RFC3339)
	return &v
}

func rawOr(b []byte, fallback string) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(b)
}

func productJSON(p store.Product) map[string]any {
	return map[string]any{"id": p.ID, "slug": p.Slug, "name": p.Name, "status": p.Status, "created_at": tsPtr(p.CreatedAt)}
}

func licenseJSON(l store.License) map[string]any {
	return map[string]any{
		"id": l.ID, "product_id": l.ProductID, "key_last4": l.KeyLast4, "plan_name": l.PlanName,
		"key_type": l.KeyType, "trial_minutes": l.TrialMinutes, "validity_days": l.ValidityDays,
		"starts_on_activation": l.StartsOnActivation, "one_time_activation": l.OneTimeActivation,
		"grace_seconds": l.GraceSeconds, "first_activated_at": tsPtr(l.FirstActivatedAt),
		"activation_expires_at": tsPtr(l.ActivationExpiresAt), "used_at": tsPtr(l.UsedAt),
		"limits": rawOr(l.LimitsJson, "{}"), "features": rawOr(l.FeaturesJson, "[]"),
		"expires_at": tsPtr(l.ExpiresAt), "max_activations": l.MaxActivations, "status": l.Status,
		"notes": l.Notes, "created_at": tsPtr(l.CreatedAt),
	}
}

func (a *App) audit(ctx context.Context, actor, action string, productID *int64, target string, metadata map[string]any) {
	raw, _ := json.Marshal(metadata)
	_ = a.q.InsertAudit(ctx, store.InsertAuditParams{ProductID: productID, Actor: actor, Action: action, Target: target, ConvertFrom: raw})
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func parseInt(v string) (int64, error) {
	return strconv.ParseInt(v, 10, 64)
}

func publicFromEnv(product, kind string) string {
	seed, err := config.SigningKey(product, kind)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
}
