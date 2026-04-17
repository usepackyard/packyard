//go:build windows

package cli

// umask is a no-op on Windows. Included so the package builds on
// every target the stdlib supports; v1 doesn't actually run on Windows.
func umask(_ int) int { return 0 }
