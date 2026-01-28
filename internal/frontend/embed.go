package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFS embed.FS

// FS returns the embedded frontend filesystem rooted at the dist/ directory.
// The dist/ directory must be populated by the frontend build before compiling.
func FS() (fs.FS, error) {
	return fs.Sub(embeddedFS, "dist")
}
