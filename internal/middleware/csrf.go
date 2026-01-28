package middleware

import "net/http"

// RequireCSRFHeader rejects state-changing requests that lack the
// X-Requested-With header. Browsers cannot set custom headers on
// cross-origin requests without a CORS preflight, which our server
// does not allow — so presence of the header proves the request
// came from our same-origin SPA (or an intentional API client).
//
// Apply only to session-cookie-authed routes. Endpoints authenticated
// by Authorization headers, HMAC, or shared secrets are not CSRF-reachable
// from a browser and must not go through this middleware.
func RequireCSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Requested-With") == "" {
			http.Error(w, "missing X-Requested-With header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
