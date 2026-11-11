package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vaproh/license-desk/web"
)

func Public() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Get("/healthz", health)
	// License and OTA endpoints live here. Admin routes must never be mounted
	// on this listener because it is exposed on the public license domain.
	return r
}

func Admin() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Get("/healthz", health)

	assets, err := fs.Sub(web.Dist, "build")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))

	r.Handle("/admin", spa(assets, files))
	r.Handle("/admin/*", spa(assets, files))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})
	return r
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
