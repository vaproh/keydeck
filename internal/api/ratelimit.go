package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ipWindow struct {
	start time.Time
	count int
}

type ipLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*ipWindow
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	return &ipLimiter{limit: limit, window: window, entries: map[string]*ipWindow{}}
}

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[ip]
	if entry == nil || now.Sub(entry.start) >= l.window {
		l.entries[ip] = &ipWindow{start: now, count: 1}
		if len(l.entries) > 10000 {
			for key, value := range l.entries {
				if now.Sub(value.start) >= l.window {
					delete(l.entries, key)
				}
			}
		}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	return true
}

func (a *App) rateLimit(limiter *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(clientIP(r), time.Now()) {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP trusts X-Forwarded-For only when the TCP peer is private, which
// covers HAProxy on the same Docker network without trusting header spoofing
// from a public client hitting the published port directly.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsPrivate() {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			if parsed := net.ParseIP(forwarded); parsed != nil {
				return parsed.String()
			}
		}
	}
	return host
}
