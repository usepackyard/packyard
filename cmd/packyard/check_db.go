package main

import (
	"context"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database"
)

// runCheckDB opens a database connection using the same env-driven
// config the server uses and runs `SELECT 1`. Prints a human-friendly
// summary and exits 0 on success, 1 on any failure (connect, ping, or
// query). Used by `packyard init` to probe MySQL/Postgres credentials
// before writing them to the env file.
func runCheckDB(_ []string) int {
	cfg := config.Load()

	// Connect with a short overall deadline so a mistyped host doesn't
	// leave the wizard hanging on TCP timeouts.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := database.Open(cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-db: open %s: %v\n", cfg.DB.Driver, err)
		return 1
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "check-db: ping %s: %v\n", cfg.DB.Driver, err)
		return 1
	}

	var one int
	if err := db.NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
		fmt.Fprintf(os.Stderr, "check-db: select 1: %v\n", err)
		return 1
	}
	if one != 1 {
		fmt.Fprintf(os.Stderr, "check-db: unexpected result %d\n", one)
		return 1
	}

	switch cfg.DB.Driver {
	case "sqlite":
		fmt.Printf("ok — sqlite at %s\n", cfg.DB.Path)
	default:
		fmt.Printf("ok — %s://%s@%s:%d/%s\n",
			cfg.DB.Driver, cfg.DB.User, cfg.DB.Host, cfg.DB.Port, cfg.DB.Name)
	}
	return 0
}
