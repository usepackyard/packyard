// Package testutil provides shared helpers for tests across the codebase.
//
// Don't import this package from non-test code. The Go convention is that
// `_test.go` files can import any internal/* package; testutil is one of them.
package testutil

import (
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/database"
)

// NewDB returns an in-memory SQLite database with all migrations applied.
// The DB is closed automatically when the test completes.
//
// Each call returns a fresh, isolated database — tests don't share state.
func NewDB(t *testing.T) *bun.DB {
	t.Helper()

	// shared cache + a unique per-test name keeps the in-memory DB alive
	// across the multiple connections in Bun's pool.
	dsn := "file::memory:?cache=shared&_pragma=foreign_keys(1)"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Single connection — avoids cross-connection visibility issues with
	// :memory: even with shared cache.
	sqldb.SetMaxOpenConns(1)

	if err := sqldb.Ping(); err != nil {
		t.Fatalf("ping sqlite: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
