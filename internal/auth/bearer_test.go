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

func TestBearerAdminAuth_NoHeader(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.BearerAdminAuth(stores.AdminTokens)

	req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBearerAdminAuth_MalformedHeader(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.BearerAdminAuth(stores.AdminTokens)

	cases := []string{
		"Basic abcd",
		"Bearer",
		"bearer ",
		"Bearer   ",
		"Token xyz",
	}
	for _, h := range cases {
		t.Run(h, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
			req.Header.Set("Authorization", h)
			rec := httptest.NewRecorder()
			mw(okHandlerSuper()).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("header %q: status = %d, want 401", h, rec.Code)
			}
		})
	}
}

func TestBearerAdminAuth_UnknownToken(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.BearerAdminAuth(stores.AdminTokens)

	req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer unknown-token-value")
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBearerAdminAuth_ValidToken(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")

	plaintext := "admin-valid-token-plaintext"
	hash := sha256.Sum256([]byte(plaintext))
	tok := &model.AdminToken{
		Name:        "test",
		TokenHash:   hex.EncodeToString(hash[:]),
		TokenPrefix: plaintext[:8],
		CreatedBy:   admin.ID,
		IsActive:    true,
	}
	if err := stores.AdminTokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BearerAdminAuth(stores.AdminTokens)
	req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestBearerAdminAuth_ExpiredToken(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")

	plaintext := "admin-expired-token"
	hash := sha256.Sum256([]byte(plaintext))
	past := time.Now().Add(-1 * time.Hour)
	tok := &model.AdminToken{
		Name:        "expired",
		TokenHash:   hex.EncodeToString(hash[:]),
		TokenPrefix: plaintext[:8],
		CreatedBy:   admin.ID,
		IsActive:    true,
		ExpiresAt:   &past,
	}
	if err := stores.AdminTokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BearerAdminAuth(stores.AdminTokens)
	req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBearerAdminAuth_InactiveToken(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")

	plaintext := "admin-inactive-token"
	hash := sha256.Sum256([]byte(plaintext))
	tok := &model.AdminToken{
		Name:        "inactive",
		TokenHash:   hex.EncodeToString(hash[:]),
		TokenPrefix: plaintext[:8],
		CreatedBy:   admin.ID,
		IsActive:    false,
	}
	if err := stores.AdminTokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.BearerAdminAuth(stores.AdminTokens)
	req := httptest.NewRequest("POST", "/api/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
