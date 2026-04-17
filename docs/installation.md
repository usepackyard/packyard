# Installing Packyard

The happy path is a single line:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh
```

That downloads the right binary for your host, verifies the SHA-256 checksum, drops it into a `PATH` directory, then hands off to `packyard init` — the interactive configuration wizard. Most people are done here.

The rest of this page is for everything else: unattended installs, air-gapped hosts, reverse-proxy setup, upgrades, uninstall.

## What the install script actually does

1. **Preflight** — checks `curl` / `wget`, `tar`, `sha256sum` / `shasum`. Detects Linux + arch (`amd64` / `arm64`). Refuses to run on macOS — build from source there.
2. **Download** — resolves `https://github.com/usepackyard/packyard/releases/latest/download/packyard-linux-<arch>.tar.gz` (GitHub redirects to the current release) plus `SHA256SUMS`.
3. **Verify** — hashes the tarball and compares against the expected value in `SHA256SUMS`. Any mismatch is a hard fail.
4. **Install** — extracts the binary, places it in `/usr/local/bin/packyard` (root install) or `~/.local/bin/packyard` (non-root). `chmod 0755`.
5. **Handoff** — `exec packyard init` with any arguments you passed after `--`.

Nothing else. No env files, no services, no DB writes — all of that lives inside `packyard init` and can be inspected independently.

## Pinning to a specific version

By default the installer grabs the most recent release. To pin:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh -s -- --version v1.0.0-beta.1
```

See [the Releases page](https://github.com/usepackyard/packyard/releases) for available tags.

## Unattended install

Every wizard prompt has a matching flag and env var. Pass `--unattended` to skip the TUI entirely:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh -s -- \
  -- --unattended \
     --port 9090 \
     --url https://repo.example.com \
     --db sqlite \
     --admin-email admin@example.com
```

Everything after the `--` gets forwarded to `packyard init`. Flag reference (see also `packyard init --help`):

| Flag | Env var | Purpose |
|---|---|---|
| `--unattended` | `PACKYARD_UNATTENDED` | Skip prompts, fail on missing required values |
| `--config PATH` | `PACKYARD_CONFIG_FILE` | Where to write the env file |
| `--data-dir PATH` | `PACKYARD_DATA_DIR` | SQLite DB + local storage live here |
| `--mode single\|multi` | `PACKYARD_MODE` | Single-tenant or multi-tenant |
| `--port N` | `PACKYARD_PORT` | HTTP listen port (default 9090) |
| `--url URL` | `PACKYARD_BASE_URL` | Public URL, embedded in Composer dist URLs |
| `--db sqlite\|mysql\|postgres` | `PACKYARD_DB_DRIVER` | Database driver |
| `--db-host`, `--db-port`, `--db-name`, `--db-user`, `--db-password`, `--db-sslmode` | `PACKYARD_DB_*` | MySQL/Postgres connection |
| `--storage local\|s3` | `PACKYARD_STORAGE_TYPE` | Where package zips live |
| `--storage-path PATH` | `PACKYARD_STORAGE_LOCAL_PATH` | Local storage directory |
| `--s3-bucket`, `--s3-region`, `--s3-endpoint`, `--s3-access-key`, `--s3-secret-key` | `PACKYARD_S3_*` | S3 connection |
| `--admin-email EMAIL` | `PACKYARD_ADMIN_EMAIL` | Admin user email |
| `--admin-password PASS` | `PACKYARD_ADMIN_PASSWORD` | Admin password (auto-generated when omitted) |
| `--no-service` | `PACKYARD_NO_SERVICE` | Skip systemd/launchd service install |
| `--force-port` | `PACKYARD_FORCE_PORT` | Skip port-in-use check |

## Air-gapped install

The install script just wraps a download-verify-place cycle. On a host without internet:

```bash
# On a networked machine (pick a tag from the Releases page):
TAG=v1.0.0-beta.1
curl -LO "https://github.com/usepackyard/packyard/releases/download/${TAG}/packyard-${TAG}-linux-amd64.tar.gz"
curl -LO "https://github.com/usepackyard/packyard/releases/download/${TAG}/SHA256SUMS"

# Transfer both files to the air-gapped host, then:
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf packyard-${TAG}-linux-amd64.tar.gz
sudo install -m 0755 packyard /usr/local/bin/packyard

# Continue with the interactive wizard, or use --unattended as above.
sudo packyard init
```

## Upgrade

Re-run the installer. The download-verify-install part is idempotent; `packyard init` detects the existing config and leaves it alone:

```bash
curl -sSf https://get.packyard.dev/install.sh | sh
# or, to pin:
curl -sSf https://get.packyard.dev/install.sh | sh -s -- --version v1.0.0-beta.2
```

If you installed the systemd unit, `install.sh` replaces the binary and then hands off to `packyard init`, which re-applies migrations (no-op if already applied) and re-creates the service unit. `systemctl restart packyard` after the install completes to pick up the new binary.

## Reverse proxy

Packyard binds HTTP only. For anything public, put a TLS-terminating proxy in front. Three ready-to-copy snippets:

### Caddy

```caddy
repo.example.com {
    reverse_proxy 127.0.0.1:9090
}
```

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name repo.example.com;

    ssl_certificate     /etc/letsencrypt/live/repo.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/repo.example.com/privkey.pem;

    # Composer uploads can be 100 MB; let them through.
    client_max_body_size 100m;

    location / {
        proxy_pass http://127.0.0.1:9090;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

After putting Packyard behind a reverse proxy, set `PACKYARD_TRUSTED_PROXIES` in `/etc/packyard/packyard.env` to the proxy's CIDR (e.g. `127.0.0.1/32` for a local proxy). Without this, per-IP rate limiting keys off the proxy's IP and can be bypassed by header injection from an attacker.

### Traefik

```yaml
http:
  routers:
    packyard:
      rule: "Host(`repo.example.com`)"
      entryPoints: [websecure]
      service: packyard
      tls:
        certResolver: letsencrypt
  services:
    packyard:
      loadBalancer:
        servers: [{ url: "http://127.0.0.1:9090" }]
```

## Uninstall

```bash
packyard init --uninstall
```

Stops and disables the systemd/launchd service, removes the binary, env file, and system user. The data directory (SQLite DB + package zips) is preserved by default — pass `--purge-data` to wipe it too.

## Administering without the dashboard

Some ops tasks need to happen before the server is up or when the dashboard is down. The binary has a small set of admin subcommands for exactly that:

```bash
# Create a user directly in the DB (works without the server running):
packyard admin user create --email ops@example.com --super-admin

# Validate config without starting anything:
packyard check-config

# Probe the DB:
packyard check-db

# Run pending migrations:
packyard migrate

# Check the server's health:
packyard healthcheck
```

All of these read the env file at `PACKYARD_CONFIG_FILE` (or the systemd `EnvironmentFile`).

## Getting help

- Full CLI reference: `packyard --help` and `packyard <subcommand> --help`.
- Architecture and development notes: [AGENTS.md](https://github.com/usepackyard/packyard/blob/main/AGENTS.md).
- Bugs / requests: [github.com/usepackyard/packyard/issues](https://github.com/usepackyard/packyard/issues).
