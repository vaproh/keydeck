package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/argon2"

	"github.com/vaproh/keydeck/internal/config"
	"github.com/vaproh/keydeck/internal/store"
)

const cookieName = "desk_session"
const sessionTTL = 7 * 24 * time.Hour

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrRateLimited  = errors.New("too many attempts")
)

type Service struct {
	cfg       config.Config
	q         *store.Queries
	mu        sync.Mutex
	fails     map[string][]time.Time
	turnstile TurnstileVerifier
}

type Session struct {
	ID    string
	Actor string
	CSRF  string
}

// Agent is a scoped machine identity. ProductID 0 means all products.
type Agent struct {
	TokenID   int64
	Name      string
	Scopes    map[string]bool
	ProductID int64
	Bootstrap bool
}

func (a Agent) Has(scope string) bool { return a.Scopes[scope] }

func New(cfg config.Config, q *store.Queries) *Service {
	return NewWithTurnstile(cfg, q, NewTurnstile(cfg.TurnstileSecretKey, cfg.TurnstileExpectedHost))
}

func NewWithTurnstile(cfg config.Config, q *store.Queries, verifier TurnstileVerifier) *Service {
	return &Service{cfg: cfg, q: q, fails: map[string][]time.Time{}, turnstile: verifier}
}

func (s *Service) Login(ctx context.Context, ip, password, code, turnstileToken string) (Session, error) {
	if !s.allow(ip) {
		return Session{}, ErrRateLimited
	}
	if s.turnstile == nil || s.turnstile.Verify(ctx, turnstileToken, ip) != nil {
		s.fail(ip)
		return Session{}, ErrTurnstile
	}
	if !verifyArgon2(s.cfg.AdminPasswordHash, password) {
		s.fail(ip)
		return Session{}, ErrUnauthorized
	}
	if !totp.Validate(code, s.cfg.AdminTOTPSecret) {
		s.fail(ip)
		return Session{}, ErrUnauthorized
	}
	s.clear(ip)

	raw := randomToken(32)
	id := hash(raw)
	csrf := randomToken(32)
	if err := s.q.CreateSession(ctx, store.CreateSessionParams{
		ID: id, Actor: s.cfg.AdminActor, CsrfToken: csrf, ExpiresAt: pgTime(time.Now().Add(sessionTTL)),
	}); err != nil {
		return Session{}, err
	}
	return Session{ID: raw, Actor: s.cfg.AdminActor, CSRF: csrf}, nil
}

func (s *Service) Find(ctx context.Context, raw string) (Session, error) {
	if raw == "" {
		return Session{}, ErrUnauthorized
	}
	row, err := s.q.GetSession(ctx, hash(raw))
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	if !row.ExpiresAt.Time.After(time.Now()) {
		return Session{}, ErrUnauthorized
	}
	return Session{ID: raw, Actor: row.Actor, CSRF: row.CsrfToken}, nil
}

func (s *Service) Logout(ctx context.Context, raw string) {
	if raw != "" {
		_ = s.q.DeleteSession(ctx, hash(raw))
	}
}

func (s *Service) Cookie(raw string) *http.Cookie {
	return &http.Cookie{
		Name: cookieName, Value: raw, Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	}
}

func (s *Service) ClearCookie() *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, MaxAge: -1}
}

func (s *Service) CookieName() string { return cookieName }

var AllScopes = []string{
	"products:read", "products:write", "licenses:read", "licenses:write", "releases:write", "audit:read",
}

func allScopes() map[string]bool {
	scopes := make(map[string]bool, len(AllScopes))
	for _, scope := range AllScopes {
		scopes[scope] = true
	}
	return scopes
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := bearer(r); raw != "" && s.cfg.BootstrapAdminToken != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(s.cfg.BootstrapAdminToken)) == 1 {
			agent := Agent{Name: "bootstrap", Scopes: allScopes(), Bootstrap: true}
			ctx := context.WithValue(r.Context(), agentKey{}, agent)
			ctx = context.WithValue(ctx, sessionKey{}, Session{Actor: "agent:bootstrap"})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if raw := bearer(r); strings.HasPrefix(raw, "kd_admin_") {
			row, err := s.q.GetAdminTokenByHash(r.Context(), hash(raw))
			if err != nil || row.RevokedAt.Valid || (row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(time.Now())) {
				writeAuthError(w, http.StatusUnauthorized)
				return
			}
			var scopes []string
			_ = json.Unmarshal(row.ScopesJson, &scopes)
			agent := Agent{TokenID: row.ID, Name: row.Name, Scopes: map[string]bool{}, ProductID: row.ProductID}
			for _, scope := range scopes {
				agent.Scopes[scope] = true
			}
			_ = s.q.TouchAdminToken(r.Context(), row.ID)
			ctx := context.WithValue(r.Context(), agentKey{}, agent)
			session := Session{Actor: "agent:" + row.Name}
			ctx = context.WithValue(ctx, sessionKey{}, session)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized)
			return
		}
		session, err := s.Find(r.Context(), cookie.Value)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				writeAuthError(w, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, session)))
	})
}

func SessionFrom(ctx context.Context) Session {
	session, _ := ctx.Value(sessionKey{}).(Session)
	return session
}

type sessionKey struct{}
type agentKey struct{}

func AgentFrom(ctx context.Context) (Agent, bool) {
	agent, ok := ctx.Value(agentKey{}).(Agent)
	return agent, ok
}

func bearer(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) > 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func writeAuthError(w http.ResponseWriter, status int) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

func (s *Service) allow(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	recent := s.fails[ip][:0]
	for _, t := range s.fails[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	s.fails[ip] = recent
	return len(recent) < 5
}

func (s *Service) fail(ip string) {
	s.mu.Lock()
	s.fails[ip] = append(s.fails[ip], time.Now())
	s.mu.Unlock()
}

func (s *Service) clear(ip string) {
	s.mu.Lock()
	delete(s.fails, ip)
	s.mu.Unlock()
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func verifyArgon2(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory = 64 * 1024
	const iterations = 3
	const parallelism = 2
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// NewAdminToken returns the one-time display value and the values stored server-side.
func NewAdminToken() (raw, hashed, hint string) {
	raw = "kd_admin_" + randomToken(32)
	hinted := raw[len(raw)-4:]
	return raw, hash(raw), hinted
}

// HashToken exposes the same SHA-256 representation used by bearer auth.
func HashToken(raw string) string { return hash(raw) }
