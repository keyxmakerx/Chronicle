#!/bin/sh
# scripts/restore.sh -- Restore a Chronicle deployment from a scripts/backup.sh
# snapshot set, or from the automatic pre-update capture
# (chronicle_pre_migrate_manifest_*.txt), which uses the same manifest format
# — see docs/deployment.md §7 Scenario A.
#
# A single gated command, not a sequence of scripts, because that's what an
# operator types at 2 AM under stress: safety comes from confirmation flags
# and pre-flight checks. It does NOT run migrations — RunStartupHealthChecks
# refuses to boot if the restored DB doesn't match the running image's
# expected migration version.
#
# Usage: docker compose exec chronicle /app/scripts/restore.sh
#   --manifest PATH [--db-only] [--media-only] [--yes] [--force]
#   [--no-safety-backup] [--from-inside]
# --manifest (required) pairs the DB/media/redis artifacts, so a stale mix
#   (e.g. last week's DB with last month's media) can't slip through.
# --force allows restoring over a non-empty target (default: refuse).
# Before the database is dropped the script saves the current one as
#   chronicle_pre_restore_manifest_<TS>.txt (a manifest this script can
#   restore, and the admin restore page lists); if that save fails the
#   restore stops. --no-safety-backup skips it, for a database too broken to
#   dump.
# --from-inside (or CHRONICLE_RESTORE_IN_PROCESS=1) is for the admin restore
#   page, which runs this script from inside Chronicle. It lifts the "server
#   is running" refusal only when Chronicle's own maintenance marker, the run
#   token it handed this process and its /healthz all say it is in maintenance
#   for this manifest. Run by hand against a live site it still refuses.
#
# Exit codes: 0 success / 1 operator error / 2 precondition / 3 tool failure.

set -eu

# ---- Args ----
MANIFEST=""
DB_ONLY=0
MEDIA_ONLY=0
ASSUME_YES=0
FORCE=0
NO_SAFETY=0
FROM_INSIDE=0
[ "${CHRONICLE_RESTORE_IN_PROCESS:-}" = "1" ] && FROM_INSIDE=1

while [ "$#" -gt 0 ]; do
    case "$1" in
        --manifest) MANIFEST="${2:-}"; shift ;;
        --db-only) DB_ONLY=1 ;;
        --media-only) MEDIA_ONLY=1 ;;
        --yes) ASSUME_YES=1 ;;
        --force) FORCE=1 ;;
        --no-safety-backup) NO_SAFETY=1 ;;
        --from-inside) FROM_INSIDE=1 ;;
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
# 1. Refuse if a chronicle server is running. The /healthz probe is the most
# general check (works inside the container too via localhost).
BACKUP_DIR_ENV="${BACKUP_DIR:-$MANIFEST_DIR}"
MAINT_FILE="${CHRONICLE_MAINTENANCE_FILE:-$BACKUP_DIR_ENV/.chronicle-maintenance}"
HEALTH_URL="http://localhost:${PORT:-8080}/healthz"

# marker_value KEY prints one value from Chronicle's maintenance marker.
marker_value() {
    sed -n "s/^$1=//p" "$MAINT_FILE" 2>/dev/null | head -n 1
}

# maintenance_confirmed succeeds only when Chronicle itself says it is closed
# for this very restore: the marker is in the running state for this manifest,
# the token Chronicle passed to this process matches the marker's, and the
# live health probe answers "maintenance". Any one missing refuses.
maintenance_confirmed() {
    [ -f "$MAINT_FILE" ] || return 1
    [ "$(marker_value state)" = "running" ] || return 1
    marker_token="$(marker_value token)"
    [ -n "$marker_token" ] || return 1
    [ "${CHRONICLE_RESTORE_TOKEN:-}" = "$marker_token" ] || return 1
    [ "$(marker_value manifest)" = "$(basename "$MANIFEST")" ] || return 1
    command -v wget >/dev/null 2>&1 || return 1
    health_body="$(wget -qO- "$HEALTH_URL" 2>/dev/null || true)"
    case "$health_body" in
        *'"maintenance"'*) return 0 ;;
    esac
    return 1
}

if command -v wget >/dev/null 2>&1; then
    if wget -qO- "$HEALTH_URL" >/dev/null 2>&1; then
        if [ "$FROM_INSIDE" = "1" ]; then
            maintenance_confirmed || fail_pre maintenance_not_confirmed \
                "chronicle is up at $HEALTH_URL but does not say it is in maintenance for this restore; stop it first (docker compose stop chronicle)"
        else
            fail_pre server_running \
                "chronicle is up at $HEALTH_URL; stop it first (docker compose stop chronicle)"
        fi
    fi
fi
if [ "$FROM_INSIDE" = "1" ]; then
    # Asked to run inside Chronicle: the confirmation must hold even when
    # the probe could not run, so a missing wget never weakens the check.
    maintenance_confirmed || fail_pre maintenance_not_confirmed \
        "--from-inside needs Chronicle's maintenance marker, run token and health answer"
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
    [ "$MEDIA_ONLY" = "0" ] && [ "$NO_SAFETY" = "0" ] && \
        printf '  - first save a copy of the current DB next to the backups\n'
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

# ---- Safety backup ----
# The restore replaces the live database, so save it first. The copy is
# written as a manifest of its own so this script (and the admin restore
# page) can put it back if the restored data turns out to be wrong.
if [ "$MEDIA_ONLY" = "0" ] && [ "$NO_SAFETY" = "0" ]; then
    # A failed count must not read as "empty": that would skip the copy
    # and then drop the live database with nothing saved.
    if ! safety_tables="$(MYSQL_PWD="$DB_PASSWORD" mysql \
            -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -N -B \
            -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME'" \
            2>/dev/null)"; then
        fail_pre safety_backup_failed "could not check the current database before saving a copy; nothing was changed (pass --no-safety-backup to restore anyway)"
    fi
    case "$safety_tables" in
        ''|*[!0-9]*)
            fail_pre safety_backup_failed "could not read the current database's table count; nothing was changed (pass --no-safety-backup to restore anyway)" ;;
    esac
    if [ "$safety_tables" -gt 0 ]; then
        command -v mysqldump >/dev/null 2>&1 || \
            fail_pre safety_backup_failed "mysqldump not on PATH; install mariadb-client or pass --no-safety-backup"
        command -v gzip >/dev/null 2>&1 || \
            fail_pre safety_backup_failed "gzip not on PATH; pass --no-safety-backup to skip the safety copy"
        mkdir -p "$BACKUP_DIR_ENV" 2>/dev/null || \
            fail_pre safety_backup_failed "cannot create $BACKUP_DIR_ENV"
        SAFE_TS="$(date -u +'%Y%m%dT%H%M%SZ')"
        SAFE_DB="chronicle_pre_restore_db_${SAFE_TS}.sql.gz"
        SAFE_MANIFEST="$BACKUP_DIR_ENV/chronicle_pre_restore_manifest_${SAFE_TS}.txt"
        printf 'step=safety_backup file=%s\n' "$SAFE_DB"
        # Two steps so a failing mysqldump is not hidden behind gzip's exit
        # status (no PIPESTATUS in POSIX sh).
        if ! MYSQL_PWD="$DB_PASSWORD" mysqldump \
                -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
                --single-transaction --routines --triggers \
                "$DB_NAME" > "$BACKUP_DIR_ENV/$SAFE_DB.sql.partial"; then
            rm -f "$BACKUP_DIR_ENV/$SAFE_DB.sql.partial"
            fail_pre safety_backup_failed "could not save the current database; nothing was changed (pass --no-safety-backup to restore anyway)"
        fi
        if ! gzip -9 -c "$BACKUP_DIR_ENV/$SAFE_DB.sql.partial" > "$BACKUP_DIR_ENV/$SAFE_DB.partial"; then
            rm -f "$BACKUP_DIR_ENV/$SAFE_DB.sql.partial" "$BACKUP_DIR_ENV/$SAFE_DB.partial"
            fail_pre safety_backup_failed "could not compress the saved copy; nothing was changed"
        fi
        rm -f "$BACKUP_DIR_ENV/$SAFE_DB.sql.partial"
        if ! gzip -t "$BACKUP_DIR_ENV/$SAFE_DB.partial" 2>/dev/null; then
            rm -f "$BACKUP_DIR_ENV/$SAFE_DB.partial"
            fail_pre safety_backup_failed "the saved copy is unreadable; nothing was changed"
        fi
        mv "$BACKUP_DIR_ENV/$SAFE_DB.partial" "$BACKUP_DIR_ENV/$SAFE_DB"
        SAFE_SHA="$(sha256sum "$BACKUP_DIR_ENV/$SAFE_DB" | awk '{print $1}')"
        SAFE_SIZE="$(wc -c < "$BACKUP_DIR_ENV/$SAFE_DB")"
        SAFE_VER="$(MYSQL_PWD="$DB_PASSWORD" mysql \
            -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -N -B \
            -e "SELECT CONCAT(version, '/', dirty) FROM schema_migrations LIMIT 1" \
            "$DB_NAME" 2>/dev/null || echo "unknown")"
        {
            printf 'chronicle_manifest_version=1\n'
            printf 'chronicle_pre_restore=1\n'
            printf 'timestamp=%s\n' "$SAFE_TS"
            printf 'chronicle_version=%s\n' "${CHRONICLE_VERSION:-unknown}"
            printf 'migration_version=%s\n' "$SAFE_VER"
            printf 'db_file=%s sha256=%s size=%s\n' "$SAFE_DB" "$SAFE_SHA" "$SAFE_SIZE"
        } > "$SAFE_MANIFEST.partial"
        mv "$SAFE_MANIFEST.partial" "$SAFE_MANIFEST"
        printf 'safety_backup=%s\n' "$SAFE_MANIFEST"
    else
        printf 'step=safety_backup skipped reason=db_empty\n'
    fi
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

# A restore finished by hand supersedes a maintenance marker Chronicle left
# behind (a failed or interrupted in-app restore), so the next start opens
# normally. The in-app run clears its own marker.
if [ "$FROM_INSIDE" = "0" ] && [ "$MEDIA_ONLY" = "0" ] && [ -f "$MAINT_FILE" ]; then
    rm -f "$MAINT_FILE" "$MAINT_FILE.log"
    printf 'step=maintenance_marker_cleared\n'
fi

printf 'RESTORE=ok manifest=%s migration_version=%s\n' \
    "$MANIFEST" "${MIGRATION_VER:-unknown}"
printf 'next=Start chronicle. RunStartupHealthChecks will validate the schema.\n'
