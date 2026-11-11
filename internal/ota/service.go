package ota

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/mod/semver"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
)

var (
	ErrNoProduct = errors.New("unknown product")
	ErrDisabled  = errors.New("product disabled")
	ErrToken     = errors.New("invalid download token")
)

type Service struct {
	cfg config.Config
	q   *store.Queries
	now func() time.Time
}

func New(pool *pgxpool.Pool, cfg config.Config) *Service {
	return &Service{cfg: cfg, q: store.New(pool), now: func() time.Time { return time.Now().UTC() }}
}

type CheckRequest struct {
	Product  string
	Channel  string
	Platform string
	Current  string
}

type CheckResponse struct {
	Available   bool   `json:"available"`
	Version     string `json:"version,omitempty"`
	Mandatory   bool   `json:"mandatory,omitempty"`
	Security    bool   `json:"security,omitempty"`
	MinVersion  string `json:"min_version,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Sha256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Manifest    string `json:"manifest,omitempty"`
	Signature   string `json:"signature,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

func (s *Service) Check(ctx context.Context, req CheckRequest) (CheckResponse, error) {
	product, err := s.q.GetProductBySlug(ctx, req.Product)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckResponse{}, ErrNoProduct
		}
		return CheckResponse{}, err
	}
	if product.Status != "active" {
		return CheckResponse{}, ErrDisabled
	}
	if req.Channel == "" {
		req.Channel = "stable"
	}
	if req.Platform == "" {
		req.Platform = "windows-amd64"
	}
	release, err := s.q.FindUpdate(ctx, store.FindUpdateParams{
		ProductID: product.ID, Channel: req.Channel, Platform: req.Platform,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckResponse{Available: false}, nil
		}
		return CheckResponse{}, err
	}
	if req.Current != "" && compareVersions(release.Version, req.Current) <= 0 {
		return CheckResponse{Available: false}, nil
	}
	token := s.token(release.ID, s.now().Add(15*time.Minute))
	return CheckResponse{
		Available:   true,
		Version:     release.Version,
		Mandatory:   release.Mandatory,
		Security:    release.Security,
		MinVersion:  release.MinVersion,
		Notes:       release.Notes,
		Sha256:      release.Sha256,
		Size:        release.SizeBytes,
		Manifest:    release.Manifest,
		Signature:   release.ManifestSignature,
		DownloadURL: s.cfg.PublicBaseURL + "/v1/download/" + token,
	}, nil
}

func (s *Service) ResolveDownload(ctx context.Context, token string) (store.Release, error) {
	id, ok := s.verifyToken(token)
	if !ok {
		return store.Release{}, ErrToken
	}
	release, err := s.q.GetRelease(ctx, id)
	if err != nil || release.Status != "published" {
		return store.Release{}, ErrToken
	}
	return release, nil
}

func (s *Service) ArtifactPath(release store.Release) (string, error) {
	root, err := filepath.Abs(s.cfg.ArtifactsDir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(release.ArtifactPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", ErrToken
	}
	return path, nil
}

func (s *Service) token(id int64, expiry time.Time) string {
	payload := strconv.FormatInt(id, 10) + ":" + strconv.FormatInt(expiry.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.cfg.DownloadSecret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) verifyToken(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.DownloadSecret))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, false
	}
	fields := strings.Split(string(payload), ":")
	if len(fields) != 2 {
		return 0, false
	}
	id, err1 := strconv.ParseInt(fields[0], 10, 64)
	exp, err2 := strconv.ParseInt(fields[1], 10, 64)
	if err1 != nil || err2 != nil || s.now().Unix() > exp {
		return 0, false
	}
	return id, true
}

func compareVersions(a, b string) int {
	return semver.Compare(normalize(a), normalize(b))
}

func normalize(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func (s *Service) SignManifest(product string, manifest map[string]any) (string, string, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", "", err
	}
	key, err := config.SigningKey(product, "UPDATE")
	if err != nil {
		return "", "", err
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(key), raw)
	return string(raw), base64.StdEncoding.EncodeToString(sig), nil
}
