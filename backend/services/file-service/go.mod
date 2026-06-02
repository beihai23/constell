module github.com/constell/constell/backend/services/file-service

go 1.25.0

require github.com/constell/constell/backend/pkg v0.0.0-00010101000000-000000000000

require (
	connectrpc.com/connect v1.20.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/constell/constell/backend/pkg => ../../pkg
