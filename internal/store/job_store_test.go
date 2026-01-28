package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestJobStore_Enqueue_And_GetByID(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	job := &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"}
	if err := stores.Jobs.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.ID == 0 {
		t.Fatal("ID not populated after Enqueue")
	}
	if job.Status != "queued" {
		t.Errorf("Status = %q, want queued", job.Status)
	}

	got, err := stores.Jobs.GetByID(ctx, org.ID, job.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v / %+v", err, got)
	}
	if got.PackageID != pkg.ID {
		t.Errorf("PackageID = %d, want %d", got.PackageID, pkg.ID)
	}

	// GetByID must be org-scoped — a different org shouldn't see it.
	other := testutil.MakeOrg(t, stores, "other", "Other")
	miss, _ := stores.Jobs.GetByID(ctx, other.ID, job.ID)
	if miss != nil {
		t.Errorf("GetByID leaked cross-tenant: %+v", miss)
	}
}

// ClaimNext under concurrent workers must never hand the same job to two
// of them. Spawn N goroutines claiming in parallel; count the total number
// of distinct claims and verify it matches the number of queued jobs.
func TestJobStore_ClaimNext_ConcurrencyIsSafe(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	const total = 25
	for i := 0; i < total; i++ {
		stores.Jobs.Enqueue(ctx, &model.SyncJob{
			OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual",
		})
	}

	// Run 10 concurrent claimers, each draining as many as possible.
	const workers = 10
	claimedIDs := make(chan int64, total*2)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			for {
				job, err := stores.Jobs.ClaimNext(ctx, "w")
				if err != nil {
					t.Errorf("ClaimNext: %v", err)
					return
				}
				if job == nil {
					return
				}
				claimedIDs <- job.ID
			}
		}(w)
	}
	wg.Wait()
	close(claimedIDs)

	seen := make(map[int64]bool, total)
	for id := range claimedIDs {
		if seen[id] {
			t.Errorf("job %d claimed twice — race!", id)
		}
		seen[id] = true
	}
	if len(seen) != total {
		t.Errorf("claimed %d jobs, want %d", len(seen), total)
	}
}

func TestJobStore_ActiveForPackage(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Initially nothing active.
	got, _ := stores.Jobs.ActiveForPackage(ctx, pkg.ID)
	if got != nil {
		t.Errorf("want nil active, got %+v", got)
	}

	// Queued job is active.
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"})
	got, _ = stores.Jobs.ActiveForPackage(ctx, pkg.ID)
	if got == nil || got.Status != "queued" {
		t.Errorf("want queued active, got %+v", got)
	}

	// Claim it — it stays active (status=running).
	_, err := stores.Jobs.ClaimNext(ctx, "w")
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	got, _ = stores.Jobs.ActiveForPackage(ctx, pkg.ID)
	if got == nil || got.Status != "running" {
		t.Errorf("want running active, got %+v", got)
	}

	// Mark succeeded — no longer active.
	stores.Jobs.MarkSucceeded(ctx, got.ID, `{}`, 0, 0, 0, 0)
	got, _ = stores.Jobs.ActiveForPackage(ctx, pkg.ID)
	if got != nil {
		t.Errorf("should be nil after completion, got %+v", got)
	}
}

// RecoverStuck with threshold=0 must recover any running job regardless of
// age (the boot-time recovery semantic).
func TestJobStore_RecoverStuck_BootTimeRecoversAll(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// Two queued → claim both to make them running.
	for i := 0; i < 2; i++ {
		stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"})
	}
	stores.Jobs.ClaimNext(ctx, "w1")
	stores.Jobs.ClaimNext(ctx, "w2")

	n, err := stores.Jobs.RecoverStuck(ctx, 0)
	if err != nil {
		t.Fatalf("RecoverStuck: %v", err)
	}
	if n != 2 {
		t.Errorf("recovered %d, want 2", n)
	}

	// Both should be back to queued with no worker assignment.
	jobs, _ := stores.Jobs.ListForPackage(ctx, org.ID, pkg.ID, 10)
	for _, j := range jobs {
		if j.Status != "queued" {
			t.Errorf("job %d status = %q, want queued", j.ID, j.Status)
		}
		if j.WorkerID != "" {
			t.Errorf("job %d WorkerID = %q, want empty", j.ID, j.WorkerID)
		}
	}
}

// With a positive threshold, only jobs older than `now - threshold` get
// recovered. Recent running jobs are left alone.
func TestJobStore_RecoverStuck_RespectsThreshold(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	// One "old" running job (started_at backdated 1h) + one fresh.
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual"})
	old, _ := stores.Jobs.ClaimNext(ctx, "w")
	backdate := time.Now().UTC().Add(-1 * time.Hour)
	// Manually push started_at into the past via the same table.
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Trigger: "manual",
		Status: "running", StartedAt: &backdate, WorkerID: "old-worker"})

	n, err := stores.Jobs.RecoverStuck(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStuck: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered %d, want 1 (only the old job)", n)
	}

	// Freshly-claimed job still running.
	got, _ := stores.Jobs.GetByID(ctx, org.ID, old.ID)
	if got.Status != "running" {
		t.Errorf("fresh job = %q, want running", got.Status)
	}
}

// Retention prunes terminal jobs but leaves active ones alone.
func TestJobStore_PruneOlderThan_OnlyFinishedRows(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-5 * 24 * time.Hour)

	// Old succeeded (should prune), recent succeeded (keep), queued (keep).
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Status: "succeeded", FinishedAt: &old})
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Status: "succeeded", FinishedAt: &recent})
	stores.Jobs.Enqueue(ctx, &model.SyncJob{OrgID: org.ID, PackageID: pkg.ID, Status: "queued"})

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	n, err := stores.Jobs.PruneOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}

	// Two rows remain.
	jobs, _ := stores.Jobs.ListForPackage(ctx, org.ID, pkg.ID, 10)
	if len(jobs) != 2 {
		t.Errorf("remaining = %d, want 2", len(jobs))
	}
}

