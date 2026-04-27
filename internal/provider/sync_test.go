package provider_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/testutil"
)

// fakeProvider implements provider.Provider with canned responses.
// Use the knobs in each test to control what happens.
type fakeProvider struct {
	releases         []provider.Release
	listReleasesErr  error
	downloadAssetFn  func(url string) (io.ReadCloser, error)
	downloadSourceFn func(owner, repo, tag string) (io.ReadCloser, error)
}

func (f *fakeProvider) ListReleases(ctx context.Context, owner, repo string) ([]provider.Release, error) {
	if f.listReleasesErr != nil {
		return nil, f.listReleasesErr
	}
	return f.releases, nil
}

func (f *fakeProvider) DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	if f.downloadAssetFn != nil {
		return f.downloadAssetFn(url)
	}
	return nil, errors.New("no asset download implementation")
}

func (f *fakeProvider) DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error) {
	if f.downloadSourceFn != nil {
		return f.downloadSourceFn(owner, repo, tag)
	}
	return nil, errors.New("no source archive implementation")
}

func (f *fakeProvider) ParseWebhook(body []byte) (*provider.WebhookEvent, error) {
	return nil, errors.New("not used in sync tests")
}

func (f *fakeProvider) ValidateWebhook(r *http.Request, secret string, body []byte) error {
	return errors.New("not used in sync tests")
}

// buildZip creates an in-memory zip whose composer.json holds the given name+version.
func buildZip(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("composer.json")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	fmt.Fprintf(w, `{"name":%q,"version":%q}`, name, version)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// zipReader returns an io.ReadCloser over an in-memory zip.
func zipReader(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}

func gitSource(t *testing.T, packageID int64, strategy, assetPattern string) *model.PackageSource {
	t.Helper()
	return &model.PackageSource{
		PackageID:      packageID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "o", "r", strategy, assetPattern),
	}
}

func TestSync_ImportsNewRelease(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	zipBytes := buildZip(t, "vendor/pkg", "1.0.0")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			return zipReader(zipBytes), nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "1.0.0" {
		t.Errorf("Imported = %v, want [1.0.0]", result.Imported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Version row exists.
	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Errorf("expected 1 version after sync, got %d", len(versions))
	}
}

func TestSync_SkipsExistingVersion(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	p := &fakeProvider{
		releases: []provider.Release{{TagName: "v1.0.0"}},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			t.Error("DownloadAsset should not be called for existing version")
			return nil, nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Tag != "v1.0.0" || result.Skipped[0].Reason != "already-exists" {
		t.Errorf("Skipped = %+v, want [{v1.0.0 already-exists}]", result.Skipped)
	}
	if len(result.Imported) != 0 {
		t.Errorf("Imported should be empty: %v", result.Imported)
	}
}

func TestSync_CollectsErrorsPerRelease(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	goodZip := buildZip(t, "vendor/pkg", "1.0.0")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://ok"}}},
			{TagName: "v2.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://bad"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			if url == "http://bad" {
				return nil, errors.New("network down")
			}
			return zipReader(goodZip), nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 {
		t.Errorf("Imported len = %d, want 1", len(result.Imported))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors len = %d, want 1", len(result.Errors))
	}
}

func TestSync_RewritesNameMismatch(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	zipBytes := buildZip(t, "attacker/evil", "1.0.0")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://x"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			return zipReader(zipBytes), nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "1.0.0" {
		t.Errorf("Imported = %v, want [1.0.0]", result.Imported)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !strings.Contains(versions[0].ComposerJSON, `"name":"vendor/pkg"`) {
		t.Errorf("ComposerJSON should contain rewritten name, got: %s", versions[0].ComposerJSON)
	}
	if strings.Contains(versions[0].ComposerJSON, "attacker/evil") {
		t.Errorf("ComposerJSON should not contain original name, got: %s", versions[0].ComposerJSON)
	}
}

func TestSync_NameMatchUnchanged(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	zipBytes := buildZip(t, "vendor/pkg", "1.0.0")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://x"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			return zipReader(zipBytes), nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "1.0.0" {
		t.Errorf("Imported = %v, want [1.0.0]", result.Imported)
	}

	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !strings.Contains(versions[0].ComposerJSON, `"name":"vendor/pkg"`) {
		t.Errorf("ComposerJSON should contain original name, got: %s", versions[0].ComposerJSON)
	}
}

func TestSync_ListReleasesError(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	p := &fakeProvider{listReleasesErr: errors.New("API down")}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	_, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err == nil {
		t.Fatal("expected error from provider.ListReleases")
	}
}

func TestSync_NoAssetMatchingPattern(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.tar", URL: "http://x"}}},
		},
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// "No matching asset" is expected for old releases that pre-date
	// published artifacts — classify as Skipped, not Errored, so the UI
	// doesn't flag them as failures.
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want empty (no-matching-asset should be a skip)", result.Errors)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "no-matching-asset" {
		t.Errorf("Skipped = %+v, want [{_, no-matching-asset}]", result.Skipped)
	}
}

// Manual metadata: the release zip has no composer.json, but we still
// import the release by synthesizing metadata from the Package row plus
// the configured ManualRequire. This is the sanctioned WordPress-plugin
// path: a production-drop zip goes to clients as-is, with a synthesized
// composer_json describing it.
func TestSync_ManualMetadata_SynthesizesComposerJSON(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Production-drop zip — no composer.json inside.
	var prodDrop bytes.Buffer
	zw := zip.NewWriter(&prodDrop)
	rw, _ := zw.Create("plugin.php")
	rw.Write([]byte("<?php // the real plugin file"))
	zw.Close()
	prodBytes := prodDrop.Bytes()

	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) {
			return zipReader(prodBytes), nil
		},
	}

	src := gitSource(t, pkg.ID, "release_asset", "*.zip")
	src.MetadataSource = "manual"
	src.VersionSource = "git_tag"
	src.ManualRequire = `{"composer/installers": "^2.0"}`

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "1.0.0" {
		t.Fatalf("Imported = %v, want [1.0.0]; errors=%v", result.Imported, result.Errors)
	}

	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Fatalf("want 1 version, got %d", len(versions))
	}
	v := versions[0]
	if v.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", v.Version)
	}
	// Stored composer_json is a synthesized stub with the right shape.
	if !strings.Contains(v.ComposerJSON, `"name":"vendor/pkg"`) {
		t.Errorf("ComposerJSON missing synthesized name: %s", v.ComposerJSON)
	}
	if !strings.Contains(v.ComposerJSON, `"version":"1.0.0"`) {
		t.Errorf("ComposerJSON missing version: %s", v.ComposerJSON)
	}
	// RequireJSON mirrors the manual require config.
	if !strings.Contains(v.RequireJSON, "composer/installers") {
		t.Errorf("RequireJSON missing manual require: %s", v.RequireJSON)
	}
	// Stored dist matches the release asset bytes verbatim.
	if v.FileSize != int64(len(prodBytes)) {
		t.Errorf("FileSize = %d, want %d", v.FileSize, len(prodBytes))
	}
}

// VersionSource=git_tag must force the tag's version even when composer.json
// declares something else, AND must rewrite the stored composer_json so it
// agrees (otherwise served metadata and stored metadata disagree).
func TestSync_VersionSource_GitTagOverridesComposerJSON(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// composer.json declares 9.9.9-dev but the tag is v1.2.3. git_tag wins.
	zipBytes := buildZip(t, "vendor/pkg", "9.9.9-dev")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.2.3", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) { return zipReader(zipBytes), nil },
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")
	src.VersionSource = "git_tag"

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "1.2.3" {
		t.Fatalf("Imported = %v, want [1.2.3]", result.Imported)
	}

	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	v := versions[0]
	if !strings.Contains(v.ComposerJSON, `"version":"1.2.3"`) {
		t.Errorf("ComposerJSON should be rewritten to 1.2.3, got: %s", v.ComposerJSON)
	}
	if strings.Contains(v.ComposerJSON, "9.9.9-dev") {
		t.Errorf("old version should be overwritten, got: %s", v.ComposerJSON)
	}
}

// VersionSource=composer_json with an empty version must Skip the release
// with a specific reason, not Error.
func TestSync_VersionSource_ComposerJSON_RequiresVersion(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// composer.json omits version — under composer_json source we must skip.
	zipBytes := buildZip(t, "vendor/pkg", "")
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}}},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) { return zipReader(zipBytes), nil },
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")
	src.VersionSource = "composer_json"

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "composer-version-missing" {
		t.Fatalf("Skipped = %+v, want one entry with reason composer-version-missing", result.Skipped)
	}
}

// The Version's CreatedAt should reflect the upstream publish time when
// the provider supplies one — otherwise every sync'd version would show
// the sync date in the UI's "Released" column.
func TestSync_UsesUpstreamPublishedAtForVersionDate(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	zipBytes := buildZip(t, "vendor/pkg", "1.0.0")
	publishedAt := time.Date(2021, 6, 15, 10, 0, 0, 0, time.UTC)
	p := &fakeProvider{
		releases: []provider.Release{
			{
				TagName:     "v1.0.0",
				PublishedAt: publishedAt,
				Assets:      []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}},
			},
		},
		downloadAssetFn: func(url string) (io.ReadCloser, error) { return zipReader(zipBytes), nil },
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	_, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	versions, _ := stores.Packages.ListVersions(context.Background(), org.ID, pkg.ID)
	if len(versions) != 1 {
		t.Fatalf("want 1 version, got %d", len(versions))
	}
	if !versions[0].CreatedAt.Equal(publishedAt) {
		t.Errorf("CreatedAt = %v, want upstream publish time %v", versions[0].CreatedAt, publishedAt)
	}
}

// Re-sync must backfill an existing version's release date when the
// upstream provider reports a different (non-zero) PublishedAt. The tag
// lands in Refreshed, not Skipped; no duplicate import happens.
func TestSync_BackfillsReleaseDateOnReSync(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Pre-existing version stamped with the old (wrong) import time.
	importTime := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	existing := &model.Version{
		PackageID: pkg.ID, Version: "1.0.0", VersionNormalized: "1.0.0.0",
		DistType: "zip", DistSHA1: "sha", StoragePath: "old/path.zip", FileSize: 100,
		ComposerJSON: `{"name":"vendor/pkg","version":"1.0.0"}`,
		CreatedAt:    importTime,
	}
	if err := stores.Packages.CreateVersion(context.Background(), existing); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	upstreamPublished := time.Date(2020, 6, 15, 10, 0, 0, 0, time.UTC)
	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", PublishedAt: upstreamPublished},
		},
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Refreshed) != 1 || result.Refreshed[0] != "v1.0.0" {
		t.Errorf("Refreshed = %v, want [v1.0.0]", result.Refreshed)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %+v, want empty", result.Skipped)
	}
	if len(result.Imported) != 0 {
		t.Errorf("Imported = %v, want empty (no re-import)", result.Imported)
	}

	// Row's CreatedAt is now the upstream date, not the import time.
	// Dist fields stay untouched.
	got, _ := stores.Packages.GetVersionByID(context.Background(), existing.ID)
	if !got.CreatedAt.Equal(upstreamPublished) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, upstreamPublished)
	}
	if got.StoragePath != "old/path.zip" {
		t.Errorf("StoragePath changed to %q — dist fields must be immutable", got.StoragePath)
	}
}

// Running sync twice with no upstream change should be a no-op on the
// second run — same version stays Skipped{already-exists}, Refreshed 0.
func TestSync_NoopWhenReleaseDateAlreadyCorrect(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	publishedAt := time.Date(2023, 1, 5, 8, 30, 0, 0, time.UTC)
	existing := &model.Version{
		PackageID: pkg.ID, Version: "1.0.0", VersionNormalized: "1.0.0.0",
		DistType: "zip", DistSHA1: "sha", StoragePath: "p.zip", FileSize: 1,
		ComposerJSON: `{}`,
		CreatedAt:    publishedAt, // already matches upstream
	}
	stores.Packages.CreateVersion(context.Background(), existing)

	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", PublishedAt: publishedAt},
		},
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, _ := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if len(result.Refreshed) != 0 {
		t.Errorf("Refreshed = %v, want empty (date unchanged)", result.Refreshed)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "already-exists" {
		t.Errorf("Skipped = %+v, want [{v1.0.0 already-exists}]", result.Skipped)
	}
}

// If the provider returns no PublishedAt (e.g. a minimal webhook payload
// or API that doesn't expose it), don't overwrite an existing CreatedAt
// with zero — that would corrupt data we already had.
func TestSync_NoUpdateWhenProviderPublishedAtZero(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	preserved := time.Date(2022, 8, 1, 12, 0, 0, 0, time.UTC)
	existing := &model.Version{
		PackageID: pkg.ID, Version: "1.0.0", VersionNormalized: "1.0.0.0",
		DistType: "zip", DistSHA1: "sha", StoragePath: "p.zip", FileSize: 1,
		ComposerJSON: `{}`,
		CreatedAt:    preserved,
	}
	stores.Packages.CreateVersion(context.Background(), existing)

	p := &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0"}, // PublishedAt zero value
		},
	}
	src := gitSource(t, pkg.ID, "release_asset", "*.zip")

	result, _ := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if len(result.Refreshed) != 0 {
		t.Errorf("Refreshed = %v, want empty", result.Refreshed)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("Skipped = %+v, want 1", result.Skipped)
	}

	got, _ := stores.Packages.GetVersionByID(context.Background(), existing.ID)
	if !got.CreatedAt.Equal(preserved) {
		t.Errorf("CreatedAt changed to %v — should be preserved %v", got.CreatedAt, preserved)
	}
}

func TestSync_UnknownStrategy(t *testing.T) {
	stores := testutil.NewStores(t)
	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	p := &fakeProvider{
		releases: []provider.Release{{TagName: "v1.0.0"}},
	}
	src := &model.PackageSource{
		PackageID:      pkg.ID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "o", "r", "bogus", "*.zip"),
	}

	result, err := provider.Sync(context.Background(), p, src, pkg, stores.Packages, strg, cache, org.ID, provider.SyncOpts{})
	if err == nil {
		t.Fatalf("Sync err = nil, result=%+v; want invalid strategy error", result)
	}
}
