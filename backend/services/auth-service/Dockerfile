# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/auth-service/go.mod ./backend/services/auth-service/go.sum ./backend/services/auth-service/

WORKDIR /app/backend/services/auth-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/auth-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /auth-service .

EXPOSE 9081

CMD ["./auth-service"]
