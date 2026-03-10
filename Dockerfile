# ============================================================================
# Chronicle Dockerfile -- Multi-Stage Build
# ============================================================================
# Stage 1: Build Tailwind CSS (standalone binary, no Node.js)
# Stage 2: Generate Templ Go files + compile Go binary
# Stage 3: Minimal runtime image (~30MB)
# ============================================================================

# --- Stage 1: Tailwind CSS ---
FROM alpine:3.20 AS tailwind

# The standalone Tailwind CLI is a glibc binary; install compat layer for Alpine.
RUN apk add --no-cache libc6-compat \
    && wget -O /usr/local/bin/tailwindcss \
    https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64 \
    && chmod +x /usr/local/bin/tailwindcss

COPY . /src
WORKDIR /src

# Generate minified CSS from Tailwind input.
RUN tailwindcss -i static/css/input.css -o static/css/app.css --minify

# --- Stage 2: Go Build ---
FROM golang:1.24-alpine AS builder

# Install templ CLI for generating Go code from .templ files.
RUN go install github.com/a-h/templ/cmd/templ@latest

COPY . /src
# Copy the generated Tailwind CSS from stage 1.
COPY --from=tailwind /src/static/css/app.css /src/static/css/app.css

WORKDIR /src

# Generate Go code from Templ templates.
RUN templ generate

# Build the Go binary. CGO disabled for a fully static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -o /chronicle ./cmd/server

# --- Stage 3: Runtime ---
FROM alpine:3.20

# Install CA certificates for HTTPS calls (if needed), timezone data, and
# su-exec for dropping privileges in the entrypoint.
RUN apk add --no-cache ca-certificates tzdata su-exec

# Create non-root user for runtime security.
RUN adduser -D -H -s /sbin/nologin chronicle

# Copy the compiled binary.
COPY --from=builder /chronicle /usr/local/bin/chronicle

# Copy static assets (CSS, JS, vendor libs, fonts, images).
COPY --from=builder /src/static /app/static

# Copy database migrations for auto-migration on startup.
COPY --from=builder /src/db/migrations /app/db/migrations

# Create persistent data directory owned by the non-root user.
# Media uploads go under /app/data/media (matches MEDIA_PATH default "./data/media").
# Mount a volume at /app/data to persist media across container rebuilds.
RUN mkdir -p /app/data/media && chown -R chronicle:chronicle /app/data

# Copy entrypoint script that fixes bind-mount permissions before dropping to
# the non-root chronicle user.
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

WORKDIR /app

# Container starts as root so the entrypoint can fix bind-mount ownership,
# then drops to the chronicle user via su-exec.
ENTRYPOINT ["docker-entrypoint.sh"]

# The Go binary serves HTTP directly on this port.
EXPOSE 8080

# Health check endpoint (implemented in the app).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

# Run the Chronicle server.
CMD ["chronicle"]
