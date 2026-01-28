package auth

import "context"

// SetUserIDForTest injects a user ID into the context the way SessionAuth
// middleware would. Intended for tests that exercise downstream handlers
// without going through the real session-cookie flow.
func SetUserIDForTest(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

// SetSessionIDForTest injects a session ID into the context alongside the
// user ID. Used by tests that need auth.SessionIDFromContext to return a value.
func SetSessionIDForTest(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionContextKey, sessionID)
}

// SetOrgIDFromTokenForTest injects an org ID into the context the way the
// BasicAuth middleware would. Intended for Composer-protocol handler tests.
func SetOrgIDFromTokenForTest(ctx context.Context, orgID int64) context.Context {
	return context.WithValue(ctx, orgIDContextKey, orgID)
}
