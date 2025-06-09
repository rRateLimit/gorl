# Build stage
FROM golang:1.23-alpine AS builder

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gorl .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/gorl .

# Create a non-root user
RUN adduser -D -s /bin/sh gorluser
USER gorluser

# Expose port (if needed for future web interface)
EXPOSE 8080

# Command to run
ENTRYPOINT ["./gorl"]
