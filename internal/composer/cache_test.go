package composer_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestCache_RebuildPerOrg(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	orgA := testutil.MakeOrg(t, stores, "org-a", "A")
	orgB := testutil.MakeOrg(t, stores, "org-b", "B")
	pkgA := testutil.MakePackage(t, stores, orgA.ID, "vendor/a")
	pkgB := testutil.MakePackage(t, stores, orgB.ID, "vendor/b")
	testutil.MakeVersion(t, stores, pkgA.ID, "1.0.0", "sha", 100)
	testutil.MakeVersion(t, stores, pkgB.ID, "2.0.0", "sha", 100)

	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")

	// Before Rebuild, both orgs are empty.
	if c.GetPackagesJSON(orgA.ID) != nil {
		t.Fatal("cache should be empty before Rebuild")
	}

	// Rebuild only A.
	if err := c.Rebuild(ctx, orgA.ID); err != nil {
		t.Fatalf("Rebuild A: %v", err)
	}
	dataA := c.GetPackagesJSON(orgA.ID)
	if dataA == nil {
		t.Fatal("A's packages.json should be available after Rebuild")
	}
	if !strings.Contains(string(dataA), "vendor/a") {
		t.Errorf("A's packages.json missing vendor/a: %s", dataA)
	}
	if strings.Contains(string(dataA), "vendor/b") {
		t.Errorf("A's packages.json leaked vendor/b: %s", dataA)
	}

	// B is still empty.
	if c.GetPackagesJSON(orgB.ID) != nil {
		t.Error("B should still be empty after rebuilding only A")
	}
}

func TestCache_RebuildAll(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	orgA := testutil.MakeOrg(t, stores, "a", "A")
	orgB := testutil.MakeOrg(t, stores, "b", "B")
	pA := testutil.MakePackage(t, stores, orgA.ID, "vendor/a")
	pB := testutil.MakePackage(t, stores, orgB.ID, "vendor/b")
	testutil.MakeVersion(t, stores, pA.ID, "1.0.0", "sha", 100)
	testutil.MakeVersion(t, stores, pB.ID, "1.0.0", "sha", 100)

	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")
	if err := c.RebuildAll(ctx); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}

	if c.GetPackagesJSON(orgA.ID) == nil {
		t.Error("A missing after RebuildAll")
	}
	if c.GetPackagesJSON(orgB.ID) == nil {
		t.Error("B missing after RebuildAll")
	}
}

func TestCache_GetProviderJSON(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "a", "A")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "deadbeef", 100)

	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")
	if err := c.Rebuild(ctx, org.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	data := c.GetProviderJSON(org.ID, "vendor/pkg")
	if data == nil {
		t.Fatal("provider JSON should be present after Rebuild")
	}
	if !strings.Contains(string(data), "deadbeef") {
		t.Errorf("shasum missing from provider JSON: %s", data)
	}

	miss := c.GetProviderJSON(org.ID, "vendor/unknown")
	if miss != nil {
		t.Error("unknown package should return nil")
	}

	crossOrg := c.GetProviderJSON(9999, "vendor/pkg")
	if crossOrg != nil {
		t.Error("unknown org should return nil")
	}
}

func TestCache_Invalidate_RepopulatesFromStore(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "a", "A")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/a")
	testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")
	if err := c.Rebuild(ctx, org.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Add a new package after initial cache build.
	pkg2 := testutil.MakePackage(t, stores, org.ID, "vendor/b")
	testutil.MakeVersion(t, stores, pkg2.ID, "1.0.0", "sha", 100)

	// Before invalidate, cache is stale.
	data := c.GetPackagesJSON(org.ID)
	if strings.Contains(string(data), "vendor/b") {
		t.Fatal("stale cache already showing vendor/b — test assumption wrong")
	}

	// Invalidate rebuilds.
	c.Invalidate(ctx, org.ID)

	fresh := c.GetPackagesJSON(org.ID)
	if !strings.Contains(string(fresh), "vendor/b") {
		t.Errorf("Invalidate did not refresh cache: %s", fresh)
	}
}

func TestCache_ConcurrentReadsSafe(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "a", "A")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/a")
	testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test", "single")
	if err := c.Rebuild(ctx, org.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Hammer the cache from many readers while a writer rebuilds repeatedly.
	// Run with `go test -race ./internal/composer/...` to catch data races.
	var readers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 20; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = c.GetPackagesJSON(org.ID)
					_ = c.GetProviderJSON(org.ID, "vendor/a")
				}
			}
		}()
	}

	// Writer: rebuild 50 times while readers hammer the cache.
	for i := 0; i < 50; i++ {
		if err := c.Rebuild(ctx, org.ID); err != nil {
			t.Errorf("Rebuild during race: %v", err)
			break
		}
	}

	close(stop)
	readers.Wait()
}
