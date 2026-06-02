# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/api-gateway/go.mod ./backend/services/api-gateway/go.sum ./backend/services/api-gateway/

WORKDIR /app/backend/services/api-gateway
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/api-gateway/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /api-gateway .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /api-gateway .

EXPOSE 8080

CMD ["./api-gateway"]
