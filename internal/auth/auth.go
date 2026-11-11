package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strconv"
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
const sessionTTL = 12 * time.Hour

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrRateLimited  = errors.New("too many attempts")
	ErrCaptcha      = errors.New("captcha failed")
)

type Service struct {
	cfg      config.Config
	q        *store.Queries
	mu       sync.Mutex
	captchas map[string]captcha
	fails    map[string][]time.Time
}

type captcha struct {
	answer  string
	expires time.Time
}

type Session struct {
	ID    string
	Actor string
	CSRF  string
}

func New(cfg config.Config, q *store.Queries) *Service {
	return &Service{cfg: cfg, q: q, captchas: map[string]captcha{}, fails: map[string][]time.Time{}}
}

func (s *Service) Captcha() (string, string, int) {
	a, _ := rand.Int(rand.Reader, big.NewInt(9))
	b, _ := rand.Int(rand.Reader, big.NewInt(9))
	id := randomToken(18)
	answer := strconv.FormatInt(a.Int64()+b.Int64(), 10)
	s.mu.Lock()
	s.captchas[id] = captcha{answer: answer, expires: time.Now().Add(5 * time.Minute)}
	s.mu.Unlock()
	return id, fmt.Sprintf("%d + %d", a.Int64(), b.Int64()), 300
}

func (s *Service) Login(ctx context.Context, ip, password, code, captchaID, captchaAnswer string) (Session, error) {
	if !s.allow(ip) {
		return Session{}, ErrRateLimited
	}
	if !s.checkCaptcha(captchaID, captchaAnswer) {
		s.fail(ip)
		return Session{}, ErrCaptcha
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

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func writeAuthError(w http.ResponseWriter, status int) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

func (s *Service) checkCaptcha(id, answer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.captchas[id]
	if !ok || !item.expires.After(time.Now()) {
		delete(s.captchas, id)
		return false
	}
	delete(s.captchas, id)
	return subtle.ConstantTimeCompare([]byte(item.answer), []byte(strings.TrimSpace(answer))) == 1
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
