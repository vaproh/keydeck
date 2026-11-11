package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					logger.ErrorContext(r.Context(), "panic",
						slog.Any("panic", value),
						slog.String("request_id", middleware.GetReqID(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("stack", string(debug.Stack())),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger records metadata only. It never logs keys, tokens, bodies,
// cookies, or query strings.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			if id := middleware.GetReqID(r.Context()); id != "" {
				w.Header().Set("X-Request-Id", id)
			}
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)

			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if r.URL.Path == "/healthz" {
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", sw.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_ip", r.RemoteAddr),
			)
		})
	}
}
