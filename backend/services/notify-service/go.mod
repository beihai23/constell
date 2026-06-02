module github.com/constell/constell/backend/services/notify-service

go 1.25.0

require (
	github.com/alicebob/miniredis/v2 v2.38.0
	github.com/constell/constell/backend/pkg v0.0.0-00010101000000-000000000000
	github.com/redis/go-redis/v9 v9.20.0
)

require (
	connectrpc.com/connect v1.20.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/constell/constell/backend/pkg => ../../pkg
