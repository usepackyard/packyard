package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

const (
	sessionCookieName = "packyard_session"
	userContextKey    = contextKey("user_id")
	sessionContextKey = contextKey("session_id")
)

// SessionAuth returns middleware that validates session cookies. The cookie
// value is "<sessionID>.<hmac-sha256(secret, sessionID)>"; the HMAC is
// verified constant-time before any DB lookup, so attackers can't probe for
// valid IDs by submitting random cookies.
func SessionAuth(sessions store.SessionStore, secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			id, ok := verifyCookie(cookie.Value, secret)
			if !ok {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			session, err := sessions.GetByID(r.Context(), id)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if session == nil || session.ExpiresAt.Before(time.Now()) {
				http.Error(w, "session expired", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, session.UserID)
			ctx = context.WithValue(ctx, sessionContextKey, session.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userContextKey).(int64)
	return id, ok
}

// SessionIDFromContext extracts the session ID from the request context.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionContextKey).(string)
	return id, ok
}

// CreateSession creates a new session for a user and sets the signed cookie.
func CreateSession(w http.ResponseWriter, sessions store.SessionStore, secret string, userID int64, maxAge int, secure bool) error {
	id, err := generateSessionID()
	if err != nil {
		return err
	}

	session := &model.Session{
		ID:        id,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Duration(maxAge) * time.Second),
	}

	if err := sessions.Create(context.Background(), session); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signCookie(session.ID, secret),
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})

	return nil
}

// ClearSession deletes the session and clears the cookie. The secure flag
// must mirror what was set in CreateSession so the browser correctly
// matches and replaces the original cookie.
func ClearSession(w http.ResponseWriter, r *http.Request, sessions store.SessionStore, secret string, secure bool) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if id, ok := verifyCookie(cookie.Value, secret); ok {
			_ = sessions.Delete(r.Context(), id)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// signCookie returns "<id>.<mac-hex>" suitable for placing in a Set-Cookie value.
func signCookie(id, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(id))
	return id + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyCookie splits a cookie value, recomputes the MAC, and compares it
// constant-time. Returns the session ID and true on success.
func verifyCookie(value, secret string) (string, bool) {
	dot := strings.IndexByte(value, '.')
	if dot <= 0 || dot == len(value)-1 {
		return "", false
	}
	id, gotMACHex := value[:dot], value[dot+1:]

	gotMAC, err := hex.DecodeString(gotMACHex)
	if err != nil {
		return "", false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(id))
	if !hmac.Equal(gotMAC, mac.Sum(nil)) {
		return "", false
	}
	return id, true
}
