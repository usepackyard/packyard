package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

// seedDownload inserts one event at an exact timestamp so tests can pin
// time windows precisely. Wraps the store's Record for brevity.
func seedDownload(t *testing.T, stores *store.Stores, orgID, pkgID, versionID int64, at time.Time) {
	t.Helper()
	ev := &model.DownloadEvent{
		OrgID: orgID, PackageID: pkgID, VersionID: versionID, At: at,
	}
	if err := stores.Downloads.Record(context.Background(), ev); err != nil {
		t.Fatalf("Record event: %v", err)
	}
}

func TestDownloadStore_Record_PersistsRow(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	ev := &model.DownloadEvent{
		OrgID: org.ID, PackageID: pkg.ID, VersionID: v.ID,
	}
	if err := stores.Downloads.Record(ctx, ev); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if ev.ID == 0 {
		t.Error("ID not set after Record")
	}
	if ev.At.IsZero() {
		t.Error("At should be set when not provided")
	}

	total, err := stores.Downloads.TotalSince(ctx, org.ID, time.Time{})
	if err != nil {
		t.Fatalf("TotalSince: %v", err)
	}
	if total != 1 {
		t.Errorf("TotalSince = %d, want 1", total)
	}
}

func TestDownloadStore_TotalSince_ExcludesBeforeCutoff(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	now := time.Now().UTC()
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now.Add(-8*24*time.Hour))
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now.Add(-2*24*time.Hour))
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now)

	// All-time.
	total, _ := stores.Downloads.TotalSince(ctx, org.ID, time.Time{})
	if total != 3 {
		t.Errorf("all-time total = %d, want 3", total)
	}
	// Last 7 days excludes the t-8d event.
	week, _ := stores.Downloads.TotalSince(ctx, org.ID, now.Add(-7*24*time.Hour))
	if week != 2 {
		t.Errorf("7d total = %d, want 2", week)
	}
}

func TestDownloadStore_TopPackages_OrdersDesc_RespectsLimit(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkgA := testutil.MakePackage(t, stores, org.ID, "vendor/a")
	pkgB := testutil.MakePackage(t, stores, org.ID, "vendor/b")
	pkgC := testutil.MakePackage(t, stores, org.ID, "vendor/c")
	vA := testutil.MakeVersion(t, stores, pkgA.ID, "1.0.0", "sha", 100)
	vB := testutil.MakeVersion(t, stores, pkgB.ID, "1.0.0", "sha", 100)
	vC := testutil.MakeVersion(t, stores, pkgC.ID, "1.0.0", "sha", 100)

	now := time.Now().UTC()
	// A: 5 downloads, B: 2, C: 3.
	for i := 0; i < 5; i++ {
		seedDownload(t, stores, org.ID, pkgA.ID, vA.ID, now)
	}
	for i := 0; i < 2; i++ {
		seedDownload(t, stores, org.ID, pkgB.ID, vB.ID, now)
	}
	for i := 0; i < 3; i++ {
		seedDownload(t, stores, org.ID, pkgC.ID, vC.ID, now)
	}

	top, err := stores.Downloads.TopPackages(ctx, org.ID, time.Time{}, 2)
	if err != nil {
		t.Fatalf("TopPackages: %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("len = %d, want 2 (limit)", len(top))
	}
	if top[0].PackageName != "vendor/a" || top[0].Count != 5 {
		t.Errorf("top[0] = %+v, want {vendor/a, 5}", top[0])
	}
	if top[1].PackageName != "vendor/c" || top[1].Count != 3 {
		t.Errorf("top[1] = %+v, want {vendor/c, 3}", top[1])
	}
}

func TestDownloadStore_Recent_OrdersByAtDesc_JoinsPackageAndVersion(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v1 := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)
	v2 := testutil.MakeVersion(t, stores, pkg.ID, "2.0.0", "sha", 100)

	now := time.Now().UTC()
	seedDownload(t, stores, org.ID, pkg.ID, v1.ID, now.Add(-2*time.Minute))
	seedDownload(t, stores, org.ID, pkg.ID, v2.ID, now.Add(-1*time.Minute))
	seedDownload(t, stores, org.ID, pkg.ID, v1.ID, now)

	recent, err := stores.Downloads.Recent(ctx, org.ID, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("len = %d, want 3", len(recent))
	}
	// Newest first: v1 → v2 → v1.
	if recent[0].Version != "1.0.0" || recent[1].Version != "2.0.0" || recent[2].Version != "1.0.0" {
		t.Errorf("ordering wrong: %+v", recent)
	}
	if recent[0].PackageName != "vendor/pkg" {
		t.Errorf("join missing package name: %+v", recent[0])
	}
}

func TestDownloadStore_DailySeries_IncludesZeroDays(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	// Seed events on today and 2 days ago. Series for 5 days should return
	// 5 buckets, with today=1, t-2d=1, the other three = 0.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, today.Add(2*time.Hour))
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, today.AddDate(0, 0, -2).Add(2*time.Hour))

	series, err := stores.Downloads.DailySeries(ctx, org.ID, 5)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}
	if len(series) != 5 {
		t.Fatalf("len = %d, want 5", len(series))
	}
	// Buckets are in chronological order: oldest first.
	wantTodayKey := today.Format("2006-01-02")
	wantMinus2Key := today.AddDate(0, 0, -2).Format("2006-01-02")
	if series[4].Day != wantTodayKey || series[4].Count != 1 {
		t.Errorf("today bucket = %+v, want %s / count 1", series[4], wantTodayKey)
	}
	if series[2].Day != wantMinus2Key || series[2].Count != 1 {
		t.Errorf("t-2d bucket = %+v, want %s / count 1", series[2], wantMinus2Key)
	}
	// Days 1, 3 should be zero (indices 3 and 1).
	if series[3].Count != 0 {
		t.Errorf("yesterday bucket should be 0, got %d", series[3].Count)
	}
	if series[1].Count != 0 {
		t.Errorf("t-3d bucket should be 0, got %d", series[1].Count)
	}
}

func TestDownloadStore_PruneOlderThan_RemovesOldOnly(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	now := time.Now().UTC()
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now.Add(-100*24*time.Hour)) // old
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now.Add(-10*24*time.Hour))  // kept
	seedDownload(t, stores, org.ID, pkg.ID, v.ID, now)                        // kept

	n, err := stores.Downloads.PruneOlderThan(ctx, now.Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}

	total, _ := stores.Downloads.TotalSince(ctx, org.ID, time.Time{})
	if total != 2 {
		t.Errorf("remaining total = %d, want 2", total)
	}
}

// The store clamps absurd `limit` values to a safe ceiling rather than
// propagating them into the DB. Guards against a buggy or hostile caller
// turning one request into an unbounded scan.
func TestDownloadStore_ListLimit_Clamped(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	// Seed 150 events so a request for ≥150 would come back in full
	// without the cap. 100 is the documented ceiling.
	now := time.Now().UTC()
	for i := 0; i < 150; i++ {
		seedDownload(t, stores, org.ID, pkg.ID, v.ID, now.Add(-time.Duration(i)*time.Second))
	}

	recent, err := stores.Downloads.Recent(ctx, org.ID, 10_000)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 100 {
		t.Errorf("Recent len = %d, want 100 (clamped)", len(recent))
	}

	top, err := stores.Downloads.TopPackages(ctx, org.ID, time.Time{}, 10_000)
	if err != nil {
		t.Fatalf("TopPackages: %v", err)
	}
	// Only one package in the fixture, but the clamp should still apply
	// to the query's LIMIT clause.
	if len(top) > 100 {
		t.Errorf("TopPackages len = %d, want ≤ 100", len(top))
	}
}

func TestDownloadStore_CrossTenantIsolation(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	orgA := testutil.MakeOrg(t, stores, "alpha", "Alpha")
	orgB := testutil.MakeOrg(t, stores, "beta", "Beta")
	pkgA := testutil.MakePackage(t, stores, orgA.ID, "vendor/a")
	pkgB := testutil.MakePackage(t, stores, orgB.ID, "vendor/b")
	vA := testutil.MakeVersion(t, stores, pkgA.ID, "1.0.0", "sha", 100)
	vB := testutil.MakeVersion(t, stores, pkgB.ID, "1.0.0", "sha", 100)

	now := time.Now().UTC()
	// orgA gets 3 events, orgB gets 1.
	for i := 0; i < 3; i++ {
		seedDownload(t, stores, orgA.ID, pkgA.ID, vA.ID, now)
	}
	seedDownload(t, stores, orgB.ID, pkgB.ID, vB.ID, now)

	// Each org sees only its own count.
	if n, _ := stores.Downloads.TotalSince(ctx, orgA.ID, time.Time{}); n != 3 {
		t.Errorf("orgA total = %d, want 3", n)
	}
	if n, _ := stores.Downloads.TotalSince(ctx, orgB.ID, time.Time{}); n != 1 {
		t.Errorf("orgB total = %d, want 1", n)
	}

	// TopPackages is org-scoped.
	topA, _ := stores.Downloads.TopPackages(ctx, orgA.ID, time.Time{}, 10)
	if len(topA) != 1 || topA[0].PackageName != "vendor/a" {
		t.Errorf("orgA top = %+v, want only vendor/a", topA)
	}
	topB, _ := stores.Downloads.TopPackages(ctx, orgB.ID, time.Time{}, 10)
	if len(topB) != 1 || topB[0].PackageName != "vendor/b" {
		t.Errorf("orgB top = %+v, want only vendor/b", topB)
	}

	// Recent is org-scoped.
	recentA, _ := stores.Downloads.Recent(ctx, orgA.ID, 10)
	if len(recentA) != 3 {
		t.Errorf("orgA recent = %d events, want 3", len(recentA))
	}
	for _, r := range recentA {
		if r.PackageName != "vendor/a" {
			t.Errorf("orgA recent leaked cross-tenant: %+v", r)
		}
	}
}
