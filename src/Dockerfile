# =============================================================================
# SimpleHTTPRedirector - Multi-stage Build
# Lightweight HTTP redirect service with JSON configuration
# =============================================================================

# === Build Stage ===
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

# Copy go module files first for better caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY main.go ./
COPY templates/ ./templates/

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o redirector main.go

# === Runtime Stage ===
FROM alpine:3.23

# Metadata
LABEL vendor="BAUER GROUP"
LABEL maintainer="Karl Bauer <karl.bauer@bauer-group.com>"

# Opencontainers Metadata
LABEL org.opencontainers.image.title="SimpleHTTPRedirector"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.vendor="BAUER GROUP"
LABEL org.opencontainers.image.authors="Karl Bauer <karl.bauer@bauer-group.com>"
LABEL org.opencontainers.image.source="https://github.com/bauer-group/CS-SimpleHTTPRedirector"
LABEL org.opencontainers.image.url="https://github.com/bauer-group/CS-SimpleHTTPRedirector"
LABEL org.opencontainers.image.documentation="https://github.com/bauer-group/CS-SimpleHTTPRedirector#readme"
LABEL org.opencontainers.image.description="Lightweight HTTP redirect service with JSON-based multi-rule configuration"
LABEL org.opencontainers.image.version="0.1.1"

# Install runtime dependencies (curl for healthcheck, ca-certificates for HTTPS)
RUN apk add --no-cache ca-certificates curl tzdata \
    && adduser -D -u 1000 app

# Setup application directory
WORKDIR /app

# Copy binary and templates
COPY --from=builder /build/redirector /app/redirector
COPY --from=builder /build/templates/ /app/templates/

# Create config directory
RUN mkdir -p /app/config && chown -R app:app /app

# Environment
ENV TZ=UTC
ENV CONFIG_PATH=/app/config/redirects.json
ENV TEMPLATE_PATH=/app/templates/error.html
ENV SERVER_PORT=8080
ENV TRUST_PROXY=true

# Expose default port
EXPOSE 8080/tcp

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -sf http://localhost:${SERVER_PORT}/health || exit 1

# Run as non-root
USER app

# Entrypoint
ENTRYPOINT ["/app/redirector"]
