package store

import (
	"context"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

var (
	ErrSSOTicketNotFound        = errors.New("sso ticket not found")
	ErrSSOTicketExpired         = errors.New("sso ticket expired")
	ErrSSOTicketConsumed        = errors.New("sso ticket already consumed")
	ErrSSOTicketAudienceInvalid = errors.New("sso ticket audience mismatch")
)

type PackageStore interface {
	List(ctx context.Context, orgID int64) ([]model.Package, error)
	GetByID(ctx context.Context, orgID, id int64) (*model.Package, error)
	GetByIDGlobal(ctx context.Context, id int64) (*model.Package, error)
	GetByName(ctx context.Context, orgID int64, name string) (*model.Package, error)
	Create(ctx context.Context, pkg *model.Package) error
	Update(ctx context.Context, pkg *model.Package) error
	Delete(ctx context.Context, orgID, id int64) error
	ListVersions(ctx context.Context, orgID, packageID int64) ([]model.Version, error)
	GetVersionByID(ctx context.Context, id int64) (*model.Version, error)
	CreateVersion(ctx context.Context, v *model.Version) error
	DeleteVersion(ctx context.Context, id int64) error
	// UpdateVersionCreatedAt backfills the upstream release date on an
	// existing version. Only this one field is mutable — the dist bytes
	// (dist_sha1, file_size, storage_path) must never change after
	// import because Composer clients rely on them for integrity.
	UpdateVersionCreatedAt(ctx context.Context, id int64, createdAt time.Time) error
	ListAllWithVersions(ctx context.Context, orgID int64) ([]model.Package, error)
	// IncrementDownload atomically bumps a version's download_count and
	// stamps last_downloaded_at. Called in a fire-and-forget goroutine
	// from the dist handler; never blocks the response.
	IncrementDownload(ctx context.Context, versionID int64, at time.Time) error
}

type TokenStore interface {
	List(ctx context.Context, orgID int64) ([]model.APIToken, error)
	Create(ctx context.Context, t *model.APIToken) error
	GetByHash(ctx context.Context, hash string) (*model.APIToken, error)
	Delete(ctx context.Context, orgID, id int64) error
	UpdateLastUsed(ctx context.Context, id int64) error
}

type UserStore interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	List(ctx context.Context) ([]model.User, error)
	ListSuperAdmins(ctx context.Context) ([]model.User, error)
	Create(ctx context.Context, u *model.User) error
	Update(ctx context.Context, u *model.User) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}

type SessionStore interface {
	Create(ctx context.Context, s *model.Session) error
	GetByID(ctx context.Context, id string) (*model.Session, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteOthersByUserID(ctx context.Context, userID int64, keepSessionID string) error
}

type SourceStore interface {
	GetByPackageID(ctx context.Context, packageID int64) (*model.PackageSource, error)
	GetByRepo(ctx context.Context, provider, owner, name string) (*model.PackageSource, error)
	Create(ctx context.Context, src *model.PackageSource) error
	Update(ctx context.Context, src *model.PackageSource) error
	Delete(ctx context.Context, packageID int64) error
	UpdateLastSynced(ctx context.Context, packageID int64) error
}

type AdminTokenStore interface {
	List(ctx context.Context) ([]model.AdminToken, error)
	Create(ctx context.Context, t *model.AdminToken) error
	GetByHash(ctx context.Context, hash string) (*model.AdminToken, error)
	Delete(ctx context.Context, id int64) error
	UpdateLastUsed(ctx context.Context, id int64) error
}

// PackageDownloadCount is a leaderboard row for TopPackages.
type PackageDownloadCount struct {
	PackageID   int64  `json:"package_id"`
	PackageName string `json:"package_name"`
	Count       int64  `json:"count"`
}

// DownloadEventView is a download event joined with its package/version,
// shaped for "recent activity" feeds.
type DownloadEventView struct {
	At          time.Time `json:"at"`
	PackageID   int64     `json:"package_id"`
	PackageName string    `json:"package_name"`
	Version     string    `json:"version"`
}

// DailyCount is one bucket in a time-series of downloads per day.
type DailyCount struct {
	Day   string `json:"day"` // YYYY-MM-DD (UTC)
	Count int64  `json:"count"`
}

// DownloadStore persists per-download events and serves aggregated reads
// for the dashboard. Mutations are fire-and-forget from the request path;
// reads are used interactively so should be fast (indexed on org_id + at).
type DownloadStore interface {
	Record(ctx context.Context, ev *model.DownloadEvent) error
	// TotalSince returns the count of events for an org since a cutoff.
	// Pass the zero time.Time for the all-time total.
	TotalSince(ctx context.Context, orgID int64, since time.Time) (int64, error)
	// TopPackages returns up to `limit` packages with the most downloads
	// since `since`, joined with package name, sorted count desc.
	TopPackages(ctx context.Context, orgID int64, since time.Time, limit int) ([]PackageDownloadCount, error)
	// Recent returns up to `limit` most-recent download events for an org,
	// joined with package name + version string.
	Recent(ctx context.Context, orgID int64, limit int) ([]DownloadEventView, error)
	// DailySeries returns per-day counts (UTC) for the last `days` days
	// ending today, including zero-count days so the chart is gap-less.
	DailySeries(ctx context.Context, orgID int64, days int) ([]DailyCount, error)
	// PruneOlderThan deletes events older than the cutoff, returning the
	// number of rows removed. Called from a daily retention goroutine.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// JobStore persists sync_jobs and implements the claim protocol workers
// use to safely pull jobs without double-execution. Storage for the
// persistent background sync queue.
type JobStore interface {
	// Enqueue inserts a new queued job. ID is populated on return.
	Enqueue(ctx context.Context, job *model.SyncJob) error
	// ClaimNext finds the oldest queued job and atomically marks it
	// running under the given worker ID. Returns (nil, nil) when the
	// queue is empty or another worker won the race.
	ClaimNext(ctx context.Context, workerID string) (*model.SyncJob, error)
	// UpdateProgress writes the done/total counters mid-run. Workers
	// throttle these (see internal/jobs/pool.go) to avoid DB churn.
	UpdateProgress(ctx context.Context, id int64, done, total int) error
	// MarkSucceeded transitions a running job to succeeded, storing the
	// full SyncResult JSON and summary counters.
	MarkSucceeded(ctx context.Context, id int64, resultJSON string, imported, refreshed, skipped, errored int) error
	// MarkFailed transitions a running job to failed with a fatal error
	// message (distinct from per-release errors, which go in result).
	MarkFailed(ctx context.Context, id int64, errMsg string) error
	// GetByID returns a single job scoped to org (defense in depth so
	// one tenant can't poll another's sync state via numeric ID guess).
	GetByID(ctx context.Context, orgID, id int64) (*model.SyncJob, error)
	// ListForPackage returns recent jobs for one package, newest first.
	ListForPackage(ctx context.Context, orgID, packageID int64, limit int) ([]model.SyncJob, error)
	// ActiveForPackage returns the current queued-or-running job for a
	// package, if one exists. Used to enforce the invariant that only
	// one sync runs per package at a time.
	ActiveForPackage(ctx context.Context, packageID int64) (*model.SyncJob, error)
	// RecoverStuck re-queues running jobs whose started_at is older
	// than `threshold`. Called periodically by the sweeper and once at
	// boot (with threshold=0 to recover from ungraceful shutdowns).
	// Returns the number of rows recovered.
	RecoverStuck(ctx context.Context, threshold time.Duration) (int64, error)
	// PruneOlderThan deletes finished jobs (succeeded/failed/stale) older
	// than cutoff. Active jobs (queued/running) are never pruned.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type OrgStore interface {
	GetByID(ctx context.Context, id int64) (*model.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*model.Organization, error)
	List(ctx context.Context) ([]model.Organization, error)
	Create(ctx context.Context, org *model.Organization) error
	Update(ctx context.Context, org *model.Organization) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	Delete(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, orgID int64) ([]model.OrgMember, error)
	GetMember(ctx context.Context, orgID, userID int64) (*model.OrgMember, error)
	AddMember(ctx context.Context, m *model.OrgMember) error
	UpdateMember(ctx context.Context, m *model.OrgMember) error
	RemoveMember(ctx context.Context, orgID, userID int64) error
	ListUserOrgs(ctx context.Context, userID int64) ([]model.Organization, error)
}

type SSOTicketStore interface {
	Create(ctx context.Context, t *model.SSOTicket) error
	Consume(ctx context.Context, tokenHash, audience string, now time.Time) (*model.SSOTicket, error)
}

// Stores groups all store implementations.
type Stores struct {
	Packages    PackageStore
	Tokens      TokenStore
	AdminTokens AdminTokenStore
	Users       UserStore
	Sessions    SessionStore
	Sources     SourceStore
	Orgs        OrgStore
	SSOTickets  SSOTicketStore
	Downloads   DownloadStore
	Jobs        JobStore
}

func NewStores(db *bun.DB) *Stores {
	return &Stores{
		Packages:    NewPackageStoreDB(db),
		Tokens:      NewTokenStoreDB(db),
		AdminTokens: NewAdminTokenStoreDB(db),
		Users:       NewUserStoreDB(db),
		Sessions:    NewSessionStoreDB(db),
		Sources:     NewSourceStoreDB(db),
		Orgs:        NewOrgStoreDB(db),
		SSOTickets:  NewSSOTicketStoreDB(db),
		Downloads:   NewDownloadStoreDB(db),
		Jobs:        NewJobStoreDB(db),
	}
}
