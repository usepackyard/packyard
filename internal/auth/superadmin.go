package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/store"
)

const superAdminContextKey contextKey = "super_admin"

// RequireSuperAdmin returns middleware that requires the caller to be
// authenticated (UserID in context) AND for that user to have IsSuperAdmin=true
// and IsActive=true. Use after SessionAuth or BearerAdminAuth.
//
// BearerAdminAuth sets super-admin context directly, so RequireSuperAdmin
// accepts either path. For session-authenticated users we verify via
// user store lookup.
func RequireSuperAdmin(users store.UserStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fast path: BearerAdminAuth already promoted the context.
			if IsSuperAdminContext(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			userID, ok := UserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			user, err := users.GetByID(r.Context(), userID)
			if err != nil {
				slog.Error("super-admin check: user lookup error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if user == nil || !user.IsActive || !user.IsSuperAdmin {
				http.Error(w, "super-admin access required", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), superAdminContextKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsSuperAdminContext reports whether the context has been flagged as
// super-admin by RequireSuperAdmin or BearerAdminAuth.
func IsSuperAdminContext(ctx context.Context) bool {
	v, _ := ctx.Value(superAdminContextKey).(bool)
	return v
}
