// Package auth: password hashing, session tokens, and the request middleware.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// CookieName is the session cookie.
const CookieName = "planner_session"

// SessionTTL is how long a login lasts. Sliding expiry is deferred.
const SessionTTL = 30 * 24 * time.Hour

// bcrypt cost 10 ≈ 1s on a Pi 3B. Logins are rare with 30-day sessions,
// so don't lower it.
const bcryptCost = 10

// HashPassword returns the bcrypt hash for storage.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(h), err
}

// CheckPassword reports whether pw matches hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// NewToken returns 32 random bytes hex-encoded — the session key.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetCookie writes the session cookie. Behind `tailscale serve` the request
// arrives over plain HTTP (r.TLS == nil), so Secure can't be inferred from the
// request; the caller decides via dev. Safari refuses Secure cookies over
// http://localhost, which is why dev mode drops the flag.
func SetCookie(w http.ResponseWriter, token string, dev bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !dev,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

// ClearCookie expires the session cookie.
func ClearCookie(w http.ResponseWriter, dev bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !dev,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// User is what handlers get from the context.
type User struct {
	ID   int64
	Name string
}

type ctxKey struct{}

// WithUser attaches u to ctx.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the authenticated user, or ok=false.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Resolver looks a session token up. Implemented by the store.
type Resolver interface {
	UserBySession(ctx context.Context, token string, now time.Time) (User, bool, error)
}

// Middleware rejects requests without a valid session with a JSON 401 and
// otherwise passes the user through the context.
func Middleware(res Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			unauthorized(w)
			return
		}
		u, ok, err := res.UserBySession(r.Context(), c.Value, time.Now())
		if err != nil {
			http.Error(w, `{"error":"session lookup failed"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"로그인이 필요해요"}`))
}
