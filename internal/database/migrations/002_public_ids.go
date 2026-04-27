package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/pid"
)

// Migration 002 introduces the `public_id` string column on every
// URL-facing table. Internal int64 primary keys stay; `public_id` is
// what users and external callers see (e.g. pkg_01JHZ...).
//
// The migration is idempotent and dialect-portable:
//  1. ALTER TABLE ADD COLUMN (ignore "already exists" — fresh DBs get the
//     column in 001 via the updated model tags).
//  2. Backfill any row whose public_id is NULL or empty.
//  3. CREATE UNIQUE INDEX (ignore "already exists").
//
// We don't try to ALTER the column to NOT NULL on existing tables because
// SQLite doesn't support it without table recreation, and the store layer
// always populates public_id on insert — the Go code is the source of
// truth here; the index + Go-side discipline are enough.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		tables := []struct {
			name   string
			prefix string
		}{
			{"packages", pid.Package},
			{"versions", pid.Version},
			{"users", pid.User},
			{"org_members", pid.OrgMember},
			{"api_tokens", pid.APIToken},
			{"admin_tokens", pid.AdminToken},
			{"provider_connections", pid.ProviderConnection},
			{"package_sources", pid.PackageSource},
			{"sync_jobs", pid.SyncJob},
		}

		for _, t := range tables {
			if err := addPublicIDColumn(ctx, db, t.name); err != nil {
				return err
			}
			if err := backfillPublicIDs(ctx, db, t.name, t.prefix); err != nil {
				return err
			}
			if err := createPublicIDIndex(ctx, db, t.name); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		tables := []string{
			"sync_jobs",
			"package_sources",
			"provider_connections",
			"admin_tokens",
			"api_tokens",
			"org_members",
			"users",
			"versions",
			"packages",
		}
		for _, name := range tables {
			idx := fmt.Sprintf("idx_%s_public_id", name)
			if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", idx)); err != nil {
				return fmt.Errorf("drop index %s: %w", idx, err)
			}
			if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP COLUMN public_id", name)); err != nil {
				msg := strings.ToLower(err.Error())
				if !strings.Contains(msg, "no such column") && !strings.Contains(msg, "does not exist") {
					return fmt.Errorf("drop %s.public_id: %w", name, err)
				}
			}
		}
		return nil
	})
}

func addPublicIDColumn(ctx context.Context, db *bun.DB, table string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN public_id VARCHAR(32)", table))
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "exists") {
		return nil
	}
	return fmt.Errorf("add %s.public_id: %w", table, err)
}

func backfillPublicIDs(ctx context.Context, db *bun.DB, table, prefix string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s WHERE public_id IS NULL OR public_id = ''", table))
	if err != nil {
		return fmt.Errorf("scan %s for backfill: %w", table, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan %s row: %w", table, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close %s rows: %w", table, err)
	}

	for _, id := range ids {
		newID := pid.New(prefix)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET public_id = ? WHERE id = ?", table), newID, id); err != nil {
			return fmt.Errorf("backfill %s id=%d: %w", table, id, err)
		}
	}
	return nil
}

func createPublicIDIndex(ctx context.Context, db *bun.DB, table string) error {
	idx := fmt.Sprintf("idx_%s_public_id", table)
	_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (public_id)", idx, table))
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "exists") || strings.Contains(msg, "already") {
		return nil
	}
	return fmt.Errorf("create unique index on %s.public_id: %w", table, err)
}
