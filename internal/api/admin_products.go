package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r, "products:read", 0) {
		return
	}
	products, err := a.q.ListProducts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load products")
		return
	}
	if agent, ok := auth.AgentFrom(r.Context()); ok && agent.ProductID != 0 {
		scoped, err := a.q.GetProduct(r.Context(), agent.ProductID)
		if err != nil {
			writeError(w, http.StatusForbidden, "token product is unavailable")
			return
		}
		products = []store.Product{scoped}
	}
	out := make([]map[string]any, 0, len(products))
	for _, p := range products {
		out = append(out, productJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": out})
}

func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	if agent, ok := auth.AgentFrom(r.Context()); ok {
		if agent.ProductID != 0 || !a.authorize(w, r, "products:write", 0) {
			writeError(w, http.StatusForbidden, "token cannot create products")
			return
		}
	}
	var body struct {
		Slug             string `json:"slug"`
		Name             string `json:"name"`
		LicensePublicKey string `json:"license_public_key"`
		UpdatePublicKey  string `json:"update_public_key"`
		KeyPrefix        string `json:"key_prefix"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.Name = strings.TrimSpace(body.Name)
	if !slugPattern.MatchString(body.Slug) || body.Name == "" {
		writeError(w, http.StatusBadRequest, "slug and name are required")
		return
	}
	if body.LicensePublicKey == "" || body.UpdatePublicKey == "" {
		licensePublic, updatePublic, err := config.EnsureSigningKeys(body.Slug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not generate signing keys")
			return
		}
		if body.LicensePublicKey == "" {
			body.LicensePublicKey = licensePublic
		}
		if body.UpdatePublicKey == "" {
			body.UpdatePublicKey = updatePublic
		}
	}
	if body.KeyPrefix == "" {
		body.KeyPrefix = strings.ToUpper(body.Slug)
	}
	product, err := a.q.CreateProduct(r.Context(), store.CreateProductParams{Slug: body.Slug, Name: body.Name, Status: "active"})
	if err != nil {
		writeError(w, http.StatusConflict, "product already exists")
		return
	}
	if _, err := a.q.UpsertProductKey(r.Context(), store.UpsertProductKeyParams{
		ProductID: product.ID, LicensePublicKey: body.LicensePublicKey,
		UpdatePublicKey: body.UpdatePublicKey, KeyPrefix: body.KeyPrefix,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store product keys")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "product.create", &product.ID, product.Slug, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"product": productJSON(product)})
}

func (a *App) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:read", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	product, err := a.q.GetProduct(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	key, _ := a.q.GetProductKey(r.Context(), id)
	limits, _ := a.q.ListLimits(r.Context(), id)
	features, _ := a.q.ListFeatures(r.Context(), id)
	presets, _ := a.q.ListPresets(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"product": productJSON(product), "key": key, "limits": limits, "features": features, "presets": presets,
	})
}

func (a *App) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	var body struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Status != "active" && body.Status != "disabled" {
		writeError(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	product, err := a.q.UpdateProduct(r.Context(), store.UpdateProductParams{ID: id, Name: body.Name, Status: body.Status})
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "product.update", &id, product.Slug, nil)
	writeJSON(w, http.StatusOK, map[string]any{"product": productJSON(product)})
}

func (a *App) addLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	var body struct{ Key, Label string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, "limit key is required")
		return
	}
	row, err := a.q.AddLimit(r.Context(), store.AddLimitParams{ProductID: id, Key: strings.TrimSpace(body.Key), Label: body.Label})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add limit")
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (a *App) deleteLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	limitID, err := parseInt(chi.URLParam(r, "limitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	_ = a.q.DeleteLimit(r.Context(), store.DeleteLimitParams{ID: limitID, ProductID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) addFeature(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	var body struct{ Key, Label string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, "feature key is required")
		return
	}
	row, err := a.q.AddFeature(r.Context(), store.AddFeatureParams{ProductID: id, Key: strings.TrimSpace(body.Key), Label: body.Label})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add feature")
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (a *App) deleteFeature(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	featureID, err := parseInt(chi.URLParam(r, "featureID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid feature")
		return
	}
	_ = a.q.DeleteFeature(r.Context(), store.DeleteFeatureParams{ID: featureID, ProductID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) createPreset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	var body struct {
		Name           string          `json:"name"`
		Limits         json.RawMessage `json:"limits"`
		Features       json.RawMessage `json:"features"`
		ValidityDays   *int32          `json:"validity_days"`
		MaxActivations int32           `json:"max_activations"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "preset name is required")
		return
	}
	if len(body.Limits) == 0 {
		body.Limits = json.RawMessage("{}")
	}
	if len(body.Features) == 0 {
		body.Features = json.RawMessage("[]")
	}
	if body.MaxActivations <= 0 {
		body.MaxActivations = 1
	}
	row, err := a.q.CreatePreset(r.Context(), store.CreatePresetParams{
		ProductID: id, Name: body.Name, ConvertFrom: body.Limits, ConvertFrom_2: body.Features,
		ValidityDays: body.ValidityDays, MaxActivations: body.MaxActivations,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save preset")
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (a *App) deletePreset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok || !a.authorize(w, r, "products:write", id) {
		writeError(w, http.StatusBadRequest, "invalid product")
		return
	}
	presetID, err := parseInt(chi.URLParam(r, "presetID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset")
		return
	}
	_ = a.q.DeletePreset(r.Context(), store.DeletePresetParams{ID: presetID, ProductID: id})
	w.WriteHeader(http.StatusNoContent)
}
