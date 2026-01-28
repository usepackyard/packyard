package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders returns middleware that sets hardening headers on every
// response. HSTS is only set when baseURL uses https to avoid breaking
// plain-HTTP dev deployments.
func SecurityHeaders(baseURL string) func(http.Handler) http.Handler {
	hsts := strings.HasPrefix(baseURL, "https://")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
