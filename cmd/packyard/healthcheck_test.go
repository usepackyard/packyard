package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheck_Returns0When200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	code := runHealthcheck([]string{"--url", srv.URL + "/healthz", "--timeout", "500ms"})
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

func TestHealthcheck_ReturnsNonZeroWhenNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	code := runHealthcheck([]string{"--url", srv.URL + "/healthz", "--timeout", "500ms"})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestHealthcheck_ReturnsNonZeroWhenServerDown(t *testing.T) {
	// Port 1 is reliably not listening in a normal container.
	code := runHealthcheck([]string{"--url", "http://127.0.0.1:1/healthz", "--timeout", "200ms"})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestHealthcheck_DefaultsToEnvPort(t *testing.T) {
	// Spin up a server, point PACKYARD_PORT at it, call with no --url
	// and make sure it picks the port up from the environment.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// httptest.NewUnstartedServer picks an ephemeral port; we need the
	// port number before starting to stuff into PACKYARD_PORT.
	if err := srv.Listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	// Actually the simplest way: use NewServer then extract the port.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()

	var port int
	if _, err := fmt.Sscanf(srv2.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse URL %q: %v", srv2.URL, err)
	}
	t.Setenv("PACKYARD_PORT", fmt.Sprintf("%d", port))

	code := runHealthcheck([]string{"--timeout", "500ms"})
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}
