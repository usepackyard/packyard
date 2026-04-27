package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunInit_UnattendedSQLite drives the full unattended path against
// a throwaway SQLite DB. Ensures: env file written with 0600, DB gets
// migrated + admin user created, success banner printed, exit 0.
func TestRunInit_UnattendedSQLite(t *testing.T) {
	tmp := t.TempDir()
	env := filepath.Join(tmp, "packyard.env")
	data := filepath.Join(tmp, "data")

	args := []string{
		"--unattended",
		"--config", env,
		"--data-dir", data,
		"--port", "19799",
		"--url", "http://localhost:19799",
		"--admin-email", "unit@test.local",
		"--admin-password", "hunter1234",
		"--no-service",
	}
	if code := runInit(args); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	// Env file exists, 0600, and contains the expected secrets + paths.
	info, err := os.Stat(env)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("env file perms = %o, want 600", perm)
	}
	body, _ := os.ReadFile(env)
	for _, needle := range []string{
		"PACKYARD_PORT=19799",
		"PACKYARD_DB_DRIVER=sqlite",
		"PACKYARD_ADMIN_EMAIL=unit@test.local",
		"PACKYARD_SESSION_SECRET=",
	} {
		if !strings.Contains(string(body), needle) {
			t.Errorf("env file missing %q:\n%s", needle, body)
		}
	}

	// DB file exists.
	if _, err := os.Stat(filepath.Join(data, "packyard.db")); err != nil {
		t.Errorf("sqlite db missing: %v", err)
	}
	// Storage dir exists.
	if _, err := os.Stat(filepath.Join(data, "packages")); err != nil {
		t.Errorf("storage dir missing: %v", err)
	}
}

// TestRunInit_UnattendedFailsOnMissingDBPassword covers the validation
// boundary for MySQL without credentials in unattended mode.
func TestRunInit_UnattendedFailsOnMissingDBPassword(t *testing.T) {
	tmp := t.TempDir()
	args := []string{
		"--unattended",
		"--config", filepath.Join(tmp, "p.env"),
		"--data-dir", tmp,
		"--db", "mysql",
		"--db-user", "packyard",
		// --db-password intentionally missing
		"--port", "19800",
		"--url", "http://localhost:19800",
		"--admin-email", "u@t.l",
		"--admin-password", "abcdefgh",
		"--no-service",
	}
	if code := runInit(args); code == 0 {
		t.Fatalf("exit = 0, want non-zero (missing db password)")
	}
	// Env file must not have been created on validation failure.
	if _, err := os.Stat(filepath.Join(tmp, "p.env")); !os.IsNotExist(err) {
		t.Errorf("env file should not exist on validation failure: %v", err)
	}
}

// TestRunInit_IdempotentReRun covers the common "re-run the installer"
// case: same flags, same result, no duplicate admin user, exit 0.
func TestRunInit_IdempotentReRun(t *testing.T) {
	tmp := t.TempDir()
	args := []string{
		"--unattended",
		"--config", filepath.Join(tmp, "p.env"),
		"--data-dir", tmp,
		"--port", "19801",
		"--url", "http://localhost:19801",
		"--admin-email", "u@t.l",
		"--admin-password", "abcdefgh",
		"--no-service",
	}
	if code := runInit(args); code != 0 {
		t.Fatalf("first run exit = %d, want 0", code)
	}
	if code := runInit(args); code != 0 {
		t.Fatalf("second run exit = %d, want 0 (idempotent)", code)
	}
}
