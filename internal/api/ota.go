package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/vaproh/keydeck/internal/ota"
)

func (a *App) OTARouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/update-check", a.updateCheck)
	r.Get("/download/{token}", a.download)
	return r
}

func (a *App) updateCheck(w http.ResponseWriter, r *http.Request) {
	resp, err := a.ota.Check(r.Context(), ota.CheckRequest{
		Product:  r.URL.Query().Get("product"),
		Channel:  r.URL.Query().Get("channel"),
		Platform: r.URL.Query().Get("platform"),
		Current:  r.URL.Query().Get("current"),
	})
	if err != nil {
		switch {
		case errors.Is(err, ota.ErrNoProduct):
			writeError(w, http.StatusNotFound, "unknown product")
		case errors.Is(err, ota.ErrDisabled):
			writeError(w, http.StatusForbidden, "product disabled")
		default:
			writeError(w, http.StatusInternalServerError, "update service unavailable")
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	release, err := a.ota.ResolveDownload(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}
	path, err := a.ota.ArtifactPath(release)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}
	w.Header().Set("content-type", "application/zip")
	w.Header().Set("content-disposition", `attachment; filename="`+filepath.Base(release.ArtifactName)+`"`)
	http.ServeContent(w, r, release.ArtifactName, info.ModTime(), file)
}
