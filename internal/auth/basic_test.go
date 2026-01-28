package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// hashToken returns the hex-encoded SHA-256 of a plaintext token string,
// matching what BasicAuth computes on every request.
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

func TestBasicAuth_MissingCredentials(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)

	req := httptest.NewRequest("GET", "/packages.json", nil)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("WWW-Authenticate header should be set on 401")
	}
}

func TestBasicAuth_WrongPassword(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "default", Name: "Default"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}

	plaintext := "token-for-wrong-pw"
	tok := &model.APIToken{
		OrgID: org.ID, Name: "x",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, "correct-pw"),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth(plaintext, "wrong-password")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBasicAuth_UnknownToken(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)

	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth("unknown-token", "any-password")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBasicAuth_ExpiredToken(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "default", Name: "Default"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}

	plaintext := "expired-token-plaintext"
	password := "my-password"
	past := time.Now().Add(-1 * time.Hour)
	tok := &model.APIToken{
		OrgID: org.ID, Name: "expired",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, password),
		IsActive:     true, ExpiresAt: &past,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for expired token", rec.Code)
	}
}

func TestBasicAuth_SuspendedOrgReturns402(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "acme", Name: "Acme"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := stores.Orgs.UpdateStatus(ctx, org.ID, "suspended"); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	plaintext := "suspended-org-token"
	password := "my-password"
	tok := &model.APIToken{
		OrgID: org.ID, Name: "x",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, password),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for suspended org", rec.Code)
	}
}

func TestBasicAuth_ArchivedOrgReturns404(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "acme", Name: "Acme"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := stores.Orgs.UpdateStatus(ctx, org.ID, "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	plaintext := "archived-org-token"
	password := "my-password"
	tok := &model.APIToken{
		OrgID: org.ID, Name: "x",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, password),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for archived org", rec.Code)
	}
}

func TestBasicAuth_ValidTokenSetsOrgInContext(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "acme", Name: "Acme"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}

	plaintext := "valid-token-plaintext"
	password := "my-password"
	tok := &model.APIToken{
		OrgID: org.ID, Name: "valid",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, password),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	var seenOrgID int64
	captured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.OrgIDFromToken(r.Context())
		if !ok {
			t.Error("OrgIDFromToken: not set in context after BasicAuth")
		}
		seenOrgID = id
		w.WriteHeader(http.StatusOK)
	})

	mw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mw(captured).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if seenOrgID != org.ID {
		t.Errorf("org ID in context = %d, want %d", seenOrgID, org.ID)
	}
}
