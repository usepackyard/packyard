package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEnvFile_Mode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.env")
	if err := WriteEnvFile(path, map[string]string{"FOO": "bar"}); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perms = %o, want 600", perm)
	}
}

func TestWriteEnvFile_QuotesValuesWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.env")
	err := WriteEnvFile(path, map[string]string{
		"SIMPLE":   "no-spaces",
		"SPACED":   "has spaces",
		"QUOTED":   `with 'single' quotes`,
		"PASSWORD": "p@ss w0rd!",
	})
	if err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	body, _ := os.ReadFile(path)
	s := string(body)

	if !strings.Contains(s, "SIMPLE=no-spaces\n") {
		t.Errorf("simple value should not be quoted: %s", s)
	}
	if !strings.Contains(s, "SPACED='has spaces'\n") {
		t.Errorf("spaced value should be single-quoted: %s", s)
	}
	if !strings.Contains(s, `QUOTED='with '\''single'\'' quotes'`+"\n") {
		t.Errorf("single-quote escape missing: %s", s)
	}
}

func TestWriteEnvFile_DeterministicOrder(t *testing.T) {
	// Two writes with the same keys in different insertion order
	// should produce the same bytes — makes diffs readable.
	path1 := filepath.Join(t.TempDir(), "pkg.env")
	path2 := filepath.Join(t.TempDir(), "pkg.env")
	m1 := map[string]string{"A": "1", "B": "2", "C": "3"}
	m2 := map[string]string{"C": "3", "A": "1", "B": "2"}
	if err := WriteEnvFile(path1, m1); err != nil {
		t.Fatal(err)
	}
	if err := WriteEnvFile(path2, m2); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(path1)
	b2, _ := os.ReadFile(path2)
	if string(b1) != string(b2) {
		t.Errorf("output differs:\n--- 1 ---\n%s\n--- 2 ---\n%s", b1, b2)
	}
}
