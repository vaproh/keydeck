package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vaproh/license-desk/internal/server"
)

func main() {
	licensePort := env("PORT_LICENSE", "8080")
	otaPort := env("PORT_OTA", "8081")
	adminPort := env("PORT_ADMIN", "8082")

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:" + licensePort + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	servers := []*http.Server{
		{Addr: ":" + licensePort, Handler: server.License(logger), ReadHeaderTimeout: 10 * time.Second},
		{Addr: ":" + otaPort, Handler: server.OTA(logger), ReadHeaderTimeout: 10 * time.Second},
		{Addr: ":" + adminPort, Handler: server.Admin(logger), ReadHeaderTimeout: 10 * time.Second},
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
