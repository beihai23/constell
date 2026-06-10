# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy workspace file and all module definitions so go.work use directives resolve.
COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod backend/pkg/go.sum ./backend/pkg/
COPY backend/services/api-gateway/go.mod backend/services/api-gateway/go.sum ./backend/services/api-gateway/
COPY backend/services/auth-service/go.mod backend/services/auth-service/go.sum ./backend/services/auth-service/
COPY backend/services/community-service/go.mod backend/services/community-service/go.sum ./backend/services/community-service/
COPY backend/services/file-service/go.mod backend/services/file-service/go.sum ./backend/services/file-service/
COPY backend/services/notify-service/go.mod backend/services/notify-service/go.sum ./backend/services/notify-service/
COPY backend/services/search-service/go.mod backend/services/search-service/go.sum ./backend/services/search-service/
COPY backend/services/user-service/go.mod backend/services/user-service/go.sum ./backend/services/user-service/
COPY backend/services/ws-gateway/go.mod backend/services/ws-gateway/go.sum ./backend/services/ws-gateway/
COPY backend/tests/integration/go.mod backend/tests/integration/go.sum ./backend/tests/integration/
COPY backend/tools/migrate/go.mod backend/tools/migrate/go.sum ./backend/tools/migrate/

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
