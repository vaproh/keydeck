package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/store"
)

func (a *App) revokeActivation(w http.ResponseWriter, r *http.Request) {
	licenseID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid license")
		return
	}
	activationID, err := parseInt(chi.URLParam(r, "activationID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid activation")
		return
	}
	license, err := a.q.GetLicense(r.Context(), licenseID)
	if err != nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}
	if !a.authorize(w, r, "licenses:write", license.ProductID) {
		return
	}
	if _, err := a.q.GetActivationByID(r.Context(), store.GetActivationByIDParams{ID: activationID, LicenseID: licenseID}); err != nil {
		writeError(w, http.StatusNotFound, "activation not found")
		return
	}
	if err := a.q.RevokeActivationByID(r.Context(), store.RevokeActivationByIDParams{ID: activationID, LicenseID: licenseID}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke activation")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "activation.revoke", &license.ProductID, license.KeyLast4, map[string]any{"activation_id": activationID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) productOverview(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	if !a.authorize(w, r, "products:read", productID) {
		return
	}
	product, err := a.q.GetProduct(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	key, _ := a.q.GetProductKey(r.Context(), productID)
	limits, _ := a.q.ListLimits(r.Context(), productID)
	features, _ := a.q.ListFeatures(r.Context(), productID)
	presets, _ := a.q.ListPresets(r.Context(), productID)
	licenseRows, _ := a.q.ListLicenses(r.Context(), store.ListLicensesParams{Column1: productID, Limit: 500})
	releases, _ := a.q.ListReleases(r.Context(), productID)

	var activeKeys, activations int64
	recentKeys := make([]map[string]any, 0, 8)
	for i, row := range licenseRows {
		license := licenseFromListRow(row)
		status := licenseStatus(license, row.ActivationCount)
		if status == "active" {
			activeKeys++
		}
		activations += row.ActivationCount
		if i < 8 {
			item := licenseJSON(license)
			item["display_status"] = status
			item["activation_count"] = row.ActivationCount
			item["product_slug"] = row.ProductSlug
			recentKeys = append(recentKeys, item)
		}
	}
	recentReleases := releases
	if len(recentReleases) > 8 {
		recentReleases = recentReleases[:8]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"product": productJSON(product), "key": key, "limits": limits, "features": features, "presets": presetJSONList(presets),
		"counts": map[string]any{"keys": len(licenseRows), "active_keys": activeKeys, "activations": activations, "releases": len(releases)},
		"recent_keys": recentKeys, "recent_releases": recentReleases,
	})
}

func (a *App) updatePreset(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	if !a.authorize(w, r, "products:write", productID) {
		return
	}
	presetID, err := parseInt(chi.URLParam(r, "presetID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset")
		return
	}
	preset, err := a.q.GetPreset(r.Context(), store.GetPresetParams{ID: presetID, ProductID: productID})
	if err != nil {
		writeError(w, http.StatusNotFound, "preset not found")
		return
	}
	var body struct {
		Name           *string         `json:"name"`
		Limits         json.RawMessage `json:"limits"`
		Features       json.RawMessage `json:"features"`
		ValidityDays   *int32          `json:"validity_days"`
		MaxActivations *int32          `json:"max_activations"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name != nil {
		preset.Name = strings.TrimSpace(*body.Name)
	}
	if len(body.Limits) > 0 {
		preset.LimitsJson = body.Limits
	}
	if len(body.Features) > 0 {
		preset.FeaturesJson = body.Features
	}
	if body.ValidityDays != nil {
		preset.ValidityDays = body.ValidityDays
	}
	if body.MaxActivations != nil {
		preset.MaxActivations = *body.MaxActivations
	}
	updated, err := a.q.UpdatePreset(r.Context(), store.UpdatePresetParams{
		ID: presetID, ProductID: productID, Name: preset.Name, ConvertFrom: preset.LimitsJson,
		ConvertFrom_2: preset.FeaturesJson, ValidityDays: preset.ValidityDays, MaxActivations: preset.MaxActivations,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update preset")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "preset.update", &productID, updated.Name, nil)
	writeJSON(w, http.StatusOK, presetJSON(updated))
}
