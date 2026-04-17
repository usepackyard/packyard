package main

import (
	"os"
	"testing"
)

// TestRunAdminUserCreate_MissingEmail covers the CLI-layer validation
// shape. Full DB round-trips live in TestAdminUserStore_Create in the
// store package — here we just want to confirm the command surface
// behaves sensibly when flags are missing.
func TestRunAdminUserCreate_MissingEmail(t *testing.T) {
	code := runAdminUserCreate([]string{"--password", "secret12345"})
	if code != 2 {
		t.Errorf("exit = %d, want 2 (validation error)", code)
	}
}

func TestRunAdminUserCreate_MissingPassword(t *testing.T) {
	// Clear any inherited PACKYARD_ADMIN_PASSWORD from the test env so
	// this subcase genuinely tests the "no password" path.
	t.Setenv("PACKYARD_ADMIN_PASSWORD", "")
	code := runAdminUserCreate([]string{"--email", "x@y.z"})
	if code != 2 {
		t.Errorf("exit = %d, want 2 (validation error)", code)
	}
}

func TestRunAdmin_UnknownSubcommand(t *testing.T) {
	code := runAdmin([]string{"robots", "rule"})
	if code != 2 {
		t.Errorf("exit = %d, want 2 (unknown subcommand)", code)
	}
}

func TestRunAdmin_NoArgs(t *testing.T) {
	code := runAdmin(nil)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestIsUniqueConstraintError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic error", os.ErrNotExist, false},
		{"sqlite unique", errString("UNIQUE constraint failed: users.email"), true},
		{"mysql duplicate", errString("Error 1062: Duplicate entry 'x@y.z' for key 'users.email'"), true},
		{"postgres constraint", errString("ERROR: duplicate key value violates unique constraint"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isUniqueConstraintError(c.err)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
