# Build stage
FROM golang:1.21-alpine AS builder

# Install git for fetching dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o gorl ./cmd/gorl

# Final stage
FROM alpine:3.18

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create a non-root user
RUN addgroup -g 1001 -S gorl && \
    adduser -u 1001 -S gorl -G gorl

# Set working directory
WORKDIR /home/gorl

# Copy the binary from builder stage
COPY --from=builder /app/gorl .

# Copy example config file
COPY --from=builder /app/config.example.json .

# Change ownership to non-root user
RUN chown -R gorl:gorl /home/gorl

# Switch to non-root user
USER gorl

# Expose no ports as this is a client tool
# The application will make outbound HTTP requests

# Add labels for metadata
LABEL maintainer="GoRL Team"
LABEL description="Go Rate Limiter - A tool for testing HTTP request rate limits"
LABEL version="latest"

# Set environment variables with defaults
ENV GORL_RATE=1.0
ENV GORL_DURATION=10s
ENV GORL_CONCURRENCY=1
ENV GORL_ALGORITHM=token-bucket
ENV GORL_HTTP_TIMEOUT=30s
ENV GORL_TCP_KEEP_ALIVE=true
ENV GORL_TCP_KEEP_ALIVE_PERIOD=30s
ENV GORL_DISABLE_KEEP_ALIVES=false
ENV GORL_MAX_IDLE_CONNS=100
ENV GORL_MAX_IDLE_CONNS_PER_HOST=10

# Health check (optional - checks if binary exists and is executable)
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ./gorl -help > /dev/null 2>&1 || exit 1

# Default entrypoint
ENTRYPOINT ["./gorl"]

# Default command shows help
CMD ["-help"]
