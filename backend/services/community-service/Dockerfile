# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/community-service/go.mod ./backend/services/community-service/go.sum ./backend/services/community-service/

WORKDIR /app/backend/services/community-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/community-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /community-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /community-service .

EXPOSE 9083

CMD ["./community-service"]
