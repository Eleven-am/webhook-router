FROM golang:1.24-alpine AS builder

# Install build dependencies for CGO (required for SQLite)
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for multi-arch support
ARG TARGETOS
ARG TARGETARCH

# Build the application with embedded assets
# CGO is required for SQLite driver
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o webhook-router .

FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates sqlite tzdata

WORKDIR /app

# Copy only the binary (web assets are now embedded)
COPY --from=builder /app/webhook-router .

# Create data directory for database
RUN mkdir -p /data && \
    chown nobody:nobody /data

# Set environment variables with sensible defaults
ENV DATABASE_PATH=/data/webhook_router.db
ENV PORT=8080
ENV DEFAULT_QUEUE=webhooks
ENV LOG_LEVEL=info

# Use non-root user for security
USER nobody

# Expose port
EXPOSE 8080

# Create volume for persistent database
VOLUME ["/data"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT}/health || exit 1

CMD ["./webhook-router"]