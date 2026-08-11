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

# Install templ CLI for generating Go code from .templ files. Pin to the
# runtime version in go.mod — `@latest` drifts ahead and emits symbols
# the pinned runtime doesn't have (and newer templ now requires Go 1.25,
# which would force a base-image bump). Keep generator and runtime in
# lockstep; bumping templ is a deliberate change in three places
# (go.mod, ci.yml, Dockerfile).
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001

# Install git so the Go toolchain can stamp the source revision into the
# binary. This is the whole fix for "the binary cannot say which commit built
# it", and it needs no -ldflags: given `git` on PATH and a checkout to look at,
# Go records vcs.revision / vcs.time / vcs.modified in the binary's BuildInfo
# by itself, and internal/hostinfo reads them (GET /api/version and the
# host.build admin diagnostic).
#
# WHY it was missing: golang:1.24-alpine's only `apk add` is ca-certificates —
# no git — and when the VCS tool is absent Go SKIPS STAMPING SILENTLY. Measured:
# build exits 0, the binary carries zero vcs.* settings, and Main.Version is the
# literal "(devel)". So every Chronicle image ever shipped contained a binary
# with no idea what it was, which is why the 2026-08-11 incident had to reason
# from image labels instead — and reasoned wrong. `COPY . /src` already brings
# .git along (there is no .dockerignore anywhere in the tree), so the checkout
# is here; only the tool was missing.
#
# The safe.directory line is load-bearing, not boilerplate. Git refuses to
# operate on a repository owned by another user, and `go build` escalates that
# refusal into a HARD BUILD FAILURE rather than falling back to an unstamped
# binary. Measured, on a foreign-owned checkout: `error obtaining VCS status:
# exit status 128` and exit 1; with this exception configured, the same
# checkout builds AND stamps. Installing git therefore converts a class of git
# error from "no stamp" into "no image", and this line removes that class's
# likeliest member. (Absence of .git stays harmless either way — also measured:
# git installed but no repository present builds fine and reports "(devel)".)
RUN apk add --no-cache git \
    && git config --global --add safe.directory /src

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

# --- Build metadata ---
# Declared LAST of the mutable bits so that changing them re-runs only these
# cheap layers, not the apk/adduser ones above.
#
# All three default to empty, and empty is a deliberate, honest answer rather
# than an oversight: a locally built image genuinely does not know its
# provenance unless the builder tells it. Do not substitute a plausible-looking
# default here — a guessed revision is precisely the failure this whole change
# exists to prevent.
ARG VCS_REF=""
ARG BUILD_DATE=""
ARG CHRONICLE_VERSION=""

# OCI labels. WHY the Dockerfile sets these when CI already does: it did NOT
# already do it for every image. `docker/metadata-action` labels only the
# images IT builds, and alpine:3.20 contributes nothing to inherit (its
# config.Labels is null), so an image from `docker compose build` carried no
# org.opencontainers.* labels at all. These lines are the floor for every
# image however it was produced; in CI, build-push-action's `--label` flags
# override them with the same facts computed from the workflow context.
#
# Read the description before you trust the revision. On 2026-08-11 an operator
# read `org.opencontainers.image.revision` off a `:latest` tag, found a
# six-month-old commit, and concluded the running binary was stale. The label
# was accurate — about the image that happened to hold that tag on that host.
# The container was running a different image entirely. A label is a claim
# about an artifact made by whoever last wrote the tag; it is never a claim
# about a process. So the pointer to the authoritative answer ships in the
# image, where the person making that mistake is already looking.
LABEL org.opencontainers.image.title="Chronicle" \
      org.opencontainers.image.source="https://github.com/keyxmakerx/chronicle" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${CHRONICLE_VERSION}" \
      org.opencontainers.image.description="Self-hosted TTRPG worldbuilding platform. These labels describe an IMAGE, not a running container - docker inspect on a tag answers for whichever image holds that tag now, which may not be the image a running container was created from. To identify a RUNNING Chronicle, ask the process - GET /api/version, or Admin > Diagnostics > host.build. See docs/deployment.md section 6."

# Highest-precedence input to internal/hostinfo's version resolution
# (CHRONICLE_VERSION -> compiled-in VCS revision -> main module version ->
# "unknown"). Empty is the normal case and falls straight through to the VCS
# revision that stage 2 now stamps in, so this is a way to NAME a release, not
# a second place the commit is recorded. CI passes it only for tag builds; a
# main-branch push leaves it empty on purpose, because the branch tag "latest"
# is not a version and reporting it as one would be a downgrade from the SHA.
ENV CHRONICLE_VERSION=${CHRONICLE_VERSION}

# The Go binary serves HTTP directly on this port.
EXPOSE 8080

# Health check endpoint (implemented in the app).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

# Container starts as root; the entrypoint fixes permissions then exec's
# the server as the chronicle user.
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["chronicle"]
