# 📦 Packyard

Private Composer v2 package registry. Single binary, embedded admin dashboard, SQLite/MySQL/PostgreSQL, local or S3 storage.

[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)](https://go.dev)

> For documentation and more details, visit [packyard.dev](https://packyard.dev).

<!-- screenshot: dashboard overview showing packages list, sidebar navigation, org switcher -->

## ✨ Features

- **Composer v2 protocol** — `packages.json`, `/p2/` provider metadata, `/dist/` ZIP downloads
- **Per-org API tokens** — SHA-256 hashed at rest, optional expiry, scoped to one organization
- **GitHub release sync** — webhook-driven or manual; release assets, source archives, or manual metadata
- **Organizations, members, roles** — every install ships with a `default` org; add more for per-team or per-customer isolation
- **Pluggable storage & databases** — SQLite / MySQL / PostgreSQL; local filesystem or S3-compatible (R2, MinIO)
- **Single-binary deploy** — `CGO_ENABLED=0`, React admin dashboard embedded, 7-locale i18n built in

## 🚀 Quick start

On a fresh Linux or macOS host:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh
```

The installer downloads the right binary, verifies its SHA-256, places it in `/usr/local/bin` (or `~/.local/bin` for non-root), then runs `packyard init` — an interactive wizard that configures paths, database, port, public URL, storage, admin user, and an optional systemd/launchd service.

Unattended / automation variant:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh -s -- \
  -- --unattended \
     --port 9090 \
     --url https://repo.example.com \
     --admin-email admin@example.com
```

See the [installation guide](https://packyard.dev/documentation/installation) for air-gapped installs, reverse-proxy recipes (Caddy / nginx / Traefik), unattended flags, and upgrades.

### Build from source

Prerequisites: [Go 1.25+](https://go.dev/dl/), [Bun](https://bun.sh).

```bash
git clone https://github.com/usepackyard/packyard.git
cd packyard
make dev            # build + run on :9090
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

## Configuration

All configuration is via `PACKYARD_*` environment variables. The most important ones:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PACKYARD_BASE_URL` | `http://localhost:8080` | Canonical URL embedded in Composer dist URLs |
| `PACKYARD_SESSION_SECRET` | *(required)* | 32+ char random string for signing session cookies |
| `PACKYARD_DB_DRIVER` | `sqlite` | `sqlite`, `mysql`, or `postgres` |
| `PACKYARD_STORAGE_TYPE` | `local` | `local` or `s3` |

See [`.env.example`](.env.example) for the full list with descriptions.

## Composer client setup

Add the registry to your project's `composer.json`. Composer URLs are
slug-prefixed; the seeded org slug is `default`, and you can rename it
or add more from the dashboard.

```json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://repo.example.com/default"
    }
  ]
}
```

Authenticate with an API token (minted from the dashboard — both the token and a unique password are generated and shown once):

```bash
composer config --auth http-basic.repo.example.com YOUR_TOKEN YOUR_PASSWORD
```

## 🏢 Organizations

Every Packyard install ships with a single organization (`slug: default`,
created on first boot). Add more from the dashboard or the admin API for
per-team or per-customer isolation.

- Composer URLs are slug-prefixed: `/{slug}/packages.json`, `/{slug}/p2/...`, `/{slug}/dist/...`
- Tokens are scoped to one org and cannot reach another's packages
- Members have roles (`owner`, `member`) and granular permissions
- Super-admins manage every org via the `/admin` dashboard section or the `/api/admin/*` API

See [AGENTS.md](AGENTS.md) for full architecture, routes, and deployment notes.

## 🤝 Contributing

Issues and pull requests are welcome. Before submitting, please read [CONTRIBUTING.md](CONTRIBUTING.md) — it covers how to get started, how to report security issues privately, and the contributor terms.

## 🛠️ Development

```bash
make dev           # build frontend + run backend on :9090
make test          # run the full Go test suite
make build         # produce a production binary
```

For a hot-reload loop, run the backend and frontend in separate terminals:

```bash
PACKYARD_PORT=8080 go run ./cmd/packyard/    # terminal 1
cd frontend && bun install && bun run dev  # terminal 2 (Vite proxies API to :8080)
```

## Project structure

```
cmd/packyard/        Entry point + CLI subcommands (serve, init, migrate, healthcheck, …)
internal/
  auth/              Token auth, session cookies, org middleware, permissions
  composer/          Metadata cache + Composer v2 protocol
  config/            Environment variable loading
  database/          DB connection + Go-based migrations
  handler/           HTTP handlers (admin, composer, webhooks)
  i18n/              Shared language manifest (Go + frontend)
  model/             Bun ORM models
  provider/          Git provider interface (currently GitHub)
  server/            Route registration, middleware wiring
  storage/           Local + S3 storage backends
  store/             Data access interfaces + implementations
frontend/            React 19 + TypeScript + Tailwind CSS 4 + shadcn/ui
```

## License

Packyard is licensed under the **GNU Affero General Public License v3.0** (AGPL-3.0). See [LICENSE](LICENSE) for the full text.

You can run, study, modify, and redistribute Packyard freely. If you offer Packyard (or a modified version) to others over a network, you must make the corresponding source available to those users under the same license.

For commercial licensing arrangements that don't require AGPL compliance, contact [hello@packyard.dev](mailto:hello@packyard.dev).
