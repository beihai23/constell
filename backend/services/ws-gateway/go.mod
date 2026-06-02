module github.com/constell/constell/backend/services/ws-gateway

go 1.25.0

require (
	connectrpc.com/connect v1.20.0
	github.com/constell/constell/backend/pkg v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
	github.com/nats-io/nats.go v1.52.0
	github.com/redis/go-redis/v9 v9.20.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/constell/constell/backend/pkg => ../../pkg
