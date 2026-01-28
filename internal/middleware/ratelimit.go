package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IPRateLimiter applies a sliding-window per-client-IP rate limit to a
// handler. Every request counts toward the limit — unlike LoginRateLimiter,
// successful responses do NOT reset the IP's history. This is the right
// shape for endpoints where 2xx traffic is itself the abuse vector
// (e.g. inflating download counters or flooding an append-only log table).
//
// Storage is in-process. Behind a load balancer, either run with sticky
// sessions or replace with a shared store (e.g. Redis).
type IPRateLimiter struct {
	mu             sync.Mutex
	attempts       map[string][]time.Time
	limit          int
	window         time.Duration
	trustedProxies []*net.IPNet
}

// NewIPRateLimiter returns a limiter. trustedProxies is the set of CIDRs
// whose X-Forwarded-For header is honored when resolving the client IP.
// Pass nil/empty when the server is directly exposed — otherwise header
// injection bypasses the limit.
func NewIPRateLimiter(limit int, window time.Duration, trustedProxies []*net.IPNet) *IPRateLimiter {
	return &IPRateLimiter{
		attempts:       make(map[string][]time.Time),
		limit:          limit,
		window:         window,
		trustedProxies: trustedProxies,
	}
}

// Middleware wraps a handler with the rate limit. Exceeded requests get
// 429 Too Many Requests with a Retry-After hint.
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := l.clientIP(r)
		if !l.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow checks whether the IP is under the limit and records this attempt.
func (l *IPRateLimiter) allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Drop expired timestamps.
	hits := l.attempts[ip]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.limit {
		l.attempts[ip] = kept
		return false
	}

	l.attempts[ip] = append(kept, now)
	return true
}

// reset clears an IP's recorded attempts. Used by LoginRateLimiter after a
// successful login so legitimate users aren't punished for a few earlier
// typos. Not invoked by IPRateLimiter itself.
func (l *IPRateLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.attempts, ip)
	l.mu.Unlock()
}

// clientIP resolves the originating client IP. Walks X-Forwarded-For only
// when the immediate peer is in the trusted-proxies set. Without trusted
// proxies (default), the header is ignored entirely.
func (l *IPRateLimiter) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if !l.isTrusted(host) {
		return host
	}

	// Peer is a trusted proxy. Walk X-Forwarded-For right-to-left, returning
	// the rightmost entry that isn't itself a trusted proxy. Each trusted
	// proxy appends its peer on the right, so the first untrusted value from
	// the right is the real client.
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		if !l.isTrusted(candidate) {
			return candidate
		}
	}
	return host
}

func (l *IPRateLimiter) isTrusted(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range l.trustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// LoginRateLimiter is an IPRateLimiter tuned for login endpoints: it clears
// an IP's history on a 2xx response so users aren't punished indefinitely
// for a few mistyped passwords before a successful login.
type LoginRateLimiter struct {
	*IPRateLimiter
}

// NewLoginRateLimiter returns a login rate limiter. Default policy in main
// is 10 requests per 15 minutes per remote IP.
func NewLoginRateLimiter(limit int, window time.Duration, trustedProxies []*net.IPNet) *LoginRateLimiter {
	return &LoginRateLimiter{IPRateLimiter: NewIPRateLimiter(limit, window, trustedProxies)}
}

// Middleware wraps a handler with the login rate limit: over-limit returns
// 429; successful 2xx responses reset the IP's history.
func (l *LoginRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := l.clientIP(r)
		if !l.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "too many login attempts", http.StatusTooManyRequests)
			return
		}

		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		if rw.status >= 200 && rw.status < 300 {
			l.reset(ip)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
