package pid

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestNew_HasPrefixAndValidULID(t *testing.T) {
	id := New(Package)
	if !strings.HasPrefix(id, "pkg_") {
		t.Fatalf("missing prefix: %q", id)
	}
	if len(id) != 4+26 {
		t.Fatalf("unexpected length %d for %q", len(id), id)
	}
	if _, err := Parse(id, Package); err != nil {
		t.Fatalf("Parse roundtrip: %v", err)
	}
}

func TestParse_RejectsWrongPrefix(t *testing.T) {
	id := New(Package)
	_, err := Parse(id, Version)
	if !errors.Is(err, ErrWrongPrefix) {
		t.Fatalf("want ErrWrongPrefix, got %v", err)
	}
}

func TestParse_RejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"pkg_",
		"pkg_tooshort",
		"pkg_01JHZ8K3Y5WQ9V2N6TRB4XE7!!", // invalid base32 chars
		"no-underscore",
	}
	for _, c := range cases {
		if _, err := Parse(c, Package); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestParse_ReturnsBodyWithoutPrefix(t *testing.T) {
	id := New(User)
	body, err := Parse(id, User)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(body) != 26 {
		t.Fatalf("want 26-char body, got %d: %q", len(body), body)
	}
	if strings.Contains(body, "_") {
		t.Fatalf("body should not contain underscore: %q", body)
	}
}

func TestNew_MonotonicWithinMillisecond(t *testing.T) {
	const n = 1000
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = New(Package)
	}
	// ULIDs minted via ulid.Monotonic must be strictly increasing when
	// stringified, even if generated inside the same millisecond.
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("not monotonic at %d: %q <= %q", i, ids[i], ids[i-1])
		}
	}
}

func TestNew_ConcurrentSafety(t *testing.T) {
	const goroutines = 16
	const perGoroutine = 500
	seen := sync.Map{}
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id := New(Package)
				if _, loaded := seen.LoadOrStore(id, struct{}{}); loaded {
					t.Errorf("duplicate id: %q", id)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestValid(t *testing.T) {
	if !Valid(New(Package), Package) {
		t.Fatal("want Valid=true")
	}
	if Valid("pkg_garbage", Package) {
		t.Fatal("want Valid=false for garbage body")
	}
	if Valid(New(Package), Version) {
		t.Fatal("want Valid=false for wrong prefix")
	}
}
