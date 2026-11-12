package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/vaproh/keydeck/internal/api"
	"github.com/vaproh/keydeck/internal/auth"
	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/db"
	"github.com/vaproh/keydeck/internal/server"
)

var version = "dev"

func main() {
	licensePort := env("PORT_LICENSE", "8080")
	otaPort := env("PORT_OTA", "8081")
	adminPort := env("PORT_ADMIN", "8082")

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "passwd":
			runPasswd()
			return
		case "keygen":
			runKeygen()
			return
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:" + licensePort + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}
	cfg.Version = version
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("database migration failed", "err", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		logger.Info("migrations complete")
		return
	}
	app := api.New(cfg, pool)
	defer stop()

	servers := []*http.Server{
		{Addr: ":" + licensePort, Handler: server.License(logger, app.LicenseRouter()), ReadHeaderTimeout: 10 * time.Second},
		{Addr: ":" + otaPort, Handler: server.OTA(logger, app.OTARouter()), ReadHeaderTimeout: 10 * time.Second},
		{Addr: ":" + adminPort, Handler: server.Admin(logger, app.AdminRouter()), ReadHeaderTimeout: 10 * time.Second},
	}
	labels := []string{"license", "ota", "admin"}
	errc := make(chan error, len(servers))
	for i, srv := range servers {
		go func(label string, s *http.Server) {
			logger.Info("listening", "service", label, "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}(labels[i], srv)
	}

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errc:
		logger.Error("server failed", "err", err)
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i, srv := range servers {
		logger.Info("stopping", "service", labels[i])
		_ = srv.Shutdown(shutdown)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func runPasswd() {
	fmt.Fprint(os.Stderr, "Password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil || len(password) < 12 {
		fmt.Fprintln(os.Stderr, "password must be at least 12 characters")
		os.Exit(1)
	}
	hash, err := auth.HashPassword(string(password))
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not hash password")
		os.Exit(1)
	}
	fmt.Println(hash)
}

func runKeygen() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: desk keygen <product-slug>")
		os.Exit(1)
	}
	slug := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(os.Args[2]))
	licenseSeed := make([]byte, ed25519.SeedSize)
	updateSeed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(licenseSeed); err != nil {
		os.Exit(1)
	}
	if _, err := rand.Read(updateSeed); err != nil {
		os.Exit(1)
	}
	licenseKey := ed25519.NewKeyFromSeed(licenseSeed)
	updateKey := ed25519.NewKeyFromSeed(updateSeed)
	fmt.Printf("LICENSE_SIGNING_KEY_%s=%s\n", slug, base64.StdEncoding.EncodeToString(licenseSeed))
	fmt.Printf("UPDATE_SIGNING_KEY_%s=%s\n", slug, base64.StdEncoding.EncodeToString(updateSeed))
	fmt.Printf("# license public: %s\n", base64.StdEncoding.EncodeToString(licenseKey.Public().(ed25519.PublicKey)))
	fmt.Printf("# update public:  %s\n", base64.StdEncoding.EncodeToString(updateKey.Public().(ed25519.PublicKey)))
}
