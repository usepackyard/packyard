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

// ComposerTenantAuth returns middleware for the slug-prefixed Composer
// routes used in multi mode (e.g. /{slug}/packages.json). It:
//
//   - reads the org slug from the URL ({slug} path value)
//   - validates the Basic-auth password (constant-time)
//   - resolves the API token by SHA-256 of username
//   - **rejects cross-tenant token misuse**: token.OrgID must match the
//     org loaded from the slug
//   - enforces org lifecycle status (suspended → 402, archived → 404)
//   - sets org + token in context so handlers behave like under BasicAuth
//
// Distinct from BasicAuth, which serves the legacy single-mode tenant-less
// URLs. Both routes can coexist; the server registers one or the other
// based on cfg.Mode.
func ComposerTenantAuth(tokens store.TokenStore, orgs store.OrgStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			if slug == "" {
				http.Error(w, "tenant slug required", http.StatusBadRequest)
				return
			}

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
				slog.Error("composer tenant: token lookup error", "error", err)
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

			org, err := orgs.GetBySlug(r.Context(), slug)
			if err != nil {
				slog.Error("composer tenant: org lookup error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if org == nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}

			// Critical invariant: token's org must match the URL slug. Otherwise
			// a customer with one tenant's token could probe other tenants.
			if token.OrgID != org.ID {
				http.Error(w, "token does not belong to this organization", http.StatusUnauthorized)
				return
			}

			if !enforceOrgStatus(w, org) {
				return
			}

			// Update last-used in background.
			go func() {
				_ = tokens.UpdateLastUsed(context.Background(), token.ID)
			}()

			ctx := context.WithValue(r.Context(), tokenContextKey, token.ID)
			ctx = context.WithValue(ctx, orgIDContextKey, token.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
