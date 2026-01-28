package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiter_LimitAndReset(t *testing.T) {
	limiter := NewLoginRateLimiter(3, time.Minute, nil)
	pass := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // success → resets
	})
	fail := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // failure → counts toward limit
	})

	mwFail := limiter.Middleware(fail)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		mwFail.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	// 4th attempt should be rate-limited.
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptest.NewRecorder()
	mwFail.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request: status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header should be set on 429")
	}

	// A different IP is unaffected.
	req2 := httptest.NewRequest("POST", "/login", nil)
	req2.RemoteAddr = "198.51.100.7:12345"
	rec2 := httptest.NewRecorder()
	mwFail.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("different IP: status = %d, want 401 (not throttled)", rec2.Code)
	}

	// Successful login on a fresh IP that's about to hit the limit clears it.
	srcIP := "192.0.2.5:12345"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = srcIP
		mwFail.ServeHTTP(httptest.NewRecorder(), req)
	}
	// Now succeed.
	mwPass := limiter.Middleware(pass)
	req3 := httptest.NewRequest("POST", "/login", nil)
	req3.RemoteAddr = srcIP
	rec3 := httptest.NewRecorder()
	mwPass.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("success: status = %d, want 200", rec3.Code)
	}
	// IP should be reset — three more failures should be allowed.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = srcIP
		rec := httptest.NewRecorder()
		mwFail.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("post-success attempt %d throttled — counter should have been reset", i+1)
		}
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse CIDR %s: %v", s, err)
	}
	return n
}

func TestLoginRateLimiter_XForwardedFor_BypassPrevented(t *testing.T) {
	// No trusted proxies → X-Forwarded-For must be ignored, otherwise
	// rotating header values trivially bypasses the per-IP limit.
	limiter := NewLoginRateLimiter(2, time.Minute, nil)
	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		// Spoof a fresh "client" each iteration. With the bypass in place
		// every request was a fresh counter; the fix makes them all share one.
		req.Header.Set("X-Forwarded-For", "10.0.0."+itoa(i))
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if i >= 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("attempt %d: X-Forwarded-For trusted without configuration — bypass possible (status=%d)", i+1, rec.Code)
		}
	}
}

func TestLoginRateLimiter_XForwardedFor_HonoredForTrustedProxy(t *testing.T) {
	trusted := []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}
	limiter := NewLoginRateLimiter(2, time.Minute, trusted)
	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	send := func(xff string) int {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "10.1.2.3:5555" // immediate peer is in the trusted CIDR
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	// Two attempts from "real client" 198.51.100.10 (untrusted) — both allowed.
	if got := send("198.51.100.10"); got != http.StatusUnauthorized {
		t.Fatalf("attempt 1: %d", got)
	}
	if got := send("198.51.100.10"); got != http.StatusUnauthorized {
		t.Fatalf("attempt 2: %d", got)
	}
	// Third → throttled.
	if got := send("198.51.100.10"); got != http.StatusTooManyRequests {
		t.Fatalf("attempt 3 should be throttled: %d", got)
	}
	// A different real client through the same proxy is unaffected.
	if got := send("198.51.100.99"); got != http.StatusUnauthorized {
		t.Fatalf("different client: %d (should not be throttled)", got)
	}
}

func TestLoginRateLimiter_RightmostUntrusted(t *testing.T) {
	// X-Forwarded-For: "real-client, proxy-1, proxy-2" with both proxies
	// in the trusted set. We should pick "real-client" as the client IP.
	trusted := []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}
	limiter := NewLoginRateLimiter(2, time.Minute, trusted)
	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	send := func() int {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "10.1.1.1:5555"
		req.Header.Set("X-Forwarded-For", "198.51.100.5, 10.0.0.1, 10.0.0.2")
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	send()
	send()
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("third attempt should be throttled (same real client): got %d", got)
	}
}

// IPRateLimiter doesn't clear on 2xx — inflation attacks use successful
// responses, so "you succeeded" is not a reason to forgive the IP.
func TestIPRateLimiter_SuccessDoesNotReset(t *testing.T) {
	limiter := NewIPRateLimiter(3, time.Minute, nil)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := limiter.Middleware(ok)

	send := func() int {
		req := httptest.NewRequest("GET", "/dist/x/y/1.0.0", nil)
		req.RemoteAddr = "203.0.113.20:4242"
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	// First 3 must pass even though each is a 200; the 4th must be throttled.
	for i := 0; i < 3; i++ {
		if got := send(); got != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", i+1, got)
		}
	}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("fourth attempt: status = %d, want 429", got)
	}
}

// tiny stdlib-only int → string. Avoids strconv import for the few cases we have.
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
