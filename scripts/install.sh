#!/usr/bin/env bash
# Packyard install script.
#
#   curl -sSf https://get.packyard.dev/install.sh | bash
#
# Downloads the latest `packyard` binary for this Linux host, verifies
# the SHA-256 checksum, places it in a PATH directory, then hands off
# to `packyard init` for the interactive configuration wizard.
#
# Everything interesting happens inside `packyard init` (Go code with
# real prompts, DB probes, and port checks). This script is deliberately
# small — its job is to get the binary on disk, nothing more.
#
# Platform: Linux amd64 only. Packyard is a server-side product and
# the overwhelming majority of target hosts are x86_64 VPS / bare
# metal. arm64 and macOS installs go through the source path
# documented in the repo README.
#
# Flags (before the double-dash) are handled by this script:
#
#   --version TAG         Install a specific release tag (default: latest)
#                         e.g. --version v1.0.0-beta.1
#   --bin-dir DIR         Where to place the binary (default: /usr/local/bin,
#                         or ~/.local/bin for non-root installs)
#   --no-init             Skip the `packyard init` handoff (just install)
#   --help                Print this help
#
# Everything after the first `--` is passed verbatim to `packyard init`,
# so you can do:
#
#   curl -sSf https://get.packyard.dev/install.sh | bash -s -- \
#     -- --unattended --port 9090 --admin-email admin@example.com
#
# Safety notes:
#   - set -euo pipefail from the first line
#   - Whole script wrapped in main(); nothing runs until the full file
#     is downloaded (partial `curl | bash` can't execute half a script)
#   - Downloaded tarball is SHA-256 verified against a checksum published
#     alongside it on the same release
#   - Never uses `eval` on untrusted input

# Bootstrap under bash. The script body uses bash-only features (set -o
# pipefail, arrays). When invoked via `curl … | sh` — where `sh` is dash
# on Debian/Ubuntu and chokes on `set -o pipefail` — we re-exec under
# bash. This guard is intentionally POSIX so it parses cleanly on dash,
# ash, and ksh; everything below it is allowed to use bashisms.
if [ -z "${BASH_VERSION:-}" ]; then
    if ! command -v bash >/dev/null 2>&1; then
        printf 'install.sh: bash is required but was not found on PATH.\n' >&2
        printf 'Install bash and re-run: curl -sSf https://get.packyard.dev/install.sh | bash\n' >&2
        exit 1
    fi
    # If we were invoked from a real script file (./install.sh or
    # `sh install.sh`), exec bash on it directly. When piped via
    # `curl | sh`, $0 is the shell binary itself ("sh", "dash") — not
    # a usable path — so we fall through to a re-fetch.
    case "$0" in
        *.sh)
            if [ -f "$0" ] && [ -r "$0" ]; then
                exec bash "$0" "$@"
            fi
            ;;
    esac
    _packyard_install_url="${PACKYARD_INSTALL_URL:-https://get.packyard.dev/install.sh}"
    if command -v curl >/dev/null 2>&1; then
        _packyard_install_body="$(curl -sSfL "$_packyard_install_url")" || {
            printf 'install.sh: failed to refetch installer from %s\n' "$_packyard_install_url" >&2
            exit 1
        }
    elif command -v wget >/dev/null 2>&1; then
        _packyard_install_body="$(wget -qO- "$_packyard_install_url")" || {
            printf 'install.sh: failed to refetch installer from %s\n' "$_packyard_install_url" >&2
            exit 1
        }
    else
        printf 'install.sh: need curl or wget on PATH to bootstrap bash interpreter\n' >&2
        exit 1
    fi
    exec bash -c "$_packyard_install_body" install.sh "$@"
fi

set -euo pipefail

PACKYARD_REPO="${PACKYARD_REPO:-usepackyard/packyard}"

main() {
    local version="latest"
    local bin_dir=""
    local run_init="true"
    local -a init_args=()

    # Parse our own flags; everything after `--` goes to packyard init.
    while [ $# -gt 0 ]; do
        case "$1" in
            --version) shift; version="${1:-latest}" ;;
            --bin-dir) shift; bin_dir="${1:-}" ;;
            --no-init) run_init="false" ;;
            --help|-h)
                usage
                return 0
                ;;
            --)
                shift
                init_args=("$@")
                break
                ;;
            *)
                err "unknown flag: $1 (use --help for usage)"
                return 2
                ;;
        esac
        shift || true
    done

    preflight

    local arch
    arch="$(detect_arch)"
    info "detected linux/${arch}"

    local tmpdir
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    download_and_verify "$arch" "$version" "$tmpdir"
    local bin_src="$tmpdir/packyard"

    if [ -z "$bin_dir" ]; then
        bin_dir="$(default_bin_dir)"
    fi
    install_binary "$bin_src" "$bin_dir"

    info ""
    info "Packyard installed to $bin_dir/packyard"

    if [ "$run_init" = "true" ]; then
        info "Running the install wizard…"
        info ""
        exec "$bin_dir/packyard" init "${init_args[@]}"
    else
        info ""
        info "Next: run '$bin_dir/packyard init' to configure."
    fi
}

preflight() {
    # Linux only. Packyard binaries for macOS would mean a separate
    # build matrix and most self-hosters run on Linux servers — so
    # we just bail early on anything else.
    if [ "$(uname -s)" != "Linux" ]; then
        err "unsupported OS: $(uname -s). Packyard ships Linux binaries only."
        err "Build from source on macOS/other: https://github.com/${PACKYARD_REPO}#from-source-contributors"
        exit 1
    fi

    need_cmd bash
    need_cmd tar
    need_cmd uname
    # Need one of curl/wget and one of sha256sum/shasum.
    if ! has curl && ! has wget; then
        err "neither curl nor wget is on PATH"
        exit 1
    fi
    if ! has sha256sum && ! has shasum; then
        err "neither sha256sum nor shasum is on PATH"
        exit 1
    fi
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        *)
            err "unsupported architecture: $(uname -m). Packyard ships amd64 binaries only."
            err "For arm64 or other architectures, build from source: https://github.com/${PACKYARD_REPO}#from-source-contributors"
            exit 1
            ;;
    esac
}

download_and_verify() {
    local arch="$1" version="$2" tmpdir="$3"

    # Resolve "latest" to an actual tag. We hit the /releases list
    # endpoint (which includes prereleases) instead of GitHub's
    # /releases/latest/download redirect (which skips them), so pre-1.0
    # betas are installable through this script.
    if [ "$version" = "latest" ]; then
        version="$(resolve_latest_tag "$tmpdir")"
        info "resolved latest → ${version}"
    fi

    # Single URL pattern for both auto-resolved and --version paths:
    #   /releases/download/<tag>/packyard-<tag>-linux-<arch>.tar.gz
    local asset_name base_url checksum_url
    asset_name="packyard-${version}-linux-${arch}.tar.gz"
    base_url="https://github.com/${PACKYARD_REPO}/releases/download/${version}"
    checksum_url="${base_url}/SHA256SUMS"

    info "downloading ${asset_name}"
    fetch "${base_url}/${asset_name}" "$tmpdir/$asset_name"
    info "downloading SHA256SUMS"
    fetch "$checksum_url" "$tmpdir/SHA256SUMS"

    # Verify checksum. SHA256SUMS has one line per asset; grep for the
    # one matching the binary we downloaded and check only that.
    info "verifying sha256"
    local expected
    expected="$(grep " ${asset_name}$" "$tmpdir/SHA256SUMS" | awk '{print $1}' || true)"
    if [ -z "$expected" ]; then
        err "no checksum found for ${asset_name} in SHA256SUMS — corrupt download or stale release?"
        exit 1
    fi
    local actual
    actual="$(sha256_of "$tmpdir/$asset_name")"
    if [ "$expected" != "$actual" ]; then
        err "checksum mismatch!"
        err "  expected: $expected"
        err "  actual:   $actual"
        err "This is a hard fail — either the download is corrupt or the"
        err "release was tampered with. Do not run the binary."
        exit 1
    fi

    info "extracting"
    tar -xzf "$tmpdir/$asset_name" -C "$tmpdir"
    if [ ! -x "$tmpdir/packyard" ]; then
        err "tarball did not contain an executable 'packyard'"
        exit 1
    fi
}

# resolve_latest_tag fetches the repo's /releases list and returns the
# first tag_name in the response. Unlike /releases/latest, this endpoint
# includes prereleases, so pre-1.0 betas are discoverable.
#
# Output goes to stdout; failures go to stderr + exit. No jq dependency;
# we grep the first "tag_name" field, which is always the most recent
# release in GitHub's default ordering.
resolve_latest_tag() {
    local tmpdir="$1"
    local body tag
    body="$tmpdir/releases.json"
    if ! fetch "https://api.github.com/repos/${PACKYARD_REPO}/releases?per_page=1" "$body"; then
        err "failed to query the GitHub releases API for ${PACKYARD_REPO}"
        err "Possible causes: repo is private, rate-limited, or offline."
        err "Workaround: pin a version with --version vX.Y.Z"
        exit 1
    fi
    tag="$(grep -m1 '"tag_name"' "$body" | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
    if [ -z "$tag" ]; then
        err "could not parse a tag from the GitHub releases API response"
        err "first line: $(head -n1 "$body")"
        exit 1
    fi
    printf '%s\n' "$tag"
}

default_bin_dir() {
    if [ "$(id -u)" = "0" ]; then
        echo "/usr/local/bin"
    else
        echo "$HOME/.local/bin"
    fi
}

install_binary() {
    local src="$1" dst="$2"
    mkdir -p "$dst"
    # Prefer `install` for atomic replace + permission set in one shot.
    # Fall back to cp+chmod if install isn't available.
    if has install; then
        install -m 0755 "$src" "$dst/packyard"
    else
        cp "$src" "$dst/packyard"
        chmod 0755 "$dst/packyard"
    fi

    # Warn if the chosen bin dir isn't on PATH — common for user installs.
    case ":$PATH:" in
        *":$dst:"*) ;;
        *)
            warn ""
            warn "Note: $dst is not on your PATH. Add it with:"
            warn "  echo 'export PATH=\"$dst:\$PATH\"' >> ~/.profile && source ~/.profile"
            warn ""
            ;;
    esac
}

# --- tiny helpers ---------------------------------------------------------

fetch() {
    local url="$1" dest="$2"
    if has curl; then
        curl -sSfL -o "$dest" "$url"
    else
        wget -qO "$dest" "$url"
    fi
}

sha256_of() {
    if has sha256sum; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

has() { command -v "$1" >/dev/null 2>&1; }

need_cmd() {
    if ! has "$1"; then
        err "required command not found: $1"
        exit 1
    fi
}

info() { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
err()  { printf 'install.sh: %s\n' "$*" >&2; }

usage() {
    cat <<'EOF'
Packyard installer (Linux amd64).

Usage:
  curl -sSf https://get.packyard.dev/install.sh | bash [-- INIT_ARGS...]
  ./install.sh [flags] [-- INIT_ARGS...]

Flags:
  --version TAG   Install a specific release tag (default: latest)
                  e.g. --version v1.0.0-beta.1
  --bin-dir DIR   Install directory (default: /usr/local/bin or
                  ~/.local/bin for non-root)
  --no-init       Skip the `packyard init` handoff after install
  --help          Show this help

Arguments after `--` are passed to `packyard init`. Example:

  curl -sSf https://get.packyard.dev/install.sh | bash -s -- \
    -- --unattended --port 9090 --admin-email admin@example.com

See https://get.packyard.dev/installation for the long-form guide.
EOF
}

main "$@"
