package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		// Create tables in FK dependency order.
		models := []interface{}{
			(*model.Organization)(nil),
			(*model.User)(nil),
			(*model.OrgMember)(nil),
			(*model.Session)(nil),
			(*model.SSOTicket)(nil),
			(*model.Package)(nil),
			(*model.Version)(nil),
			(*model.APIToken)(nil),
			(*model.AdminToken)(nil),
			(*model.ProviderConnection)(nil),
			(*model.PackageSource)(nil),
			(*model.DownloadEvent)(nil),
			(*model.SyncJob)(nil),
		}

		for _, m := range models {
			_, err := db.NewCreateTable().Model(m).IfNotExists().Exec(ctx)
			if err != nil {
				return fmt.Errorf("create table for %T: %w", m, err)
			}
		}

		// Foreign keys for org_members.
		db.NewCreateIndex().Model((*model.OrgMember)(nil)).Index("idx_org_members_org_id").Column("org_id").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.OrgMember)(nil)).Index("idx_org_members_user_id").Column("user_id").IfNotExists().Exec(ctx)

		// Indexes for sessions.
		db.NewCreateIndex().Model((*model.Session)(nil)).Index("idx_sessions_user_id").Column("user_id").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.Session)(nil)).Index("idx_sessions_expires_at").Column("expires_at").IfNotExists().Exec(ctx)

		// Indexes for sso_tickets.
		db.NewCreateIndex().Model((*model.SSOTicket)(nil)).Index("idx_sso_tickets_user_id").Column("user_id").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.SSOTicket)(nil)).Index("idx_sso_tickets_expires_at").Column("expires_at").IfNotExists().Exec(ctx)

		// Indexes for packages.
		db.NewCreateIndex().Model((*model.Package)(nil)).Index("idx_packages_org_id").Column("org_id").IfNotExists().Exec(ctx)

		// Indexes for versions.
		db.NewCreateIndex().Model((*model.Version)(nil)).Index("idx_versions_package_id").Column("package_id").IfNotExists().Exec(ctx)

		// Indexes for api_tokens.
		db.NewCreateIndex().Model((*model.APIToken)(nil)).Index("idx_api_tokens_org_id").Column("org_id").IfNotExists().Exec(ctx)

		// Indexes for provider_connections.
		db.NewCreateIndex().Model((*model.ProviderConnection)(nil)).Index("idx_provider_connections_org_id").Column("org_id").IfNotExists().Exec(ctx)

		// Indexes for package_sources.
		db.NewCreateIndex().Model((*model.PackageSource)(nil)).Index("idx_package_sources_package_id").Column("package_id").IfNotExists().Exec(ctx)

		// Indexes for download_events. (org_id, at) powers "recent" and
		// time-window aggregations; (org_id, package_id) powers top-N.
		db.NewCreateIndex().Model((*model.DownloadEvent)(nil)).Index("idx_download_events_org_at").Column("org_id", "at").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.DownloadEvent)(nil)).Index("idx_download_events_org_pkg").Column("org_id", "package_id").IfNotExists().Exec(ctx)

		// Indexes for sync_jobs.
		//   (status, created_at) drives the worker claim query.
		//   (package_id, status) enforces one-active-job-per-package and
		//     powers the per-package history listing.
		//   (org_id, created_at) drives org-scoped history listings.
		db.NewCreateIndex().Model((*model.SyncJob)(nil)).Index("idx_sync_jobs_status_created").Column("status", "created_at").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.SyncJob)(nil)).Index("idx_sync_jobs_pkg_status").Column("package_id", "status").IfNotExists().Exec(ctx)
		db.NewCreateIndex().Model((*model.SyncJob)(nil)).Index("idx_sync_jobs_org_created").Column("org_id", "created_at").IfNotExists().Exec(ctx)

		// Idempotent column additions for databases created before a
		// column was part of the model. CreateTable.IfNotExists() above
		// only creates missing tables — it won't alter an existing one.
		// For fresh DBs this is a no-op (column already exists from
		// create); for existing DBs it brings the schema up to date.
		if _, err := db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN language VARCHAR NOT NULL DEFAULT 'en'"); err != nil {
			// Every supported dialect returns a different error string
			// for "column exists", so match conservatively on substrings.
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "exists") {
				return fmt.Errorf("add users.language: %w", err)
			}
		}

		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		// Drop tables in reverse order.
		models := []interface{}{
			(*model.SyncJob)(nil),
			(*model.DownloadEvent)(nil),
			(*model.PackageSource)(nil),
			(*model.ProviderConnection)(nil),
			(*model.AdminToken)(nil),
			(*model.APIToken)(nil),
			(*model.Version)(nil),
			(*model.Package)(nil),
			(*model.SSOTicket)(nil),
			(*model.Session)(nil),
			(*model.OrgMember)(nil),
			(*model.User)(nil),
			(*model.Organization)(nil),
		}

		for _, m := range models {
			_, err := db.NewDropTable().Model(m).IfExists().Cascade().Exec(ctx)
			if err != nil {
				return fmt.Errorf("drop table for %T: %w", m, err)
			}
		}

		return nil
	})
}
