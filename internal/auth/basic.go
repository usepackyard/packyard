package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/usepackyard/packyard/internal/store"
)

type contextKey string

const (
	tokenContextKey contextKey = "api_token_id"
	orgIDContextKey contextKey = "token_org_id"
)

// BasicAuth returns middleware that validates API tokens via HTTP Basic auth.
// The username is the token string, the password is the per-token generated
// password (validated via bcrypt against the stored hash). After token
// resolution, the token's organization is loaded and its lifecycle status
// enforced: suspended → 402, archived → 404. Active orgs proceed.
func BasicAuth(tokens store.TokenStore, orgs store.OrgStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Composer Repository"`)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			hash := sha256.Sum256([]byte(username))
			tokenHash := hex.EncodeToString(hash[:])

			token, err := tokens.GetByHash(r.Context(), tokenHash)
			if err != nil {
				slog.Error("token lookup error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if token == nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			if !CheckPassword(token.PasswordHash, password) {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}

			if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
				http.Error(w, "token expired", http.StatusUnauthorized)
				return
			}

			org, err := orgs.GetByID(r.Context(), token.OrgID)
			if err != nil {
				slog.Error("org lookup error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if org == nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}
			if !enforceOrgStatus(w, org) {
				return
			}

			// Update last used in background.
			go func() {
				_ = tokens.UpdateLastUsed(context.Background(), token.ID)
			}()

			ctx := context.WithValue(r.Context(), tokenContextKey, token.ID)
			ctx = context.WithValue(ctx, orgIDContextKey, token.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OrgIDFromToken extracts the org ID set by BasicAuth from the context.
func OrgIDFromToken(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(orgIDContextKey).(int64)
	return id, ok
}
