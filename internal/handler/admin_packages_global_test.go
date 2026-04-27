package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

func newAdminPackagesHandler(t *testing.T) (*handler.AdminGlobalPackageHandler, *store.Stores) {
	t.Helper()
	stores := testutil.NewStores(t)
	st, _ := storage.NewLocal(t.TempDir())
	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test")
	h := handler.NewAdminGlobalPackageHandler(stores.Orgs, stores.Packages, st, c)
	return h, stores
}

func TestAdminGlobalPackageHandler_List(t *testing.T) {
	h, stores := newAdminPackagesHandler(t)

	a := testutil.MakeOrg(t, stores, "acme", "Acme")
	b := testutil.MakeOrg(t, stores, "beta", "Beta")
	testutil.MakePackage(t, stores, a.ID, "vendor/in-acme")
	testutil.MakePackage(t, stores, b.ID, "vendor/in-beta")

	rec := testutil.DoJSON(t, http.HandlerFunc(h.List), "GET", "/api/admin/packages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "vendor/in-acme") || !strings.Contains(body, "vendor/in-beta") {
		t.Errorf("response should list packages from both orgs: %s", body)
	}
}

func TestAdminGlobalPackageHandler_Delete(t *testing.T) {
	h, stores := newAdminPackagesHandler(t)
	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/packages/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/admin/packages/"+pkg.PublicID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminGlobalPackageHandler_Delete_NotFound(t *testing.T) {
	h, _ := newAdminPackagesHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/packages/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/admin/packages/pkg_01JHZ8K3Y5WQ9V2N6TRB4XE7CM", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
