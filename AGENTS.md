# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Packyard is a private Composer package registry built in Go. It serves the Composer v2 repository protocol, provides a React admin dashboard, and supports syncing packages from GitHub/GitLab releases. Single binary deployment with embedded SPA.

- **Go module**: `github.com/usepackyard/packyard`
- **License**: BSL 1.1 (converts to Apache 2.0 after 4 years per release)
- **ORM**: Bun (`github.com/uptrace/bun`)
- **Frontend**: React 19 + TypeScript + Tailwind CSS 4 + shadcn/ui
- **Databases**: SQLite, MySQL, PostgreSQL
- **Storage**: Local filesystem, Amazon S3, CloudFlare R2

## Common Commands

```bash
make dev              # Build + run single-tenant mode on :9090
make dev-multi        # Build + run multi-tenant mode on :9090
make frontend         # Build frontend (bun) + copy to internal/frontend/dist
make build            # Production binary (CGO_ENABLED=0)
make test             # go test ./...
make docker-up        # docker compose up --build -d
make docker-down      # docker compose down
make clean            # Remove binaries, databases, build artifacts
```

Binary is `packyard`. All config via `PACKYARD_*` env vars (see `.env.example`).

**Important**: `PACKYARD_BASE_URL` is load-bearing — it's embedded in dist URLs inside the Composer metadata cache and exposed to the frontend via `/api/config`. `make dev` / `make dev-multi` set it to `http://localhost:9090` explicitly. If the server runs on a non-default host/port without `PACKYARD_BASE_URL`, Composer clients will try to download from the wrong URL.

### Frontend Development

Frontend uses **Bun** (not npm/yarn). The dev server proxies API requests to the Go backend:

```bash
# Terminal 1: Run Go backend
PACKYARD_PORT=8080 go run ./cmd/packyard/

# Terminal 2: Run Vite dev server (proxies /api, /packages.json, /p2, /dist → localhost:8080)
cd frontend && bun install && bun run dev
```

The `@` alias maps to `frontend/src/` in imports.

### Running a Single Test

```bash
go test ./internal/store/...          # Test a specific package
go test -run TestPackageCreate ./...  # Test a specific function
```

## Testing Discipline (TDD)

This project follows **test-driven development**. Every new feature, bug fix, or behavior change ships with tests. No exceptions.

**Workflow for any code change:**
1. Write a failing test that describes the desired behavior (or captures the bug)
2. Implement the minimum code to make it pass
3. Refactor with the test as a safety net
4. Run `make test` — all green before committing

**Where tests live:**
- **Pure functions / types** → same-package `_test.go` (e.g. `internal/composer/validate_test.go`)
- **Handlers** → `internal/handler/*_test.go`, using `testutil.NewStores(t)` for real in-memory SQLite and `testutil.DoJSON` / `DoMultipart` helpers
- **Middleware** → `internal/middleware/*_test.go`, usually with `httptest.NewRecorder()` and a stub handler
- **Stores** → `internal/store/store_test.go`, one section per store interface
- **Cross-cutting / wire-format** → `e2e/composer_flow_test.go` drives the full server via `httptest.NewServer`
- **Fixtures** → `internal/testutil/fixtures.go` (`MakeOrg`, `MakeUser`, `MakeMember`, `MakeToken`, `MakePackage`, `MakeVersion`)

**Conventions:**
- Table-driven tests with `name string` field and `t.Run(tt.name, ...)` for subtests
- Stdlib `testing` only — no `testify`. Assert with `if got != want { t.Fatalf(...) }`
- One `testutil.NewStores(t)` per test — no shared state
- For handler tests, inject context via `auth.SetOrgInContext`, `auth.SetUserIDForTest`, `auth.SetOrgIDFromTokenForTest`
- Security-relevant changes get a **regression test** — the test's name should make the protected invariant obvious (e.g. `TestWebhookHandler_FailsClosed_NoSecret`, `TestLoginRateLimiter_XForwardedFor_BypassPrevented`)

**When tests feel hard to write:** that's usually a design signal. A handler that's hard to test in isolation probably has too many collaborators; a store method that requires elaborate setup probably has too many responsibilities. Refactor the code, not the test.

**Never merge code with no tests** — even "trivial" changes. The audit already caught one regression (SHA-256 vs SHA-1 for Composer dist checksums) that existed because the code had never been exercised end-to-end. The E2E suite catches that class of bug in 10 ms; missing tests will cost far more than the time to write them.

## CLI surface

The binary supports a few subcommands in addition to the default server mode. Running `packyard` with no arguments still starts the server (same as pre-subcommand versions), so existing Dockerfiles, systemd units, and k8s manifests keep working untouched.

| Command | Purpose |
|---|---|
| `packyard serve` | Run the HTTP server. Default when no subcommand is given. |
| `packyard init` | Interactive installer (`huh` TUI): paths, mode, database + live connection probe, port + availability check, public URL, storage, admin user, systemd/launchd service install. Supports `--unattended` with flag/env-driven answers for automation. `--uninstall` reverses the install (data dir preserved unless `--purge-data`). |
| `packyard version [--short]` | Print version, commit SHA, build date, Go version, OS/arch. `--short` prints only the version string. |
| `packyard healthcheck [--url URL] [--timeout DUR]` | Hit `/healthz` and exit 0 on HTTP 200, non-zero otherwise. Defaults to `http://127.0.0.1:$PACKYARD_PORT/healthz`. Used by the Dockerfile `HEALTHCHECK` directive. |
| `packyard check-config` | Load env into a Config, run Validate(), exit 0 if healthy. No side effects — no DB connection, no port bind. |
| `packyard check-db` | Open the configured DB and run `SELECT 1`. Used by `packyard init` to probe MySQL/Postgres credentials before writing them to the env file. |
| `packyard migrate` | Run pending migrations. Idempotent (bun tracks applied groups). Designed for k8s init containers. |
| `packyard admin user create --email E --password P [--name N] [--super-admin]` | Create a new user directly in the DB. Works without the server running. Reads `PACKYARD_ADMIN_PASSWORD` if `--password` is omitted. Useful for install bootstrap and disaster recovery. Exit codes: 0 success, 1 DB error, 2 validation error, 3 email already exists. |

## Architecture

### Modes

- **Single mode** (`PACKYARD_MODE=single`, default): Self-hosted, one implicit org, routes at `/api/...`
- **Multi mode** (`PACKYARD_MODE=multi`): multi-tenant, multiple orgs, routes at `/api/orgs/{org}/...`

On first run (empty users table) in **both** modes, `seedDefaults` creates:
- A "default" org (id=1, slug `default`)
- An admin user from `PACKYARD_ADMIN_EMAIL`/`PACKYARD_ADMIN_PASSWORD` as owner of that org

Seeding is idempotent — no-ops once any user exists. If login fails after config changes, delete `packyard.db*` to re-seed.

In multi mode, additional organizations are provisioned by the super-admin through the dashboard or the admin API. From the dashboard: log in as the super-admin and use the `/admin` section. Programmatically, mint an admin Bearer token from the dashboard and call:

```bash
curl -X POST http://localhost:9090/api/admin/orgs \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Requested-With: XMLHttpRequest" \
  -d '{"slug":"acme","name":"Acme"}'

curl -X POST http://localhost:9090/api/admin/orgs/acme/members \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Requested-With: XMLHttpRequest" \
  -d '{"email":"owner@acme.com","password":"...","name":"Owner","role":"owner"}'
```

### Startup Lifecycle

`cmd/packyard/serve.go` runs: config load → DB connect (WAL mode for SQLite, pool: 25 open / 5 idle) → migrations → seed default org + admin user (single mode, first run) → warm Composer metadata cache → start session cleanup goroutine (every 1h) → HTTP server with 30s graceful shutdown.

### Request Flow

Middleware chain: **Logging → Recovery → (route-specific auth) → Handler**

- **Composer endpoints** (`/packages.json`, `/p2/...`, `/dist/...`): HTTP Basic auth middleware resolves API token → sets org ID in context
- **Admin API** (`/api/orgs/{slug}/...`): Session auth middleware → org middleware (resolves org from URL path in multi mode, or injects id=1 in single mode) → handler
- **Super-admin API** (`/api/admin/...`): Either session-cookie super-admin OR `Authorization: Bearer <admin-token>`. Used by the dashboard's super-admin section and by external automation for org lifecycle (suspend/reactivate/archive). Admin tokens are minted and revoked through the same endpoint surface.

Permission checks happen **inside handlers** (not as route middleware). Handlers call `auth.MemberFromContext(r.Context())` and check role/permissions manually. Single mode handlers skip permission checks entirely.

### Directory Structure

```
cmd/packyard/serve.go              Entry point
internal/
  auth/                         HTTP Basic (API tokens), session cookies, org middleware, permissions
  composer/                     Composer v2 metadata cache + protocol helpers
  config/                       Env var loading into Config struct
  database/                     Bun DB factory + Go-based migrations
  frontend/                     Embedded SPA (embed.FS)
  handler/                      HTTP handlers (admin CRUD, composer protocol, webhooks, internal API)
  middleware/                   Request logging, panic recovery
  model/                        Bun model structs (8 tables)
  provider/                     Git provider interface + GitHub implementation
  server/                       Route registration, middleware wiring
  storage/                      Storage interface (local + S3)
  store/                        Store interfaces + Bun implementations
frontend/                       React SPA (Vite + TypeScript + Tailwind + shadcn/ui)
```

### Database

8 tables: `organizations`, `users`, `org_members`, `sessions`, `packages`, `versions`, `api_tokens`, `package_sources`.

Migrations are Go-based (single file at `internal/database/migrations/001_initial.go`), dialect-agnostic via Bun. No separate SQL files per database.

Models at `internal/model/` have both `bun:` and `json:` tags. Sensitive fields use `json:"-"` (passwords, token hashes, auth tokens).

### Store Layer

Interfaces at `internal/store/store.go`. Implementations use Bun query builder. Key interfaces:

- `PackageStore` — packages + versions CRUD, org-scoped
- `TokenStore` — API tokens, org-scoped
- `UserStore` — global users (not org-scoped)
- `SessionStore` — session management
- `SourceStore` — git source configs per package
- `OrgStore` — organizations + memberships + permissions

Handlers talk to interfaces only — never to `*bun.DB` directly. All handlers follow the constructor pattern: `NewXHandler(stores..., config) *XHandler`.

### Composer Metadata Cache

`internal/composer/cache.go` — Per-org in-memory cache holds pre-built JSON bytes for `packages.json` and per-package provider metadata. Thread-safe via `sync.RWMutex`. Warmed on startup via `RebuildAll()`. Invalidated per-org after package sync operations. Serves zero-copy JSON to Composer clients.

### Auth

- **Composer clients**: HTTP Basic auth. Token is the username, the per-token generated password is the password. Both are generated when creating a token and shown once. Token is SHA256-hashed in the DB (`token_hash`), password is bcrypt-hashed (`password_hash`); neither stored plaintext. Expiration checked on every request. Last-used timestamp updated asynchronously in a background goroutine.
- **Admin dashboard**: Session cookies (`packyard_session`). Session ID is 32 random bytes hex-encoded, signed with HMAC-SHA256 (`PACKYARD_SESSION_SECRET`, ≥32 chars, fail-fast at startup). Secure flag set based on `PACKYARD_BASE_URL` scheme. SameSite=Strict. Org resolved from URL path (multi mode) or injected as id=1 (single mode).
- **Super-admin API** (`/api/admin/*`): Either session-cookie super-admin OR `Authorization: Bearer <admin-token>`. Admin tokens live in `admin_tokens` (separate from org-scoped Composer tokens), prefix `adm_`, SHA-256 hashed in DB. Used for machine-to-machine org lifecycle (suspend/reactivate/archive).
- **Org lifecycle**: `organizations.status` ∈ {`active`, `suspended`, `archived`}. Suspended → 402, archived → 404 (data preserved either way). Hard delete requires `?force=true` and refuses if packages exist without it.

### Permissions

Stored as JSON array on `org_members.permissions`. Owner role bypasses all checks.

Available permissions: `packages:read`, `packages:write`, `packages:delete`, `tokens:manage`, `sources:manage`, `members:manage`.

### Provider System

Provider-agnostic interface at `internal/provider/provider.go`. Providers self-register via `init()`.

GitHub implementation at `internal/provider/github/`. Adding a new provider (e.g., GitLab) requires only:
1. `internal/provider/gitlab/client.go` — implement `Provider` interface
2. `internal/provider/gitlab/webhook.go` — webhook parsing + validation
3. Blank import in `cmd/packyard/serve.go`

Sync logic at `internal/provider/sync.go` is provider-agnostic.

### API Routes

**Composer protocol** (HTTP Basic auth, org from token):
- `GET /packages.json` — package index (returns `metadata-url: /p2/%package%.json`)
- `GET /p2/{vendor}/{package}` — per-package metadata (handler must strip `.json` suffix from the `{package}` path value since Composer v2 always appends `.json`)
- `GET /dist/{vendor}/{package}/{version}` — ZIP download

**Admin API** (session auth + org middleware):
- Single mode: `/api/packages`, `/api/tokens`, `/api/members`, `/api/users`
- Multi mode: `/api/orgs/{org}/packages`, `/api/orgs/{org}/tokens`, `/api/orgs/{org}/members`

**Super-admin API** (`/api/admin/*`, session-cookie super-admin OR `Authorization: Bearer <admin-token>`):
- `GET/POST /api/admin/orgs`, `GET /api/admin/orgs/{slug}`, `PUT /api/admin/orgs/{slug}/status`, `DELETE /api/admin/orgs/{slug}?force=true`
- `GET/POST/PUT/DELETE /api/admin/orgs/{slug}/members[/...]`
- `GET/POST/DELETE /api/admin/users[/{id}]`, `PUT /api/admin/users/{id}/super-admin`
- `GET /api/admin/packages`, `DELETE /api/admin/packages/{id}`
- `GET/POST/DELETE /api/admin/tokens[/{id}]` — admin Bearer tokens for machine-to-machine integrations

**Webhooks**: `POST /hooks/{provider}` — HMAC-validated (GitHub uses `X-Hub-Signature-256`), runs sync in background goroutine with `context.Background()`. Returns immediately with `{"status": "syncing"}`. Non-published or draft releases are silently ignored with 200. Max download size per release asset: 100MB.

### SPA Embedding

`internal/frontend/` embeds the built React SPA via `embed.FS`. The SPA handler (`internal/handler/spa.go`) serves static files directly and falls back to `index.html` for all non-file paths, enabling React Router client-side routing.

## Code Style

- Go stdlib `http.ServeMux` for routing (Go 1.22+ pattern matching)
- `log/slog` for structured logging
- Store interfaces for data access — handlers never touch DB directly
- `writeJSON()` / `writeError()` helpers in `internal/handler/helpers.go`
- `pathID()` helper for extracting path params
- Frontend uses `useAuth()` hook providing `user`, `config`, `org`, `orgs`, and an org-scoped API client. API client auto-redirects to `/login` on 401
- Context accessors: `auth.UserIDFromContext()`, `auth.OrgFromContext()`, `auth.MemberFromContext()`, `auth.TokenFromContext()`
- Background work (webhook sync, token tracking) uses `context.Background()`, not request context
- Error responses always `{"error": "message"}` via `writeError()` helper

## i18n

The dashboard ships with 7 locales: en, mk, de, fr, es, pt-BR, it. The canonical list lives in `internal/i18n/languages.json` (shared by Go and frontend).

**When adding or changing any UI string in a locale file (`frontend/src/locales/en/*.json`), you must translate it into all other locales immediately.** Do not leave English placeholder text in non-English files. The `TestLocaleParity` Go test enforces key parity; it does not catch values that are identical to English but should be translated.

Locale catalogs: `frontend/src/locales/{en,mk,de,fr,es,pt-BR,it}/`. Each locale has the same set of JSON namespace files (common, auth, profile, errors, dashboard, orgSelector, tokens, users, members, packages, admin, layout).

## Environment Variables

All prefixed with `PACKYARD_`. Key ones:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PACKYARD_PORT` | 8080 | HTTP port |
| `PACKYARD_MODE` | single | `single` or `multi` |
| `PACKYARD_DB_DRIVER` | sqlite | `sqlite`, `mysql`, `postgres` |
| `PACKYARD_DB_PATH` | ./packyard.db | SQLite file path |
| `PACKYARD_STORAGE_TYPE` | local | `local` or `s3` |
| `PACKYARD_ADMIN_EMAIL` | admin@example.com | Seeded on first run |
| `PACKYARD_ADMIN_PASSWORD` | changeme | Seeded on first run |
| `PACKYARD_GITHUB_TOKEN` | (empty) | PAT for private repo sync |
| `PACKYARD_SESSION_SECRET` | (required, ≥32 chars) | HMAC key for signing session cookies. Server refuses to start without it. |
| `PACKYARD_TRUSTED_PROXIES` | (empty) | Comma-separated CIDRs of reverse proxies whose X-Forwarded-For is honored. Required when behind a proxy — otherwise rate limiter is bypassable. |

Full list in `.env.example`.

## Default Credentials

- **Super-admin**: `PACKYARD_ADMIN_EMAIL` / `PACKYARD_ADMIN_PASSWORD` (default `admin@example.com` / `changeme`). Seeded on first run in **both** modes — `seedDefaults` in `cmd/packyard/serve.go` flips `is_super_admin=true` on this user. Single mode also seeds a "default" org with the admin as owner; multi mode creates only the super-admin (additional orgs are provisioned via `/api/admin/orgs`).
- **Composer password**: Generated per-token at creation time. Shown once alongside the token, never stored plaintext.

### Promoting an existing user to super-admin

If you've already deployed and need to add a super-admin without minting a new account, use the dashboard (Super-admin → Users → Shield icon) — or, for emergency disaster-recovery, hit the DB directly:

```sql
UPDATE users SET is_super_admin = true WHERE email = 'someone@example.com';
```

## Deployment

### URL shape (multi mode)

A typical multi-tenant deployment splits hostnames:

- **Dashboard host** (e.g. `app.example.com`): admin UI + org-scoped API at `/api/orgs/{slug}/...` + super-admin API at `/api/admin/...`.
- **Composer host** (e.g. `repo.example.com`): `repo.example.com/{slug}/packages.json`, `p2/...`, `dist/...` — all tenant-prefixed. Tenants configure `composer.json` as `{"type": "composer", "url": "https://repo.example.com/{slug}"}`.

A single binary serves both — there's no separate process. Route them through the same ingress; differentiate by `Host` header at the load balancer if you want different rate limits or caching policies for the Composer endpoint vs the dashboard.

### DNS + TLS

- Two A/CNAME records (e.g. `app.example.com` and `repo.example.com`). **No wildcard needed** — multi-tenant routing is path-based, not subdomain-based.
- Single TLS cert covering both hostnames (or two separate certs). cert-manager handles this automatically on Kubernetes.
- `PACKYARD_BASE_URL=https://repo.example.com` — embedded into the Composer metadata `dist.url` values, so clients see the canonical URL even when requests are served through an internal hostname.

### Single mode (self-hosted) URL shape

- Everything served from one host.
- Composer URLs are tenant-less: `/packages.json`, `/p2/...`, `/dist/...`.
- `/api/admin/*` exists but isn't required for normal operation — the super-admin role is available if you want it.

### External automation (multi mode)

The admin API (`/api/admin/*`) accepts either a session-cookie super-admin or an `Authorization: Bearer <admin-token>`. Tokens are minted from the dashboard (Super-admin → Admin Tokens) and used by external automation (CI, provisioning scripts) to manage organizations programmatically.
