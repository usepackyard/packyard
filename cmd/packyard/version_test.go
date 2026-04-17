package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestRunVersion_Short asserts the --short flag prints only the version
// string, so scripts can do `VER=$(packyard version --short)`.
func TestRunVersion_Short(t *testing.T) {
	got := captureStdout(t, func() {
		if code := runVersion([]string{"--short"}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
	})

	got = strings.TrimSpace(got)
	if got != version {
		t.Errorf("stdout = %q, want %q", got, version)
	}
}

func TestRunVersion_Long(t *testing.T) {
	got := captureStdout(t, func() {
		if code := runVersion(nil); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
	})

	if !strings.Contains(got, "packyard version") {
		t.Errorf("missing banner: %q", got)
	}
	if !strings.Contains(got, "go:") {
		t.Errorf("missing Go version line: %q", got)
	}
	if !strings.Contains(got, "os/arch:") {
		t.Errorf("missing os/arch line: %q", got)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// what fn wrote to it. Needed because runVersion goes through fmt.Print*
// directly.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	w.Close()
	return <-done
}
