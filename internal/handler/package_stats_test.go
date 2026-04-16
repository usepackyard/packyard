package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

// statsResponse mirrors the handler's shape but is parse-only to keep the
// test independent of struct-tag churn.
type statsResponse struct {
	TotalDownloads   int64 `json:"total_downloads"`
	DownloadsLast7d  int64 `json:"downloads_last_7d"`
	DownloadsLast30d int64 `json:"downloads_last_30d"`
	TopPackages      []struct {
		PackageID   string `json:"package_id"`
		PackageName string `json:"package_name"`
		Count       int64  `json:"count"`
	} `json:"top_packages"`
	RecentDownloads []struct {
		At          time.Time `json:"at"`
		PackageID   string    `json:"package_id"`
		PackageName string    `json:"package_name"`
		Version     string    `json:"version"`
	} `json:"recent_downloads"`
	DailySeries30d []struct {
		Day   string `json:"day"`
		Count int64  `json:"count"`
	} `json:"daily_series_30d"`
}

func TestPackageStatsHandler_EmptyOrg_ReturnsZeroes(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewPackageStatsHandler(stores.Downloads, 0)

	req := httptest.NewRequest("GET", "/api/packages/stats", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Stats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp statsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalDownloads != 0 {
		t.Errorf("total = %d, want 0", resp.TotalDownloads)
	}
	if resp.TopPackages == nil {
		t.Error("TopPackages should be empty slice, not null")
	}
	if resp.RecentDownloads == nil {
		t.Error("RecentDownloads should be empty slice, not null")
	}
	if len(resp.DailySeries30d) != 30 {
		t.Errorf("DailySeries30d len = %d, want 30 (zero-filled)", len(resp.DailySeries30d))
	}
}

func TestPackageStatsHandler_WithEvents_AggregatesCorrectly(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkgA := testutil.MakePackage(t, stores, org.ID, "vendor/a")
	pkgB := testutil.MakePackage(t, stores, org.ID, "vendor/b")
	vA := testutil.MakeVersion(t, stores, pkgA.ID, "1.0.0", "sha", 100)
	vB := testutil.MakeVersion(t, stores, pkgB.ID, "2.0.0", "sha", 100)

	now := time.Now().UTC()
	// Inside 7d window.
	for i := 0; i < 3; i++ {
		stores.Downloads.Record(ctx, &model.DownloadEvent{
			OrgID: org.ID, PackageID: pkgA.ID, VersionID: vA.ID, At: now,
		})
	}
	// Inside 30d but outside 7d window.
	for i := 0; i < 2; i++ {
		stores.Downloads.Record(ctx, &model.DownloadEvent{
			OrgID: org.ID, PackageID: pkgB.ID, VersionID: vB.ID, At: now.Add(-10 * 24 * time.Hour),
		})
	}
	// Outside 30d window.
	stores.Downloads.Record(ctx, &model.DownloadEvent{
		OrgID: org.ID, PackageID: pkgA.ID, VersionID: vA.ID, At: now.Add(-45 * 24 * time.Hour),
	})

	h := handler.NewPackageStatsHandler(stores.Downloads, 0)
	req := httptest.NewRequest("GET", "/api/packages/stats", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Stats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp statsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.TotalDownloads != 6 {
		t.Errorf("TotalDownloads = %d, want 6", resp.TotalDownloads)
	}
	if resp.DownloadsLast7d != 3 {
		t.Errorf("DownloadsLast7d = %d, want 3", resp.DownloadsLast7d)
	}
	if resp.DownloadsLast30d != 5 {
		t.Errorf("DownloadsLast30d = %d, want 5", resp.DownloadsLast30d)
	}

	// Top packages: A has 4 (3 recent + 1 old), B has 2.
	if len(resp.TopPackages) != 2 {
		t.Fatalf("TopPackages len = %d, want 2", len(resp.TopPackages))
	}
	if resp.TopPackages[0].PackageName != "vendor/a" || resp.TopPackages[0].Count != 4 {
		t.Errorf("top[0] = %+v", resp.TopPackages[0])
	}

	// Recent: newest first, with package/version joined in.
	if len(resp.RecentDownloads) == 0 {
		t.Fatal("RecentDownloads empty")
	}
	if resp.RecentDownloads[0].PackageName != "vendor/a" || resp.RecentDownloads[0].Version != "1.0.0" {
		t.Errorf("recent[0] = %+v", resp.RecentDownloads[0])
	}
}

func TestPackageStatsHandler_CrossTenantIsolation(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	orgA := testutil.MakeOrg(t, stores, "alpha", "Alpha")
	orgB := testutil.MakeOrg(t, stores, "beta", "Beta")
	pkgA := testutil.MakePackage(t, stores, orgA.ID, "alpha/pkg")
	pkgB := testutil.MakePackage(t, stores, orgB.ID, "beta/pkg")
	vA := testutil.MakeVersion(t, stores, pkgA.ID, "1.0.0", "sha", 100)
	vB := testutil.MakeVersion(t, stores, pkgB.ID, "1.0.0", "sha", 100)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		stores.Downloads.Record(ctx, &model.DownloadEvent{OrgID: orgA.ID, PackageID: pkgA.ID, VersionID: vA.ID, At: now})
	}
	stores.Downloads.Record(ctx, &model.DownloadEvent{OrgID: orgB.ID, PackageID: pkgB.ID, VersionID: vB.ID, At: now})

	h := handler.NewPackageStatsHandler(stores.Downloads, 0)

	// orgA sees 5.
	reqA := httptest.NewRequest("GET", "/api/packages/stats", nil)
	reqA = reqA.WithContext(auth.SetOrgInContext(reqA.Context(), orgA, nil))
	recA := httptest.NewRecorder()
	h.Stats(recA, reqA)
	var respA statsResponse
	json.NewDecoder(recA.Body).Decode(&respA)
	if respA.TotalDownloads != 5 {
		t.Errorf("orgA total = %d, want 5", respA.TotalDownloads)
	}
	for _, p := range respA.TopPackages {
		if p.PackageName != "alpha/pkg" {
			t.Errorf("orgA leaked cross-tenant: %+v", p)
		}
	}

	// orgB sees 1.
	reqB := httptest.NewRequest("GET", "/api/packages/stats", nil)
	reqB = reqB.WithContext(auth.SetOrgInContext(reqB.Context(), orgB, nil))
	recB := httptest.NewRecorder()
	h.Stats(recB, reqB)
	var respB statsResponse
	json.NewDecoder(recB.Body).Decode(&respB)
	if respB.TotalDownloads != 1 {
		t.Errorf("orgB total = %d, want 1", respB.TotalDownloads)
	}
}

func TestPackageStatsHandler_MissingOrg_Returns500(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewPackageStatsHandler(stores.Downloads, 0)

	req := httptest.NewRequest("GET", "/api/packages/stats", nil)
	rec := httptest.NewRecorder()
	h.Stats(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// With a nonzero cache TTL, a second request within the window must be
// served from cache. We verify by writing a fresh download event between
// the two calls and checking the second response still reports the old
// total (proving it skipped the DB).
func TestPackageStatsHandler_CachesWithinTTL(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	h := handler.NewPackageStatsHandler(stores.Downloads, 1*time.Hour)

	call := func() statsResponse {
		req := httptest.NewRequest("GET", "/api/packages/stats", nil)
		req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
		rec := httptest.NewRecorder()
		h.Stats(rec, req)
		var resp statsResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		return resp
	}

	// First call: empty, caches 0.
	if got := call(); got.TotalDownloads != 0 {
		t.Fatalf("first total = %d, want 0", got.TotalDownloads)
	}

	// Record a download out-of-band.
	stores.Downloads.Record(ctx, &model.DownloadEvent{
		OrgID: org.ID, PackageID: pkg.ID, VersionID: v.ID, At: time.Now().UTC(),
	})

	// Second call within TTL: must still see 0 (cache served).
	if got := call(); got.TotalDownloads != 0 {
		t.Errorf("second call within TTL: total = %d, want 0 (should be cached)", got.TotalDownloads)
	}
}

// With caching disabled (TTL=0), writes between calls are visible immediately.
// Pins that our cache control actually honors the 0 disable sentinel.
func TestPackageStatsHandler_NoCacheWhenTTLZero(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	h := handler.NewPackageStatsHandler(stores.Downloads, 0)

	call := func() statsResponse {
		req := httptest.NewRequest("GET", "/api/packages/stats", nil)
		req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
		rec := httptest.NewRecorder()
		h.Stats(rec, req)
		var resp statsResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		return resp
	}

	if got := call(); got.TotalDownloads != 0 {
		t.Fatalf("first = %d, want 0", got.TotalDownloads)
	}
	stores.Downloads.Record(ctx, &model.DownloadEvent{
		OrgID: org.ID, PackageID: pkg.ID, VersionID: v.ID, At: time.Now().UTC(),
	})
	if got := call(); got.TotalDownloads != 1 {
		t.Errorf("second = %d, want 1 (no cache)", got.TotalDownloads)
	}
}
