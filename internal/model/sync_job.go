package model

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncJob is one run of the package sync pipeline — enqueued by a user
// click or an incoming webhook, claimed by a worker, and driven to a
// terminal status (succeeded | failed | stale). Rows persist so sync
// survives a server restart: anything left `running` after a crash is
// re-queued by the sweeper on boot.
//
// The `Imported` / `Skipped` / `Errored` counters are maintained
// incrementally so the UI can display live progress; the full detailed
// result (per-release imported/skipped/errors) lands in ResultJSON on
// terminal completion and is what the Sync Result card renders.
type SyncJob struct {
	bun.BaseModel `bun:"table:sync_jobs,alias:sj" json:"-"`

	ID        int64  `bun:"id,pk,autoincrement" json:"-"`
	PublicID  string `bun:"public_id,notnull,unique" json:"id"`
	OrgID     int64  `bun:"org_id,notnull" json:"-"`
	PackageID int64  `bun:"package_id,notnull" json:"-"`
	// Trigger records what caused this job to exist. "manual" from the
	// dashboard, "webhook" from a provider push. Used purely for audit.
	Trigger string `bun:"trigger,notnull" json:"trigger"`
	// Status follows a small state machine:
	//   queued    → (claim)    → running
	//   running   → (complete) → succeeded | failed
	//   running   → (sweeper)  → queued   (if stuck)
	//   running   → (boot)     → queued   (RecoverStuck on startup)
	Status string `bun:"status,notnull,default:'queued'" json:"status"`

	// Incremental progress, written by the worker (throttled) so the UI
	// can show "Running 42 / 183" without hammering the DB.
	ProgressDone  int `bun:"progress_done,notnull,default:0" json:"progress_done"`
	ProgressTotal int `bun:"progress_total,notnull,default:0" json:"progress_total"`

	// Summary counters populated at terminal completion. Imported+Skipped
	// +Errored ≤ ProgressTotal (some releases may not reach counting, e.g.
	// on abort).
	Imported  int `bun:"imported,notnull,default:0" json:"imported"`
	Refreshed int `bun:"refreshed,notnull,default:0" json:"refreshed"`
	Skipped   int `bun:"skipped,notnull,default:0" json:"skipped"`
	Errored   int `bun:"errored,notnull,default:0" json:"errored"`

	// Full SyncResult JSON at completion — the Sync Result card reads
	// this to show per-release breakdown.
	ResultJSON string `bun:"result_json,type:text" json:"result_json,omitempty"`
	// Fatal error that took down the whole sync (not per-release failures,
	// which go in the result). Set when Status == "failed".
	ErrorMsg string `bun:"error_msg,type:text" json:"error_msg,omitempty"`

	// WorkerID is the claiming worker's identifier ("host-pid-N"). Kept
	// server-side only; useful for diagnosing which instance got stuck
	// when chasing a recovery event.
	WorkerID string `bun:"worker_id" json:"-"`

	CreatedAt  time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	StartedAt  *time.Time `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt *time.Time `bun:"finished_at" json:"finished_at,omitempty"`
}
