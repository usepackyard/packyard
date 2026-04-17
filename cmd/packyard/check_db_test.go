package main

import (
	"testing"
)

// TestRunCheckDB_SQLiteSucceeds spins up an in-memory SQLite config
// and confirms `packyard check-db` exits 0 against it.
func TestRunCheckDB_SQLiteSucceeds(t *testing.T) {
	tmp := t.TempDir() + "/test.db"
	t.Setenv("PACKYARD_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("PACKYARD_DB_DRIVER", "sqlite")
	t.Setenv("PACKYARD_DB_PATH", tmp)

	code := runCheckDB(nil)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

// TestRunCheckConfig_ValidEnvSucceeds confirms the check-config
// subcommand exits 0 when required env is present.
func TestRunCheckConfig_ValidEnvSucceeds(t *testing.T) {
	t.Setenv("PACKYARD_SESSION_SECRET", "0123456789abcdef0123456789abcdef")

	code := runCheckConfig(nil)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

// TestRunCheckConfig_ShortSecretFails confirms the session-secret
// minimum-length rule (load-bearing for CSRF defenses) is enforced.
func TestRunCheckConfig_ShortSecretFails(t *testing.T) {
	t.Setenv("PACKYARD_SESSION_SECRET", "too-short")

	code := runCheckConfig(nil)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// TestRunMigrate_Idempotent confirms migrate → migrate is a no-op.
func TestRunMigrate_Idempotent(t *testing.T) {
	tmp := t.TempDir() + "/test.db"
	t.Setenv("PACKYARD_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("PACKYARD_DB_DRIVER", "sqlite")
	t.Setenv("PACKYARD_DB_PATH", tmp)

	if code := runMigrate(nil); code != 0 {
		t.Fatalf("first migrate exit = %d, want 0", code)
	}
	if code := runMigrate(nil); code != 0 {
		t.Fatalf("second migrate exit = %d, want 0 (idempotent)", code)
	}
}
