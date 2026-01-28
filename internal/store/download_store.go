package store

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

// Store-side safety ceilings. These exist so a buggy or hostile caller
// can't translate a single request into an unbounded scan. They're set
// well above any realistic dashboard use; exceed them and you almost
// certainly want a different query shape instead.
const (
	// maxListLimit caps TopPackages / Recent returns. The handler
	// currently asks for 5 / 10; 100 leaves plenty of headroom for
	// future callers.
	maxListLimit = 100

	// maxDailySeriesRows caps the rows we pull into Go memory to
	// bucket into daily counts. At ~3.3k events/day over 30 days this
	// exceeds the load of any realistic org; past that we accept
	// degraded day-level accuracy to protect the process.
	maxDailySeriesRows = 100_000
)

type downloadStoreDB struct {
	db *bun.DB
}

func NewDownloadStoreDB(db *bun.DB) DownloadStore {
	return &downloadStoreDB{db: db}
}

func (s *downloadStoreDB) Record(ctx context.Context, ev *model.DownloadEvent) error {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	_, err := s.db.NewInsert().Model(ev).Returning("id").Exec(ctx)
	return err
}

func (s *downloadStoreDB) TotalSince(ctx context.Context, orgID int64, since time.Time) (int64, error) {
	q := s.db.NewSelect().Model((*model.DownloadEvent)(nil)).Where("org_id = ?", orgID)
	if !since.IsZero() {
		q = q.Where("at >= ?", since)
	}
	count, err := q.Count(ctx)
	return int64(count), err
}

func (s *downloadStoreDB) TopPackages(ctx context.Context, orgID int64, since time.Time, limit int) ([]PackageDownloadCount, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	var rows []PackageDownloadCount
	q := s.db.NewSelect().
		TableExpr("download_events AS d").
		ColumnExpr("d.package_id AS package_id").
		ColumnExpr("p.name AS package_name").
		ColumnExpr("COUNT(*) AS count").
		Join("JOIN packages AS p ON p.id = d.package_id").
		Where("d.org_id = ?", orgID).
		GroupExpr("d.package_id, p.name").
		OrderExpr("count DESC, p.name ASC").
		Limit(limit)
	if !since.IsZero() {
		q = q.Where("d.at >= ?", since)
	}
	if err := q.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *downloadStoreDB) Recent(ctx context.Context, orgID int64, limit int) ([]DownloadEventView, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	var rows []DownloadEventView
	err := s.db.NewSelect().
		TableExpr("download_events AS d").
		ColumnExpr("d.at AS at").
		ColumnExpr("d.package_id AS package_id").
		ColumnExpr("p.name AS package_name").
		ColumnExpr("v.version AS version").
		Join("JOIN packages AS p ON p.id = d.package_id").
		Join("JOIN versions AS v ON v.id = d.version_id").
		Where("d.org_id = ?", orgID).
		OrderExpr("d.at DESC").
		Limit(limit).
		Scan(ctx, &rows)
	return rows, err
}

func (s *downloadStoreDB) DailySeries(ctx context.Context, orgID int64, days int) ([]DailyCount, error) {
	if days <= 0 {
		days = 30
	}
	// Build a UTC-aligned window: today's midnight minus (days-1) → today end-of-day.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	windowStart := today.AddDate(0, 0, -(days - 1))

	// Fetch raw events in the window and bucket in Go.
	// SQL date-trunc isn't portable across sqlite/mysql/postgres dialects, and
	// this query is small on a healthy org. Go-side bucketing keeps the store
	// dialect-agnostic. maxDailySeriesRows caps the scan so a pathologically
	// busy org can't translate a dashboard load into an OOM: past the cap we
	// accept degraded accuracy for that render rather than crash the process.
	var events []model.DownloadEvent
	err := s.db.NewSelect().
		Model(&events).
		Column("at").
		Where("org_id = ?", orgID).
		Where("at >= ?", windowStart).
		Limit(maxDailySeriesRows).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64, days)
	for _, e := range events {
		key := e.At.UTC().Format("2006-01-02")
		counts[key]++
	}

	out := make([]DailyCount, 0, days)
	for i := 0; i < days; i++ {
		day := windowStart.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		out = append(out, DailyCount{Day: key, Count: counts[key]})
	}
	return out, nil
}

func (s *downloadStoreDB) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.NewDelete().Model((*model.DownloadEvent)(nil)).
		Where("at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
