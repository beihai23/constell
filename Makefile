.PHONY: proto-gen migrate-up migrate-down test docker-up docker-down build lint \
        build/ws-gateway test/ws-gateway run/ws-gateway \
        run/auth-service run/user-service run/community-service run/file-service \
        run/search-service run/notify-service run/api-gateway \
        infra-up infra-down \
        test/e2e test/e2e/ws test/e2e/file test/e2e/playwright test/e2e/all

# --- Common dev config ---
# Docker maps infra to non-default ports to avoid conflicts with local installs.
# When using infra-up, override these so services can connect.
DEV_DATABASE_URL := postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable
DEV_REDIS_URL    := localhost:16379
DEV_NATS_URL     := nats://localhost:4222
DEV_MINIO_ENDPOINT := localhost:9000
DEV_JWT_SECRET   := dev-secret-change-me

# --- Buf / Protobuf ---
proto-gen:
	buf generate

lint:
	buf lint

# --- Database Migrations ---
# Run from backend/ (where go.mod lives) against the Docker-mapped port 15432.
# Override with: make migrate-up DEV_DATABASE_URL=...
MIGRATE_DSN := $(DEV_DATABASE_URL)
MIGRATE_DIR := $(CURDIR)/deploy/migrations
migrate-up:
	cd backend && go run ./tools/migrate/main.go -dir $(MIGRATE_DIR) -dsn "$(MIGRATE_DSN)" up

migrate-down:
	cd backend && go run ./tools/migrate/main.go -dir $(MIGRATE_DIR) -dsn "$(MIGRATE_DSN)" down

# --- Tests ---
test:
	cd backend && go test ./...

test/ws-gateway:
	cd backend/services/ws-gateway && go test -v -count=1 ./...

test/all:
	cd backend && go test -v -count=1 ./...

# Integration tests (requires Docker Compose running)
test/integration:
	cd backend/tests/integration && go test -v -count=1 -timeout 180s ./...

# --- E2E Tests (requires Docker Compose running) ---

# All Go E2E tests (backend API + WebSocket)
test/e2e:
	cd backend/tests/integration && go test -v -count=1 -timeout 300s ./...

# WebSocket real-time E2E only
test/e2e/ws:
	cd backend/tests/integration && go test -v -count=1 -run TestWS -timeout 120s ./...

# File upload/download E2E only
test/e2e/file:
	cd backend/tests/integration && go test -v -count=1 -run TestFile -timeout 120s ./...

# Playwright browser E2E tests
test/e2e/playwright:
	cd clients/web && npx playwright test

# Run all E2E tests (Go + Playwright)
test/e2e/all:
	@echo "=== Running Go Backend E2E Tests ==="
	cd backend/tests/integration && go test -v -count=1 -timeout 300s ./...
	@echo "=== Running Playwright Browser E2E Tests ==="
	cd clients/web && npx playwright test

# --- Build ---
build:
	cd backend && go build ./pkg/... ./services/ws-gateway/... ./services/api-gateway/... ./services/auth-service/... ./services/user-service/... ./services/community-service/... ./tests/integration/...

build/ws-gateway:
	mkdir -p bin
	cd backend/services/ws-gateway && go build -o ../../../bin/ws-gateway .

# --- Run Locally (use infra-up first) ---
run/auth-service:
	cd backend/services/auth-service && \
		PORT=9081 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		REDIS_URL=$(DEV_REDIS_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		go run .

run/user-service:
	cd backend/services/user-service && \
		PORT=9082 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		REDIS_URL=$(DEV_REDIS_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		NATS_URL=$(DEV_NATS_URL) \
		REGISTRY_TYPE=static \
		SERVICES_CONFIG_PATH=deploy/configs/services.yaml \
		go run .

run/community-service:
	cd backend/services/community-service && \
		PORT=9083 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		REDIS_URL=$(DEV_REDIS_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		NATS_URL=$(DEV_NATS_URL) \
		REGISTRY_TYPE=static \
		SERVICES_CONFIG_PATH=deploy/configs/services.yaml \
		go run .

run/file-service:
	cd backend/services/file-service && \
		PORT=9084 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		MINIO_ENDPOINT=$(DEV_MINIO_ENDPOINT) \
		MINIO_ACCESS_KEY=minioadmin \
		MINIO_SECRET_KEY=minioadmin \
		MINIO_BUCKET=constell \
		MINIO_USE_SSL=false \
		MINIO_BASE_URL=http://localhost:9000 \
		go run .

run/search-service:
	cd backend/services/search-service && \
		PORT=9085 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		go run .

run/notify-service:
	cd backend/services/notify-service && \
		PORT=9086 \
		DATABASE_URL=$(DEV_DATABASE_URL) \
		REDIS_URL=$(DEV_REDIS_URL) \
		NATS_URL=$(DEV_NATS_URL) \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		go run .

run/api-gateway:
	cd backend/services/api-gateway && \
		GATEWAY_ADDR=:8080 \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		REGISTRY_TYPE=static \
		SERVICES_CONFIG_PATH=deploy/configs/services.yaml \
		go run .

run/ws-gateway:
	cd backend/services/ws-gateway && \
		LISTEN_ADDR=:8081 \
		REDIS_ADDR=$(DEV_REDIS_URL) \
		NATS_URL=$(DEV_NATS_URL) \
		USER_SERVICE_ADDR=http://localhost:9082 \
		COMMUNITY_SERVICE_ADDR=http://localhost:9083 \
		JWT_SECRET=$(DEV_JWT_SECRET) \
		GATEWAY_ID=ws-gateway-local \
		REGISTRY_TYPE=static \
		SERVICES_CONFIG_PATH=deploy/configs/services.yaml \
		go run .

# --- Docker Compose ---

# Start only infrastructure (postgres, redis, nats, minio, openobserve)
infra-up:
	docker compose -f deploy/docker/docker-compose.yml up -d postgres redis nats minio openobserve

# Stop infrastructure
infra-down:
	docker compose -f deploy/docker/docker-compose.yml down

# Start all services (infra + backend + web)
docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

docker-build:
	docker compose -f deploy/docker/docker-compose.yml build
