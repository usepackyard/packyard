package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestAdminOrgHandler_Create(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/orgs",
		map[string]string{"slug": "acme", "name": "Acme"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"slug":"acme"`) {
		t.Errorf("body should include slug: %s", rec.Body.String())
	}
	// Default status is active.
	if !strings.Contains(rec.Body.String(), `"status":"active"`) {
		t.Errorf("body should include status=active: %s", rec.Body.String())
	}
}

func TestAdminOrgHandler_Create_DuplicateSlug(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/orgs",
		map[string]string{"slug": "acme", "name": "Acme 2"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAdminOrgHandler_Create_RejectsReservedSlug(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	for _, slug := range []string{"www", "api", "admin", "default"} {
		t.Run(slug, func(t *testing.T) {
			rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/orgs",
				map[string]string{"slug": slug, "name": "X"})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminOrgHandler_Create_RejectsBadSlugFormat(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	for _, slug := range []string{"Acme", "1team", "ab", "my_team", "../etc"} {
		t.Run(slug, func(t *testing.T) {
			rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/orgs",
				map[string]string{"slug": slug, "name": "X"})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminOrgHandler_Create_RequiresFields(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/orgs",
		map[string]string{"slug": "", "name": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminOrgHandler_List(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	testutil.MakeOrg(t, stores, "beta", "Beta")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	rec := testutil.DoJSON(t, http.HandlerFunc(h.List), "GET", "/api/admin/orgs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "acme") || !strings.Contains(body, "beta") {
		t.Errorf("response should list both orgs: %s", body)
	}
}

func TestAdminOrgHandler_Get(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/orgs/{slug}", h.Get)

	req := httptest.NewRequest("GET", "/api/admin/orgs/acme", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Acme") {
		t.Errorf("response should contain Acme: %s", rec.Body.String())
	}
}

func TestAdminOrgHandler_Get_NotFound(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/orgs/{slug}", h.Get)

	req := httptest.NewRequest("GET", "/api/admin/orgs/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminOrgHandler_UpdateStatus(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/admin/orgs/{slug}/status", h.UpdateStatus)

	for _, status := range []string{"suspended", "active", "archived"} {
		t.Run(status, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/admin/orgs/acme/status",
				strings.NewReader(`{"status":"`+status+`"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
			}

			got, _ := stores.Orgs.GetBySlug(context.Background(), "acme")
			if got.Status != status {
				t.Errorf("DB status = %q, want %q", got.Status, status)
			}
		})
	}
}

func TestAdminOrgHandler_UpdateStatus_RejectsInvalid(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/admin/orgs/{slug}/status", h.UpdateStatus)

	req := httptest.NewRequest("PUT", "/api/admin/orgs/acme/status",
		strings.NewReader(`{"status":"unknown"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminOrgHandler_Delete_RefusesIfPackagesExist(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/orgs/{slug}", h.Delete)

	// Without ?force=true → 409 because packages exist.
	req := httptest.NewRequest("DELETE", "/api/admin/orgs/acme", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAdminOrgHandler_Delete_Force(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/orgs/{slug}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/admin/orgs/acme?force=true", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := stores.Orgs.GetBySlug(context.Background(), "acme")
	if got != nil {
		t.Error("org still present after force delete")
	}
}

func TestAdminOrgHandler_Delete_EmptyOrgWithoutForce(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeOrg(t, stores, "acme", "Acme")
	h := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/orgs/{slug}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/admin/orgs/acme", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (empty org should delete); body=%s", rec.Code, rec.Body.String())
	}
}
