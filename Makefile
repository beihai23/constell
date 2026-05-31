.PHONY: proto-gen migrate-up migrate-down test docker-up docker-down build lint

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

# --- Docker Compose ---
docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

# --- Build ---
build:
	cd backend && go build ./...
