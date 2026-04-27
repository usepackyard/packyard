package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ChownTree recursively sets uid/gid on root and every entry beneath it.
// Used by `packyard init` to flip the freshly-created data dir (SQLite db,
// WAL/SHM files, package zips) to the unprivileged service user — the
// non-recursive os.Chown that lived here previously only renamed the
// directory and left every file inside it root-owned, which made SQLite
// report the database as readonly once the service started as `packyard`.
//
// Uses Lchown so symlinks themselves are chowned rather than their
// targets, avoiding accidental writes outside the tree if someone seeds
// a symlink under the data dir. Idempotent: chowning to an already-
// correct uid/gid is a kernel-level no-op.
func ChownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		return nil
	})
}
