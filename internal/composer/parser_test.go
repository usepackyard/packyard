package composer

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeZipFile writes a zip with the given entries to a temp file and returns
// its path. Cleaned up via t.Cleanup.
func makeZipFile(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	return path
}

func TestParseZIP_HappyPath(t *testing.T) {
	path := makeZipFile(t, map[string]string{
		"composer.json": `{"name":"vendor/pkg","version":"1.0.0","require":{"php":">=8.0"}}`,
		"src/Foo.php":   "<?php class Foo {}",
	})

	cj, err := ParseZIP(path)
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}
	if cj.Name != "vendor/pkg" {
		t.Errorf("Name = %q, want vendor/pkg", cj.Name)
	}
	if cj.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", cj.Version)
	}
	if cj.RawJSON == "" {
		t.Error("RawJSON empty — should hold original bytes")
	}
	if cj.Require["php"] != ">=8.0" {
		t.Errorf("Require[php] = %q, want >=8.0", cj.Require["php"])
	}
}

func TestParseZIP_OneLevelDeep(t *testing.T) {
	// Composer accepts composer.json one directory deep (e.g., "plugin-name/composer.json").
	path := makeZipFile(t, map[string]string{
		"plugin-name/composer.json": `{"name":"vendor/plugin-name"}`,
		"plugin-name/src/Foo.php":   "<?php",
	})
	cj, err := ParseZIP(path)
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}
	if cj.Name != "vendor/plugin-name" {
		t.Errorf("Name = %q", cj.Name)
	}
}

func TestParseZIP_MissingComposerJSON(t *testing.T) {
	path := makeZipFile(t, map[string]string{
		"README.md": "no composer.json here",
	})
	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected error for missing composer.json")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not found") {
		t.Errorf("error should say 'not found': %v", err)
	}
	// The error must tell the user what was inside the archive and point
	// them at the source_archive strategy; otherwise operators are flying blind.
	if !strings.Contains(msg, "README.md") {
		t.Errorf("error should list archive entries: %v", err)
	}
	if !strings.Contains(msg, "source_archive") {
		t.Errorf("error should suggest the source_archive strategy: %v", err)
	}
}

// When the archive is huge, we must cap the entry listing so the error
// message stays readable (and doesn't leak the whole archive layout).
func TestParseZIP_MissingComposerJSON_EntriesCapped(t *testing.T) {
	entries := make(map[string]string, 20)
	for i := 0; i < 20; i++ {
		entries[fmt.Sprintf("file%02d.txt", i)] = "x"
	}
	path := makeZipFile(t, entries)
	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "and 15 more") {
		t.Errorf("error should cap listing with '... and N more', got: %v", err)
	}
}

func TestParseZIP_InvalidJSON(t *testing.T) {
	path := makeZipFile(t, map[string]string{
		"composer.json": `{ this is not json`,
	})
	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseZIP_MissingNameField(t *testing.T) {
	path := makeZipFile(t, map[string]string{
		"composer.json": `{"version":"1.0.0"}`,
	})
	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name field: %v", err)
	}
}

func TestParseZIP_TooManyEntries(t *testing.T) {
	// Build a zip with > maxZipEntries entries.
	entries := make(map[string]string, maxZipEntries+5)
	entries["composer.json"] = `{"name":"vendor/pkg"}`
	for i := 0; i < maxZipEntries+5; i++ {
		entries["dummy/"+strings.Repeat("a", 4)+"-"+itoa(i)+".txt"] = "x"
	}
	path := makeZipFile(t, entries)

	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected zip-bomb (entry count) rejection")
	}
	if !strings.Contains(err.Error(), "too many entries") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseZIP_OversizedComposerJSON(t *testing.T) {
	// composer.json larger than maxComposerJSONSize must be rejected.
	huge := strings.Repeat("a", maxComposerJSONSize+10)
	path := makeZipFile(t, map[string]string{
		// JSON is malformed but that doesn't matter — size check fires first.
		"composer.json": `{"name":"vendor/pkg","junk":"` + huge + `"}`,
	})
	_, err := ParseZIP(path)
	if err == nil {
		t.Fatal("expected oversize rejection")
	}
	if !strings.Contains(err.Error(), "too large") && !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size-related error, got: %v", err)
	}
}

// tiny stdlib-only int → string to avoid pulling in strconv just for this.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
