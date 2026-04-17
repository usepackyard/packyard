package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/middleware"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

func NewMux(cfg *config.Config, stores *store.Stores, strg storage.Storage, cache *composer.Cache, frontendFS fs.FS) *http.ServeMux {
	mux := http.NewServeMux()

	// Middleware wrappers.
	basicAuthMw := auth.BasicAuth(stores.Tokens, stores.Orgs)
	sessionAuthMw := auth.SessionAuth(stores.Sessions, cfg.Session.Secret)
	orgMw := auth.OrgMiddleware(stores.Orgs, cfg.Mode)

	// Handlers.
	composerH := handler.NewComposerHandler(cache, strg, stores.Packages, stores.Downloads)
	adminAuthH := handler.NewAdminAuthHandler(stores.Users, stores.Sessions, stores.Orgs, cfg, cfg.BcryptCost)
	adminPkgH := handler.NewAdminPackageHandler(stores.Packages, stores.Sources, strg, cache)
	adminVerH := handler.NewAdminVersionHandler(stores.Packages, stores.Sources, strg, cache)
	adminTokH := handler.NewAdminTokenHandler(stores.Tokens, cfg.BcryptCost)
	adminUserH := handler.NewAdminUserHandler(stores.Users, cfg.BcryptCost)
	adminSrcH := handler.NewAdminSourceHandler(stores.Sources, stores.Packages, stores.Jobs, strg, cache, cfg)
	adminMemH := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, cfg.BcryptCost)
	adminSSOH := handler.NewAdminSSOHandler(stores.Users, stores.SSOTickets, stores.Sessions, cfg)
	webhookH := handler.NewWebhookHandler(stores.Sources, stores.Packages, stores.Jobs, cfg)

	// Super-admin handlers.
	adminOrgH := handler.NewAdminOrgHandler(stores.Orgs, stores.Packages)
	adminGlobalPkgH := handler.NewAdminGlobalPackageHandler(stores.Orgs, stores.Packages, strg, cache)
	adminBearerTokH := handler.NewAdminBearerTokenHandler(stores.AdminTokens)
	pkgStatsH := handler.NewPackageStatsHandler(stores.Downloads, cfg.StatsCacheTTL)

	// Health check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	// Public config (tells frontend which mode we're in, and optionally
	// a public-facing homepage URL the dashboard can surface as a
	// "back" link).
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]string{
			"mode":     cfg.Mode,
			"base_url": cfg.BaseURL,
		}
		if cfg.PublicURL != "" {
			payload["public_url"] = cfg.PublicURL
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	// Composer protocol. URL shape depends on mode:
	//   - single mode: tenant-less URLs (/packages.json, /p2/..., /dist/...)
	//   - multi mode:  /{slug}/packages.json etc., enforcing token-to-slug binding
	//
	// Dist is rate-limited per-IP: every 200 triggers a download counter
	// update + event insert, so an authenticated attacker with a valid
	// token could otherwise inflate counts and bloat the events table.
	// Setting DistRateLimit <= 0 disables the limiter — useful when a
	// reverse proxy already enforces a per-IP cap.
	wrapDist := func(h http.Handler) http.Handler { return h }
	if cfg.DistRateLimit > 0 && cfg.DistRateLimitWindow > 0 {
		limiter := middleware.NewIPRateLimiter(cfg.DistRateLimit, cfg.DistRateLimitWindow, cfg.TrustedProxies)
		wrapDist = limiter.Middleware
	}
	if cfg.Mode == "multi" {
		composerTenantMw := auth.ComposerTenantAuth(stores.Tokens, stores.Orgs)
		mux.Handle("GET /{slug}/packages.json", composerTenantMw(http.HandlerFunc(composerH.PackagesJSON)))
		mux.Handle("GET /{slug}/p2/{vendor}/{package}", composerTenantMw(http.HandlerFunc(composerH.ProviderJSON)))
		mux.Handle("GET /{slug}/dist/{vendor}/{package}/{version}",
			wrapDist(composerTenantMw(http.HandlerFunc(composerH.Dist))))
	} else {
		mux.Handle("GET /packages.json", basicAuthMw(http.HandlerFunc(composerH.PackagesJSON)))
		mux.Handle("GET /p2/{vendor}/{package}", basicAuthMw(http.HandlerFunc(composerH.ProviderJSON)))
		mux.Handle("GET /dist/{vendor}/{package}/{version}",
			wrapDist(basicAuthMw(http.HandlerFunc(composerH.Dist))))
	}

	// Admin auth. POST endpoints require X-Requested-With header (CSRF defense).
	// Browsers can't set custom headers cross-origin without a CORS preflight
	// we never grant, so presence of the header proves the request came from our SPA.
	// Login is also rate-limited per-IP to slow brute force.
	loginLimiter := middleware.NewLoginRateLimiter(10, 15*time.Minute, cfg.TrustedProxies)
	mux.Handle("POST /api/auth/login",
		loginLimiter.Middleware(middleware.RequireCSRFHeader(http.HandlerFunc(adminAuthH.Login))))
	mux.Handle("POST /api/auth/logout", middleware.RequireCSRFHeader(sessionAuthMw(http.HandlerFunc(adminAuthH.Logout))))
	mux.Handle("GET /api/auth/me", sessionAuthMw(http.HandlerFunc(adminAuthH.Me)))
	mux.Handle("PUT /api/auth/me",
		middleware.RequireCSRFHeader(sessionAuthMw(http.HandlerFunc(adminAuthH.UpdateMe))))
	mux.Handle("PUT /api/auth/password",
		middleware.RequireCSRFHeader(sessionAuthMw(http.HandlerFunc(adminAuthH.ChangePassword))))
	mux.Handle("GET /api/orgs", sessionAuthMw(http.HandlerFunc(adminAuthH.ListOrgs)))

	// Webhook (no auth — validated per-provider).
	mux.HandleFunc("POST /hooks/{provider}", webhookH.Handle)
	mux.Handle("GET /sso/login", http.HandlerFunc(adminSSOH.Login))

	// Admin API. Auth: session-cookie super-admin OR Authorization: Bearer
	// admin-token. Either path satisfies RequireSuperAdmin.
	bearerAdminMw := auth.BearerAdminAuth(stores.AdminTokens)
	requireSuperAdminMw := auth.RequireSuperAdmin(stores.Users)
	adminOrgFromSlugMw := auth.AdminOrgFromSlug(stores.Orgs)

	// adminAuth gates with EITHER session+super-admin OR Bearer admin token.
	// Tries Bearer first; falls through to session if no Bearer header.
	adminAuthEither := func(h http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				bearerAdminMw(http.HandlerFunc(h)).ServeHTTP(w, r)
				return
			}
			sessionAuthMw(requireSuperAdminMw(http.HandlerFunc(h))).ServeHTTP(w, r)
		})
	}
	// adminAuth + CSRF (for unsafe methods).
	adminWrite := func(h http.HandlerFunc) http.Handler {
		return middleware.RequireCSRFHeader(adminAuthEither(h))
	}
	// adminAuth + AdminOrgFromSlug (for nested /api/admin/orgs/{slug}/... routes).
	adminScopedRead := func(h http.HandlerFunc) http.Handler {
		return adminAuthEither(func(w http.ResponseWriter, r *http.Request) {
			adminOrgFromSlugMw(http.HandlerFunc(h)).ServeHTTP(w, r)
		})
	}
	adminScopedWrite := func(h http.HandlerFunc) http.Handler {
		return middleware.RequireCSRFHeader(adminScopedRead(h))
	}

	// /api/admin/orgs
	mux.Handle("GET /api/admin/orgs", adminAuthEither(adminOrgH.List))
	mux.Handle("POST /api/admin/orgs", adminWrite(adminOrgH.Create))
	mux.Handle("GET /api/admin/orgs/{slug}", adminAuthEither(adminOrgH.Get))
	mux.Handle("PUT /api/admin/orgs/{slug}/status", adminWrite(adminOrgH.UpdateStatus))
	mux.Handle("DELETE /api/admin/orgs/{slug}", adminWrite(adminOrgH.Delete))

	// /api/admin/orgs/{slug}/members — reuses AdminMemberHandler with
	// AdminOrgFromSlug populating org context (no membership check).
	mux.Handle("GET /api/admin/orgs/{slug}/members", adminScopedRead(adminMemH.List))
	mux.Handle("POST /api/admin/orgs/{slug}/members", adminScopedWrite(adminMemH.Add))
	mux.Handle("PUT /api/admin/orgs/{slug}/members/{id}", adminScopedWrite(adminMemH.Update))
	mux.Handle("DELETE /api/admin/orgs/{slug}/members/{id}", adminScopedWrite(adminMemH.Remove))

	// /api/admin/users — global user management.
	mux.Handle("GET /api/admin/users", adminAuthEither(adminUserH.List))
	mux.Handle("POST /api/admin/users", adminWrite(adminUserH.Create))
	mux.Handle("DELETE /api/admin/users/{id}", adminWrite(adminUserH.Delete))
	mux.Handle("PUT /api/admin/users/{id}/super-admin", adminWrite(adminUserH.SetSuperAdmin))
	mux.Handle("PUT /api/admin/users/{id}/password", adminWrite(adminUserH.SetPassword))

	// /api/admin/packages — cross-org browser + force-delete.
	mux.Handle("GET /api/admin/packages", adminAuthEither(adminGlobalPkgH.List))
	mux.Handle("DELETE /api/admin/packages/{id}", adminWrite(adminGlobalPkgH.Delete))

	// /api/admin/tokens — long-lived super-admin Bearer tokens for external automation.
	mux.Handle("GET /api/admin/tokens", adminAuthEither(adminBearerTokH.List))
	mux.Handle("POST /api/admin/tokens", adminWrite(adminBearerTokH.Create))
	mux.Handle("DELETE /api/admin/tokens/{id}", adminWrite(adminBearerTokH.Delete))
	mux.Handle("POST /api/admin/sso-tickets", adminWrite(adminSSOH.Create))

	// Chain: csrf -> sessionAuth -> orgMiddleware -> handler.
	// RequireCSRFHeader is a no-op on GET/HEAD/OPTIONS, so it's safe to apply uniformly.
	orgAuth := func(h http.HandlerFunc) http.Handler {
		return middleware.RequireCSRFHeader(sessionAuthMw(orgMw(http.HandlerFunc(h))))
	}

	// Chain: csrf -> sessionAuth -> orgMiddleware -> requirePermission -> handler.
	// In single mode, member is nil, so RequirePermission is a no-op (see auth.RequirePermission).
	orgAuthPerm := func(perm string, h http.HandlerFunc) http.Handler {
		return middleware.RequireCSRFHeader(sessionAuthMw(orgMw(auth.RequirePermission(perm)(http.HandlerFunc(h)))))
	}

	if cfg.Mode == "single" {
		// Single mode: routes stay at /api/...
		mux.Handle("GET /api/packages", orgAuth(adminPkgH.List))
		mux.Handle("POST /api/packages", orgAuth(adminPkgH.Create))
		mux.Handle("GET /api/packages/stats", orgAuth(pkgStatsH.Stats))
		mux.Handle("GET /api/packages/{id}", orgAuth(adminPkgH.Get))
		mux.Handle("DELETE /api/packages/{id}", orgAuth(adminPkgH.Delete))

		mux.Handle("POST /api/packages/{id}/versions", orgAuth(adminVerH.Upload))
		mux.Handle("DELETE /api/versions/{id}", orgAuth(adminVerH.Delete))

		mux.Handle("GET /api/tokens", orgAuth(adminTokH.List))
		mux.Handle("POST /api/tokens", orgAuth(adminTokH.Create))
		mux.Handle("DELETE /api/tokens/{id}", orgAuth(adminTokH.Delete))

		mux.Handle("GET /api/users", orgAuth(adminUserH.List))
		mux.Handle("POST /api/users", orgAuth(adminUserH.Create))
		mux.Handle("DELETE /api/users/{id}", orgAuth(adminUserH.Delete))

		mux.Handle("GET /api/packages/{id}/source", orgAuth(adminSrcH.Get))
		mux.Handle("PUT /api/packages/{id}/source", orgAuth(adminSrcH.Set))
		mux.Handle("DELETE /api/packages/{id}/source", orgAuth(adminSrcH.Delete))
		mux.Handle("POST /api/packages/{id}/source/sync", orgAuth(adminSrcH.Sync))
		mux.Handle("GET /api/packages/{id}/sync", orgAuth(adminSrcH.ListSyncJobs))
		mux.Handle("GET /api/packages/{id}/sync/{job_id}", orgAuth(adminSrcH.GetSyncJob))
		mux.Handle("POST /api/sources/preview", orgAuth(adminSrcH.PreviewReleases))

		mux.Handle("GET /api/members", orgAuth(adminMemH.List))
		mux.Handle("POST /api/members", orgAuth(adminMemH.Add))
		mux.Handle("PUT /api/members/{id}", orgAuth(adminMemH.Update))
		mux.Handle("DELETE /api/members/{id}", orgAuth(adminMemH.Remove))
	} else {
		// Multi mode: org-scoped routes under /api/orgs/{org}/...
		// Permissions enforced via RequirePermission middleware. Owners bypass all checks.
		mux.Handle("GET /api/orgs/{org}/packages", orgAuthPerm("packages:read", adminPkgH.List))
		mux.Handle("POST /api/orgs/{org}/packages", orgAuthPerm("packages:write", adminPkgH.Create))
		mux.Handle("GET /api/orgs/{org}/packages/stats", orgAuthPerm("packages:read", pkgStatsH.Stats))
		mux.Handle("GET /api/orgs/{org}/packages/{id}", orgAuthPerm("packages:read", adminPkgH.Get))
		mux.Handle("DELETE /api/orgs/{org}/packages/{id}", orgAuthPerm("packages:delete", adminPkgH.Delete))

		mux.Handle("POST /api/orgs/{org}/packages/{id}/versions", orgAuthPerm("packages:write", adminVerH.Upload))
		mux.Handle("DELETE /api/orgs/{org}/versions/{id}", orgAuthPerm("packages:delete", adminVerH.Delete))

		mux.Handle("GET /api/orgs/{org}/tokens", orgAuthPerm("tokens:manage", adminTokH.List))
		mux.Handle("POST /api/orgs/{org}/tokens", orgAuthPerm("tokens:manage", adminTokH.Create))
		mux.Handle("DELETE /api/orgs/{org}/tokens/{id}", orgAuthPerm("tokens:manage", adminTokH.Delete))

		// Members: list is visible to any org member; writes require members:manage.
		mux.Handle("GET /api/orgs/{org}/members", orgAuth(adminMemH.List))
		mux.Handle("POST /api/orgs/{org}/members", orgAuthPerm("members:manage", adminMemH.Add))
		mux.Handle("PUT /api/orgs/{org}/members/{id}", orgAuthPerm("members:manage", adminMemH.Update))
		mux.Handle("DELETE /api/orgs/{org}/members/{id}", orgAuthPerm("members:manage", adminMemH.Remove))

		mux.Handle("GET /api/orgs/{org}/packages/{id}/source", orgAuthPerm("sources:manage", adminSrcH.Get))
		mux.Handle("PUT /api/orgs/{org}/packages/{id}/source", orgAuthPerm("sources:manage", adminSrcH.Set))
		mux.Handle("DELETE /api/orgs/{org}/packages/{id}/source", orgAuthPerm("sources:manage", adminSrcH.Delete))
		mux.Handle("POST /api/orgs/{org}/packages/{id}/source/sync", orgAuthPerm("sources:manage", adminSrcH.Sync))
		mux.Handle("GET /api/orgs/{org}/packages/{id}/sync", orgAuthPerm("sources:manage", adminSrcH.ListSyncJobs))
		mux.Handle("GET /api/orgs/{org}/packages/{id}/sync/{job_id}", orgAuthPerm("sources:manage", adminSrcH.GetSyncJob))
		mux.Handle("POST /api/orgs/{org}/sources/preview", orgAuthPerm("sources:manage", adminSrcH.PreviewReleases))
	}

	// SPA fallback (serves React frontend).
	if frontendFS != nil {
		mux.Handle("/", handler.SPAHandler(frontendFS))
	}

	return mux
}

// Wrap applies global middleware to a handler: Recovery -> Logging -> SecurityHeaders -> h.
func Wrap(cfg *config.Config, h http.Handler) http.Handler {
	return middleware.Recovery(middleware.Logging(middleware.SecurityHeaders(cfg.BaseURL)(h)))
}
