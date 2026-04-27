#!/bin/sh
# scripts/backup.sh -- One-shot complete backup of a Chronicle deployment.
#
# Snapshots three things:
#   1. The MariaDB schema + data via mysqldump | gzip.
#   2. The media directory (uploads, avatars, packages) via tar | gzip.
#   3. Optionally, the Redis dataset via redis-cli --rdb (sessions only;
#      survivable, so this is best-effort).
#
# Each artifact gets a sibling chronicle_manifest_<TS>.txt listing
# SHA-256 sums + the migration version + the chronicle build version, so
# restore.sh can validate that the artifacts were produced from a
# consistent snapshot.
#
# Distinct from the in-process pre-migration backup
# (internal/database/healthcheck.go PreMigrationBackup) — that one fires
# automatically on every boot before migrations run, gated by BACKUP_DIR.
# This script is the operator-driven complete snapshot, suitable for cron
# and for pre-upgrade safety. Both write into $BACKUP_DIR; this script
# does not touch chronicle_pre_migrate_* files (those have their own
# rotator in the Go code).
#
# Invocation (compose, primary):
#   docker compose exec -T chronicle /app/scripts/backup.sh
# Invocation (standalone host):
#   DB_HOST=... DB_USER=... DB_PASSWORD=... DB_NAME=... \
#     MEDIA_PATH=/var/lib/chronicle/media BACKUP_DIR=/var/backups/chronicle \
#     ./scripts/backup.sh
#
# Args:
#   --check          Validate env + tool availability, then exit (0/non-0).
#                    No I/O. Cheap to run from CI or cron-precondition.
#   --out DIR        Override BACKUP_DIR for this run.
#   --no-media       Skip the media tarball.
#   --no-redis       Skip the Redis dump.
#   --retention N    Override BACKUP_RETENTION_DAYS for this run.
#
# Env contract:
#   BACKUP_DIR              (default /app/data/backups)
#   BACKUP_RETENTION_DAYS   (default 7)
#   DB_HOST                 (default localhost:3306; "host" or "host:port")
#   DB_USER                 (required)
#   DB_PASSWORD             (required)
#   DB_NAME                 (default chronicle)
#   MEDIA_PATH              (default /app/data/media)
#   REDIS_URL               (default redis://localhost:6379; optional)
#
# Exit codes: 0 success / 1 operator error / 2 precondition / 3 tool failure.
#
# Output: line-oriented `KEY=value` to stdout, suitable for cron-mailto +
# log scrapers. No spinner, no color. Final line is BACKUP=ok or
# BACKUP=failed reason=<short>.

set -eu

# ---- Args ----
CHECK_ONLY=0
OUT_OVERRIDE=""
SKIP_MEDIA=0
SKIP_REDIS=0
RETENTION_OVERRIDE=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --check) CHECK_ONLY=1 ;;
        --out) OUT_OVERRIDE="${2:-}"; shift ;;
        --no-media) SKIP_MEDIA=1 ;;
        --no-redis) SKIP_REDIS=1 ;;
        --retention) RETENTION_OVERRIDE="${2:-}"; shift ;;
        -h|--help)
            sed -n '1,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *)
            printf 'BACKUP=failed reason=unknown_arg arg=%s\n' "$1" >&2
            exit 1 ;;
    esac
    shift
done

# ---- Env ----
BACKUP_DIR="${OUT_OVERRIDE:-${BACKUP_DIR:-/app/data/backups}}"
BACKUP_RETENTION_DAYS="${RETENTION_OVERRIDE:-${BACKUP_RETENTION_DAYS:-7}}"
DB_HOST_RAW="${DB_HOST:-localhost:3306}"
DB_USER="${DB_USER:-}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-chronicle}"
MEDIA_PATH="${MEDIA_PATH:-/app/data/media}"
REDIS_URL="${REDIS_URL:-redis://localhost:6379}"

# Split DB_HOST into host + port (default 3306).
case "$DB_HOST_RAW" in
    *:*) DB_HOST="${DB_HOST_RAW%:*}"; DB_PORT="${DB_HOST_RAW##*:}" ;;
    *)   DB_HOST="$DB_HOST_RAW"; DB_PORT="3306" ;;
esac

# ---- Pre-flight ----
fail_pre() {
    printf 'BACKUP=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 2
}
fail_op() {
    printf 'BACKUP=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 1
}
fail_tool() {
    printf 'BACKUP=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 3
}

[ -n "$DB_USER" ] || fail_op env_missing "DB_USER not set"
[ -n "$DB_PASSWORD" ] || fail_op env_missing "DB_PASSWORD not set"

command -v mysqldump >/dev/null 2>&1 || \
    fail_tool tool_missing "mysqldump not on PATH (install mariadb-client)"
command -v gzip >/dev/null 2>&1 || \
    fail_tool tool_missing "gzip not on PATH"
command -v tar >/dev/null 2>&1 || \
    fail_tool tool_missing "tar not on PATH"
command -v sha256sum >/dev/null 2>&1 || \
    fail_tool tool_missing "sha256sum not on PATH (install coreutils)"

# Probe DB connectivity (a cheap query that doesn't dump). Stdout is
# discarded but stderr flows up to the parent so an operator who hits
# this can see the actual mysql error (auth, TLS, host unreachable)
# in the admin UI's "Show stderr" panel rather than only the generic
# "cannot connect to ..." line below.
if ! MYSQL_PWD="$DB_PASSWORD" mysql \
        -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
        -e "SELECT 1" "$DB_NAME" >/dev/null; then
    fail_pre db_unreachable "cannot connect to $DB_HOST:$DB_PORT/$DB_NAME as $DB_USER"
fi

# Ensure backup dir exists and is writable.
if ! mkdir -p "$BACKUP_DIR" 2>/dev/null; then
    fail_pre backup_dir_unwritable "cannot create $BACKUP_DIR"
fi
if [ ! -w "$BACKUP_DIR" ]; then
    fail_pre backup_dir_unwritable "$BACKUP_DIR not writable"
fi

if [ "$CHECK_ONLY" = "1" ]; then
    printf 'BACKUP=check_ok backup_dir=%s db=%s:%s/%s\n' \
        "$BACKUP_DIR" "$DB_HOST" "$DB_PORT" "$DB_NAME"
    exit 0
fi

# ---- Snapshot ----
TS="$(date -u +'%Y%m%dT%H%M%SZ')"
DB_OUT="$BACKUP_DIR/chronicle_db_${TS}.sql.gz"
DB_TMP="$DB_OUT.partial"
MEDIA_OUT="$BACKUP_DIR/chronicle_media_${TS}.tar.gz"
MEDIA_TMP="$MEDIA_OUT.partial"
REDIS_OUT="$BACKUP_DIR/chronicle_redis_${TS}.rdb"
REDIS_TMP="$REDIS_OUT.partial"
MANIFEST="$BACKUP_DIR/chronicle_manifest_${TS}.txt"
MIGRATION_VER=""
ARTIFACTS=""

cleanup_partials() {
    rm -f "$DB_TMP" "$MEDIA_TMP" "$REDIS_TMP" "$MANIFEST.partial" 2>/dev/null || true
}
trap cleanup_partials EXIT

# 1) DB dump.
# Two-step (dump → gzip) so we can read mysqldump's exit status
# without relying on PIPESTATUS (not POSIX). Trade some IO for
# correctness; gzip -9 is fast on a uncompressed dump.
DB_SQL_TMP="$BACKUP_DIR/chronicle_db_${TS}.sql.partial"
# Stderr intentionally NOT redirected to /dev/null: the admin UI captures
# the script's stderr and surfaces it in the "Show stderr" disclosure.
# Without the actual mysqldump error (privilege denied, server has gone
# away, version mismatch, etc.), an operator has no way to diagnose the
# generic "exited non-zero" message below.
if ! MYSQL_PWD="$DB_PASSWORD" mysqldump \
        -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
        --single-transaction --routines --triggers \
        --set-gtid-purged=OFF \
        "$DB_NAME" > "$DB_SQL_TMP"; then
    rm -f "$DB_SQL_TMP"
    fail_tool mysqldump_failed "mysqldump exited non-zero (see stderr above for the real reason)"
fi
if ! gzip -9 -c "$DB_SQL_TMP" > "$DB_TMP"; then
    rm -f "$DB_SQL_TMP" "$DB_TMP"
    fail_tool gzip_failed "gzip of db dump exited non-zero"
fi
rm -f "$DB_SQL_TMP"
mv "$DB_TMP" "$DB_OUT"
DB_SHA="$(sha256sum "$DB_OUT" | awk '{print $1}')"
DB_SIZE="$(wc -c < "$DB_OUT")"
ARTIFACTS="$ARTIFACTS db=$DB_OUT"
printf 'artifact=db file=%s size=%s sha256=%s\n' "$DB_OUT" "$DB_SIZE" "$DB_SHA"

# 1b) Capture migration version from the dump for the manifest.
MIGRATION_VER="$(MYSQL_PWD="$DB_PASSWORD" mysql \
    -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -N -B \
    -e "SELECT CONCAT(version, '/', dirty) FROM schema_migrations LIMIT 1" \
    "$DB_NAME" 2>/dev/null || echo "unknown")"

# 2) Media tarball.
MEDIA_SHA=""
MEDIA_SIZE=""
if [ "$SKIP_MEDIA" = "0" ] && [ -d "$MEDIA_PATH" ]; then
    MEDIA_PARENT="$(dirname "$MEDIA_PATH")"
    MEDIA_BASE="$(basename "$MEDIA_PATH")"
    # Stderr passes through so disk-full / permission errors are visible
    # in the admin UI rather than being lost.
    if ! tar -czf "$MEDIA_TMP" -C "$MEDIA_PARENT" "$MEDIA_BASE"; then
        rm -f "$MEDIA_TMP"
        fail_tool tar_failed "tar of $MEDIA_PATH exited non-zero (see stderr above)"
    fi
    mv "$MEDIA_TMP" "$MEDIA_OUT"
    MEDIA_SHA="$(sha256sum "$MEDIA_OUT" | awk '{print $1}')"
    MEDIA_SIZE="$(wc -c < "$MEDIA_OUT")"
    ARTIFACTS="$ARTIFACTS media=$MEDIA_OUT"
    printf 'artifact=media file=%s size=%s sha256=%s\n' \
        "$MEDIA_OUT" "$MEDIA_SIZE" "$MEDIA_SHA"
else
    printf 'artifact=media skipped reason=%s\n' \
        "$([ "$SKIP_MEDIA" = "1" ] && echo flag || echo no_media_dir)"
fi

# 3) Redis dump (optional, best-effort).
REDIS_SHA=""
REDIS_SIZE=""
if [ "$SKIP_REDIS" = "0" ] && command -v redis-cli >/dev/null 2>&1; then
    if redis-cli -u "$REDIS_URL" --rdb "$REDIS_TMP" >/dev/null 2>&1; then
        mv "$REDIS_TMP" "$REDIS_OUT"
        REDIS_SHA="$(sha256sum "$REDIS_OUT" | awk '{print $1}')"
        REDIS_SIZE="$(wc -c < "$REDIS_OUT")"
        ARTIFACTS="$ARTIFACTS redis=$REDIS_OUT"
        printf 'artifact=redis file=%s size=%s sha256=%s\n' \
            "$REDIS_OUT" "$REDIS_SIZE" "$REDIS_SHA"
    else
        rm -f "$REDIS_TMP"
        printf 'artifact=redis skipped reason=cli_failed (sessions only; non-fatal)\n'
    fi
else
    printf 'artifact=redis skipped reason=%s\n' \
        "$([ "$SKIP_REDIS" = "1" ] && echo flag || echo cli_missing)"
fi

# 4) Manifest. Drives restore.sh — same TS pairs DB + media + redis.
{
    printf 'chronicle_manifest_version=1\n'
    printf 'timestamp=%s\n' "$TS"
    printf 'chronicle_version=%s\n' "${CHRONICLE_VERSION:-unknown}"
    printf 'migration_version=%s\n' "$MIGRATION_VER"
    printf 'db_file=%s sha256=%s size=%s\n' \
        "$(basename "$DB_OUT")" "$DB_SHA" "$DB_SIZE"
    if [ -n "$MEDIA_SHA" ]; then
        printf 'media_file=%s sha256=%s size=%s\n' \
            "$(basename "$MEDIA_OUT")" "$MEDIA_SHA" "$MEDIA_SIZE"
    fi
    if [ -n "$REDIS_SHA" ]; then
        printf 'redis_file=%s sha256=%s size=%s\n' \
            "$(basename "$REDIS_OUT")" "$REDIS_SHA" "$REDIS_SIZE"
    fi
} > "$MANIFEST.partial"
mv "$MANIFEST.partial" "$MANIFEST"
printf 'artifact=manifest file=%s\n' "$MANIFEST"

# 5) Retention sweep. Mirrors the rotator pattern in
# internal/database/healthcheck.go:352-385: parse the timestamp out of
# the filename and compare against the cutoff. Glob across all four
# operator-script artifact families; never touches chronicle_pre_migrate_*
# (those are managed by the Go in-process rotator).
CUTOFF="$(date -u -d "${BACKUP_RETENTION_DAYS} days ago" +%Y%m%d 2>/dev/null \
    || date -u -v-"${BACKUP_RETENTION_DAYS}d" +%Y%m%d 2>/dev/null \
    || echo "")"
if [ -n "$CUTOFF" ]; then
    REMOVED=0
    for f in "$BACKUP_DIR"/chronicle_db_*.sql.gz \
             "$BACKUP_DIR"/chronicle_media_*.tar.gz \
             "$BACKUP_DIR"/chronicle_redis_*.rdb \
             "$BACKUP_DIR"/chronicle_manifest_*.txt; do
        [ -e "$f" ] || continue
        base="$(basename "$f")"
        # Extract the YYYYMMDD prefix from the timestamp portion.
        stamp="$(printf '%s' "$base" \
            | sed -n 's/.*_\([0-9]\{8\}\)T[0-9]\{6\}Z.*/\1/p')"
        [ -n "$stamp" ] || continue
        if [ "$stamp" -lt "$CUTOFF" ]; then
            rm -f "$f" && REMOVED=$((REMOVED + 1))
        fi
    done
    printf 'retention=ok pruned=%s cutoff_day=%s\n' "$REMOVED" "$CUTOFF"
else
    printf 'retention=skipped reason=no_date_arithmetic\n'
fi

trap - EXIT
printf 'BACKUP=ok\n'
