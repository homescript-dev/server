FROM golang:1.24.1-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o homescript-server ./cmd/server

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binary
COPY --from=builder /app/homescript-server .

# Create directories
RUN mkdir -p /config/devices /config/events /data

# Expose no ports (MQTT client only)

# Default command
CMD ["./homescript-server", "run"]
