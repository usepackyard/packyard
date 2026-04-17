//go:build unix

package cli

import "syscall"

// umask wraps syscall.Umask. Separate file so the Windows build (which
// has no umask concept) can stub it out without conditional code in
// envfile.go.
func umask(mask int) int {
	return syscall.Umask(mask)
}
