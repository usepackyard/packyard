package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ServiceManager identifies which init system is available on the
// host. The installer uses this to pick which unit file to write and
// which tool to hand off to.
type ServiceManager string

const (
	ServiceSystemd ServiceManager = "systemd"
	ServiceLaunchd ServiceManager = "launchd"
	ServiceNone    ServiceManager = "none"
)

// DetectServiceManager returns the best-match init system, or ServiceNone
// if the wizard should fall back to "just print the start command".
// The detection is conservative: we look for the CLI tool, not just OS
// family, so a Linux box without systemctl (Alpine without openrc-plus,
// containers, some bootstraps) correctly reports None.
func DetectServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err == nil {
			return ServiceSystemd
		}
	case "darwin":
		if _, err := exec.LookPath("launchctl"); err == nil {
			return ServiceLaunchd
		}
	}
	return ServiceNone
}

// SystemdUnit holds the values templated into the service file.
type SystemdUnit struct {
	BinaryPath string // absolute, matches Paths.Binary
	EnvFile    string // absolute, matches Paths.ConfigFile
	DataDir    string // absolute; listed under ReadWritePaths
	User       string // service user (typically "packyard" for root installs)
	Group      string // service group
}

// SystemdUnitContent renders the unit file. Hardening flags included:
// NoNewPrivileges prevents setuid/file-caps promotion, ProtectSystem=strict
// makes /usr read-only to the service, ProtectHome=true hides other users'
// homes, ReadWritePaths narrows the write surface to the data dir, and
// AmbientCapabilities=CAP_NET_BIND_SERVICE lets the non-root user bind
// low ports if the operator picked one (common for direct-to-port
// deployments without a reverse proxy in front).
func SystemdUnitContent(u SystemdUnit) string {
	return fmt.Sprintf(`[Unit]
Description=Packyard private Composer registry
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
EnvironmentFile=%s
ExecStart=%s serve
Restart=on-failure
RestartSec=5s
# Hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=%s
# Allow the non-root user to bind low ports (for installs that skip a
# reverse proxy and expose 80/443 directly).
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`, u.User, u.Group, u.EnvFile, u.BinaryPath, u.DataDir)
}

// WriteSystemdUnit drops the unit at the standard path (requires root).
// Returns the path it wrote to so the caller can print it or clean it
// up on uninstall.
func WriteSystemdUnit(u SystemdUnit) (string, error) {
	path := "/etc/systemd/system/packyard.service"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create systemd dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(SystemdUnitContent(u)), 0o644); err != nil {
		return "", fmt.Errorf("write unit file: %w", err)
	}
	return path, nil
}

// LaunchdPlist holds the values templated into the plist file.
type LaunchdPlist struct {
	BinaryPath string
	EnvFile    string // unused by launchd directly, mentioned in Disabled comment
	Label      string // reverse-DNS: dev.packyard
	LogDir     string // StandardOut/ErrorPath go under here
	EnvVars    map[string]string
}

// LaunchdPlistContent renders a LaunchAgent that mirrors the systemd
// unit: starts the server, restarts on crash, captures stdout/stderr to
// files under LogDir. macOS launchd doesn't support env-file loading,
// so callers pass EnvVars explicitly from the parsed env file.
func LaunchdPlistContent(p LaunchdPlist) string {
	var envXML string
	for k, v := range p.EnvVars {
		envXML += fmt.Sprintf("\n    <key>%s</key><string>%s</string>", xmlEscape(k), xmlEscape(v))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict><key>SuccessfulExit</key><false/></dict>
  <key>StandardOutPath</key><string>%s/packyard.out.log</string>
  <key>StandardErrorPath</key><string>%s/packyard.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>%s
  </dict>
</dict>
</plist>
`, p.Label, p.BinaryPath, p.LogDir, p.LogDir, envXML)
}

func xmlEscape(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch r {
		case '<':
			out = append(out, []byte("&lt;")...)
		case '>':
			out = append(out, []byte("&gt;")...)
		case '&':
			out = append(out, []byte("&amp;")...)
		case '"':
			out = append(out, []byte("&quot;")...)
		default:
			out = append(out, []byte(string(r))...)
		}
	}
	return string(out)
}
