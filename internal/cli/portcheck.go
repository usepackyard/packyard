package cli

import (
	"fmt"
	"net"
	"time"
)

// PortAvailable reports whether the given TCP port is bindable on
// 127.0.0.1. It attempts an actual `net.Listen` rather than parsing
// /proc/net/tcp or shelling out to `ss`/`lsof`: the bind is portable,
// authoritative, and immediately closed before returning.
//
// This is what the wizard uses to validate the user's chosen port.
// A "busy" result is advisory — the user can force-past it with
// `--force-port` if they know what they're doing (another process
// about to exit, reverse-proxy health probe that trips the check, etc.)
func PortAvailable(port int) (bool, error) {
	if port < 1 || port > 65535 {
		return false, fmt.Errorf("port %d out of range (1-65535)", port)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		// Bind failure is the expected "port busy" signal — don't
		// surface it as a hard error.
		return false, nil
	}
	_ = l.Close()

	// Linger briefly before retrying; on Linux SO_REUSEADDR makes
	// immediate rebind fine, but a subsequent bind by the server
	// itself needs the ephemeral state to settle on stricter OSes.
	time.Sleep(10 * time.Millisecond)
	return true, nil
}
