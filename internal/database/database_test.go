package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/database"
)

// openTestDB opens an in-memory SQLite suitable for migration testing.
// We don't use testutil.NewDB here because that helper runs migrations
// for us — this test file needs to control the migrator directly.
func openTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrate_AppliesCleanly(t *testing.T) {
	db := openTestDB(t)

	if err := database.Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	// Every table we expect should exist.
	expected := []string{
		"organizations", "users", "org_members", "sessions", "sso_tickets",
		"packages", "versions", "api_tokens", "package_sources",
	}
	for _, table := range expected {
		var count int
		err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %q missing", table)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := openTestDB(t)

	if err := database.Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	// Second call should not error and should be a no-op.
	if err := database.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Sanity: we can still insert into users.
	_, err := db.ExecContext(context.Background(),
		"INSERT INTO users (email, password, name, is_active) VALUES (?, ?, ?, ?)",
		"test@example.com", "hash", "Test", true)
	if err != nil {
		t.Fatalf("insert after re-migrate: %v", err)
	}
}
