package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/testutil"
)

func okHandlerSuper() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireSuperAdmin_NotAuthenticated(t *testing.T) {
	stores := testutil.NewStores(t)
	mw := auth.RequireSuperAdmin(stores.Users)

	req := httptest.NewRequest("GET", "/api/admin/orgs", nil)
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireSuperAdmin_RegularUserRejected(t *testing.T) {
	stores := testutil.NewStores(t)
	user := testutil.MakeUser(t, stores, "regular@example.com", "p")
	mw := auth.RequireSuperAdmin(stores.Users)

	req := httptest.NewRequest("GET", "/api/admin/orgs", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireSuperAdmin_SuperAdminAllowed(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")
	admin.IsSuperAdmin = true
	if err := stores.Users.Update(context.Background(), admin); err != nil {
		t.Fatalf("promote: %v", err)
	}

	mw := auth.RequireSuperAdmin(stores.Users)
	req := httptest.NewRequest("GET", "/api/admin/orgs", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), admin.ID))
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireSuperAdmin_InactiveSuperAdminRejected(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")
	admin.IsSuperAdmin = true
	admin.IsActive = false
	if err := stores.Users.Update(context.Background(), admin); err != nil {
		t.Fatalf("update: %v", err)
	}

	mw := auth.RequireSuperAdmin(stores.Users)
	req := httptest.NewRequest("GET", "/api/admin/orgs", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), admin.ID))
	rec := httptest.NewRecorder()
	mw(okHandlerSuper()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (inactive)", rec.Code)
	}
}
