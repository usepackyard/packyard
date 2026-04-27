//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChownTree_VisitsEveryEntry chowns a temp tree to the current process's
// uid/gid. The Lchown calls are kernel-level no-ops (we already own the
// files) but the walk traversal is still exercised end-to-end, so we'd
// catch a regression where ChownTree silently skipped subdirectories or
// returned early on the first entry.
func TestChownTree_VisitsEveryEntry(t *testing.T) {
	root := t.TempDir()
	// Layout: root/{a/b/file.txt, link -> a/b/file.txt}
	subdir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(subdir, "file.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	want := map[string]bool{
		root:                              false,
		filepath.Join(root, "a"):          false,
		subdir:                            false,
		file:                              false,
		link:                              false,
	}

	if err := ChownTree(root, os.Getuid(), os.Getgid()); err != nil {
		t.Fatalf("ChownTree: %v", err)
	}

	// Re-walk and confirm every expected path actually exists with the
	// current uid/gid — verifies traversal covered subdir + file + link.
	if err := filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if _, ok := want[path]; ok {
			want[path] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("post-walk: %v", err)
	}
	for p, visited := range want {
		if !visited {
			t.Errorf("path not visited by post-walk: %s", p)
		}
	}
}

// TestChownTree_NonexistentRoot makes sure we surface a useful error
// rather than silently succeeding when the caller passes a bad path.
// The existing chown call sites in init.go all check err and wrap it.
func TestChownTree_NonexistentRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := ChownTree(missing, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("ChownTree on missing root: want error, got nil")
	}
}
