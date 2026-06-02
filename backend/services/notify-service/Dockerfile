# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/notify-service/go.mod ./backend/services/notify-service/go.sum ./backend/services/notify-service/

WORKDIR /app/backend/services/notify-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/notify-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /notify-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /notify-service .

EXPOSE 9086

CMD ["./notify-service"]
