# Packyard

Private Composer v2 package registry. Single binary, embedded admin dashboard, SQLite/MySQL/PostgreSQL, local or S3 storage.

[![License: BSL 1.1](https://img.shields.io/badge/license-BSL--1.1-blue)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)](https://go.dev)

> Don't want to self-host? [packyard.dev](https://packyard.dev) offers managed hosting with zero ops.

<!-- screenshot: dashboard overview showing packages list, sidebar navigation, org switcher -->

## Features

- **Composer v2 protocol** -- `packages.json`, `/p2/` provider metadata, `/dist/` ZIP downloads
- **Per-org API tokens** -- SHA-256 hashed at rest, optional expiry, scoped to one organization
- **GitHub and GitLab release sync** -- webhook-driven or manual; supports release assets, source archives, and manual metadata
- **Admin dashboard** -- React SPA embedded in the binary; no separate frontend deploy
- **Multi-tenant mode** -- organizations, members, role-based permissions, per-org isolation
- **Three database backends** -- SQLite (default, zero-config), MySQL, PostgreSQL
- **Local or S3/R2 storage** -- pluggable; S3-compatible endpoints (CloudFlare R2, MinIO) work out of the box
- **Single binary** -- `CGO_ENABLED=0`, no runtime dependencies beyond the database
- **i18n** -- dashboard ships with English, Macedonian, German, French, Spanish, Portuguese (BR), and Italian

## Quick start

### From source

Prerequisites: [Go 1.25+](https://go.dev/dl/), [Bun](https://bun.sh)

```bash
git clone https://github.com/usepackyard/packyard.git
cd packyard
make dev            # single-tenant mode on :9090
# or
make dev-multi      # multi-tenant mode on :9090
```

Open `http://localhost:9090` and log in with `admin@example.com` / `changeme`.

### Docker

```bash
docker build -t packyard .

docker run -p 8080:8080 \
  -e PACKYARD_SESSION_SECRET=$(openssl rand -hex 32) \
  -e PACKYARD_BASE_URL=http://localhost:8080 \
  -v packyard-data:/data/packages \
  packyard
```

### Docker Compose

A `docker-compose.yml` is included for development. It starts Packyard with SQLite and local storage:

```bash
docker compose up --build -d
```

## Configuration

All configuration is via `PACKYARD_*` environment variables. The most important ones:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PACKYARD_MODE` | `single` | `single` (self-hosted) or `multi` (multi-tenant) |
| `PACKYARD_BASE_URL` | `http://localhost:8080` | Canonical URL embedded in Composer dist URLs |
| `PACKYARD_SESSION_SECRET` | *(required)* | 32+ char random string for signing session cookies |
| `PACKYARD_DB_DRIVER` | `sqlite` | `sqlite`, `mysql`, or `postgres` |
| `PACKYARD_STORAGE_TYPE` | `local` | `local` or `s3` |

See [`.env.example`](.env.example) for the full list with descriptions.

## Composer client setup

Add the registry to your project's `composer.json`:

```json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://repo.example.com"
    }
  ]
}
```

Authenticate with an API token (minted from the dashboard — both the token and a unique password are generated and shown once):

```bash
composer config --auth http-basic.repo.example.com YOUR_TOKEN YOUR_PASSWORD
```

In multi-tenant mode, the URL includes the organization slug:

```json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://repo.example.com/acme"
    }
  ]
}
```

## Multi-tenant mode

Set `PACKYARD_MODE=multi` to enable organizations. Each org gets its own slug, packages, tokens, and members.

- Composer URLs are prefixed: `/{slug}/packages.json`, `/{slug}/p2/...`, `/{slug}/dist/...`
- Tokens are scoped to one org and cannot access another's packages
- Members have roles (`owner`, `member`) and granular permissions
- Super-admins manage all orgs via the `/admin` section of the dashboard or the `/api/admin/*` API

See [AGENTS.md](AGENTS.md) for the full architecture, API routes, and deployment guide.

## Development

Prerequisites: Go 1.25+, Bun

```bash
# Run the Go backend
make dev

# Or run backend and frontend separately for hot-reload:
PACKYARD_PORT=8080 go run ./cmd/server/    # terminal 1
cd frontend && bun install && bun run dev  # terminal 2 (Vite proxies API to :8080)

# Run all tests
make test

# Build a production binary
make build
```

## Project structure

```
cmd/server/          Entry point
internal/
  auth/              Token auth, session cookies, org middleware, permissions
  composer/          Metadata cache + Composer v2 protocol
  config/            Environment variable loading
  database/          DB connection + Go-based migrations
  handler/           HTTP handlers (admin, composer, webhooks)
  i18n/              Shared language manifest (Go + frontend)
  model/             Bun ORM models
  provider/          Git provider interface (GitHub, GitLab)
  server/            Route registration, middleware wiring
  storage/           Local + S3 storage backends
  store/             Data access interfaces + implementations
frontend/            React 19 + TypeScript + Tailwind CSS 4 + shadcn/ui
```

## License

[Business Source License 1.1](LICENSE) (Ideologix Media DOOEL). Each release converts to Apache License 2.0 four years after its publication date. You may use the software for any purpose except offering it as a managed Composer registry service. See the [LICENSE](LICENSE) file for the full terms.
