package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vaproh/license-desk/internal/server"
)

func main() {
	publicPort := env("PORT_PUBLIC", "8080")
	adminPort := env("PORT_ADMIN", "8081")

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:" + publicPort + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	servers := []*http.Server{
		{Addr: ":" + publicPort, Handler: server.Public(), ReadHeaderTimeout: 10 * time.Second},
		{Addr: ":" + adminPort, Handler: server.Admin(), ReadHeaderTimeout: 10 * time.Second},
	}
	errc := make(chan error, len(servers))
	for _, srv := range servers {
		go func(s *http.Server) {
			logger.Info("listening", "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}(srv)
	}

	select {
	case <-ctx.Done():
	case err := <-errc:
		logger.Error("server failed", "err", err)
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdown)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
