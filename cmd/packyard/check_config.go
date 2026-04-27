package main

import (
	"fmt"
	"os"

	"github.com/usepackyard/packyard/internal/config"
)

// runCheckConfig loads env into a Config and runs the server's own
// Validate() without starting anything. Useful in CI and in install
// wizards to fail fast on misconfiguration (e.g., session secret too
// short) without waiting for a server boot + crash.
//
// Exit codes:
//
//	0 — config is valid
//	1 — config failed validation (human-readable reason printed to stderr)
func runCheckConfig(_ []string) int {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "check-config: %v\n", err)
		return 1
	}
	fmt.Printf("ok — port=%d db=%s storage=%s\n",
		cfg.Port, cfg.DB.Driver, cfg.Storage.Type)
	return 0
}
