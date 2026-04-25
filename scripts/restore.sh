#!/bin/sh
# scripts/restore.sh -- Restore a Chronicle deployment from a backup set.
#
# Operator-driven, multi-gate restore. Distinct from the in-process
# pre-migration backup recovery (which is just "use the most recent
# chronicle_pre_migrate_*.sql.gz" — see docs/deployment.md §7 Scenario
# A). This script handles full snapshots produced by scripts/backup.sh.
#
# Why a single command with gates rather than multiple sequential
# scripts: a single entry point is what the operator types at 2 AM
# under stress. Safety comes from explicit confirmation flags and
# pre-flight checks, not from forcing the operator to remember a
# sequence of unique commands.
#
# The restore does NOT run migrations — it just recreates DB state,
# extracts media, and (optionally) replaces the Redis dump. Validation
# of schema-vs-code happens automatically when chronicle next starts:
# RunStartupHealthChecks (internal/database/healthcheck.go:92-133)
# refuses to boot if the restored DB doesn't match the running
# image's expected migration version. No --migrate-only flag needed.
#
# Invocation (compose, primary):
#   docker compose exec chronicle /app/scripts/restore.sh \
#     --manifest /app/data/backups/chronicle_manifest_<TS>.txt
# Invocation (standalone host):
#   DB_HOST=... DB_USER=... DB_PASSWORD=... DB_NAME=... \
#     MEDIA_PATH=... ./scripts/restore.sh --manifest <PATH>
#
# Args:
#   --manifest PATH  REQUIRED. The chronicle_manifest_<TS>.txt that
#                    drives the restore. Pairing DB + media + redis
#                    artifacts via a manifest prevents the "I restored
#                    last week's DB and last month's media" class of
#                    bug.
#   --db-only        Skip media tarball extraction.
#   --media-only     Skip DB restore.
#   --yes            Skip the interactive RESTORE confirmation.
#   --force          Allow restore over a non-empty target. Default is
#                    to refuse if DB has tables or MEDIA_PATH is non-empty.
#
# Exit codes: 0 success / 1 operator error / 2 precondition / 3 tool failure.

set -eu

# ---- Args ----
MANIFEST=""
DB_ONLY=0
MEDIA_ONLY=0
ASSUME_YES=0
FORCE=0

while [ "$#" -gt 0 ]; do
    case "$1" in
        --manifest) MANIFEST="${2:-}"; shift ;;
        --db-only) DB_ONLY=1 ;;
        --media-only) MEDIA_ONLY=1 ;;
        --yes) ASSUME_YES=1 ;;
        --force) FORCE=1 ;;
        -h|--help)
            sed -n '1,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *)
            printf 'RESTORE=failed reason=unknown_arg arg=%s\n' "$1" >&2
            exit 1 ;;
    esac
    shift
done

fail_op() {
    printf 'RESTORE=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 1
}
fail_pre() {
    printf 'RESTORE=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 2
}
fail_tool() {
    printf 'RESTORE=failed reason=%s detail=%s\n' "$1" "$2" >&2
    exit 3
}

[ -n "$MANIFEST" ] || fail_op manifest_required "use --manifest <path>"
[ -f "$MANIFEST" ] || fail_op manifest_missing "$MANIFEST not found"

if [ "$DB_ONLY" = "1" ] && [ "$MEDIA_ONLY" = "1" ]; then
    fail_op flag_conflict "--db-only and --media-only are mutually exclusive"
fi

# ---- Env ----
DB_HOST_RAW="${DB_HOST:-localhost:3306}"
DB_USER="${DB_USER:-}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-chronicle}"
MEDIA_PATH="${MEDIA_PATH:-/app/data/media}"
REDIS_URL="${REDIS_URL:-redis://localhost:6379}"

case "$DB_HOST_RAW" in
    *:*) DB_HOST="${DB_HOST_RAW%:*}"; DB_PORT="${DB_HOST_RAW##*:}" ;;
    *)   DB_HOST="$DB_HOST_RAW"; DB_PORT="3306" ;;
esac

[ -n "$DB_USER" ] || fail_op env_missing "DB_USER not set"
[ -n "$DB_PASSWORD" ] || fail_op env_missing "DB_PASSWORD not set"

# ---- Tools ----
command -v gunzip >/dev/null 2>&1 || \
    fail_tool tool_missing "gunzip not on PATH"
command -v mysql >/dev/null 2>&1 || \
    fail_tool tool_missing "mysql not on PATH (install mariadb-client)"
command -v tar >/dev/null 2>&1 || \
    fail_tool tool_missing "tar not on PATH"
command -v sha256sum >/dev/null 2>&1 || \
    fail_tool tool_missing "sha256sum not on PATH"

# ---- Parse manifest ----
MANIFEST_DIR="$(dirname "$MANIFEST")"
DB_FILE=""
DB_SHA=""
MEDIA_FILE=""
MEDIA_SHA=""
REDIS_FILE=""
REDIS_SHA=""
MIGRATION_VER=""

# shellcheck disable=SC2002
while IFS= read -r line; do
    case "$line" in
        db_file=*)
            DB_FILE="${line#db_file=}"
            DB_FILE="${DB_FILE%% *}"
            DB_SHA="$(printf '%s' "$line" | sed -n 's/.*sha256=\([^ ]*\).*/\1/p')"
            ;;
        media_file=*)
            MEDIA_FILE="${line#media_file=}"
            MEDIA_FILE="${MEDIA_FILE%% *}"
            MEDIA_SHA="$(printf '%s' "$line" | sed -n 's/.*sha256=\([^ ]*\).*/\1/p')"
            ;;
        redis_file=*)
            REDIS_FILE="${line#redis_file=}"
            REDIS_FILE="${REDIS_FILE%% *}"
            REDIS_SHA="$(printf '%s' "$line" | sed -n 's/.*sha256=\([^ ]*\).*/\1/p')"
            ;;
        migration_version=*)
            MIGRATION_VER="${line#migration_version=}"
            ;;
    esac
done < "$MANIFEST"

[ -n "$DB_FILE" ] || fail_op manifest_invalid "manifest has no db_file entry"

# ---- Pre-flight ----
# 1. Refuse if a chronicle server is running. The 8080/healthz probe is
# the most general check (works inside the container too via localhost).
if command -v wget >/dev/null 2>&1; then
    if wget -qO- "http://localhost:8080/healthz" >/dev/null 2>&1; then
        fail_pre server_running \
            "chronicle is up at localhost:8080/healthz; stop it first (docker compose stop chronicle)"
    fi
fi

# 2. Verify artifacts exist and SHAs match.
if [ "$MEDIA_ONLY" = "0" ]; then
    [ -f "$MANIFEST_DIR/$DB_FILE" ] || \
        fail_pre artifact_missing "$MANIFEST_DIR/$DB_FILE not found"
    actual="$(sha256sum "$MANIFEST_DIR/$DB_FILE" | awk '{print $1}')"
    [ "$actual" = "$DB_SHA" ] || \
        fail_pre sha_mismatch "db artifact SHA does not match manifest"
fi
if [ "$DB_ONLY" = "0" ] && [ -n "$MEDIA_FILE" ]; then
    [ -f "$MANIFEST_DIR/$MEDIA_FILE" ] || \
        fail_pre artifact_missing "$MANIFEST_DIR/$MEDIA_FILE not found"
    actual="$(sha256sum "$MANIFEST_DIR/$MEDIA_FILE" | awk '{print $1}')"
    [ "$actual" = "$MEDIA_SHA" ] || \
        fail_pre sha_mismatch "media artifact SHA does not match manifest"
fi

# 3. Refuse to overwrite a non-empty target unless --force.
if [ "$FORCE" = "0" ] && [ "$MEDIA_ONLY" = "0" ]; then
    table_count="$(MYSQL_PWD="$DB_PASSWORD" mysql \
        -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -N -B \
        -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME'" \
        2>/dev/null || echo "0")"
    if [ "${table_count:-0}" -gt 0 ]; then
        fail_pre target_not_empty \
            "DB '$DB_NAME' has $table_count tables; pass --force to overwrite"
    fi
fi
if [ "$FORCE" = "0" ] && [ "$DB_ONLY" = "0" ] && [ -n "$MEDIA_FILE" ]; then
    if [ -d "$MEDIA_PATH" ] && [ -n "$(ls -A "$MEDIA_PATH" 2>/dev/null || true)" ]; then
        fail_pre target_not_empty \
            "$MEDIA_PATH is not empty; pass --force to overwrite"
    fi
fi

# ---- Confirmation ----
if [ "$ASSUME_YES" = "0" ]; then
    printf 'About to restore from %s\n' "$MANIFEST"
    [ "$MEDIA_ONLY" = "0" ] && printf '  - DROP and recreate DB %s on %s:%s\n' \
        "$DB_NAME" "$DB_HOST" "$DB_PORT"
    [ "$DB_ONLY" = "0" ] && [ -n "$MEDIA_FILE" ] && \
        printf '  - extract media tarball over %s\n' "$MEDIA_PATH"
    [ -n "$REDIS_FILE" ] && \
        printf '  - replace Redis dataset (manual step; see deployment.md)\n'
    printf 'Type RESTORE to proceed: '
    read -r confirm
    [ "$confirm" = "RESTORE" ] || fail_op confirmation_declined "did not type RESTORE"
fi

# ---- DB restore ----
if [ "$MEDIA_ONLY" = "0" ]; then
    printf 'step=db_drop\n'
    if ! MYSQL_PWD="$DB_PASSWORD" mysql \
            -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
            -e "DROP DATABASE IF EXISTS \`$DB_NAME\`; CREATE DATABASE \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"; then
        fail_tool db_drop_failed "DROP/CREATE DATABASE failed"
    fi
    printf 'step=db_load file=%s\n' "$DB_FILE"
    if ! gunzip -c "$MANIFEST_DIR/$DB_FILE" | MYSQL_PWD="$DB_PASSWORD" mysql \
            -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" "$DB_NAME"; then
        fail_tool db_load_failed "mysql import failed; DB is in an indeterminate state"
    fi
fi

# ---- Media restore ----
if [ "$DB_ONLY" = "0" ] && [ -n "$MEDIA_FILE" ]; then
    # Validate tar listing first to catch path-traversal attempts before
    # extraction touches the filesystem. Any entry whose path contains
    # ".." or starts with "/" aborts the restore.
    printf 'step=media_inspect file=%s\n' "$MEDIA_FILE"
    if tar -tzf "$MANIFEST_DIR/$MEDIA_FILE" 2>/dev/null \
            | awk '/^\/|\.\.\// {found=1; print; exit} END {exit !found}' >/dev/null; then
        fail_pre tarball_unsafe "media tarball contains absolute or .. paths"
    fi

    media_parent="$(dirname "$MEDIA_PATH")"
    mkdir -p "$media_parent"
    printf 'step=media_extract dest=%s\n' "$media_parent"
    if ! tar -xzf "$MANIFEST_DIR/$MEDIA_FILE" -C "$media_parent"; then
        fail_tool tar_failed "tar extract failed"
    fi
fi

# Redis is intentionally manual: replacing the .rdb requires stopping
# the redis container, swapping the file in the volume, and restarting.
# The deployment doc walks the operator through it; doing it here would
# require docker access from inside the chronicle container, which is
# out of scope.
if [ -n "$REDIS_FILE" ]; then
    printf 'step=redis_skipped reason=manual_only file=%s\n' "$REDIS_FILE"
    printf 'note=See docs/deployment.md \xc2\xa79 for the Redis-restore steps.\n'
fi

printf 'RESTORE=ok manifest=%s migration_version=%s\n' \
    "$MANIFEST" "${MIGRATION_VER:-unknown}"
printf 'next=Start chronicle. RunStartupHealthChecks will validate the schema.\n'
