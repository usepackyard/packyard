package migrations

import "github.com/uptrace/bun/migrate"

// Migrations are registered by init() funcs in the NNN_*.go files in this
// package — see 001_initial.go. We don't call DiscoverCaller() because that
// walks the caller's source directory looking for .sql files, which don't
// exist at runtime inside the compiled binary's container image.
var Migrations = migrate.NewMigrations()
