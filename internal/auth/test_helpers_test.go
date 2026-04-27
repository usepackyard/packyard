package auth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
)

// Shared helpers used across the auth package's tests. Originally lived
// next to BasicAuth's tests; promoted to a dedicated file when BasicAuth
// (the single-mode Composer middleware) was removed but ComposerAuth's
// tests still needed the same primitives.

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// hashToken returns the hex-encoded SHA-256 of a plaintext token string,
// matching what ComposerAuth computes on every request.
func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// mustHashPassword hashes a password with bcrypt cost 4 (fast for tests).
func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return h
}
