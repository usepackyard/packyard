package main

import (
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database"
)

// runMigrate runs pending database migrations and exits. Designed for
// use as a k8s init container: the init pod runs `packyard migrate`
// against the shared DB, exits 0, then the main container starts. bun's
// migration table records applied groups, so subsequent in-process
// Migrate() calls at server startup become no-ops.
//
// Run twice in a row → second call prints "no new migrations to run"
// and still exits 0.
func runMigrate(_ []string) int {
	cfg := config.Load()
	setupLogger(cfg.Log.Level)

	db, err := database.Open(cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: open database: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 1
	}
	return 0
}
