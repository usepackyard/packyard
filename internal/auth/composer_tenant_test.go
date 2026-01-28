package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestComposerTenantAuth_HappyPath(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	plaintext := "tok-acme-happy"
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

	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)

	mux := http.NewServeMux()
	mux.Handle("GET /{slug}/packages.json", mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := auth.OrgIDFromToken(r.Context()); !ok || id != org.ID {
			t.Errorf("OrgID in context = %d, ok=%v, want %d", id, ok, org.ID)
		}
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/acme/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestComposerTenantAuth_CrossTenantTokenMisuse(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	acme := testutil.MakeOrg(t, stores, "acme", "Acme")
	_ = testutil.MakeOrg(t, stores, "beta", "Beta")

	plaintext := "tok-acme-cross"
	password := "my-password"
	tok := &model.APIToken{
		OrgID: acme.ID, Name: "x",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, password),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)

	mux := http.NewServeMux()
	mux.Handle("GET /{slug}/packages.json", mw(okHandler()))

	req := httptest.NewRequest("GET", "/beta/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (cross-tenant token use)", rec.Code)
	}
}

func TestComposerTenantAuth_UnknownSlug(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	plaintext := "tok-x"
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

	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)
	mux := http.NewServeMux()
	mux.Handle("GET /{slug}/packages.json", mw(okHandler()))

	req := httptest.NewRequest("GET", "/missing/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestComposerTenantAuth_SuspendedOrg(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	if err := stores.Orgs.UpdateStatus(ctx, org.ID, "suspended"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	plaintext := "tok-x"
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

	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)
	mux := http.NewServeMux()
	mux.Handle("GET /{slug}/packages.json", mw(okHandler()))

	req := httptest.NewRequest("GET", "/acme/packages.json", nil)
	req.SetBasicAuth(plaintext, password)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
}

func TestComposerTenantAuth_WrongPassword(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	plaintext := "tok-wrong-pw"
	tok := &model.APIToken{
		OrgID: org.ID, Name: "x",
		TokenHash:    hashToken(plaintext),
		PasswordHash: mustHashPassword(t, "correct"),
		IsActive:     true,
	}
	if err := stores.Tokens.Create(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)
	mux := http.NewServeMux()
	mux.Handle("GET /{slug}/packages.json", mw(okHandler()))

	req := httptest.NewRequest("GET", "/acme/packages.json", nil)
	req.SetBasicAuth(plaintext, "wrong")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestComposerTenantAuth_MissingSlug(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)

	req := httptest.NewRequest("GET", "/packages.json", nil)
	req.SetBasicAuth("x", "y")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
