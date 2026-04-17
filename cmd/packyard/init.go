package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/cli"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

// installPlan is the fully-resolved configuration the installer is
// going to write. It's populated either by the huh form (interactive)
// or by flags + env (unattended) and then handed to the doInstall step.
type installPlan struct {
	// Paths
	ConfigFile string
	DataDir    string
	Binary     string // informational; the binary is already on $PATH by the time init runs

	// Server
	Mode    string
	Port    int
	BaseURL string

	// Database
	DBDriver   string
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
	DBPath     string

	// Storage
	StorageType     string
	StorageLocal    string
	S3Bucket        string
	S3Region        string
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string

	// Admin
	AdminEmail    string
	AdminPassword string // resolved before write; shown to user once
	AdminGenerated bool  // true if we generated the password (vs. user-supplied)

	// Misc
	SessionSecret string
	LogLevel      string

	// Service install
	InstallService bool
}

// initFlags is the CLI surface for `packyard init`. Every field has a
// matching env var so automation can drive the installer without
// flags.
type initFlags struct {
	unattended   bool
	help         bool
	uninstall    bool
	purgeData    bool

	configFile string
	dataDir    string

	mode    string
	port    int
	baseURL string
	forcePort bool

	db         string
	dbHost     string
	dbPort     int
	dbName     string
	dbUser     string
	dbPassword string
	dbSSLMode  string
	dbPath     string

	storage      string
	storagePath  string
	s3Bucket     string
	s3Region     string
	s3Endpoint   string
	s3AccessKey  string
	s3SecretKey  string

	adminEmail    string
	adminPassword string

	noService bool
}

func parseInitFlags(args []string) (*initFlags, error) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := &initFlags{}

	fs.BoolVar(&f.unattended, "unattended", envBool("PACKYARD_UNATTENDED"), "Skip interactive prompts; fail if required answers missing")
	fs.BoolVar(&f.uninstall, "uninstall", false, "Reverse an existing install (service, binary, config); preserves data by default")
	fs.BoolVar(&f.purgeData, "purge-data", false, "With --uninstall, also remove the data directory")
	fs.BoolVar(&f.help, "help", false, "Print this help and exit")

	fs.StringVar(&f.configFile, "config", envStr("PACKYARD_CONFIG_FILE"), "Path to the env file to write")
	fs.StringVar(&f.dataDir, "data-dir", envStr("PACKYARD_DATA_DIR"), "Data directory (SQLite DB, local storage)")

	fs.StringVar(&f.mode, "mode", envStrDefault("PACKYARD_MODE", "single"), "`single` or `multi`")
	fs.IntVar(&f.port, "port", envIntDefault("PACKYARD_PORT", 9090), "HTTP listen port")
	fs.StringVar(&f.baseURL, "url", envStr("PACKYARD_BASE_URL"), "Public URL (embedded into Composer dist URLs)")
	fs.BoolVar(&f.forcePort, "force-port", envBool("PACKYARD_FORCE_PORT"), "Skip the port-in-use check")

	fs.StringVar(&f.db, "db", envStrDefault("PACKYARD_DB_DRIVER", "sqlite"), "`sqlite`, `mysql`, or `postgres`")
	fs.StringVar(&f.dbHost, "db-host", envStr("PACKYARD_DB_HOST"), "Database host (mysql/postgres only)")
	fs.IntVar(&f.dbPort, "db-port", envIntDefault("PACKYARD_DB_PORT", 0), "Database port (mysql/postgres only; driver default if 0)")
	fs.StringVar(&f.dbName, "db-name", envStr("PACKYARD_DB_NAME"), "Database name")
	fs.StringVar(&f.dbUser, "db-user", envStr("PACKYARD_DB_USER"), "Database user")
	fs.StringVar(&f.dbPassword, "db-password", envStr("PACKYARD_DB_PASSWORD"), "Database password")
	fs.StringVar(&f.dbSSLMode, "db-sslmode", envStr("PACKYARD_DB_SSLMODE"), "Database SSL mode (driver-specific)")
	fs.StringVar(&f.dbPath, "db-path", envStr("PACKYARD_DB_PATH"), "SQLite file path")

	fs.StringVar(&f.storage, "storage", envStrDefault("PACKYARD_STORAGE_TYPE", "local"), "`local` or `s3`")
	fs.StringVar(&f.storagePath, "storage-path", envStr("PACKYARD_STORAGE_LOCAL_PATH"), "Local storage directory")
	fs.StringVar(&f.s3Bucket, "s3-bucket", envStr("PACKYARD_S3_BUCKET"), "S3 bucket name")
	fs.StringVar(&f.s3Region, "s3-region", envStr("PACKYARD_S3_REGION"), "S3 region")
	fs.StringVar(&f.s3Endpoint, "s3-endpoint", envStr("PACKYARD_S3_ENDPOINT"), "S3 endpoint (blank for AWS; set for R2/MinIO)")
	fs.StringVar(&f.s3AccessKey, "s3-access-key", envStr("PACKYARD_S3_ACCESS_KEY"), "S3 access key")
	fs.StringVar(&f.s3SecretKey, "s3-secret-key", envStr("PACKYARD_S3_SECRET_KEY"), "S3 secret key")

	fs.StringVar(&f.adminEmail, "admin-email", envStr("PACKYARD_ADMIN_EMAIL"), "Admin user email")
	fs.StringVar(&f.adminPassword, "admin-password", envStr("PACKYARD_ADMIN_PASSWORD"), "Admin password (auto-generated when omitted)")

	fs.BoolVar(&f.noService, "no-service", envBool("PACKYARD_NO_SERVICE"), "Skip systemd/launchd service install")

	fs.Usage = func() { printInitUsage(fs) }
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return f, nil
}

func printInitUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `Usage: packyard init [flags]

Interactive installer. When run from a terminal, walks you through
paths, database, port, public URL, storage, admin user, and optional
service install. Use --unattended with flags (or PACKYARD_* env vars)
for automation.

Flags:
`)
	fs.PrintDefaults()
}

// runInit is the `packyard init` entry point. Dispatches to the
// uninstaller when --uninstall is set, otherwise resolves an install
// plan (interactively or from flags) and applies it.
func runInit(args []string) int {
	f, err := parseInitFlags(args)
	if err != nil {
		return 2
	}
	if f.help {
		printInitUsage(flag.NewFlagSet("init", flag.ContinueOnError))
		return 0
	}
	if f.uninstall {
		return runUninstall(f)
	}

	plan, err := resolvePlan(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}

	return doInstall(plan)
}

// resolvePlan builds the final installPlan either from flags (unattended)
// or by running the huh form (interactive). The form fills in the plan
// in place so defaults from flags/env pre-populate the prompts.
func resolvePlan(f *initFlags) (*installPlan, error) {
	root := cli.IsRootInstall()
	paths := cli.DefaultPaths(root)

	plan := &installPlan{
		ConfigFile:   firstNonEmpty(f.configFile, paths.ConfigFile),
		DataDir:      firstNonEmpty(f.dataDir, paths.DataDir),
		Binary:       paths.Binary,
		Mode:         f.mode,
		Port:         f.port,
		BaseURL:      f.baseURL,
		DBDriver:     f.db,
		DBHost:       f.dbHost,
		DBPort:       f.dbPort,
		DBName:       f.dbName,
		DBUser:       f.dbUser,
		DBPassword:   f.dbPassword,
		DBSSLMode:    f.dbSSLMode,
		DBPath:       f.dbPath,
		StorageType: f.storage,
		// Default local-storage path tracks the (possibly overridden)
		// data dir so a user who sets --data-dir=/srv/pk doesn't end
		// up with packages under their home directory.
		StorageLocal: f.storagePath,
		S3Bucket:     f.s3Bucket,
		S3Region:     f.s3Region,
		S3Endpoint:   f.s3Endpoint,
		S3AccessKey:  f.s3AccessKey,
		S3SecretKey:  f.s3SecretKey,
		AdminEmail:   f.adminEmail,
		AdminPassword: f.adminPassword,
		LogLevel:     "info",
	}

	// If the user didn't set a SQLite path, land it inside the data dir.
	if plan.DBDriver == "sqlite" && plan.DBPath == "" {
		plan.DBPath = filepath.Join(plan.DataDir, "packyard.db")
	}
	// Same idea for local storage.
	if plan.StorageType == "local" && plan.StorageLocal == "" {
		plan.StorageLocal = filepath.Join(plan.DataDir, "packages")
	}

	// Default BaseURL tracks the port the user chose; wizard can override.
	if plan.BaseURL == "" {
		plan.BaseURL = fmt.Sprintf("http://localhost:%d", plan.Port)
	}

	// Populate sane MySQL/Postgres defaults so unattended flows only need
	// to supply non-default values.
	if plan.DBDriver == "mysql" {
		if plan.DBHost == "" {
			plan.DBHost = "127.0.0.1"
		}
		if plan.DBPort == 0 {
			plan.DBPort = 3306
		}
		if plan.DBSSLMode == "" {
			plan.DBSSLMode = "disable"
		}
	}
	if plan.DBDriver == "postgres" {
		if plan.DBHost == "" {
			plan.DBHost = "127.0.0.1"
		}
		if plan.DBPort == 0 {
			plan.DBPort = 5432
		}
		if plan.DBSSLMode == "" {
			plan.DBSSLMode = "prefer"
		}
	}

	if plan.DBName == "" && plan.DBDriver != "sqlite" {
		plan.DBName = "packyard"
	}
	if plan.DBUser == "" && plan.DBDriver != "sqlite" {
		plan.DBUser = "packyard"
	}

	// Service install defaults to yes when a service manager is
	// present, no when --no-service was passed or when none detected.
	plan.InstallService = !f.noService && cli.DetectServiceManager() != cli.ServiceNone

	// Secret is always generated — never prompt for it. Storing a
	// user-typed secret too often means they pick something weak.
	secret, err := cli.GenerateSessionSecret()
	if err != nil {
		return nil, err
	}
	plan.SessionSecret = secret

	if f.unattended {
		if err := validatePlan(plan); err != nil {
			return nil, err
		}
	} else {
		if err := runWizard(plan, f); err != nil {
			return nil, err
		}
	}

	// Port availability check is run at the very end, after the wizard
	// has settled on a final port, so the user doesn't get blocked if
	// they change the value mid-form.
	if !f.forcePort {
		free, err := cli.PortAvailable(plan.Port)
		if err != nil {
			return nil, err
		}
		if !free {
			return nil, fmt.Errorf("port %d is already in use (pass --force-port to override)", plan.Port)
		}
	}

	// If no admin password was set (unattended + no flag + no env), generate one.
	if plan.AdminPassword == "" {
		pw, err := cli.GeneratePassword()
		if err != nil {
			return nil, err
		}
		plan.AdminPassword = pw
		plan.AdminGenerated = true
	}
	if plan.AdminEmail == "" {
		plan.AdminEmail = "admin@example.com"
	}

	return plan, nil
}

func validatePlan(p *installPlan) error {
	if p.Mode != "single" && p.Mode != "multi" {
		return fmt.Errorf("invalid --mode %q (want single|multi)", p.Mode)
	}
	switch p.DBDriver {
	case "sqlite":
		if p.DBPath == "" {
			return errors.New("--db-path is required for sqlite")
		}
	case "mysql", "postgres":
		if p.DBPassword == "" {
			return fmt.Errorf("--db-password is required for %s", p.DBDriver)
		}
	default:
		return fmt.Errorf("invalid --db %q (want sqlite|mysql|postgres)", p.DBDriver)
	}
	switch p.StorageType {
	case "local":
		if p.StorageLocal == "" {
			return errors.New("--storage-path is required for local storage")
		}
	case "s3":
		if p.S3Bucket == "" || p.S3AccessKey == "" || p.S3SecretKey == "" {
			return errors.New("--s3-bucket, --s3-access-key, --s3-secret-key are all required for S3 storage")
		}
	default:
		return fmt.Errorf("invalid --storage %q (want local|s3)", p.StorageType)
	}
	if _, err := url.Parse(p.BaseURL); err != nil {
		return fmt.Errorf("invalid --url: %w", err)
	}
	return nil
}

// doInstall executes the resolved plan: creates directories, writes
// the env file, probes the DB, runs migrations, creates the admin
// user, installs the service unit, and starts it. Order matters —
// DB probe comes before writing the env file so credentials that fail
// don't end up persisted.
func doInstall(p *installPlan) int {
	fmt.Println()
	fmt.Println("Writing configuration…")

	// Directories first — SQLite needs its parent dir before the
	// probe, and creating dirs is cheap to roll back if something
	// fails downstream.
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "init: create data dir: %v\n", err)
		return 1
	}
	if p.StorageType == "local" {
		if err := os.MkdirAll(p.StorageLocal, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "init: create storage dir: %v\n", err)
			return 1
		}
	}

	// Probe the DB after dirs exist but before writing the env file.
	// A failing probe means the user's credentials are wrong; we'd
	// rather surface that now than persist them and have the service
	// fail to start later.
	if err := probeDatabase(p); err != nil {
		fmt.Fprintf(os.Stderr, "init: database check failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "init: nothing has been written to config. Fix the DB connection and retry.")
		return 1
	}

	if err := cli.WriteEnvFile(p.ConfigFile, planToEnv(p)); err != nil {
		fmt.Fprintf(os.Stderr, "init: write env file: %v\n", err)
		return 1
	}
	fmt.Printf("  wrote %s\n", p.ConfigFile)

	// Run migrations + create admin user via the in-process store,
	// so the first login works immediately without the server being up.
	if err := bootstrapDB(p); err != nil {
		fmt.Fprintf(os.Stderr, "init: bootstrap DB: %v\n", err)
		return 1
	}
	fmt.Println("  applied migrations")
	fmt.Printf("  created admin user %s\n", p.AdminEmail)

	// Service install is best-effort — the wizard already asked whether
	// to install and set InstallService accordingly; honor it.
	var serviceHint string
	if p.InstallService {
		if hint, err := installService(p); err != nil {
			fmt.Fprintf(os.Stderr, "init: service install failed (continuing): %v\n", err)
			serviceHint = manualStartHint(p)
		} else {
			serviceHint = hint
		}
	} else {
		serviceHint = manualStartHint(p)
	}

	printSuccessReport(p, serviceHint)
	return 0
}

// probeDatabase drives the check-db logic against the plan's DB
// config. For SQLite this creates the file if missing (no side effects
// since the file is part of the install anyway).
func probeDatabase(p *installPlan) error {
	dbcfg := config.DBConfig{
		Driver:   p.DBDriver,
		Host:     p.DBHost,
		Port:     p.DBPort,
		Name:     p.DBName,
		User:     p.DBUser,
		Password: p.DBPassword,
		SSLMode:  p.DBSSLMode,
		Path:     p.DBPath,
	}
	db, err := database.Open(dbcfg)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

// bootstrapDB runs migrations and seeds the admin user. Separate from
// the serve path so new installs don't need a running server to get
// an admin account.
func bootstrapDB(p *installPlan) error {
	dbcfg := config.DBConfig{
		Driver:   p.DBDriver,
		Host:     p.DBHost,
		Port:     p.DBPort,
		Name:     p.DBName,
		User:     p.DBUser,
		Password: p.DBPassword,
		SSLMode:  p.DBSSLMode,
		Path:     p.DBPath,
	}
	db, err := database.Open(dbcfg)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	stores := store.NewStores(db)
	ctx := context.Background()

	// Idempotency: if the email is already taken (re-run over an
	// existing install), leave the stored user alone.
	if existing, _ := stores.Users.GetByEmail(ctx, p.AdminEmail); existing != nil {
		return nil
	}
	hash, err := auth.HashPassword(p.AdminPassword, 12)
	if err != nil {
		return err
	}
	user := &model.User{
		Email:        p.AdminEmail,
		Password:     hash,
		Name:         localPartOf(p.AdminEmail),
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := stores.Users.Create(ctx, user); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	// Provision a default org so the admin has somewhere to land on
	// first login, matching the behavior of server-side seedDefaults.
	org := &model.Organization{Slug: "default", Name: "Default"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		return fmt.Errorf("create default org: %w", err)
	}
	member := &model.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}
	if err := stores.Orgs.AddMember(ctx, member); err != nil {
		return fmt.Errorf("add admin to default org: %w", err)
	}
	return nil
}

// installService writes the unit/plist, reloads the daemon, and
// enables+starts the service. Returns a one-line "how to check"
// message the caller prints at the end.
func installService(p *installPlan) (string, error) {
	mgr := cli.DetectServiceManager()
	switch mgr {
	case cli.ServiceSystemd:
		serviceUser, serviceGroup, err := ensureSystemUser("packyard")
		if err != nil {
			return "", fmt.Errorf("create service user: %w", err)
		}
		if err := os.Chown(p.DataDir, serviceUser, serviceGroup); err != nil {
			return "", fmt.Errorf("chown data dir: %w", err)
		}
		if err := os.Chown(p.ConfigFile, serviceUser, serviceGroup); err != nil {
			return "", fmt.Errorf("chown config: %w", err)
		}
		unitPath, err := cli.WriteSystemdUnit(cli.SystemdUnit{
			BinaryPath: p.Binary,
			EnvFile:    p.ConfigFile,
			DataDir:    p.DataDir,
			User:       "packyard",
			Group:      "packyard",
		})
		if err != nil {
			return "", err
		}
		if err := runSilent("systemctl", "daemon-reload"); err != nil {
			return "", fmt.Errorf("systemctl daemon-reload: %w", err)
		}
		if err := runSilent("systemctl", "enable", "--now", "packyard.service"); err != nil {
			return "", fmt.Errorf("systemctl enable --now: %w", err)
		}
		fmt.Printf("  installed systemd unit at %s\n", unitPath)
		return "systemctl status packyard", nil

	case cli.ServiceLaunchd:
		// LaunchAgent in the current user's ~/Library so it starts
		// when the user logs in. System-wide LaunchDaemons require
		// root + more plumbing and are out of scope for v1 on macOS.
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "dev.packyard.plist")
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return "", err
		}
		logDir := filepath.Join(p.DataDir, "logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return "", err
		}
		content := cli.LaunchdPlistContent(cli.LaunchdPlist{
			BinaryPath: p.Binary,
			EnvFile:    p.ConfigFile,
			Label:      "dev.packyard",
			LogDir:     logDir,
			EnvVars:    planToEnv(p),
		})
		if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
			return "", err
		}
		if err := runSilent("launchctl", "load", plistPath); err != nil {
			return "", fmt.Errorf("launchctl load: %w", err)
		}
		fmt.Printf("  installed launchd plist at %s\n", plistPath)
		return "launchctl list | grep packyard", nil

	default:
		return manualStartHint(p), nil
	}
}

// ensureSystemUser creates a `name` system user if missing, returning
// its uid and gid. Used only on Linux (systemd path); uses `useradd`
// with `--system --no-create-home`.
func ensureSystemUser(name string) (int, int, error) {
	u, err := user.Lookup(name)
	if err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return uid, gid, nil
	}
	// Not found; create.
	if err := runSilent("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", name); err != nil {
		return 0, 0, err
	}
	u, err = user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid, nil
}

func manualStartHint(p *installPlan) string {
	return fmt.Sprintf("EnvironmentFile=%s %s serve", p.ConfigFile, p.Binary)
}

func printSuccessReport(p *installPlan, serviceHint string) {
	fmt.Println()
	fmt.Println("Packyard is ready.")
	fmt.Println()
	fmt.Printf("  URL:      %s\n", p.BaseURL)
	fmt.Printf("  Email:    %s\n", p.AdminEmail)
	if p.AdminGenerated {
		fmt.Printf("  Password: %s  (save this — it will not be shown again)\n", p.AdminPassword)
	} else {
		fmt.Printf("  Password: (the one you provided)\n")
	}
	fmt.Println()
	fmt.Printf("  Config:     %s\n", p.ConfigFile)
	fmt.Printf("  Data dir:   %s\n", p.DataDir)
	fmt.Println()
	if p.InstallService {
		fmt.Printf("  Check:      %s\n", serviceHint)
	} else {
		fmt.Printf("  Start:      %s\n", serviceHint)
	}
	fmt.Printf("  Uninstall:  packyard init --uninstall\n")
	fmt.Println()
	if !strings.HasPrefix(p.BaseURL, "https://") &&
		!strings.Contains(p.BaseURL, "localhost") &&
		!strings.Contains(p.BaseURL, "127.0.0.1") {
		fmt.Println("  Heads-up: PACKYARD_BASE_URL isn't HTTPS and isn't local. Composer")
		fmt.Println("            over plain HTTP leaks credentials — front this with a TLS")
		fmt.Println("            reverse proxy (Caddy/nginx/Traefik) before anyone uses it.")
		fmt.Println()
	}
}

// planToEnv renders the plan into a map of KEY=VALUE for the env file.
// Separate helper because the systemd and launchd paths both need it.
func planToEnv(p *installPlan) map[string]string {
	m := map[string]string{
		"PACKYARD_MODE":              p.Mode,
		"PACKYARD_PORT":              strconv.Itoa(p.Port),
		"PACKYARD_BASE_URL":          p.BaseURL,
		"PACKYARD_SESSION_SECRET":    p.SessionSecret,
		"PACKYARD_LOG_LEVEL":         p.LogLevel,
		"PACKYARD_DB_DRIVER":         p.DBDriver,
		"PACKYARD_ADMIN_EMAIL":       p.AdminEmail,
		"PACKYARD_ADMIN_PASSWORD":    p.AdminPassword,
		"PACKYARD_STORAGE_TYPE":      p.StorageType,
	}
	switch p.DBDriver {
	case "sqlite":
		m["PACKYARD_DB_PATH"] = p.DBPath
	case "mysql", "postgres":
		m["PACKYARD_DB_HOST"] = p.DBHost
		m["PACKYARD_DB_PORT"] = strconv.Itoa(p.DBPort)
		m["PACKYARD_DB_NAME"] = p.DBName
		m["PACKYARD_DB_USER"] = p.DBUser
		m["PACKYARD_DB_PASSWORD"] = p.DBPassword
		m["PACKYARD_DB_SSLMODE"] = p.DBSSLMode
	}
	switch p.StorageType {
	case "local":
		m["PACKYARD_STORAGE_LOCAL_PATH"] = p.StorageLocal
	case "s3":
		m["PACKYARD_S3_BUCKET"] = p.S3Bucket
		m["PACKYARD_S3_REGION"] = p.S3Region
		if p.S3Endpoint != "" {
			m["PACKYARD_S3_ENDPOINT"] = p.S3Endpoint
		}
		m["PACKYARD_S3_ACCESS_KEY"] = p.S3AccessKey
		m["PACKYARD_S3_SECRET_KEY"] = p.S3SecretKey
	}
	return m
}

// runSilent runs a command, swallowing stdout but bubbling up stderr on
// failure so real errors surface to the user.
func runSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func localPartOf(email string) string {
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i]
	}
	return email
}

// --- env helpers shared by parseInitFlags ---

func envStr(key string) string {
	return os.Getenv(key)
}

func envStrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string) int {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func envIntDefault(key string, def int) int {
	if n := envInt(key); n > 0 {
		return n
	}
	return def
}

func envBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "1" || v == "true" || v == "yes"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
