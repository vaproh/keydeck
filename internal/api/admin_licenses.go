package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/licensing"
	"github.com/vaproh/keydeck/internal/store"
)

func (a *App) listLicenses(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r, "licenses:read", 0) {
		return
	}
	productID, _ := parseInt(r.URL.Query().Get("product_id"))
	if agent, ok := auth.AgentFrom(r.Context()); ok && agent.ProductID != 0 {
		productID = agent.ProductID
	}
	if productID < 0 {
		productID = 0
	}
	limit, _ := parseInt(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset, _ := parseInt(r.URL.Query().Get("offset"))
	rows, err := a.q.ListLicenses(r.Context(), store.ListLicensesParams{Column1: productID, Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load licenses")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := licenseJSON(licenseFromListRow(row))
		item["product_slug"] = row.ProductSlug
		item["activation_count"] = row.ActivationCount
		item["display_status"] = licenseStatus(licenseFromListRow(row), row.ActivationCount)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"licenses": out})
}

func (a *App) createLicense(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	if !a.authorize(w, r, "licenses:write", productID) {
		return
	}
	var body struct {
		PlanName           string          `json:"plan_name"`
		HolderName         string          `json:"holder_name"`
		PresetID           int64           `json:"preset_id"`
		KeyType            string          `json:"key_type"`
		TrialMinutes       *int32          `json:"trial_minutes"`
		ValidityDays       *int32          `json:"validity_days"`
		StartsOnActivation bool            `json:"starts_on_activation"`
		OneTimeActivation  bool            `json:"one_time_activation"`
		GraceSeconds       *int32          `json:"grace_seconds"`
		LeaseTTLSeconds    int32           `json:"lease_ttl_seconds"`
		Limits             json.RawMessage `json:"limits"`
		Features           json.RawMessage `json:"features"`
		MaxActivations     int32           `json:"max_activations"`
		Notes              string          `json:"notes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.KeyType == "" {
		body.KeyType = "timed"
	}
	if body.KeyType != "trial" && body.KeyType != "timed" && body.KeyType != "lifetime" {
		writeError(w, http.StatusBadRequest, "invalid key type")
		return
	}
	product, err := a.q.GetProduct(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if body.PresetID > 0 {
		preset, err := a.q.GetPreset(r.Context(), store.GetPresetParams{ID: body.PresetID, ProductID: productID})
		if err != nil {
			writeError(w, http.StatusBadRequest, "plan preset not found")
			return
		}
		if body.PlanName == "" {
			body.PlanName = preset.Name
		}
		if len(body.Limits) == 0 {
			body.Limits = preset.LimitsJson
		}
		if len(body.Features) == 0 {
			body.Features = preset.FeaturesJson
		}
		if body.ValidityDays == nil && preset.ValidityDays != nil {
			body.ValidityDays = preset.ValidityDays
		}
		if body.MaxActivations == 0 {
			body.MaxActivations = preset.MaxActivations
		}
	}
	keyRow, err := a.q.GetProductKey(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "register product public keys first")
		return
	}
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)
	grace := int32(86400)
	if body.GraceSeconds != nil {
		grace = *body.GraceSeconds
	}
	leaseTTL := body.LeaseTTLSeconds
	if leaseTTL == 0 {
		leaseTTL = 86400
	}
	if leaseTTL < 60 || leaseTTL > 2592000 {
		writeError(w, http.StatusBadRequest, "lease_ttl_seconds must be between 60 and 2592000")
		return
	}
	oneTime := body.OneTimeActivation
	starts := body.StartsOnActivation
	switch body.KeyType {
	case "trial":
		if body.TrialMinutes == nil || *body.TrialMinutes < 1 {
			writeError(w, http.StatusBadRequest, "trial_minutes is required")
			return
		}
		starts, oneTime, grace = true, true, 0
		if body.MaxActivations == 0 {
			body.MaxActivations = 1
		}
		expiry = now.Add(time.Duration(*body.TrialMinutes) * time.Minute)
	case "timed":
		if body.ValidityDays == nil || *body.ValidityDays < 1 {
			writeError(w, http.StatusBadRequest, "validity_days is required")
			return
		}
		expiry = now.Add(time.Duration(*body.ValidityDays) * 24 * time.Hour)
	case "lifetime":
		expiry = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	if body.MaxActivations <= 0 {
		body.MaxActivations = 1
	}
	if body.PlanName == "" {
		body.PlanName = strings.Title(body.KeyType)
	}
	body.HolderName = strings.TrimSpace(body.HolderName)
	if len(body.HolderName) > 120 {
		writeError(w, http.StatusBadRequest, "holder_name is too long")
		return
	}
	limits := body.Limits
	if len(limits) == 0 {
		limits = json.RawMessage("{}")
	}
	features := body.Features
	if len(features) == 0 {
		features = json.RawMessage("[]")
	}
	rawKey, hash, last4 := licensing.NewKey(keyRow.KeyPrefix)
	row, err := a.q.CreateLicense(r.Context(), store.CreateLicenseParams{
		ProductID: productID, KeyHash: hash, KeyLast4: last4, PlanName: body.PlanName, HolderName: body.HolderName,
		KeyType: body.KeyType, TrialMinutes: body.TrialMinutes, ValidityDays: body.ValidityDays,
		StartsOnActivation: starts, OneTimeActivation: oneTime, GraceSeconds: grace, LeaseTtlSeconds: leaseTTL,
		ConvertFrom: limits, ConvertFrom_2: features, ExpiresAt: pt(expiry),
		MaxActivations: body.MaxActivations, Notes: body.Notes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create license")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "license.issue", &product.ID, row.KeyLast4, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"key": rawKey, "license": licenseJSON(row)})
}

func (a *App) getLicense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid license")
		return
	}
	row, err := a.q.GetLicense(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}
	if !a.authorize(w, r, "licenses:read", row.ProductID) {
		return
	}
	activations, _ := a.q.ListActivations(r.Context(), id)
	item := licenseJSON(row)
	if product, err := a.q.GetProduct(r.Context(), row.ProductID); err == nil {
		item["product_slug"] = product.Slug
	}
	item["activation_count"] = len(activations)
	item["display_status"] = licenseStatus(row, int64(len(activations)))
	events, _ := a.q.ListLicenseEvents(r.Context(), store.ListLicenseEventsParams{LicenseID: id, Limit: 200})
	heartbeats, _ := a.q.ListLicenseHeartbeats(r.Context(), store.ListLicenseHeartbeatsParams{LicenseID: id, Limit: 200})
	activationLogs := make([]map[string]any, 0, len(events))
	for _, event := range events {
		activationLogs = append(activationLogs, eventJSON(event))
	}
	beatLogs := make([]map[string]any, 0, len(heartbeats))
	for _, event := range heartbeats {
		beatLogs = append(beatLogs, eventJSON(event))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"license": item, "activations": activations,
		"activation_logs": activationLogs, "heartbeats": beatLogs,
	})
}

func (a *App) updateLicense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid license")
		return
	}
	row, err := a.q.GetLicense(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}
	if !a.authorize(w, r, "licenses:write", row.ProductID) {
		return
	}
	var body struct {
		PlanName           *string         `json:"plan_name"`
		HolderName         *string         `json:"holder_name"`
		KeyType            *string         `json:"key_type"`
		TrialMinutes       *int32          `json:"trial_minutes"`
		ValidityDays       *int32          `json:"validity_days"`
		StartsOnActivation *bool           `json:"starts_on_activation"`
		OneTimeActivation  *bool           `json:"one_time_activation"`
		Limits             json.RawMessage `json:"limits"`
		Features           json.RawMessage `json:"features"`
		MaxActivations     *int32          `json:"max_activations"`
		Notes              *string         `json:"notes"`
		GraceSeconds       *int32          `json:"grace_seconds"`
		LeaseTTLSeconds    *int32          `json:"lease_ttl_seconds"`
		ExpiresAt          *string         `json:"expires_at"`
		ActivationExpires  *string         `json:"activation_expires_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.PlanName != nil {
		row.PlanName = *body.PlanName
	}
	if body.HolderName != nil {
		row.HolderName = strings.TrimSpace(*body.HolderName)
		if len(row.HolderName) > 120 {
			writeError(w, http.StatusBadRequest, "holder_name is too long")
			return
		}
	}
	if body.KeyType != nil {
		if *body.KeyType != "trial" && *body.KeyType != "timed" && *body.KeyType != "lifetime" {
			writeError(w, http.StatusBadRequest, "invalid key type")
			return
		}
		row.KeyType = *body.KeyType
	}
	if body.TrialMinutes != nil {
		if *body.TrialMinutes < 0 {
			writeError(w, http.StatusBadRequest, "trial_minutes cannot be negative")
			return
		}
		if *body.TrialMinutes == 0 {
			row.TrialMinutes = nil
		} else {
			row.TrialMinutes = body.TrialMinutes
		}
	}
	if body.ValidityDays != nil {
		if *body.ValidityDays < 0 {
			writeError(w, http.StatusBadRequest, "validity_days cannot be negative")
			return
		}
		if *body.ValidityDays == 0 {
			row.ValidityDays = nil
		} else {
			row.ValidityDays = body.ValidityDays
		}
	}
	if body.StartsOnActivation != nil {
		row.StartsOnActivation = *body.StartsOnActivation
	}
	if body.OneTimeActivation != nil {
		row.OneTimeActivation = *body.OneTimeActivation
	}
	if row.KeyType == "trial" {
		if row.TrialMinutes == nil || *row.TrialMinutes < 1 {
			writeError(w, http.StatusBadRequest, "trial keys need trial_minutes")
			return
		}
		row.StartsOnActivation, row.OneTimeActivation, row.GraceSeconds = true, true, 0
	}
	if row.KeyType == "lifetime" {
		row.ExpiresAt = pt(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
		row.ActivationExpiresAt = pgtype.Timestamptz{}
	}
	if body.TrialMinutes != nil && row.KeyType == "trial" && row.TrialMinutes != nil {
		duration := time.Duration(*row.TrialMinutes) * time.Minute
		if row.FirstActivatedAt.Valid {
			row.ActivationExpiresAt = pt(row.FirstActivatedAt.Time.Add(duration))
		} else {
			row.ExpiresAt = pt(row.CreatedAt.Time.Add(duration))
		}
	}
	if body.ValidityDays != nil && row.KeyType == "timed" && row.ValidityDays != nil {
		base := row.CreatedAt.Time
		if row.FirstActivatedAt.Valid {
			base = row.FirstActivatedAt.Time
		}
		end := base.Add(time.Duration(*row.ValidityDays) * 24 * time.Hour)
		row.ExpiresAt = pt(end)
		if row.FirstActivatedAt.Valid {
			row.ActivationExpiresAt = pt(end)
		}
	}
	if len(body.Limits) > 0 {
		row.LimitsJson = body.Limits
	}
	if len(body.Features) > 0 {
		row.FeaturesJson = body.Features
	}
	if body.MaxActivations != nil {
		row.MaxActivations = *body.MaxActivations
	}
	if body.Notes != nil {
		row.Notes = *body.Notes
	}
	if body.GraceSeconds != nil {
		row.GraceSeconds = *body.GraceSeconds
	}
	if row.KeyType == "trial" {
		row.GraceSeconds = 0
	}
	if body.LeaseTTLSeconds != nil {
		if *body.LeaseTTLSeconds < 60 || *body.LeaseTTLSeconds > 2592000 {
			writeError(w, http.StatusBadRequest, "lease_ttl_seconds must be between 60 and 2592000")
			return
		}
		row.LeaseTtlSeconds = *body.LeaseTTLSeconds
	}
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expiry")
			return
		}
		if row.KeyType != "lifetime" {
			row.ExpiresAt = pt(t)
		}
	}
	if body.ActivationExpires != nil {
		if *body.ActivationExpires == "" {
			row.ActivationExpiresAt = pgtype.Timestamptz{}
		} else {
			t, err := time.Parse(time.RFC3339, *body.ActivationExpires)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid activation expiry")
				return
			}
			row.ActivationExpiresAt = pt(t)
		}
	}
	updated, err := a.q.UpdateLicense(r.Context(), store.UpdateLicenseParams{
		ID: id, PlanName: row.PlanName, HolderName: row.HolderName,
		KeyType: row.KeyType, TrialMinutes: row.TrialMinutes, ValidityDays: row.ValidityDays,
		StartsOnActivation: row.StartsOnActivation, OneTimeActivation: row.OneTimeActivation,
		ConvertFrom: row.LimitsJson, ConvertFrom_2: row.FeaturesJson,
		MaxActivations: row.MaxActivations, Notes: row.Notes, GraceSeconds: row.GraceSeconds,
		LeaseTtlSeconds: row.LeaseTtlSeconds, ExpiresAt: row.ExpiresAt,
		ActivationExpiresAt: row.ActivationExpiresAt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update license")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "license.update", &row.ProductID, updated.KeyLast4, nil)
	writeJSON(w, http.StatusOK, licenseJSON(updated))
}

func (a *App) licenseAction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid license")
		return
	}
	action := chi.URLParam(r, "action")
	row, err := a.q.GetLicense(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}
	if !a.authorize(w, r, "licenses:write", row.ProductID) {
		return
	}
	session := auth.SessionFrom(r.Context())
	switch action {
	case "revoke":
		_ = a.q.RevokeLicense(r.Context(), id)
	case "reset":
		if _, err := a.q.ResetLicense(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, "license not found")
			return
		}
	case "extend":
		var body struct {
			Days int32 `json:"days"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Days < 1 || body.Days > 3650 {
			writeError(w, http.StatusBadRequest, "days must be between 1 and 3650")
			return
		}
		expiry := row.ExpiresAt.Time.Add(time.Duration(body.Days) * 24 * time.Hour)
		activation := row.ActivationExpiresAt
		if activation.Valid {
			activation.Time = activation.Time.Add(time.Duration(body.Days) * 24 * time.Hour)
		}
		if _, err := a.q.ExtendLicense(r.Context(), store.ExtendLicenseParams{ID: id, ExpiresAt: pt(expiry), ActivationExpiresAt: activation}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not extend license")
			return
		}
	default:
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	a.audit(r.Context(), session.Actor, "license."+action, nil, strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func pt(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func licenseFromListRow(row store.ListLicensesRow) store.License {
	return store.License{
		ID: row.ID, ProductID: row.ProductID, KeyHash: row.KeyHash, KeyLast4: row.KeyLast4,
		PlanName: row.PlanName, HolderName: row.HolderName, KeyType: row.KeyType, TrialMinutes: row.TrialMinutes,
		ValidityDays: row.ValidityDays, StartsOnActivation: row.StartsOnActivation,
		OneTimeActivation: row.OneTimeActivation, GraceSeconds: row.GraceSeconds, LeaseTtlSeconds: row.LeaseTtlSeconds,
		FirstActivatedAt: row.FirstActivatedAt, ActivationExpiresAt: row.ActivationExpiresAt,
		UsedAt: row.UsedAt, LimitsJson: row.LimitsJson, FeaturesJson: row.FeaturesJson,
		ExpiresAt: row.ExpiresAt, MaxActivations: row.MaxActivations, Status: row.Status,
		Notes: row.Notes, CreatedAt: row.CreatedAt,
	}
}
