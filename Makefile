.PHONY: dev build run clean frontend test

# Dev-only fixed session secret (32+ chars). NEVER use in production.
DEV_SESSION_SECRET = devdevdevdevdevdevdevdevdevdev01

# Development: build frontend, build backend, run on :9090. The seeded
# default org (slug=default) is auto-created on first boot.
dev: frontend
	go build -o packyard ./cmd/packyard/
	PACKYARD_PORT=9090 PACKYARD_BASE_URL=http://localhost:9090 \
		PACKYARD_SESSION_SECRET=$(DEV_SESSION_SECRET) ./packyard

# Build frontend
frontend:
	cd frontend && bun install && bun run build
	rm -rf internal/frontend/dist
	cp -r frontend/dist internal/frontend/dist

# Build production Go binary (requires frontend to be built first)
build: frontend
	CGO_ENABLED=0 go build -ldflags="-s -w" -o packyard ./cmd/packyard/

# Run tests
test:
	go test -v ./...

# Docker
docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

# Clean build artifacts
clean:
	rm -f packyard packyard.db
	rm -rf data/
	rm -rf internal/frontend/dist
	rm -rf frontend/dist frontend/node_modules
