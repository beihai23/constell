# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/ws-gateway/go.mod ./backend/services/ws-gateway/go.sum ./backend/services/ws-gateway/

WORKDIR /app/backend/services/ws-gateway
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/ws-gateway/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /ws-gateway .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /ws-gateway .

EXPOSE 8081

CMD ["./ws-gateway"]
