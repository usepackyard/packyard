package handler_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

const baseURL = "http://repo.test"

// composerSetup builds a Composer handler backed by real stores, real
// local-storage in a temp dir, and an empty cache. Returns everything so
// tests can populate stores and rebuild the cache.
type composerSetup struct {
	stores  *store.Stores
	storage storage.Storage
	cache   *composer.Cache
	handler *handler.ComposerHandler
}

func newComposerSetup(t *testing.T) composerSetup {
	t.Helper()
	s := testutil.NewStores(t)
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewLocal: %v", err)
	}
	c := composer.NewCache(s.Packages, s.Orgs, baseURL)
	return composerSetup{
		stores:  s,
		storage: st,
		cache:   c,
		handler: handler.NewComposerHandler(c, st, s.Packages, s.Downloads),
	}
}

func TestComposerHandler_PackagesJSON_RequiresOrg(t *testing.T) {
	cs := newComposerSetup(t)

	req := httptest.NewRequest("GET", "/packages.json", nil)
	rec := httptest.NewRecorder()
	cs.handler.PackagesJSON(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestComposerHandler_PackagesJSON_ServesFromCache(t *testing.T) {
	cs := newComposerSetup(t)

	org := testutil.MakeOrg(t, cs.stores, "default", "Default")
	pkg := testutil.MakePackage(t, cs.stores, org.ID, "vendor/pkg")
	testutil.MakeVersion(t, cs.stores, pkg.ID, "1.0.0", "deadbeef", 100)

	if err := cs.cache.Rebuild(context.Background(), org.ID); err != nil {
		t.Fatalf("rebuild cache: %v", err)
	}

	req := httptest.NewRequest("GET", "/packages.json", nil)
	req = req.WithContext(auth.SetOrgIDFromTokenForTest(req.Context(), org.ID))
	rec := httptest.NewRecorder()
	cs.handler.PackagesJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "vendor/pkg") {
		t.Errorf("body should list vendor/pkg: %s", rec.Body.String())
	}
}

func TestComposerHandler_ProviderJSON_StripsJSONSuffix(t *testing.T) {
	stores := testutil.NewStores(t)
	st, _ := storage.NewLocal(t.TempDir())
	c := composer.NewCache(stores.Packages, stores.Orgs, baseURL)
	h := handler.NewComposerHandler(c, st, stores.Packages, stores.Downloads)

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "deadbeef", 100)

	if err := c.Rebuild(context.Background(), org.ID); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// Composer requests "/p2/vendor/pkg.json" — the trailing .json must be
	// stripped before cache lookup.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p2/{vendor}/{package}", h.ProviderJSON)

	req := httptest.NewRequest("GET", "/p2/vendor/pkg.json", nil)
	req = req.WithContext(auth.SetOrgIDFromTokenForTest(req.Context(), org.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (the .json suffix should be stripped); body=%s", rec.Code, rec.Body.String())
	}
}

func TestComposerHandler_ProviderJSON_NotFound(t *testing.T) {
	stores := testutil.NewStores(t)
	st, _ := storage.NewLocal(t.TempDir())
	c := composer.NewCache(stores.Packages, stores.Orgs, baseURL)
	h := handler.NewComposerHandler(c, st, stores.Packages, stores.Downloads)

	org := testutil.MakeOrg(t, stores, "default", "Default")
	if err := c.Rebuild(context.Background(), org.ID); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /p2/{vendor}/{package}", h.ProviderJSON)

	req := httptest.NewRequest("GET", "/p2/unknown/pkg.json", nil)
	req = req.WithContext(auth.SetOrgIDFromTokenForTest(req.Context(), org.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestComposerHandler_Dist_ServesFile(t *testing.T) {
	stores := testutil.NewStores(t)
	dir := t.TempDir()
	st, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	c := composer.NewCache(stores.Packages, stores.Orgs, baseURL)
	h := handler.NewComposerHandler(c, st, stores.Packages, stores.Downloads)

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Put a real "zip" in storage at the path the version row points at.
	body := []byte("PK\x03\x04 fake zip body")
	storagePath := "vendor/pkg/1.0.0.zip"
	if err := st.Put(context.Background(), storagePath, strings.NewReader(string(body)), int64(len(body))); err != nil {
		t.Fatalf("storage Put: %v", err)
	}
	hash := sha1.Sum(body)
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", hex.EncodeToString(hash[:]), int64(len(body)))
	v.StoragePath = storagePath
	// Push the corrected storage path back to the DB so Dist finds the file.
	// (testutil.MakeVersion uses a default path; we override.)
	if err := stores.Packages.DeleteVersion(context.Background(), v.ID); err != nil {
		t.Fatalf("delete to re-create: %v", err)
	}
	v.ID = 0
	if err := stores.Packages.CreateVersion(context.Background(), v); err != nil {
		t.Fatalf("re-create version: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /dist/{vendor}/{package}/{version}", h.Dist)

	req := httptest.NewRequest("GET", "/dist/vendor/pkg/1.0.0", nil)
	req = req.WithContext(auth.SetOrgIDFromTokenForTest(req.Context(), org.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q", got)
	}
	if got, _ := io.ReadAll(rec.Body); string(got) != string(body) {
		t.Errorf("body bytes mismatch")
	}

	// The dist handler fires a goroutine to bump the per-version counter
	// and append a download_events row. Neither write blocks the response,
	// so we poll briefly for them.
	testutil.Eventually(t, 500*time.Millisecond, "download_count should reach 1", func() bool {
		got, _ := stores.Packages.GetVersionByID(context.Background(), v.ID)
		return got != nil && got.DownloadCount == 1
	})
	testutil.Eventually(t, 500*time.Millisecond, "download_events row should be persisted", func() bool {
		n, _ := stores.Downloads.TotalSince(context.Background(), org.ID, time.Time{})
		return n == 1
	})
}

func TestComposerHandler_Dist_VersionNotFound(t *testing.T) {
	stores := testutil.NewStores(t)
	st, _ := storage.NewLocal(t.TempDir())
	c := composer.NewCache(stores.Packages, stores.Orgs, baseURL)
	h := handler.NewComposerHandler(c, st, stores.Packages, stores.Downloads)

	org := testutil.MakeOrg(t, stores, "default", "Default")
	testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /dist/{vendor}/{package}/{version}", h.Dist)

	req := httptest.NewRequest("GET", "/dist/vendor/pkg/9.9.9", nil)
	req = req.WithContext(auth.SetOrgIDFromTokenForTest(req.Context(), org.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
