package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vaproh/keydeck/internal/auth"
)

func (a *App) AuthRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/config", a.authConfig)
	r.Post("/login", a.login)
	r.Group(func(r chi.Router) {
		r.Use(a.auth.Middleware)
		r.Get("/me", a.me)
		r.Post("/logout", a.logout)
	})
	return r
}

func (a *App) authConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"site_key": a.cfg.TurnstileSiteKey})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password       string `json:"password"`
		TOTP           string `json:"totp"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	session, err := a.auth.Login(r.Context(), auth.ClientIP(r), body.Password, body.TOTP, body.TurnstileToken)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			writeError(w, http.StatusTooManyRequests, "too many attempts")
		case errors.Is(err, auth.ErrTurnstile):
			writeError(w, http.StatusUnauthorized, "human verification failed")
		default:
			writeError(w, http.StatusUnauthorized, "invalid credentials")
		}
		return
	}
	http.SetCookie(w, a.auth.Cookie(session.ID))
	writeJSON(w, http.StatusOK, map[string]string{"actor": session.Actor, "csrf": session.CSRF})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"actor": session.Actor, "csrf": session.CSRF})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.auth.CookieName()); err == nil {
		a.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, a.auth.ClearCookie())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
