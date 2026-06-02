# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY backend/go.work ./backend/go.work
COPY backend/pkg/go.mod ./backend/pkg/go.sum ./backend/pkg/
COPY backend/services/user-service/go.mod ./backend/services/user-service/go.sum ./backend/services/user-service/

WORKDIR /app/backend/services/user-service
RUN go mod download

COPY backend/pkg/ ./../../pkg/
COPY backend/services/user-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /user-service .

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /user-service .

EXPOSE 9082

CMD ["./user-service"]
