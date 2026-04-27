package jobs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/jobs"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/testutil"
)

// fakeProvider canned responses for a test sync. Same shape as the one in
// internal/provider/sync_test.go (we can't import a _test.go from another
// package, so we mirror just what we need here).
type fakeProvider struct {
	releases []provider.Release
	zipBytes []byte
}

func (f *fakeProvider) ListReleases(ctx context.Context, owner, repo string) ([]provider.Release, error) {
	return f.releases, nil
}
func (f *fakeProvider) DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.zipBytes)), nil
}
func (f *fakeProvider) DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.zipBytes)), nil
}
func (f *fakeProvider) ParseWebhook(body []byte) (*provider.WebhookEvent, error) {
	return nil, errors.New("not used")
}
func (f *fakeProvider) ValidateWebhook(r *http.Request, secret string, body []byte) error {
	return errors.New("not used")
}

// Register a stub provider that returns our fake, so Pool's
// provider.NewProvider("stub-jobs", ...) resolves to it.
func registerStubJobsProvider(t *testing.T, fake *fakeProvider) {
	t.Helper()
	provider.Register("stub-jobs", func(token string) provider.Provider { return fake })
}

func buildZip(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("composer.json")
	fmt.Fprintf(w, `{"name":%q,"version":%q}`, name, version)
	zw.Close()
	return buf.Bytes()
}

// End-to-end: enqueue a job, start the pool with 1 worker, wait for it to
// reach a terminal status. Verifies the worker claims, sync runs, progress
// lands in the row, and the result is persisted.
func TestPool_EnqueueClaimComplete(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Source points at our stub provider.
	src := &model.PackageSource{
		PackageID: pkg.ID, Provider: "stub-jobs", RepoOwner: "o", RepoName: "r",
		Strategy: "release_asset", AssetPattern: "*.zip", MetadataSource: "from_zip", VersionSource: "auto",
	}
	if err := stores.Sources.Create(ctx, src); err != nil {
		t.Fatalf("Create source: %v", err)
	}

	// Stub provider yields one release whose asset is a valid composer zip.
	zipBytes := buildZip(t, "vendor/pkg", "1.0.0")
	registerStubJobsProvider(t, &fakeProvider{
		releases: []provider.Release{
			{TagName: "v1.0.0", Assets: []provider.Asset{{Name: "pkg.zip", URL: "http://fake/pkg.zip"}}},
		},
		zipBytes: zipBytes,
	})

	cfg := &config.Config{SyncWorkers: 1}
	pool := jobs.NewPool(stores, strg, cache, cfg)
	pool.Start(ctx)

	// Enqueue a manual sync.
	job := &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"}
	if err := stores.Jobs.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Poll until terminal. 5s is generous for a single-release stub sync.
	deadline := time.Now().Add(5 * time.Second)
	var final *model.SyncJob
	for time.Now().Before(deadline) {
		got, err := stores.Jobs.GetByID(ctx, org.ID, job.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status == "succeeded" || got.Status == "failed" {
			final = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("job never reached terminal status within 5s")
	}
	if final.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded. error_msg=%q", final.Status, final.ErrorMsg)
	}
	if final.Imported != 1 {
		t.Errorf("Imported = %d, want 1", final.Imported)
	}
	// ResultJSON round-trips as a valid SyncResult.
	var res provider.SyncResult
	if err := json.Unmarshal([]byte(final.ResultJSON), &res); err != nil {
		t.Fatalf("ResultJSON parse: %v", err)
	}
	if len(res.Imported) != 1 || res.Imported[0] != "1.0.0" {
		t.Errorf("ResultJSON Imported = %v", res.Imported)
	}

	cancel()
	pool.Wait()
}

// Fatal setup error (missing source) must mark the job failed, not leave
// it running forever.
func TestPool_MissingSource_MarksFailed(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strg, _ := storage.NewLocal(t.TempDir())
	cache := composer.NewCache(stores.Packages, stores.Orgs, "http://test")

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	// Note: no source created.

	cfg := &config.Config{SyncWorkers: 1}
	pool := jobs.NewPool(stores, strg, cache, cfg)
	pool.Start(ctx)

	job := &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"}
	stores.Jobs.Enqueue(ctx, job)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := stores.Jobs.GetByID(ctx, org.ID, job.ID)
		if got.Status == "failed" {
			if !strings.Contains(got.ErrorMsg, "no source configured") {
				t.Errorf("error_msg = %q, want 'no source configured'", got.ErrorMsg)
			}
			cancel()
			pool.Wait()
			return
		}
		if got.Status == "succeeded" {
			t.Fatalf("job succeeded; expected failure")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job never reached failed status")
}
