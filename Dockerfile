# Stage 1: Build frontend
FROM oven/bun:latest AS frontend
WORKDIR /app
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ .
COPY internal/i18n/languages.json /internal/i18n/languages.json
RUN bun run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY --from=frontend /app/dist ./internal/frontend/dist
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /packyard ./cmd/server

# Stage 3: Minimal runtime
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /packyard /usr/local/bin/packyard
RUN mkdir -p /data/packages && chown -R nobody:nobody /data
USER nobody
EXPOSE 8080
ENTRYPOINT ["packyard"]
