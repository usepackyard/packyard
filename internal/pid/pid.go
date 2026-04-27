// Package pid produces prefixed ULID identifiers used as public-facing
// IDs in URLs and JSON. Internal int64 primary keys remain untouched;
// these strings are what callers see.
//
// Format: "<prefix>_<26-char Crockford Base32 ULID>" (e.g. pkg_01JHZ...).
package pid

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ID kind prefixes. Keep these short and stable — they appear in URLs
// and logs, so changes are effectively breaking.
const (
	Package       = "pkg"
	Version       = "ver"
	User          = "usr"
	OrgMember     = "mbr"
	APIToken      = "tok"
	AdminToken    = "atk"
	SyncJob       = "job"
	PackageSource = "src"
	ProviderConnection = "conn"
)

// entropy must be monotonic so IDs minted in the same millisecond still
// sort in creation order. crypto/rand reads are serialized via the
// mutex below (ulid.Monotonic is not goroutine-safe on its own).
var (
	entropyMu sync.Mutex
	entropy   io.Reader = ulid.Monotonic(rand.Reader, 0)
)

// New returns a new prefixed ULID. The prefix must be a non-empty
// lowercase kind (see constants above).
func New(prefix string) string {
	if prefix == "" {
		panic("pid.New: empty prefix")
	}
	entropyMu.Lock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	entropyMu.Unlock()
	return prefix + "_" + id.String()
}

var (
	ErrWrongPrefix = errors.New("pid: wrong prefix")
	ErrMalformed   = errors.New("pid: malformed")
)

// Parse validates that s is a public ID of the given kind and returns
// the ULID body (without prefix or underscore).
func Parse(s, prefix string) (string, error) {
	if s == "" {
		return "", ErrMalformed
	}
	want := prefix + "_"
	if !strings.HasPrefix(s, want) {
		return "", fmt.Errorf("%w: expected %q", ErrWrongPrefix, prefix)
	}
	body := s[len(want):]
	if _, err := ulid.ParseStrict(body); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return body, nil
}

// Valid reports whether s is a well-formed public ID of the given kind.
func Valid(s, prefix string) bool {
	_, err := Parse(s, prefix)
	return err == nil
}
