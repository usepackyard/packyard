// Command packyard is the server binary plus a small toolbox of ops
// subcommands (healthcheck, migrate, check-db, admin user create, …).
//
// Bare `packyard` with no arguments continues to start the HTTP server,
// same as before subcommands existed — so existing Dockerfile ENTRYPOINTs,
// systemd units, and k8s manifests keep working untouched.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// version is overridden at build time via -ldflags "-X main.version=<sha>".
// Dev builds ship with "dev". Read by the `version` subcommand.
var version = "dev"

type commandFunc func(args []string) int

type command struct {
	run  commandFunc
	help string
}

var commands = map[string]command{
	"serve":        {run: runServe, help: "Run the HTTP server (default when no subcommand is given)"},
	"init":         {run: runInit, help: "Interactive installer: configure paths, DB, port, URL, admin; write env file; install service"},
	"version":      {run: runVersion, help: "Print version, commit, and build metadata"},
	"healthcheck":  {run: runHealthcheck, help: "Check /healthz on a local or given URL; exits 0 on 200"},
	"check-config": {run: runCheckConfig, help: "Load + validate environment configuration; no side effects"},
	"check-db":     {run: runCheckDB, help: "Open the configured database and run SELECT 1"},
	"migrate":      {run: runMigrate, help: "Run pending database migrations (idempotent)"},
	"admin":        {run: runAdmin, help: "Admin operations (currently: `admin user create`)"},
}

func main() {
	args := os.Args[1:]

	// No arguments → start the server. Preserves back-compat with every
	// existing deployment that invokes the binary as just `packyard`.
	if len(args) == 0 {
		os.Exit(runServe(nil))
	}

	// A flag as the first arg (e.g. `packyard -foo`) is treated as a flag
	// to `serve`, not a mistyped subcommand. Again for back-compat with
	// operators who might have passed server-level flags directly.
	if strings.HasPrefix(args[0], "-") {
		if args[0] == "-h" || args[0] == "--help" || args[0] == "-help" {
			printUsage(os.Stdout)
			os.Exit(0)
		}
		os.Exit(runServe(args))
	}

	if args[0] == "help" {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "packyard: unknown command %q\n\n", args[0])
		printUsage(os.Stderr)
		os.Exit(2)
	}
	os.Exit(cmd.run(args[1:]))
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: packyard [command] [args]\n\nCommands:\n")
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "  %-14s %s\n", name, commands[name].help)
	}
	fmt.Fprintf(w, "\nRunning `packyard` with no arguments is equivalent to `packyard serve`.\n")
}
