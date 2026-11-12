package api

import (
	"net/http"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/store"
)

func eventJSON(e store.LicenseEvent) map[string]any {
	return map[string]any{
		"id": e.ID, "license_id": e.LicenseID, "activation_id": e.ActivationID,
		"kind": e.EventType, "matched": e.ActivationID != nil,
		"installation_hash": e.InstallationHash, "machine_hash": e.MachineHash,
		"app_version": e.AppVersion, "ip": e.Ip, "device_name": e.DeviceName,
		"os": e.Os, "os_version": e.OsVersion, "arch": e.Arch,
		"user_agent": e.UserAgent, "created_at": tsPtr(e.CreatedAt),
	}
}

func (a *App) listHeartbeats(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r, "audit:read", 0) {
		return
	}
	productID, _ := parseInt(r.URL.Query().Get("product_id"))
	licenseID, _ := parseInt(r.URL.Query().Get("license_id"))
	if agent, ok := auth.AgentFrom(r.Context()); ok && agent.ProductID != 0 {
		productID = agent.ProductID
	}
	limit, _ := parseInt(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset, _ := parseInt(r.URL.Query().Get("offset"))
	rows, err := a.q.ListHeartbeats(r.Context(), store.ListHeartbeatsParams{
		Column1: productID, Column2: licenseID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load heartbeats")
		return
	}
	total, _ := a.q.CountHeartbeats(r.Context(), store.CountHeartbeatsParams{Column1: productID, Column2: licenseID})
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id": row.ID, "license_id": row.LicenseID, "activation_id": row.ActivationID,
			"matched": row.ActivationID != nil, "installation_hash": row.InstallationHash,
			"machine_hash": row.MachineHash, "app_version": row.AppVersion, "ip": row.Ip,
			"device_name": row.DeviceName, "os": row.Os, "os_version": row.OsVersion,
			"arch": row.Arch, "user_agent": row.UserAgent, "created_at": tsPtr(row.CreatedAt),
			"holder_name": row.HolderName, "plan_name": row.PlanName, "key_last4": row.KeyLast4,
			"product_slug": row.ProductSlug, "product_name": row.ProductName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"heartbeats": out, "total": total})
}

func (a *App) licenseEvents(w http.ResponseWriter, r *http.Request, licenseID, productID int64) {
	if !a.authorize(w, r, "licenses:read", productID) {
		return
	}
	events, _ := a.q.ListLicenseEvents(r.Context(), store.ListLicenseEventsParams{LicenseID: licenseID, Limit: 200})
	heartbeats, _ := a.q.ListLicenseHeartbeats(r.Context(), store.ListLicenseHeartbeatsParams{LicenseID: licenseID, Limit: 200})
	activationLogs := make([]map[string]any, 0, len(events))
	for _, event := range events {
		activationLogs = append(activationLogs, eventJSON(event))
	}
	beatLogs := make([]map[string]any, 0, len(heartbeats))
	for _, event := range heartbeats {
		beatLogs = append(beatLogs, eventJSON(event))
	}
	writeJSON(w, http.StatusOK, map[string]any{"activation_logs": activationLogs, "heartbeats": beatLogs})
}
