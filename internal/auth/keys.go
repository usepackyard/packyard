package auth

import "context"

// contextKey is the unexported type used for all auth-related context keys
// in this package. Pinning it to a private type prevents accidental
// collisions with keys from other packages.
type contextKey string

// Shared context keys used across the auth package's middleware chain.
// Per-file keys (orgContextKey, memberContextKey, sessionContextKey,
// userContextKey, etc.) live next to the middleware that owns them.
const (
	tokenContextKey contextKey = "composer_token_id"
	orgIDContextKey contextKey = "org_id"
)

// OrgIDFromToken returns the organization id stamped on the request by
// ComposerAuth. Composer handlers use this to scope responses to the
// caller's org without re-resolving the slug.
func OrgIDFromToken(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(orgIDContextKey).(int64)
	return v, ok
}
