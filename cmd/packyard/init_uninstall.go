package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/usepackyard/packyard/internal/cli"
)

// runUninstall reverses what `packyard init` installed. It's
// conservative by default: stops/disables the service, removes the
// unit file, the binary, the env file, and the service user. Data dir
// (which contains the SQLite DB and package zips) is only removed
// when --purge-data is passed — a missed Ctrl-D on the "are you sure"
// prompt should never cost someone their packages.
func runUninstall(f *initFlags) int {
	paths := cli.DefaultPaths(cli.IsRootInstall())

	configFile := firstNonEmpty(f.configFile, paths.ConfigFile)
	dataDir := firstNonEmpty(f.dataDir, paths.DataDir)
	binary := paths.Binary

	fmt.Println()
	fmt.Println("Uninstalling Packyard")
	fmt.Println()

	// 1. Stop + disable service
	mgr := cli.DetectServiceManager()
	switch mgr {
	case cli.ServiceSystemd:
		_ = runQuiet("systemctl", "disable", "--now", "packyard.service")
		unitPath := "/etc/systemd/system/packyard.service"
		if err := os.Remove(unitPath); err == nil {
			fmt.Printf("  removed %s\n", unitPath)
		}
		_ = runQuiet("systemctl", "daemon-reload")
	case cli.ServiceLaunchd:
		home, _ := os.UserHomeDir()
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "dev.packyard.plist")
		_ = runQuiet("launchctl", "unload", plistPath)
		if err := os.Remove(plistPath); err == nil {
			fmt.Printf("  removed %s\n", plistPath)
		}
	}

	// 2. Remove binary + env file
	if err := os.Remove(binary); err == nil {
		fmt.Printf("  removed %s\n", binary)
	}
	if err := os.Remove(configFile); err == nil {
		fmt.Printf("  removed %s\n", configFile)
	}
	// Try to clean up an empty /etc/packyard/ dir; ignore if it has siblings.
	_ = os.Remove(filepath.Dir(configFile))

	// 3. Remove the system user (root install only)
	if cli.IsRootInstall() && mgr == cli.ServiceSystemd {
		if err := runQuiet("userdel", "packyard"); err == nil {
			fmt.Println("  removed system user `packyard`")
		}
	}

	// 4. Data dir — only with opt-in purge
	if f.purgeData {
		if err := os.RemoveAll(dataDir); err == nil {
			fmt.Printf("  removed %s (including DB + packages)\n", dataDir)
		}
	} else {
		fmt.Printf("  preserved %s — pass --purge-data to wipe\n", dataDir)
	}

	fmt.Println()
	fmt.Println("Done.")
	return 0
}

// runQuiet is like runSilent but also swallows stderr — used on
// uninstall paths where "not found" errors are expected (the thing we're
// trying to remove might already be gone).
func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
