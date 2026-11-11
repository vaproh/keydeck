package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/store"
)

func (a *App) listReleases(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r, "products:read", 0) {
		return
	}
	productID, _ := parseInt(r.URL.Query().Get("product_id"))
	if agent, ok := auth.AgentFrom(r.Context()); ok && agent.ProductID != 0 {
		productID = agent.ProductID
	}
	rows, err := a.q.ListReleases(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load releases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": rows})
}

func (a *App) uploadRelease(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	productID, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "product is required")
		return
	}
	if !a.authorize(w, r, "releases:write", productID) {
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))
	if !semver.IsValid(normalizeVersion(version)) {
		writeError(w, http.StatusBadRequest, "version must be semver")
		return
	}
	channel := r.FormValue("channel")
	if channel != "stable" && channel != "beta" {
		writeError(w, http.StatusBadRequest, "channel must be stable or beta")
		return
	}
	platform := r.FormValue("platform")
	if platform == "" {
		platform = "windows-amd64"
	}
	product, err := a.q.GetProduct(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	file, header, err := r.FormFile("artifact")
	if err != nil {
		writeError(w, http.StatusBadRequest, "artifact is required")
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		writeError(w, http.StatusBadRequest, "invalid artifact name")
		return
	}
	dir := filepath.Join(a.cfg.ArtifactsDir, product.Slug, version)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store artifact")
		return
	}
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store artifact")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), file)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store artifact")
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store artifact")
		return
	}
	manifest := map[string]any{
		"product": product.Slug, "version": version, "channel": channel, "platform": platform,
		"sha256": hex.EncodeToString(hash.Sum(nil)), "size": size,
		"min_version": r.FormValue("min_version"), "mandatory": r.FormValue("mandatory") == "true",
		"security": r.FormValue("security") == "true", "published_at": time.Now().UTC().Format(time.RFC3339),
	}
	manifestBody, signature, err := a.ota.SignManifest(product.Slug, manifest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update signing key is not configured")
		return
	}
	rawManifest, _ := json.Marshal(manifest)
	row, err := a.q.CreateRelease(r.Context(), store.CreateReleaseParams{
		ProductID: productID, Version: version, Channel: channel, Platform: platform,
		MinVersion: r.FormValue("min_version"), Mandatory: manifest["mandatory"].(bool),
		Security: manifest["security"].(bool), RolloutPercent: 100, Notes: r.FormValue("notes"),
		ArtifactName: name, ArtifactPath: path, Sha256: manifest["sha256"].(string), SizeBytes: size,
		Manifest: manifestBody, ManifestSignature: signature, Status: "published",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save release")
		return
	}
	session := auth.SessionFrom(r.Context())
	a.audit(r.Context(), session.Actor, "release.publish", &productID, version, map[string]any{"sha256": row.Sha256, "manifest": string(rawManifest)})
	writeJSON(w, http.StatusCreated, row)
}

func normalizeVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

func releaseFilename(version, platform string) string {
	return fmt.Sprintf("%s-%s.zip", version, platform)
}
