package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL       string
	PortLicense       string
	PortOTA           string
	PortAdmin         string
	PublicBaseURL     string
	ArtifactsDir      string
	AdminPasswordHash string
	AdminTOTPSecret   string
	AdminActor        string
	DownloadSecret    string
	CookieSecure      bool
	LogLevel          string
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:       env("DATABASE_URL", "postgres://license:license@localhost:5432/license?sslmode=disable"),
		PortLicense:       env("PORT_LICENSE", "8080"),
		PortOTA:           env("PORT_OTA", "8081"),
		PortAdmin:         env("PORT_ADMIN", "8082"),
		PublicBaseURL:     strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		ArtifactsDir:      env("ARTIFACTS_DIR", "./artifacts"),
		AdminPasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
		AdminTOTPSecret:   os.Getenv("ADMIN_TOTP_SECRET"),
		AdminActor:        env("ADMIN_ACTOR", "admin"),
		DownloadSecret:    os.Getenv("DOWNLOAD_SECRET"),
		CookieSecure:      envBool("COOKIE_SECURE", true),
		LogLevel:          env("LOG_LEVEL", "info"),
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
	if c.PublicBaseURL == "" || !strings.HasPrefix(c.PublicBaseURL, "https://") {
		return c, fmt.Errorf("PUBLIC_BASE_URL must be an https URL")
	}
	return c, nil
}

func SigningKey(product, kind string) ([]byte, error) {
	key := fmt.Sprintf("%s_SIGNING_KEY_%s", strings.ToUpper(kind), strings.ToUpper(envKey(product)))
	raw := os.Getenv(key)
	if raw == "" {
		return nil, fmt.Errorf("%s is not set", key)
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64", key)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("%s must decode to 32 bytes", key)
	}
	return b, nil
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
