package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const turnstileEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var ErrTurnstile = errors.New("turnstile verification failed")

// TurnstileVerifier lets tests exercise login without calling Cloudflare.
type TurnstileVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

type Turnstile struct {
	secret   string
	hostname string
	endpoint string
	client   *http.Client
	mu       sync.Mutex
	used     map[string]time.Time
}

func NewTurnstile(secret, expectedHostname string) *Turnstile {
	return &Turnstile{
		secret: secret, hostname: expectedHostname, endpoint: turnstileEndpoint,
		client: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		used: map[string]time.Time{},
	}
}

func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 2048 {
		return ErrTurnstile
	}
	if t.consumeReplay(token) {
		return ErrTurnstile
	}
	values := url.Values{"secret": {t.secret}, "response": {token}}
	if remoteIP != "" {
		values.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return ErrTurnstile
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return ErrTurnstile
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ErrTurnstile
	}
	var result struct {
		Success    bool     `json:"success"`
		Hostname   string   `json:"hostname"`
		Action     string   `json:"action"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&result); err != nil {
		return ErrTurnstile
	}
	if !result.Success || len(result.ErrorCodes) > 0 {
		return ErrTurnstile
	}
	if !strings.EqualFold(result.Hostname, t.hostname) || result.Action != "admin_login" {
		return ErrTurnstile
	}
	t.markUsed(token)
	return nil
}

func (t *Turnstile) consumeReplay(token string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for value, expires := range t.used {
		if !expires.After(now) {
			delete(t.used, value)
		}
	}
	expires, exists := t.used[token]
	return exists && expires.After(now)
}

func (t *Turnstile) markUsed(token string) {
	t.mu.Lock()
	t.used[token] = time.Now().Add(5 * time.Minute)
	t.mu.Unlock()
}
