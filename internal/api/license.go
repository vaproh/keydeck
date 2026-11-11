package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vaproh/keydeck/internal/licensing"
)

func (a *App) LicenseRouter() http.Handler {
	r := chi.NewRouter()
	r.Post("/activate", a.activate)
	r.Post("/heartbeat", a.heartbeat)
	r.Post("/deactivate", a.deactivate)
	return r
}

func (a *App) activate(w http.ResponseWriter, r *http.Request) {
	var req licensing.ActivateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	resp, err := a.license.Activate(r.Context(), req, r.RemoteAddr)
	if err != nil {
		licenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) heartbeat(w http.ResponseWriter, r *http.Request) {
	var req licensing.ActivateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	resp, err := a.license.Heartbeat(r.Context(), req, r.RemoteAddr)
	if err != nil {
		licenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) deactivate(w http.ResponseWriter, r *http.Request) {
	var req licensing.ActivateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := a.license.Deactivate(r.Context(), req); err != nil {
		licenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deactivated": true})
}

func licenseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, licensing.ErrInvalid), errors.Is(err, licensing.ErrRevoked):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, licensing.ErrExpired), errors.Is(err, licensing.ErrProduct), errors.Is(err, licensing.ErrProductOff):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, licensing.ErrActivation), errors.Is(err, licensing.ErrUsed), errors.Is(err, licensing.ErrNoActivation):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, licensing.ErrMalformed):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "license service unavailable")
	}
}
