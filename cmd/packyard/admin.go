package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

// runAdmin dispatches subcommands under `packyard admin`. v1 only ships
// `admin user create` — enough for install wizards, init containers, and
// break-glass recovery when the dashboard is locked out. More admin
// subcommands (token create, org create, user reset-password) land later.
func runAdmin(args []string) int {
	if len(args) == 0 {
		printAdminUsage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "user":
		return runAdminUser(args[1:])
	case "-h", "--help", "help":
		printAdminUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "admin: unknown subcommand %q\n\n", args[0])
		printAdminUsage(os.Stderr)
		return 2
	}
}

func runAdminUser(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: packyard admin user create ...\n")
		return 2
	}
	switch args[0] {
	case "create":
		return runAdminUserCreate(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "admin user: unknown subcommand %q\n", args[0])
		return 2
	}
}

// runAdminUserCreate creates (or fails loudly about) a new Packyard
// user. Reads config from env; talks directly to the DB via UserStore —
// no HTTP round-trip, so this works even when the server isn't running
// (useful for install + recovery scenarios).
//
//	packyard admin user create --email user@example.com --password SECRET
//	packyard admin user create --email user@example.com --super-admin
//	PACKYARD_ADMIN_PASSWORD=SECRET packyard admin user create --email ...
//
// Exits 0 on success, 1 on any DB/hash error, 2 on flag/validation errors,
// 3 when the email is already taken (distinct so callers can branch on it).
func runAdminUserCreate(args []string) int {
	fs := flag.NewFlagSet("admin user create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	email := fs.String("email", "", "Email address (required)")
	password := fs.String("password", "", "Password (or set PACKYARD_ADMIN_PASSWORD)")
	name := fs.String("name", "", "Display name (defaults to the email's local part)")
	superAdmin := fs.Bool("super-admin", false, "Grant super-admin privileges")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: packyard admin user create --email EMAIL [--password PASS] [--name NAME] [--super-admin]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	*email = strings.TrimSpace(*email)
	if *email == "" {
		fmt.Fprintln(os.Stderr, "admin user create: --email is required")
		return 2
	}

	pw := *password
	if pw == "" {
		pw = os.Getenv("PACKYARD_ADMIN_PASSWORD")
	}
	if pw == "" {
		fmt.Fprintln(os.Stderr, "admin user create: --password or PACKYARD_ADMIN_PASSWORD is required")
		return 2
	}

	displayName := *name
	if displayName == "" {
		if i := strings.IndexByte(*email, '@'); i > 0 {
			displayName = (*email)[:i]
		} else {
			displayName = *email
		}
	}

	cfg := config.Load()
	setupLogger(cfg.Log.Level)

	db, err := database.Open(cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "admin user create: open database: %v\n", err)
		return 1
	}
	defer db.Close()

	// Running migrations is load-bearing here: on a fresh DB the users
	// table may not exist yet (install wizard calls this right after
	// writing the env file, before starting the server).
	if err := database.Migrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "admin user create: migrate: %v\n", err)
		return 1
	}

	stores := store.NewStores(db)
	ctx := context.Background()

	existing, err := stores.Users.GetByEmail(ctx, *email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "admin user create: lookup: %v\n", err)
		return 1
	}
	if existing != nil {
		fmt.Fprintf(os.Stderr, "admin user create: a user with email %q already exists\n", *email)
		return 3
	}

	hash, err := auth.HashPassword(pw, cfg.BcryptCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "admin user create: hash password: %v\n", err)
		return 1
	}

	user := &model.User{
		Email:        *email,
		Password:     hash,
		Name:         displayName,
		IsActive:     true,
		IsSuperAdmin: *superAdmin,
	}
	if err := stores.Users.Create(ctx, user); err != nil {
		// Defense against a race with a concurrent insert: the UNIQUE
		// index on email will reject the second one. Map that to the
		// same exit code as the pre-check for consistency.
		if isUniqueConstraintError(err) {
			fmt.Fprintf(os.Stderr, "admin user create: a user with email %q already exists\n", *email)
			return 3
		}
		fmt.Fprintf(os.Stderr, "admin user create: %v\n", err)
		return 1
	}

	if *superAdmin {
		fmt.Printf("created super-admin %s (id=%s)\n", user.Email, user.PublicID)
	} else {
		fmt.Printf("created user %s (id=%s)\n", user.Email, user.PublicID)
	}
	return 0
}

// isUniqueConstraintError heuristic-matches bun's unique-violation errors
// across drivers. Matches on substrings because the typed errors differ
// per driver and bun doesn't normalize them.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var target interface{ Error() string }
	if !errors.As(err, &target) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "constraint failed")
}

func printAdminUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: packyard admin <command>\n\nCommands:\n")
	fmt.Fprintf(w, "  user create    Create a new Packyard user\n")
	fmt.Fprintf(w, "\nMore admin subcommands will be added in subsequent releases.\n")
}
