package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vaproh/keydeck/web"
)

func License(logger *slog.Logger, api http.Handler) http.Handler {
	r := chi.NewRouter()
	base(r, logger)
	r.Mount("/v1", api)
	// Activation, heartbeat, and deactivation belong here. Never mount admin
	// or OTA routes on the license listener.
	return r
}

func OTA(logger *slog.Logger, api http.Handler) http.Handler {
	r := chi.NewRouter()
	base(r, logger)
	r.Mount("/v1", api)
	// Update-check, manifest, and artifact download belong here.
	return r
}

func Admin(logger *slog.Logger, api http.Handler) http.Handler {
	r := chi.NewRouter()
	base(r, logger)
	r.Mount("/api", api)

	assets, err := fs.Sub(web.Dist, "build")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/admin", http.FileServer(http.FS(assets)))

	r.Handle("/admin", spa(assets, files))
	r.Handle("/admin/*", spa(assets, files))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})
	return r
}

func base(r chi.Router, logger *slog.Logger) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(RequestLogger(logger))
	r.Use(Recoverer(logger))
	r.Get("/healthz", health)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func spa(assets fs.FS, files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/admin")
		if path == "" || path == "/" {
			serveIndex(w, r, assets)
			return
		}
		clean := strings.TrimPrefix(path, "/")
		if _, err := fs.Stat(assets, clean); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, assets)
	})
}

func serveIndex(w http.ResponseWriter, _ *http.Request, assets fs.FS) {
	body, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "frontend build missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}
