package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database/migrations"
)

func Open(cfg config.DBConfig) (*bun.DB, error) {
	var sqldb *sql.DB
	var err error

	switch cfg.Driver {
	case "mysql":
		sqldb, err = sql.Open("mysql", cfg.DSN())
	case "postgres":
		sqldb, err = sql.Open("pgx", cfg.DSN())
	case "sqlite":
		sqldb, err = sql.Open("sqlite", cfg.DSN())
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.Driver == "sqlite" {
		if _, err := sqldb.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return nil, fmt.Errorf("enable WAL mode: %w", err)
		}
	}

	if err := sqldb.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqldb.SetMaxOpenConns(25)
	sqldb.SetMaxIdleConns(5)

	var db *bun.DB
	switch cfg.Driver {
	case "mysql":
		db = bun.NewDB(sqldb, mysqldialect.New())
	case "postgres":
		db = bun.NewDB(sqldb, pgdialect.New())
	case "sqlite":
		db = bun.NewDB(sqldb, sqlitedialect.New())
	}

	return db, nil
}

func Migrate(db *bun.DB) error {
	migrator := migrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(context.Background()); err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	group, err := migrator.Migrate(context.Background())
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if group.IsZero() {
		slog.Info("no new migrations to run")
	} else {
		slog.Info("migrations applied", "group", group)
	}
	return nil
}
