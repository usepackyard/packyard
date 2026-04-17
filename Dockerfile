# Stage 1: Build frontend.
# Alpine variant pulls ~1/3 the size of the Debian variant. bun is the
# only tool the stage runs, so the smaller base is a pure win on cold
# runners.
FROM oven/bun:alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/bun.lock ./
# Cache mount keeps bun's install cache warm within a run. Combined
# with the GHA layer cache, this makes `bun install` near-instant when
# package.json/bun.lock haven't changed and a layer cache miss bit us.
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --frozen-lockfile
COPY frontend/ .
COPY internal/i18n/languages.json /internal/i18n/languages.json
RUN bun run build

# Stage 2: Build Go binary.
# Alpine + CGO_ENABLED=0 produces a fully static binary — the runtime
# stage is glibc-based (debian:13-slim) but doesn't dlopen anything
# from this binary, so musl-vs-glibc is irrelevant.
FROM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
# Cache mounts for the module cache and build cache. Persists across
# `go mod download` and `go build` invocations within the same runner.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY --from=frontend /app/dist ./internal/frontend/dist
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# -trimpath strips absolute paths from the binary (reproducibility).
# -buildvcs=false skips the Go 1.18+ VCS stamp — we pass VERSION via
# -ldflags, so the default stamp is redundant overhead.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -buildvcs=false \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /packyard ./cmd/packyard

# Stage 3: Export-only target for extracting the binary via `buildx --output`.
# Not used by the default `docker build` (which stops at the last stage, runtime).
FROM scratch AS export
COPY --from=builder /packyard /packyard

# Stage 4: Minimal runtime (default build target).
FROM debian:13-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
COPY --from=builder /packyard /usr/local/bin/packyard
RUN mkdir -p /data/packages && chown -R nobody:nogroup /data
USER nobody
EXPOSE 8080
# Built-in healthcheck uses the binary itself — no bash, wget, or curl
# needed, which future-proofs us against switching to a distroless base.
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=5 \
    CMD ["packyard", "healthcheck"]
ENTRYPOINT ["packyard"]
