package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

type pkgSetup struct {
	stores  *store.Stores
	storage storage.Storage
	cache   *composer.Cache
	handler *handler.AdminPackageHandler
	org     *model.Organization
}

func newPkgSetup(t *testing.T) pkgSetup {
	t.Helper()
	stores := testutil.NewStores(t)
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewLocal: %v", err)
	}
	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")
	h := handler.NewAdminPackageHandler(stores.Packages, st, c)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	return pkgSetup{stores, st, c, h, org}
}

// withOrg returns a new request with the given org injected into context.
// In single-mode handlers, member is nil — RequirePermission is not called
// at the handler level so we don't need a member.
func (s pkgSetup) withOrg(req *http.Request) *http.Request {
	ctx := auth.SetOrgInContext(req.Context(), s.org, nil)
	return req.WithContext(ctx)
}

func TestAdminPackageHandler_Create_HappyPath(t *testing.T) {
	s := newPkgSetup(t)

	req := httptest.NewRequest("POST", "/api/packages", strings.NewReader(
		`{"name":"vendor/pkg","type":"library","description":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handler.Create(rec, s.withOrg(req))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"vendor/pkg"`) {
		t.Errorf("body should contain new package: %s", rec.Body.String())
	}
}

func TestAdminPackageHandler_Create_InvalidName(t *testing.T) {
	s := newPkgSetup(t)

	cases := []string{
		`{"name":"vendor/../etc"}`, // path escape
		`{"name":"NoSlash"}`,
		`{"name":"Caps/Pkg"}`,
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/packages", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.handler.Create(rec, s.withOrg(req))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminPackageHandler_Create_DuplicateConflict(t *testing.T) {
	s := newPkgSetup(t)
	testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	req := httptest.NewRequest("POST", "/api/packages",
		strings.NewReader(`{"name":"vendor/pkg"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handler.Create(rec, s.withOrg(req))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAdminPackageHandler_List_OrgScoped(t *testing.T) {
	s := newPkgSetup(t)
	otherOrg := testutil.MakeOrg(t, s.stores, "other", "Other")
	testutil.MakePackage(t, s.stores, s.org.ID, "vendor/mine")
	testutil.MakePackage(t, s.stores, otherOrg.ID, "vendor/theirs")

	req := httptest.NewRequest("GET", "/api/packages", nil)
	rec := httptest.NewRecorder()
	s.handler.List(rec, s.withOrg(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "vendor/mine") {
		t.Errorf("missing own package: %s", body)
	}
	if strings.Contains(body, "vendor/theirs") {
		t.Errorf("leak from other org: %s", body)
	}
}

func TestAdminPackageHandler_Get_HappyPath(t *testing.T) {
	s := newPkgSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")
	testutil.MakeVersion(t, s.stores, pkg.ID, "1.0.0", "abcd", 100)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}", s.handler.Get)

	req := httptest.NewRequest("GET", "/api/packages/"+pkg.PublicID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "vendor/pkg") {
		t.Errorf("body should include package: %s", body)
	}
	if !strings.Contains(body, `"version":"1.0.0"`) {
		t.Errorf("body should include version: %s", body)
	}
}

func TestAdminPackageHandler_Get_BadID(t *testing.T) {
	s := newPkgSetup(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}", s.handler.Get)

	// A malformed / wrong-prefix id is indistinguishable from "no such
	// package" from the client's perspective — both map to 404.
	req := httptest.NewRequest("GET", "/api/packages/not-a-pkg-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminPackageHandler_Get_CrossOrgIsolation(t *testing.T) {
	s := newPkgSetup(t)
	otherOrg := testutil.MakeOrg(t, s.stores, "other", "Other")
	otherPkg := testutil.MakePackage(t, s.stores, otherOrg.ID, "other/pkg")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}", s.handler.Get)

	// Try to read other org's package while in s.org context.
	req := httptest.NewRequest("GET", "/api/packages/"+otherPkg.PublicID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (cross-org leak)", rec.Code)
	}
}

func TestAdminPackageHandler_Get_NotFound(t *testing.T) {
	s := newPkgSetup(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}", s.handler.Get)

	req := httptest.NewRequest("GET", "/api/packages/pkg_01JHZ8K3Y5WQ9V2N6TRB4XE7CM", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminPackageHandler_Delete(t *testing.T) {
	s := newPkgSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/packages/{id}", s.handler.Delete)

	req := httptest.NewRequest("DELETE", "/api/packages/"+pkg.PublicID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	// Verify it's gone.
	got, err := s.stores.Packages.GetByID(req.Context(), s.org.ID, pkg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("package not deleted: %+v", got)
	}
}

// Local int->string to avoid pulling in strconv at the package level.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
