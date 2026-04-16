package handler_test

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/testutil"
)

// makeZip builds an in-memory zip with the given entries.
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// newVerSetup extends a package setup with an AdminVersionHandler.
func newVerSetup(t *testing.T) (pkgSetup, *handler.AdminVersionHandler) {
	t.Helper()
	s := newPkgSetup(t)
	vh := handler.NewAdminVersionHandler(s.stores.Packages, s.storage, s.cache)
	return s, vh
}

func TestAdminVersionHandler_Upload_HappyPath(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
		"src/Foo.php":   "<?php",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "pkg.zip", Content: zipBytes})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"version":"1.0.0"`) {
		t.Errorf("body should contain new version: %s", rec.Body.String())
	}
}

// Regression test for the storage-key collision risk: storage paths must
// include the org_id so two tenants can publish the same package name
// without overwriting each other's files.
func TestAdminVersionHandler_Upload_StoragePathIncludesOrgID(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "pkg.zip", Content: zipBytes})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}

	versions, _ := s.stores.Packages.ListVersions(context.Background(), s.org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	wantPrefix := itoa(int(s.org.ID)) + "/vendor/pkg/"
	if !strings.HasPrefix(versions[0].StoragePath, wantPrefix) {
		t.Errorf("StoragePath = %q, want prefix %q (regression: storage keys must include org_id)",
			versions[0].StoragePath, wantPrefix)
	}
}

func TestAdminVersionHandler_Upload_NameMismatch(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/different","version":"1.0.0"}`,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "pkg.zip", Content: zipBytes})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "does not match") {
		t.Errorf("body should explain name mismatch: %s", rec.Body.String())
	}
}

func TestAdminVersionHandler_Upload_DuplicateVersion(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")
	testutil.MakeVersion(t, s.stores, pkg.ID, "1.0.0", "abcd", 1)

	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "pkg.zip", Content: zipBytes})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminVersionHandler_Upload_ZipBombRejected_TooManyEntries(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	// Build a zip whose entry count alone trips ParseZIP's defense.
	entries := map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
	}
	for i := 0; i < 10005; i++ {
		entries["dummy/"+itoa(i)+".txt"] = "x"
	}
	zipBytes := makeZip(t, entries)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "bomb.zip", Content: zipBytes})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zip bomb (entries) status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "too many entries") {
		t.Errorf("expected 'too many entries' rejection, got: %s", rec.Body.String())
	}
}

func TestAdminVersionHandler_Upload_MissingFile(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		map[string]string{"version": "1.0.0"}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminVersionHandler_Upload_PackageNotFound(t *testing.T) {
	s, vh := newVerSetup(t)

	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/pkg_01JHZ8K3Y5WQ9V2N6TRB4XE7CM/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "p.zip", Content: zipBytes})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminVersionHandler_Upload_BadID(t *testing.T) {
	s, vh := newVerSetup(t)
	zipBytes := makeZip(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0"}`,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	// A malformed / wrong-prefix id is indistinguishable from "no such
	// package" from the client's perspective — both map to 404.
	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/not-a-pkg-id/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "p.zip", Content: zipBytes})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminVersionHandler_Upload_NoComposerJSON(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")

	zipBytes := makeZip(t, map[string]string{
		"README.md": "no composer.json here",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/versions", vh.Upload)

	rec := testutil.DoMultipart(t, contextWrap(mux, s),
		"POST", "/api/packages/"+pkg.PublicID+"/versions",
		nil, &testutil.MultipartFile{FieldName: "file", Filename: "p.zip", Content: zipBytes})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "composer.json") {
		t.Errorf("error should mention composer.json: %s", rec.Body.String())
	}
}

func TestAdminVersionHandler_Delete(t *testing.T) {
	s, vh := newVerSetup(t)
	pkg := testutil.MakePackage(t, s.stores, s.org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, s.stores, pkg.ID, "1.0.0", "abcd", 1)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/versions/{id}", vh.Delete)

	req := httptest.NewRequest("DELETE", "/api/versions/"+v.PublicID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAdminVersionHandler_Delete_BadID(t *testing.T) {
	s, vh := newVerSetup(t)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/versions/{id}", vh.Delete)

	// A malformed / wrong-prefix id is indistinguishable from "no such
	// version" from the client's perspective — both map to 404.
	req := httptest.NewRequest("DELETE", "/api/versions/not-a-ver-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminVersionHandler_Delete_NotFound(t *testing.T) {
	s, vh := newVerSetup(t)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/versions/{id}", vh.Delete)

	req := httptest.NewRequest("DELETE", "/api/versions/ver_01JHZ8K3Y5WQ9V2N6TRB4XE7CM", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, s.withOrg(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// contextWrap returns an http.Handler that injects org context before
// delegating. Used by multipart tests where DoMultipart builds the request
// for us — we need the context applied at handler entry.
func contextWrap(inner http.Handler, s pkgSetup) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.SetOrgInContext(r.Context(), s.org, nil)
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

