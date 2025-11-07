# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE} -s -w" \
    -o smtp-edge-proxy \
    ./cmd

# Runtime stage
FROM alpine:3.20

# Install CA certificates for TLS
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 smtp && \
    adduser -D -u 1000 -G smtp smtp

# Create directories
RUN mkdir -p /app /certs /config && \
    chown -R smtp:smtp /app /certs /config

# Copy binary from builder
COPY --from=builder --chown=smtp:smtp /build/smtp-edge-proxy /app/smtp-edge-proxy

# Set working directory
WORKDIR /app

# Switch to non-root user
USER smtp

# Expose ports
EXPOSE 587 465 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Set entrypoint
ENTRYPOINT ["/app/smtp-edge-proxy"]

# Default command (can be overridden)
CMD ["-config", "/config/config.yaml"]
