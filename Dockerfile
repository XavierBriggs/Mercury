# ==============================================================================
# Multi-stage Dockerfile for Mercury (Odds Aggregator)
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Builder
# ------------------------------------------------------------------------------
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o mercury \
    ./cmd/mercury

# ------------------------------------------------------------------------------
# Stage 2: Runtime
# ------------------------------------------------------------------------------
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 mercury && \
    adduser -D -u 1000 -G mercury mercury

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/mercury .

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set ownership
RUN chown -R mercury:mercury /app

# Switch to non-root user
USER mercury

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
    CMD pgrep mercury || exit 1

# Expose metrics port (if added later)
# EXPOSE 9090

# Run the binary
ENTRYPOINT ["/app/mercury"]

