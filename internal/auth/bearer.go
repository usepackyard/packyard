package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/store"
)

const adminTokenContextKey contextKey = "admin_token_id"

// BearerAdminAuth returns middleware that authenticates admin tokens via
// the Authorization: Bearer <token> header. On success the context is
// flagged as super-admin so downstream handlers can use
// RequireSuperAdmin or check IsSuperAdminContext directly.
//
// Apply to /api/admin/* routes. Session-authenticated super-admins reach
// the same handlers via RequireSuperAdmin; admin tokens are for
// machine-to-machine callers (external automation, scripts, CI).
func BearerAdminAuth(tokens store.AdminTokenStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
				http.Error(w, "bearer token required", http.StatusUnauthorized)
				return
			}
			plaintext := strings.TrimSpace(raw[len(prefix):])
			if plaintext == "" {
				http.Error(w, "bearer token required", http.StatusUnauthorized)
				return
			}

			hash := sha256.Sum256([]byte(plaintext))
			tokenHash := hex.EncodeToString(hash[:])

			token, err := tokens.GetByHash(r.Context(), tokenHash)
			if err != nil {
				slog.Error("admin token lookup error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if token == nil || !token.IsActive {
				http.Error(w, "invalid admin token", http.StatusUnauthorized)
				return
			}
			if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
				http.Error(w, "admin token expired", http.StatusUnauthorized)
				return
			}

			// Update last-used in background.
			go func() {
				_ = tokens.UpdateLastUsed(context.Background(), token.ID)
			}()

			ctx := context.WithValue(r.Context(), adminTokenContextKey, token.ID)
			ctx = context.WithValue(ctx, superAdminContextKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminTokenIDFromContext returns the admin token ID set by BearerAdminAuth.
// Returns (0, false) for session-authenticated super-admins.
func AdminTokenIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(adminTokenContextKey).(int64)
	return id, ok
}
