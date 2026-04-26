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

# Install CA certificates for HTTPS calls, timezone data, su-exec for
# dropping privileges in the entrypoint, and the backup-toolchain
# packages so the in-process PreMigrationBackup
# (internal/database/pre_migration_backup.go) and the operator
# scripts/backup.sh actually have mysqldump, gzip, and redis-cli on
# PATH. Without these:
#   - mariadb-client → PreMigrationBackup WARN-skips (or aborts boot
#     when BACKUP_REQUIRED=1) and operators who set BACKUP_DIR get
#     no DB rollback.
#   - redis-tools   → Redis snapshots are skipped (sessions are
#     recoverable, so non-fatal, but the safety net is incomplete).
#   - gzip          → tarballs and SQL dumps are uncompressed.
# Total cost: ~+18 MB.
RUN apk add --no-cache ca-certificates tzdata su-exec mariadb-client redis gzip

# Create non-root user with a fixed UID/GID for predictable bind-mount
# ownership. Host dirs must be owned by this UID for non-root operation.
RUN addgroup -g 1000 chronicle \
    && adduser -D -H -s /sbin/nologin -G chronicle -u 1000 chronicle

# Copy the compiled binary.
COPY --from=builder /chronicle /usr/local/bin/chronicle

# Copy static assets (CSS, JS, vendor libs, fonts, images).
COPY --from=builder /src/static /app/static

# Copy database migrations for auto-migration on startup.
COPY --from=builder /src/db/migrations /app/db/migrations

# Copy operator scripts (backup, restore). Invoked via
# `docker compose exec chronicle /app/scripts/backup.sh` — see
# docs/deployment.md for the full operator runbook.
COPY --from=builder /src/scripts /app/scripts
RUN chmod +x /app/scripts/*.sh

# Create persistent data directory owned by the chronicle user.
# Media uploads go under /app/data/media (matches MEDIA_PATH default "./data/media").
# Mount a volume at /app/data to persist media across container rebuilds.
RUN mkdir -p /app/data/media \
    && chown -R chronicle:chronicle /app/data

WORKDIR /app

# Copy entrypoint script that fixes bind-mount permissions, then drops to
# the unprivileged chronicle user via su-exec.
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# The Go binary serves HTTP directly on this port.
EXPOSE 8080

# Health check endpoint (implemented in the app).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

# Container starts as root; the entrypoint fixes permissions then exec's
# the server as the chronicle user.
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["chronicle"]
