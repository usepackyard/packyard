// Package jobs runs the background sync pipeline. Users enqueue a
// SyncJob via the JobStore; workers claim and execute; a sweeper
// recovers jobs left stuck in "running" by crashed workers.
//
// See cmd/packyard/serve.go for boot-time setup and docs/ for operator
// notes on workers/retention.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

// Tuneables. These are intentionally constants rather than env vars —
// the values are informed by the claim protocol and the downstream DB
// load, not deployment policy.
const (
	// pollInterval is how long an idle worker waits before checking the
	// queue again. Short enough to feel responsive (a click-to-running
	// latency under 2s), long enough to not hammer the DB when idle.
	pollInterval = 2 * time.Second

	// progressFlushInterval throttles DB writes from per-release progress
	// updates. A big sync emits ~200 callbacks; we only persist every 3s
	// (plus the first and last) to avoid burning SQLite on status noise.
	progressFlushInterval = 3 * time.Second

	// sweepInterval is how often we scan for stuck jobs.
	sweepInterval = 60 * time.Second

	// stuckThreshold — a job whose started_at is older than this is
	// considered abandoned. Bigger than any realistic sync (1000 releases
	// × ~2s/release = ~30 minutes; sweeper only kicks in well past that).
	stuckThreshold = 30 * time.Minute
)

// Pool owns the worker goroutines and the sweeper. One pool per process.
type Pool struct {
	stores     *store.Stores
	storage    storage.Storage
	cache      *composer.Cache
	cfg        *config.Config
	numWorkers int
	// workerIDBase is "hostname-pid"; each worker appends "-N".
	workerIDBase string
	wg           sync.WaitGroup
}

// NewPool constructs a Pool but does not start it. Call Start to spawn
// goroutines. Number of workers comes from cfg.SyncWorkers; a value <= 0
// disables worker execution entirely (useful for read-only replicas).
func NewPool(stores *store.Stores, strg storage.Storage, cache *composer.Cache, cfg *config.Config) *Pool {
	host, _ := os.Hostname()
	base := fmt.Sprintf("%s-%d", host, os.Getpid())
	return &Pool{
		stores:       stores,
		storage:      strg,
		cache:        cache,
		cfg:          cfg,
		numWorkers:   cfg.SyncWorkers,
		workerIDBase: base,
	}
}

// Start spawns numWorkers worker goroutines plus one sweeper. All exit
// cleanly when ctx is cancelled; callers should WaitGroup-block on
// shutdown by calling Wait if they need to ensure in-flight jobs
// finished persisting progress before the process exits.
func (p *Pool) Start(ctx context.Context) {
	if p.numWorkers <= 0 {
		slog.Info("sync worker pool disabled (PACKYARD_SYNC_WORKERS <= 0)")
		return
	}
	slog.Info("starting sync worker pool", "workers", p.numWorkers)
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		workerID := fmt.Sprintf("%s-%d", p.workerIDBase, i)
		go func() {
			defer p.wg.Done()
			p.runWorker(ctx, workerID)
		}()
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runSweeper(ctx)
	}()
}

// Wait blocks until Start's goroutines return. Useful only in tests; in
// the main binary we rely on process exit.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// runWorker loops: claim → execute → mark terminal. Idle cycles sleep
// pollInterval. Exits when ctx is cancelled.
func (p *Pool) runWorker(ctx context.Context, workerID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := p.stores.Jobs.ClaimNext(ctx, workerID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("sync worker: claim failed", "error", err, "worker", workerID)
			p.sleep(ctx, pollInterval)
			continue
		}
		if job == nil {
			p.sleep(ctx, pollInterval)
			continue
		}

		p.runJob(ctx, job)
	}
}

// sleep returns early when ctx is cancelled — avoids blocking shutdown
// on an idle worker waiting out its full poll interval.
func (p *Pool) sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// runJob executes one claimed SyncJob end-to-end. Any non-recoverable
// setup error (missing package, missing source, unknown provider,
// list-releases 404) becomes MarkFailed. Normal per-release failures
// go into the SyncResult.Errors bucket and the job is still
// MarkSucceeded (the sync *completed*; some releases failed within it).
func (p *Pool) runJob(ctx context.Context, job *model.SyncJob) {
	pkg, err := p.stores.Packages.GetByID(ctx, job.OrgID, job.PackageID)
	if err != nil || pkg == nil {
		p.fail(ctx, job, fmt.Sprintf("package %d not found", job.PackageID))
		return
	}

	src, err := p.stores.Sources.GetByPackageID(ctx, pkg.ID)
	if err != nil {
		p.fail(ctx, job, fmt.Sprintf("load source: %s", err))
		return
	}
	if src == nil {
		p.fail(ctx, job, "no source configured for this package")
		return
	}

	token := src.AuthToken
	if token == "" {
		token = p.cfg.Providers.TokenFor(src.Provider)
	}
	prov, err := provider.NewProvider(src.Provider, token)
	if err != nil {
		p.fail(ctx, job, fmt.Sprintf("provider %q: %s", src.Provider, err))
		return
	}

	opts := provider.SyncOpts{
		OnProgress: p.progressCallback(ctx, job.ID),
	}

	result, err := provider.Sync(ctx, prov, src, pkg, p.stores.Packages, p.storage, p.cache, job.OrgID, opts)
	if err != nil {
		p.fail(ctx, job, err.Error())
		return
	}

	// Persist the full result for the UI. If marshaling fails (shouldn't),
	// fall back to an empty object so the UI still renders.
	raw, mErr := json.Marshal(result)
	if mErr != nil {
		raw = []byte(`{}`)
	}
	if err := p.stores.Jobs.MarkSucceeded(ctx, job.ID,
		string(raw), len(result.Imported), len(result.Refreshed), len(result.Skipped), len(result.Errors)); err != nil {
		slog.Error("sync worker: mark succeeded failed", "error", err, "job", job.ID)
	}
	// Touch the source's last_synced_at so the UI reflects completion.
	if err := p.stores.Sources.UpdateLastSynced(ctx, pkg.ID); err != nil {
		slog.Error("sync worker: update last_synced_at", "error", err)
	}
}

// fail marks a job failed with the given message and logs it. Use for
// unrecoverable setup errors (not per-release failures).
func (p *Pool) fail(ctx context.Context, job *model.SyncJob, msg string) {
	slog.Warn("sync job failed", "job", job.ID, "package", job.PackageID, "error", msg)
	if err := p.stores.Jobs.MarkFailed(ctx, job.ID, msg); err != nil {
		slog.Error("sync worker: mark failed failed", "error", err, "job", job.ID)
	}
}

// progressCallback returns a SyncOpts.OnProgress function that writes
// into the jobs table, throttled to progressFlushInterval. The first and
// last calls are always persisted so the UI sees a sensible start/end.
func (p *Pool) progressCallback(ctx context.Context, jobID int64) func(done, total int) {
	var (
		mu        sync.Mutex
		lastWrite time.Time
		lastDone  = -1
		lastTotal = -1
	)
	return func(done, total int) {
		mu.Lock()
		defer mu.Unlock()

		first := lastDone == -1
		terminal := total > 0 && done == total
		since := time.Since(lastWrite)
		if !first && !terminal && since < progressFlushInterval {
			// Throttle: skip this update, the UI will see it on the next
			// flushed one or on completion.
			return
		}
		if done == lastDone && total == lastTotal {
			return // no change
		}

		if err := p.stores.Jobs.UpdateProgress(ctx, jobID, done, total); err != nil {
			slog.Error("sync worker: update progress failed", "error", err, "job", jobID)
			return
		}
		lastWrite = time.Now()
		lastDone = done
		lastTotal = total
	}
}

// runSweeper periodically re-queues jobs that were left running by a
// crashed/stalled worker. Safe to run indefinitely.
func (p *Pool) runSweeper(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := p.stores.Jobs.RecoverStuck(ctx, stuckThreshold)
			if err != nil {
				slog.Error("sync sweeper: RecoverStuck failed", "error", err)
				continue
			}
			if n > 0 {
				slog.Info("sync sweeper: re-queued stuck jobs", "count", n)
			}
		}
	}
}
