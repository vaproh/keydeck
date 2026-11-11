package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL           string
	PortLicense           string
	PortOTA               string
	PortAdmin             string
	PublicBaseURL         string
	ArtifactsDir          string
	SigningKeysDir        string
	AdminPasswordHash     string
	AdminTOTPSecret       string
	AdminActor            string
	BootstrapAdminToken   string
	DownloadSecret        string
	TurnstileSiteKey      string
	TurnstileSecretKey    string
	TurnstileExpectedHost string
	CookieSecure          bool
	LogLevel              string
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:           env("DATABASE_URL", "postgres://keydeck:keydeck@localhost:5432/keydeck?sslmode=disable"),
		PortLicense:           env("PORT_LICENSE", "8080"),
		PortOTA:               env("PORT_OTA", "8081"),
		PortAdmin:             env("PORT_ADMIN", "8082"),
		PublicBaseURL:         strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		ArtifactsDir:          env("ARTIFACTS_DIR", "./artifacts"),
		SigningKeysDir:        env("SIGNING_KEYS_DIR", "./data/keys"),
		AdminPasswordHash:     os.Getenv("ADMIN_PASSWORD_HASH"),
		AdminTOTPSecret:       os.Getenv("ADMIN_TOTP_SECRET"),
		AdminActor:            env("ADMIN_ACTOR", "admin"),
		BootstrapAdminToken:   os.Getenv("BOOTSTRAP_ADMIN_TOKEN"),
		DownloadSecret:        os.Getenv("DOWNLOAD_SECRET"),
		TurnstileSiteKey:      os.Getenv("TURNSTILE_SITE_KEY"),
		TurnstileSecretKey:    os.Getenv("TURNSTILE_SECRET_KEY"),
		TurnstileExpectedHost: os.Getenv("TURNSTILE_EXPECTED_HOSTNAME"),
		CookieSecure:          envBool("COOKIE_SECURE", true),
		LogLevel:              env("LOG_LEVEL", "info"),
	}
	if c.AdminPasswordHash == "" {
		return c, fmt.Errorf("ADMIN_PASSWORD_HASH is required")
	}
	if c.AdminTOTPSecret == "" {
		return c, fmt.Errorf("ADMIN_TOTP_SECRET is required")
	}
	if c.DownloadSecret == "" {
		return c, fmt.Errorf("DOWNLOAD_SECRET is required")
	}
	if c.TurnstileSiteKey == "" {
		return c, fmt.Errorf("TURNSTILE_SITE_KEY is required")
	}
	if c.TurnstileSecretKey == "" {
		return c, fmt.Errorf("TURNSTILE_SECRET_KEY is required")
	}
	if c.TurnstileExpectedHost == "" {
		return c, fmt.Errorf("TURNSTILE_EXPECTED_HOSTNAME is required")
	}
	if c.PublicBaseURL == "" || !strings.HasPrefix(c.PublicBaseURL, "https://") {
		return c, fmt.Errorf("PUBLIC_BASE_URL must be an https URL")
	}
	return c, nil
}

func SigningKey(product, kind string) ([]byte, error) {
	return signingSeed(product, kind, false)
}

// EnsureSigningKeys creates per-product seeds on disk when they are not in env.
// It returns the base64 public keys for registration in the admin API.
func EnsureSigningKeys(product string) (licensePublic, updatePublic string, err error) {
	licenseSeed, err := signingSeed(product, "LICENSE", true)
	if err != nil {
		return "", "", err
	}
	updateSeed, err := signingSeed(product, "UPDATE", true)
	if err != nil {
		return "", "", err
	}
	licenseKey := ed25519.NewKeyFromSeed(licenseSeed)
	updateKey := ed25519.NewKeyFromSeed(updateSeed)
	return base64.StdEncoding.EncodeToString(licenseKey.Public().(ed25519.PublicKey)),
		base64.StdEncoding.EncodeToString(updateKey.Public().(ed25519.PublicKey)), nil
}

func signingSeed(product, kind string, create bool) ([]byte, error) {
	envName := fmt.Sprintf("%s_SIGNING_KEY_%s", strings.ToUpper(kind), strings.ToUpper(envKey(product)))
	if raw := os.Getenv(envName); raw != "" {
		return decodeSeed(envName, raw)
	}
	if !safeSlug(product) {
		return nil, fmt.Errorf("invalid product slug")
	}
	dir := env("SIGNING_KEYS_DIR", "./data/keys")
	path := fmt.Sprintf("%s/%s.%s.seed", strings.TrimRight(dir, "/"), strings.ToLower(product), strings.ToLower(kind))
	if raw, err := os.ReadFile(path); err == nil {
		return decodeSeed(path, strings.TrimSpace(string(raw)))
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !create {
		return nil, fmt.Errorf("%s is not set and %s does not exist", envName, path)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create signing key directory: %w", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			return decodeSeed(path, strings.TrimSpace(string(raw)))
		}
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	encoded := base64.StdEncoding.EncodeToString(seed)
	if _, err := file.WriteString(encoded + "\n"); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return seed, nil
}

func decodeSeed(name, raw string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64", name)
	}
	if len(b) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s must decode to 32 bytes", name)
	}
	return b, nil
}

func safeSlug(value string) bool {
	if value == "" || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return value[0] >= 'a' && value[0] <= 'z' || value[0] >= '0' && value[0] <= '9'
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envKey(product string) string {
	product = strings.ReplaceAll(product, "-", "_")
	product = strings.ReplaceAll(product, ".", "_")
	return product
}
