package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type jobStoreDB struct {
	db *bun.DB
}

func NewJobStoreDB(db *bun.DB) JobStore {
	return &jobStoreDB{db: db}
}

func (s *jobStoreDB) Enqueue(ctx context.Context, job *model.SyncJob) error {
	if job.Status == "" {
		job.Status = "queued"
	}
	if job.Trigger == "" {
		job.Trigger = "manual"
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.NewInsert().Model(job).Returning("id").Exec(ctx)
	return err
}

// ClaimNext implements the compare-and-swap claim protocol documented in
// the interface: find the oldest queued row, atomically transition it to
// running gated on status still being queued. The gate prevents two
// workers from claiming the same row even without transactions — portable
// across SQLite / MySQL / Postgres without requiring FOR UPDATE SKIP
// LOCKED (which SQLite doesn't have).
func (s *jobStoreDB) ClaimNext(ctx context.Context, workerID string) (*model.SyncJob, error) {
	var id int64
	err := s.db.NewSelect().
		Model((*model.SyncJob)(nil)).
		Column("id").
		Where("status = ?", "queued").
		Order("created_at ASC", "id ASC").
		Limit(1).
		Scan(ctx, &id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().
		Model((*model.SyncJob)(nil)).
		Set("status = ?", "running").
		Set("started_at = ?", now).
		Set("worker_id = ?", workerID).
		Where("id = ?", id).
		Where("status = ?", "queued").
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Lost the race to another worker. Caller will retry on next tick.
		return nil, nil
	}

	// Fetch the claimed row now that we own it. Read-after-write is fine —
	// no other writer will touch status/started_at/worker_id past this point
	// until we mark it terminal.
	job := new(model.SyncJob)
	if err := s.db.NewSelect().Model(job).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *jobStoreDB) UpdateProgress(ctx context.Context, id int64, done, total int) error {
	_, err := s.db.NewUpdate().
		Model((*model.SyncJob)(nil)).
		Set("progress_done = ?", done).
		Set("progress_total = ?", total).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *jobStoreDB) MarkSucceeded(ctx context.Context, id int64, resultJSON string, imported, refreshed, skipped, errored int) error {
	now := time.Now().UTC()
	_, err := s.db.NewUpdate().
		Model((*model.SyncJob)(nil)).
		Set("status = ?", "succeeded").
		Set("finished_at = ?", now).
		Set("result_json = ?", resultJSON).
		Set("imported = ?", imported).
		Set("refreshed = ?", refreshed).
		Set("skipped = ?", skipped).
		Set("errored = ?", errored).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *jobStoreDB) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.NewUpdate().
		Model((*model.SyncJob)(nil)).
		Set("status = ?", "failed").
		Set("finished_at = ?", now).
		Set("error_msg = ?", errMsg).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *jobStoreDB) GetByID(ctx context.Context, orgID, id int64) (*model.SyncJob, error) {
	job := new(model.SyncJob)
	err := s.db.NewSelect().Model(job).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *jobStoreDB) ListForPackage(ctx context.Context, orgID, packageID int64, limit int) ([]model.SyncJob, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var jobs []model.SyncJob
	err := s.db.NewSelect().Model(&jobs).
		Where("org_id = ?", orgID).
		Where("package_id = ?", packageID).
		Order("created_at DESC", "id DESC").
		Limit(limit).
		Scan(ctx)
	return jobs, err
}

func (s *jobStoreDB) ActiveForPackage(ctx context.Context, packageID int64) (*model.SyncJob, error) {
	job := new(model.SyncJob)
	err := s.db.NewSelect().Model(job).
		Where("package_id = ?", packageID).
		Where("status IN (?)", bun.In([]string{"queued", "running"})).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// RecoverStuck re-queues running jobs whose started_at is older than the
// given threshold. Passing threshold=0 matches everything currently
// running — the boot-time recovery case, where any "running" row must be
// the result of an ungraceful shutdown.
func (s *jobStoreDB) RecoverStuck(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	q := s.db.NewUpdate().
		Model((*model.SyncJob)(nil)).
		Set("status = ?", "queued").
		Set("worker_id = ?", "").
		Set("started_at = ?", nil).
		Where("status = ?", "running")
	// Only narrow by cutoff when the caller specified a non-zero threshold.
	// threshold=0 means "recover everything running right now" (boot case).
	if threshold > 0 {
		q = q.Where("started_at < ?", cutoff)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *jobStoreDB) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.NewDelete().
		Model((*model.SyncJob)(nil)).
		Where("status IN (?)", bun.In([]string{"succeeded", "failed", "stale"})).
		Where("finished_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
