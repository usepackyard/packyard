package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// runHealthcheck hits /healthz and exits 0 on 200, 1 otherwise. Designed
// for Dockerfile HEALTHCHECK and systemd ExecStartPost / ExecReload
// hooks: zero deps beyond the binary itself, works in minimal images
// without wget/curl/nc.
//
// Default URL: http://127.0.0.1:$PACKYARD_PORT/healthz (falls back to
// :8080 when the env var isn't set, matching the server's default port).
// Override with --url for probes against a different host/scheme.
func runHealthcheck(args []string) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	url := fs.String("url", "", "Full healthz URL (overrides the default http://127.0.0.1:$PACKYARD_PORT/healthz)")
	timeout := fs.Duration("timeout", 3*time.Second, "Request timeout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: packyard healthcheck [--url URL] [--timeout DUR]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	target := *url
	if target == "" {
		port := 8080
		if v := os.Getenv("PACKYARD_PORT"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				port = n
			}
		}
		target = fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: invalid URL %q: %v\n", target, err)
		return 2
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	// Drain the body so keep-alive connections can be reused (though we
	// exit right away so this is mostly hygienic).
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
