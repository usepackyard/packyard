package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

const maxDownloadSize = 100 << 20 // 100MB

// SyncOpts carries optional knobs for a sync run — today just a progress
// callback invoked after each release completes. Workers use this to
// update the sync_jobs row so the UI can display "Running 42 / 183" live.
// Pass the zero value when no callback is needed (test paths, direct
// calls that don't care about progress).
type SyncOpts struct {
	// OnProgress is called after each release is processed (imported,
	// skipped, or errored). `done` is the count completed so far, `total`
	// is the total releases we'll see in this run. Callback errors are
	// not propagated — progress reporting is advisory.
	OnProgress func(done, total int)
}

// Sync compares releases from a provider against existing versions and
// imports new ones. See SyncOpts for optional progress reporting.
func Sync(
	ctx context.Context,
	prov Provider,
	src *model.PackageSource,
	pkg *model.Package,
	packages store.PackageStore,
	strg storage.Storage,
	cache *composer.Cache,
	orgID int64,
	opts SyncOpts,
) (*SyncResult, error) {
	releases, err := prov.ListReleases(ctx, src.RepoOwner, src.RepoName)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	existingVersions, err := packages.ListVersions(ctx, orgID, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	// existingByVersion gives O(1) lookup by version string AND carries
	// the row's ID + CreatedAt, needed for the release-date refresh path
	// below. Two structurally identical loops of the provider's releases
	// would be cleaner conceptually but cost a hash+allocation per release
	// that already exists in the DB; one map is simpler.
	existingByVersion := make(map[string]*model.Version, len(existingVersions))
	for i := range existingVersions {
		v := &existingVersions[i]
		existingByVersion[v.Version] = v
	}

	// Initialize slices as empty (not nil) so the JSON response is
	// {"imported":[],...} rather than {"imported":null,...}. Clients that
	// call .length/.map on the array shouldn't have to null-check.
	result := &SyncResult{
		Imported:  []string{},
		Refreshed: []string{},
		Skipped:   []SkippedEntry{},
		Errors:    []string{},
	}

	// Progress reporting contract: emit (N, total) at the top of each
	// iteration (meaning "N releases complete; starting release N+1")
	// and (total, total) after the loop exits. The early-continue paths
	// below don't need to touch this — the next iteration's emission
	// covers them.
	total := len(releases)
	reportProgress := func(done int) {
		if opts.OnProgress != nil {
			opts.OnProgress(done, total)
		}
	}

	for idx, rel := range releases {
		reportProgress(idx)
		version := deriveVersion(rel.TagName)
		if existing, ok := existingByVersion[version]; ok {
			// Version already imported — don't touch dist bytes. But if
			// the upstream release has a publish date we didn't capture
			// at original-import time (or got wrong for some reason),
			// backfill it now. Cheap, idempotent, and from the user's
			// POV: re-syncing fixes their historical "Released" column.
			if !rel.PublishedAt.IsZero() && !rel.PublishedAt.Equal(existing.CreatedAt) {
				if err := packages.UpdateVersionCreatedAt(ctx, existing.ID, rel.PublishedAt); err != nil {
					slog.Error("sync: refresh release date failed", "tag", rel.TagName, "error", err)
					result.Errors = append(result.Errors, fmt.Sprintf("%s: refresh date: %s", rel.TagName, err))
					continue
				}
				result.Refreshed = append(result.Refreshed, rel.TagName)
				continue
			}
			result.Skipped = append(result.Skipped, SkippedEntry{Tag: rel.TagName, Reason: "already-exists"})
			continue
		}

		if err := importRelease(ctx, prov, src, pkg, rel, version, packages, strg); err != nil {
			// Explicit skip sentinel — the release was intentionally not
			// imported (e.g. composer_json version source required but
			// the source has no version). Report as Skipped with its
			// specific reason so the UI groups them.
			var skip *skipSentinelError
			if errors.As(err, &skip) {
				result.Skipped = append(result.Skipped, SkippedEntry{Tag: skip.Tag, Reason: skip.Reason})
				continue
			}
			// "No matching asset" is a normal outcome for old tag-only
			// releases that predate published artifacts — treat as a
			// skip with a specific reason instead of noise.
			if isNoMatchingAsset(err) {
				result.Skipped = append(result.Skipped, SkippedEntry{Tag: rel.TagName, Reason: "no-matching-asset"})
				continue
			}
			slog.Error("sync: failed to import release", "tag", rel.TagName, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", rel.TagName, err))
			continue
		}

		result.Imported = append(result.Imported, version)
	}

	if len(result.Imported) > 0 {
		cache.Invalidate(ctx, orgID)
	}

	// Final progress emission — signals the UI that we're done, even if
	// total == 0 (repo with no releases).
	reportProgress(total)

	return result, nil
}

// importRelease downloads, validates, stores, and indexes a single release.
//
// Metadata source — where the composer.json comes from — is governed by
// src.MetadataSource ("from_zip" default, or "manual"). In manual mode we
// synthesize the composer.json from the Package's own fields + the user-
// configured ManualRequire, and skip ParseZIP entirely. That's the
// sanctioned way to publish WordPress-plugin zips (which legitimately
// ship without composer.json) through this registry.
//
// Strategy — where the dist bytes come from — is independent:
//   - release_asset: a zip matching AssetPattern on the GitHub release
//   - source_archive: GitHub's zipball of the tagged source tree
//
// Version source — which string becomes the version — is governed by
// src.VersionSource ("auto" default / "git_tag" / "composer_json"). See
// resolveVersion for details.
func importRelease(
	ctx context.Context,
	prov Provider,
	src *model.PackageSource,
	pkg *model.Package,
	rel Release,
	version string,
	packages store.PackageStore,
	strg storage.Storage,
) error {
	// 1. Fetch the dist bytes per strategy. Always needed — we serve them
	//    as the installable zip and compute sha1/size from them.
	distTmpPath, err := fetchDist(ctx, prov, src, rel)
	if err != nil {
		return err
	}
	defer os.Remove(distTmpPath)

	// 2. Produce a ComposerJSON describing the package: either parsed out
	//    of the dist zip (from_zip) or synthesized from the Package row
	//    (manual).
	cj, err := resolveMetadata(src, pkg, distTmpPath, version)
	if err != nil {
		return err
	}

	// 3. Apply the version-source policy. May rewrite cj.Version and
	//    cj.RawJSON so the stored composer_json agrees with the served
	//    metadata. May also return a skipSentinel to signal the release
	//    should be reported as Skipped, not Errored.
	if err := resolveVersion(src, cj, rel.TagName); err != nil {
		return err
	}

	if err := composer.ValidateVersion(cj.Version); err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}
	// Identity check — prevents a compromised or misconfigured source from
	// writing into an unrelated package path.
	if cj.Name != pkg.Name {
		return fmt.Errorf("composer.json name %q does not match package %q", cj.Name, pkg.Name)
	}

	// 4. Hash + size of the bytes we're actually serving.
	f, err := os.Open(distTmpPath)
	if err != nil {
		return fmt.Errorf("open dist temp: %w", err)
	}
	defer f.Close()

	hasher := sha1.New()
	fileSize, err := io.Copy(hasher, f)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// 5. Upload. Org-prefixed keys prevent cross-tenant collisions.
	storagePath := fmt.Sprintf("%d/%s/%s.zip", pkg.OrgID, pkg.Name, cj.Version)
	f.Seek(0, io.SeekStart)
	if err := strg.Put(ctx, storagePath, f, fileSize); err != nil {
		return fmt.Errorf("storage put: %w", err)
	}

	var requireJSON string
	if cj.Require != nil {
		data, _ := json.Marshal(cj.Require)
		requireJSON = string(data)
	}

	v := &model.Version{
		PackageID:         pkg.ID,
		Version:           cj.Version,
		VersionNormalized: composer.NormalizeVersion(cj.Version),
		DistType:          "zip",
		DistSHA1:          checksum,
		StoragePath:       storagePath,
		FileSize:          fileSize,
		ComposerJSON:      cj.RawJSON,
		RequireJSON:       requireJSON,
	}
	// Honor the upstream publish time so the "Released" column shows
	// the actual release date, not the sync-import time. A zero value
	// (provider doesn't expose it) falls through to CreateVersion's
	// time.Now default.
	if !rel.PublishedAt.IsZero() {
		v.CreatedAt = rel.PublishedAt
	}

	if err := packages.CreateVersion(ctx, v); err != nil {
		strg.Delete(ctx, storagePath)
		return fmt.Errorf("create version: %w", err)
	}

	return nil
}

// fetchDist downloads the bytes we serve as dist, per src.Strategy.
func fetchDist(ctx context.Context, prov Provider, src *model.PackageSource, rel Release) (string, error) {
	switch src.Strategy {
	case "release_asset":
		asset, matchErr := matchAsset(rel.Assets, src.AssetPattern)
		if matchErr != nil {
			return "", matchErr
		}
		body, err := prov.DownloadAsset(ctx, asset.URL)
		if err != nil {
			return "", fmt.Errorf("download asset: %w", err)
		}
		defer body.Close()
		return composer.SaveTempFile(body, maxDownloadSize)

	case "source_archive":
		body, err := prov.DownloadSourceArchive(ctx, src.RepoOwner, src.RepoName, rel.TagName)
		if err != nil {
			return "", fmt.Errorf("download source: %w", err)
		}
		defer body.Close()
		return composer.SaveTempFile(body, maxDownloadSize)

	default:
		return "", fmt.Errorf("unknown strategy: %s", src.Strategy)
	}
}

// resolveMetadata produces a ComposerJSON for the release — either by
// reading composer.json out of the dist zip, or by synthesizing one from
// the Package row plus the user-configured ManualRequire.
func resolveMetadata(src *model.PackageSource, pkg *model.Package, distPath, fallbackVersion string) (*composer.ComposerJSON, error) {
	if src.MetadataSource == "manual" {
		return synthesizeComposerJSON(src, pkg, fallbackVersion)
	}
	// Default ("from_zip" or blank): parse the zip we just fetched.
	cj, err := composer.ParseZIP(distPath)
	if err != nil {
		return nil, fmt.Errorf("parse zip: %w", err)
	}
	return cj, nil
}

// synthesizeComposerJSON builds a composer.json stub from the Package's
// own fields (name, type, description, homepage) plus the configured
// ManualRequire. Used when the release zip legitimately has no
// composer.json (WordPress plugin distributions, for example).
func synthesizeComposerJSON(src *model.PackageSource, pkg *model.Package, tagVersion string) (*composer.ComposerJSON, error) {
	cj := &composer.ComposerJSON{
		Name:        pkg.Name,
		Version:     tagVersion,
		Type:        pkg.Type,
		Description: pkg.Description,
		Homepage:    pkg.Homepage,
	}
	if strings.TrimSpace(src.ManualRequire) != "" {
		var req map[string]string
		if err := json.Unmarshal([]byte(src.ManualRequire), &req); err != nil {
			return nil, fmt.Errorf("manual_require is not a valid JSON object: %w", err)
		}
		if len(req) > 0 {
			cj.Require = req
		}
	}
	raw, err := json.Marshal(synthesizedComposerJSON{
		Name:        cj.Name,
		Version:     cj.Version,
		Type:        cj.Type,
		Description: cj.Description,
		Homepage:    cj.Homepage,
		Require:     cj.Require,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal synthesized composer.json: %w", err)
	}
	cj.RawJSON = string(raw)
	return cj, nil
}

// synthesizedComposerJSON is the shape we serialize into the stored
// composer_json column when metadata_source=manual. Fields are emitted
// only when non-empty so the stored JSON stays readable.
type synthesizedComposerJSON struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Require     map[string]string `json:"require,omitempty"`
}

// skipSentinelError carries a Skipped reason back up through sync. Used
// when a release is intentionally not imported (distinct from an error).
type skipSentinelError struct {
	Reason string
	Tag    string
}

func (s *skipSentinelError) Error() string {
	return fmt.Sprintf("skipped %s: %s", s.Tag, s.Reason)
}

// resolveVersion applies src.VersionSource to the parsed ComposerJSON.
// May mutate cj.Version and cj.RawJSON so the stored composer_json agrees
// with what we serve. Returns a *skipSentinelError to signal a Skipped
// outcome (e.g. composer_json version required but empty).
func resolveVersion(src *model.PackageSource, cj *composer.ComposerJSON, tagName string) error {
	tagVersion := deriveVersion(tagName)
	switch src.VersionSource {
	case "", "auto":
		if cj.Version == "" {
			cj.Version = tagVersion
		}
		return nil
	case "git_tag":
		if cj.Version == tagVersion {
			return nil
		}
		// Force the tag and rewrite cj.RawJSON's "version" field so the
		// stored composer.json agrees with the version we advertise.
		cj.Version = tagVersion
		if cj.RawJSON != "" {
			rewritten, err := rewriteVersionField(cj.RawJSON, tagVersion)
			if err != nil {
				return fmt.Errorf("rewrite version in composer.json: %w", err)
			}
			cj.RawJSON = rewritten
		}
		return nil
	case "composer_json":
		if cj.Version == "" {
			return &skipSentinelError{Reason: "composer-version-missing", Tag: tagName}
		}
		return nil
	default:
		return fmt.Errorf("unknown version_source: %s", src.VersionSource)
	}
}

// rewriteVersionField replaces (or inserts) the top-level "version" key in
// a composer.json document, preserving the rest of the JSON. Uses a
// generic map so we don't lose fields the ComposerJSON struct doesn't
// model.
func rewriteVersionField(raw, version string) (string, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	v, _ := json.Marshal(version)
	doc["version"] = v
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// isNoMatchingAsset detects the specific error produced by matchAsset when
// no release asset matches the configured pattern. Used to reclassify
// these as Skipped rather than Errored in sync results.
func isNoMatchingAsset(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no asset matching pattern")
}

// deriveVersion strips a leading "v" from a tag name.
func deriveVersion(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// matchAsset finds the first asset matching the glob pattern.
func matchAsset(assets []Asset, pattern string) (*Asset, error) {
	for i := range assets {
		matched, err := filepath.Match(pattern, assets[i].Name)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		if matched {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("no asset matching pattern %q (found %d assets)", pattern, len(assets))
}
