# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Constell is an open-source community IM system (similar to Discord) built with Go microservices. It uses **Connect-RPC** (not gRPC-gateway) for service-to-service communication, **Buf** for protobuf management, and a custom **groupcache** library for stateful services.

## Build & Development Commands

```bash
make proto-gen          # Generate protobuf Go code from proto/ via buf
make lint               # Lint proto files
make migrate-up         # Run DB migrations (deploy/migrations/)
make migrate-down       # Rollback DB migrations
make test               # Run all tests (short output)
make test/all           # Run all tests with -v -count=1
make test/ws-gateway    # Run ws-gateway tests with verbose output
make build              # Build all services and shared packages
make build/ws-gateway   # Build ws-gateway binary to bin/
make run/ws-gateway     # Run ws-gateway locally with dev env vars
make docker-up          # Start infra services (postgres, redis, nats, minio, 2x ws-gateway)
make docker-down        # Stop all Docker services
```

Run a single test: `cd backend && go test -v -run TestFuncName ./services/ws-gateway/...`

## Architecture

```
proto/                          # Protobuf definitions (Buf module root)
  auth/v1, user/v1, community/v1, gateway/v1, common/v1
backend/
  go.work                       # Go workspace — all services + pkg + tools + tests
  pkg/                          # Shared packages (single Go module)
    proto/                      # Generated protobuf + Connect-RPC code (do not edit)
    groupcache/                 # Generic LRU cache with consistent hashing + singleflight
    jwt/                        # JWT token creation and validation
    middleware/                 # Connect-RPC auth middleware
    nats/                       # NATS JetStream helper
    postgres/                   # PostgreSQL connection helper
    redis/                      # Redis connection helper
  services/
    api-gateway/(:8080)         # REST API entry point, proxies to backend services via Connect-RPC
    ws-gateway/(:8081)          # Stateful WebSocket gateway — holds conn map, Redis registry, NATS subscriber
    auth-service/(:9081)        # Stateless — registration, login, token refresh
    user-service/(:9082)        # Stateful (groupcache) — user profiles, DMs, relations
    community-service/(:9083)   # Stateful (groupcache) — servers, channels, roles, permissions
  tools/migrate/                # Database migration runner
  tests/integration/            # Cross-service integration tests
deploy/
  configs/dev.yaml              # Dev config (DB, Redis, NATS, MinIO, service addresses)
  docker/docker-compose.yml     # Infra services + 2 ws-gateway instances
  migrations/                   # SQL migration files (001–009)
```

### Key Design Decisions

- **Connect-RPC** for RPC (not gRPC-gateway). Generated code lives in `backend/pkg/proto/`.
- **Custom groupcache** (`backend/pkg/groupcache/`) provides consistent-hash partitioning, peer-to-peer fill, singleflight dedup, and automatic failover. Used by user-service and community-service for in-memory caching.
- **WS Gateway** is the only stateful gateway — it holds a connection map, registers presence in Redis, and subscribes to NATS push subjects for real-time delivery. Multiple instances can run for horizontal scaling.
- **API Gateway** is stateless — it translates REST requests into Connect-RPC calls to backend services.
- All services read config from **environment variables** with sensible defaults matching `deploy/configs/dev.yaml`.

### Service Communication

API Gateway and WS Gateway call backend services (auth, user, community) via **Connect-RPC over HTTP**. Backend services communicate through **NATS JetStream** for async events (e.g., push notifications for new messages). WS Gateway instances subscribe to NATS subjects to deliver real-time events to connected clients.

## Proto Code Generation

After changing `proto/*.proto`, run `make proto-gen` to regenerate Go code into `backend/pkg/proto/`. The project uses `buf.gen.yaml` with `protocolbuffers/go` and `connectrpc/go` plugins.

## Database

PostgreSQL with sequential SQL migrations in `deploy/migrations/`. Run `make migrate-up` after schema changes. Dev connection: `postgres://constell:constell_dev@localhost:5432/constell` (port 15432 in Docker).

## Dependencies

| Service | Purpose |
|---------|---------|
| PostgreSQL 16 | Primary database |
| Redis 7 | Session storage, WS gateway presence registry |
| NATS 2 (JetStream) | Async event delivery between services |
| MinIO | S3-compatible file storage (planned) |
