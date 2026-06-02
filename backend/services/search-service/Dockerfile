# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/search-service/go.mod ./backend/services/search-service/go.sum ./backend/services/search-service/

WORKDIR /app/backend/services/search-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/search-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /search-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /search-service .

EXPOSE 9085

CMD ["./search-service"]
