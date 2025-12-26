# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN cd cmd/server && CGO_ENABLED=0 GOOS=linux go build -o /app/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/cfd-platform-backend .

# Copy frontend
COPY ../frontend ./frontend

EXPOSE 8080

CMD ["./cfd-platform-backend"]
