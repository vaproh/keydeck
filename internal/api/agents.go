package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/store"
)

var allowedScopes = map[string]bool{
	"products:read":  true,
	"products:write": true,
	"licenses:read":  true,
	"licenses:write": true,
	"releases:write": true,
	"audit:read":     true,
}

// authorize enforces agent scopes and product isolation. Human sessions pass through.
func (a *App) authorize(w http.ResponseWriter, r *http.Request, scope string, productID int64) bool {
	agent, ok := auth.AgentFrom(r.Context())
	if !ok {
		return true
	}
	if !agent.Has(scope) {
		writeError(w, http.StatusForbidden, "token is missing "+scope)
		return false
	}
	if agent.ProductID != 0 && productID != 0 && agent.ProductID != productID {
		writeError(w, http.StatusForbidden, "token is scoped to another product")
		return false
	}
	return true
}

func (a *App) humanOnly(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := auth.AgentFrom(r.Context()); ok {
		writeError(w, http.StatusForbidden, "agent tokens cannot manage other tokens")
		return false
	}
	return true
}

func (a *App) listAdminTokens(w http.ResponseWriter, r *http.Request) {
	if !a.humanOnly(w, r) {
		return
	}
	rows, err := a.q.ListAdminTokens(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load tokens")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var scopes []string
		_ = json.Unmarshal(row.ScopesJson, &scopes)
		out = append(out, map[string]any{
			"id": row.ID, "name": row.Name, "hint": row.TokenHint, "scopes": scopes,
			"product_id": row.ProductID, "expires_at": tsPtr(row.ExpiresAt),
			"last_used_at": tsPtr(row.LastUsedAt), "revoked_at": tsPtr(row.RevokedAt),
			"created_at": tsPtr(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (a *App) createAdminToken(w http.ResponseWriter, r *http.Request) {
	if !a.humanOnly(w, r) {
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Scopes      []string `json:"scopes"`
		ProductID   int64    `json:"product_id"`
		ExpiresDays int32    `json:"expires_days"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(body.Scopes) == 0 {
		writeError(w, http.StatusBadRequest, "at least one scope is required")
		return
	}
	for _, scope := range body.Scopes {
		if !allowedScopes[scope] {
			writeError(w, http.StatusBadRequest, "unknown scope: "+scope)
			return
		}
	}
	if body.ExpiresDays < 0 || body.ExpiresDays > 3650 {
		writeError(w, http.StatusBadRequest, "expires_days must be between 0 and 3650")
		return
	}
	if body.ProductID > 0 {
		if _, err := a.q.GetProduct(r.Context(), body.ProductID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown product")
			return
		}
	}
	raw, hashed, hint := auth.NewAdminToken()
	scopesJSON, _ := json.Marshal(body.Scopes)
	var expiry pgtype.Timestamptz
	if body.ExpiresDays > 0 {
		t := time.Now().UTC().Add(time.Duration(body.ExpiresDays) * 24 * time.Hour)
		expiry = pgtype.Timestamptz{Time: t, Valid: true}
	}
	row, err := a.q.CreateAdminToken(r.Context(), store.CreateAdminTokenParams{
		Name: body.Name, TokenHash: hashed, TokenHint: hint,
		ConvertFrom: scopesJSON, ProductID: body.ProductID, ExpiresAt: expiry,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create token")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "token.create", nil, row.Name, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"token": raw, "id": row.ID, "hint": hint})
}

func (a *App) revokeAdminToken(w http.ResponseWriter, r *http.Request) {
	if !a.humanOnly(w, r) {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	if err := a.q.RevokeAdminToken(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke token")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "token.revoke", nil, "token", nil)
	w.WriteHeader(http.StatusNoContent)
}
