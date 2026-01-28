package testutil

import (
	"testing"
	"time"
)

// Eventually polls the predicate until it returns true, or until deadline
// elapses. Useful for waiting on fire-and-forget goroutines like the
// download counter / event writer that run after a request returns 200.
// Fails the test (t.Fatalf) with the message if the deadline is exceeded.
//
// Default polling interval is 5ms — fast enough to finish quickly in the
// happy path, coarse enough to not waste cycles.
func Eventually(t *testing.T, deadline time.Duration, msg string, fn func() bool) {
	t.Helper()
	const tick = 5 * time.Millisecond
	end := time.Now().Add(deadline)
	for {
		if fn() {
			return
		}
		if time.Now().After(end) {
			t.Fatalf("Eventually timed out after %s: %s", deadline, msg)
		}
		time.Sleep(tick)
	}
}
