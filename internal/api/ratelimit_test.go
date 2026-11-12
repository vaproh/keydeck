package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPLimiterWindow(t *testing.T) {
	limiter := newIPLimiter(2, time.Minute)
	now := time.Now()
	if !limiter.allow("1.2.3.4", now) || !limiter.allow("1.2.3.4", now) {
		t.Fatal("first two requests should pass")
	}
	if limiter.allow("1.2.3.4", now) {
		t.Fatal("third request should be limited")
	}
	if !limiter.allow("1.2.3.4", now.Add(time.Minute)) {
		t.Fatal("window should reset")
	}
}

func TestClientIPTrustsForwardedForOnlyFromPrivatePeer(t *testing.T) {
	private := httptest.NewRequest(http.MethodPost, "/", nil)
	private.RemoteAddr = "172.18.0.4:1234"
	private.Header.Set("X-Forwarded-For", "203.0.113.9, 172.18.0.4")
	if got := clientIP(private); got != "203.0.113.9" {
		t.Fatalf("private proxy XFF not used: %q", got)
	}
	public := httptest.NewRequest(http.MethodPost, "/", nil)
	public.RemoteAddr = "198.51.100.10:1234"
	public.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(public); got != "198.51.100.10" {
		t.Fatalf("public client XFF should be ignored: %q", got)
	}
}

func TestRateLimitReturns429(t *testing.T) {
	app := &App{activateLimiter: newIPLimiter(1, time.Minute)}
	handler := app.rateLimit(app.activateLimiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/activate", nil)
	req.RemoteAddr = "203.0.113.20:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d", rec.Code)
	}
}
