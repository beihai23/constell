.PHONY: proto-gen migrate-up migrate-down test docker-up docker-down build lint \
        build/ws-gateway test/ws-gateway run/ws-gateway

# --- Buf / Protobuf ---
proto-gen:
	buf generate

lint:
	buf lint

# --- Database Migrations ---
migrate-up:
	go run ./backend/tools/migrate/main.go up

migrate-down:
	go run ./backend/tools/migrate/main.go down

# --- Tests ---
test:
	cd backend && go test ./...

test/ws-gateway:
	cd backend/services/ws-gateway && go test -v -count=1 ./...

test/all:
	cd backend && go test -v -count=1 ./...

# --- Build ---
build:
	cd backend && go build ./pkg/... ./services/ws-gateway/... ./services/api-gateway/... ./services/auth-service/... ./services/user-service/... ./services/community-service/... ./tests/integration/...

build/ws-gateway:
	mkdir -p bin
	cd backend/services/ws-gateway && go build -o ../../../bin/ws-gateway .

# --- Run Locally ---
run/ws-gateway:
	cd backend/services/ws-gateway && \
		LISTEN_ADDR=:8081 \
		REDIS_ADDR=localhost:6379 \
		NATS_URL=nats://localhost:4222 \
		USER_SERVICE_ADDR=http://localhost:9082 \
		COMMUNITY_SERVICE_ADDR=http://localhost:9083 \
		JWT_SECRET=dev-secret-change-me \
		GATEWAY_ID=ws-gateway-local \
		go run .

# --- Docker Compose ---
docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

docker-build:
	docker compose -f deploy/docker/docker-compose.yml build
