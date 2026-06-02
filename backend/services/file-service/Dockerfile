# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/file-service/go.mod ./backend/services/file-service/go.sum ./backend/services/file-service/

WORKDIR /app/backend/services/file-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/file-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /file-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /file-service .

EXPOSE 9084

CMD ["./file-service"]
