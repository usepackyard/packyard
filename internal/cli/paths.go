// Package cli holds helpers shared by the `init` wizard and the other
// subcommands that write config, probe services, or call out to the
// host OS. Nothing here should import the server packages; the split
// keeps the wizard's dependency graph tight.
package cli

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths groups the filesystem locations the installer writes to.
// `Root` is distinguished for printing and so callers can derive
// sibling paths without hard-coding.
type Paths struct {
	// Root is the install prefix. For system-wide installs this is
	// empty (each sub-path is absolute); for user installs it's the
	// user's home directory so rendered paths can be made relative
	// when printed.
	Root string

	// Binary is where the `packyard` executable lives.
	Binary string

	// ConfigFile is the env file loaded by the server at startup.
	ConfigFile string

	// DataDir holds the SQLite database (when used) and any other
	// per-install state. Also the parent of StoragePath by default.
	DataDir string

	// StoragePath is the default local-storage root for package zips.
	// Overridable by the wizard; the computed value here is used as
	// the prompt default.
	StoragePath string
}

// DefaultPaths returns the standard locations for a given install style.
// `root` selects the system-wide layout (/usr/local/bin, /etc, /var/lib);
// `!root` picks the XDG-ish user layout under $HOME. Callers that need
// to override individual fields should do so after the defaults are
// computed.
func DefaultPaths(root bool) Paths {
	if root {
		return Paths{
			Root:        "",
			Binary:      "/usr/local/bin/packyard",
			ConfigFile:  "/etc/packyard/packyard.env",
			DataDir:     "/var/lib/packyard",
			StoragePath: "/var/lib/packyard/packages",
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// On a box without a discoverable HOME we fall back to /tmp
		// — not great, but the install wizard will surface this via
		// the "install dir" prompt and the user can correct it.
		home = os.TempDir()
	}
	return Paths{
		Root:        home,
		Binary:      filepath.Join(home, ".local", "bin", "packyard"),
		ConfigFile:  filepath.Join(home, ".config", "packyard", "packyard.env"),
		DataDir:     filepath.Join(home, ".local", "share", "packyard"),
		StoragePath: filepath.Join(home, ".local", "share", "packyard", "packages"),
	}
}

// IsRootInstall reports whether the running process has enough
// privilege to perform a system-wide install. Root on Unix; the Windows
// check is a future concern (not targeted by v1).
func IsRootInstall() bool {
	if runtime.GOOS == "windows" {
		// Out of scope for v1 — surface as non-root to steer users
		// toward the user-home layout.
		return false
	}
	return os.Geteuid() == 0
}
