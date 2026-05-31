module github.com/constell/constell/backend/services/ws-gateway

go 1.25.0

require (
	github.com/constell/constell/backend/pkg v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
	google.golang.org/protobuf v1.36.11
)

replace github.com/constell/constell/backend/pkg => ../../pkg
