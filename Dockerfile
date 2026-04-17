# Stage 1: Build frontend
FROM oven/bun:debian AS frontend
WORKDIR /app
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ .
COPY internal/i18n/languages.json /internal/i18n/languages.json
RUN bun run build

# Stage 2: Build Go binary
FROM golang:1.25-trixie AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY --from=frontend /app/dist ./internal/frontend/dist
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o /packyard ./cmd/packyard

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
