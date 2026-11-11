package api

import (
	"net/http"
	"strconv"

	"github.com/vaproh/keydeck/internal/store"
)

func (a *App) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.q.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	expiring, _ := a.q.ExpiringLicenses(r.Context())
	products, _ := a.q.ListProducts(r.Context())
	rows := make([]map[string]any, 0, len(expiring))
	for _, row := range expiring {
		rows = append(rows, map[string]any{
			"id": row.ID, "product_slug": row.ProductSlug, "plan_name": row.PlanName,
			"key_last4": row.KeyLast4, "expires_at": tsPtr(row.ExpiresAt), "status": row.Status,
		})
	}
	items := make([]map[string]any, 0, len(products))
	for _, p := range products {
		items = append(items, productJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats, "expiring": rows, "products": items})
}

func (a *App) auditList(w http.ResponseWriter, r *http.Request) {
	productID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	events, err := a.q.ListAudit(r.Context(), store.ListAuditParams{
		Column1: productID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
