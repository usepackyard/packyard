package model

import (
	"time"

	"github.com/uptrace/bun"
)

// DownloadEvent records a single successful Composer dist fetch. It is the
// source of truth for time-bucketed aggregations (daily series, top packages
// by window, recent-activity feeds). The per-version cumulative counter on
// Version is a denormalized cache of COUNT(*) over this table.
//
// We intentionally do NOT store IP, user-agent, or token ID here — keeping
// the table free of per-request PII so it can be retained longer without
// compliance friction. If per-token attribution is needed later, add the
// column behind an explicit opt-in config flag.
type DownloadEvent struct {
	bun.BaseModel `bun:"table:download_events,alias:d" json:"-"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	OrgID     int64     `bun:"org_id,notnull" json:"org_id"`
	PackageID int64     `bun:"package_id,notnull" json:"package_id"`
	VersionID int64     `bun:"version_id,notnull" json:"version_id"`
	At        time.Time `bun:"at,notnull,default:current_timestamp" json:"at"`
}
