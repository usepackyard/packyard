package composer

import (
	"context"
	"log/slog"
	"sync"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

type orgCache struct {
	packagesJSON []byte
	providers    map[string][]byte
}

// Cache holds pre-built JSON responses for Composer endpoints, scoped per org.
type Cache struct {
	mu       sync.RWMutex
	pkgStore store.PackageStore
	orgStore store.OrgStore
	baseURL  string
	mode     string // "single" or "multi" — controls whether URLs include the slug prefix
	orgs     map[int64]*orgCache
}

// NewCache constructs a metadata cache. mode is either "single" (URLs are
// tenant-less for self-hosters: /p2/..., /dist/...) or "multi" (URLs are
// prefixed with the org slug: /{slug}/p2/..., /{slug}/dist/...).
func NewCache(pkgStore store.PackageStore, orgStore store.OrgStore, baseURL, mode string) *Cache {
	return &Cache{
		pkgStore: pkgStore,
		orgStore: orgStore,
		baseURL:  baseURL,
		mode:     mode,
		orgs:     make(map[int64]*orgCache),
	}
}

// Rebuild rebuilds the cache for a single org.
func (c *Cache) Rebuild(ctx context.Context, orgID int64) error {
	packages, err := c.pkgStore.ListAllWithVersions(ctx, orgID)
	if err != nil {
		return err
	}

	// Resolve org slug for URL prefixing in multi mode.
	slug := ""
	if c.mode == "multi" {
		org, err := c.orgStore.GetByID(ctx, orgID)
		if err != nil {
			return err
		}
		if org != nil {
			slug = org.Slug
		}
	}

	packagesJSON, err := BuildPackagesJSON(packages, slug)
	if err != nil {
		return err
	}

	providers := make(map[string][]byte, len(packages))
	for _, pkg := range packages {
		data, err := BuildProviderJSON(pkg, c.baseURL, slug)
		if err != nil {
			slog.Error("failed to build provider JSON", "package", pkg.Name, "error", err)
			continue
		}
		providers[pkg.Name] = data
	}

	c.mu.Lock()
	c.orgs[orgID] = &orgCache{
		packagesJSON: packagesJSON,
		providers:    providers,
	}
	c.mu.Unlock()

	slog.Info("metadata cache rebuilt", "org_id", orgID, "packages", len(packages))
	return nil
}

// RebuildAll rebuilds the cache for all organizations.
func (c *Cache) RebuildAll(ctx context.Context) error {
	orgs, err := c.orgStore.List(ctx)
	if err != nil {
		return err
	}

	for _, org := range orgs {
		if err := c.Rebuild(ctx, org.ID); err != nil {
			slog.Error("failed to rebuild cache for org", "org_id", org.ID, "error", err)
		}
	}
	return nil
}

// Invalidate rebuilds the cache for a single org.
func (c *Cache) Invalidate(ctx context.Context, orgID int64) {
	if err := c.Rebuild(ctx, orgID); err != nil {
		slog.Error("failed to rebuild metadata cache", "org_id", orgID, "error", err)
	}
}

// GetPackagesJSON returns the cached packages.json response for an org.
func (c *Cache) GetPackagesJSON(orgID int64) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	oc := c.orgs[orgID]
	if oc == nil {
		return nil
	}
	return oc.packagesJSON
}

// GetProviderJSON returns the cached provider JSON for a package within an org.
func (c *Cache) GetProviderJSON(orgID int64, name string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	oc := c.orgs[orgID]
	if oc == nil {
		return nil
	}
	return oc.providers[name]
}

// GetAllPackages returns the list of packages from the store for an org.
func (c *Cache) GetAllPackages(ctx context.Context, orgID int64) ([]model.Package, error) {
	return c.pkgStore.ListAllWithVersions(ctx, orgID)
}
