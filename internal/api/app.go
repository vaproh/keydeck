package api

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/licensing"
	"github.com/vaproh/keydeck/internal/ota"
	"github.com/vaproh/keydeck/internal/store"
)

type App struct {
	cfg     config.Config
	pool    *pgxpool.Pool
	q       *store.Queries
	license *licensing.Service
	ota     *ota.Service
	auth    *auth.Service
}

func New(cfg config.Config, pool *pgxpool.Pool) *App {
	return &App{cfg: cfg, pool: pool, q: store.New(pool), license: licensing.New(pool, cfg), ota: ota.New(pool, cfg), auth: auth.New(cfg, store.New(pool))}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
