package provider

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Release is a provider-agnostic release representation.
type Release struct {
	TagName string
	Draft   bool
	Assets  []Asset
	// PublishedAt is when the release was published upstream (not when
	// we observed it). Zero value if the provider doesn't expose one;
	// the sync pipeline falls back to time.Now in that case.
	PublishedAt time.Time
}

// Asset is a downloadable file attached to a release.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// WebhookEvent is parsed from a provider-specific webhook payload.
type WebhookEvent struct {
	RepoOwner string
	RepoName  string
	TagName   string
	Action    string
	IsDraft   bool
}

// SyncResult holds the outcome of a sync operation. Empty slices (not nil)
// so the JSON encoding is always [], avoiding client-side nil crashes.
//
// Buckets:
//   - Imported:  new versions pulled in on this run
//   - Refreshed: existing versions whose metadata (release date) was
//                updated in-place because the provider reported a newer
//                value than we had stored. Dist bytes never change.
//   - Skipped:   releases intentionally not imported (already-exists,
//                no-matching-asset, composer-version-missing)
//   - Errors:    per-release failures
type SyncResult struct {
	Imported  []string       `json:"imported"`
	Refreshed []string       `json:"refreshed"`
	Skipped   []SkippedEntry `json:"skipped"`
	Errors    []string       `json:"errors"`
}

// SkippedEntry records that a release was intentionally not imported, along
// with a machine-readable reason so the UI can group/display them. Common
// reasons:
//   - "already-exists":    a version with the same tag is already indexed
//   - "no-matching-asset": strategy=release_asset but no asset matched the
//                         configured pattern (typical for old tag-only
//                         releases that predate published artifacts)
type SkippedEntry struct {
	Tag    string `json:"tag"`
	Reason string `json:"reason"`
}

// Provider abstracts a Git hosting platform's release API and webhooks.
type Provider interface {
	// ListReleases returns all non-draft releases for a repo.
	ListReleases(ctx context.Context, owner, repo string) ([]Release, error)

	// DownloadAsset downloads a release asset by URL.
	DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error)

	// DownloadSourceArchive downloads the source ZIP for a tag.
	DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error)

	// ParseWebhook extracts a WebhookEvent from the raw request body.
	ParseWebhook(body []byte) (*WebhookEvent, error)

	// ValidateWebhook checks the request signature against the secret.
	ValidateWebhook(r *http.Request, secret string, body []byte) error
}
