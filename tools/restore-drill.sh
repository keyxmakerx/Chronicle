#!/usr/bin/env bash
# tools/restore-drill.sh -- Prove a Chronicle backup actually restores.
#
# Per cordinator/plans/2026-07-10-beta-transition-plan.md §2 item 0.6: the
# beta plan's single largest data-loss risk is that a restore has NEVER been
# verified, even once. This is the one command that closes that gap.
#
# What it does, every run:
#   1. Finds the newest backup (or a --file you name).
#   2. Spins up a brand-new, disposable MariaDB container -- never the live
#      `chronicle-db` service, never a host port, never anything the live
#      compose stack can see.
#   3. Loads the backup's DB dump into that throwaway container.
#   4. Runs three checks: the migrations table is present and plausible,
#      the core tables have rows, and one foreign-key relationship is intact.
#   5. Prints one line: green "RESTORE DRILL: PASS" or red "FAIL: <why>".
#   6. Always tears the throwaway container down, pass or fail.
#
# This is a DRILL, not a real restore. It never touches scripts/backup.sh's
# output beyond reading it, never touches the chronicle-db container/volume,
# and never writes anything back into your live deployment. For an actual
# disaster-recovery restore, see docs/RESTORE-DRILL.md's "restore FOR REAL"
# section (that's scripts/restore.sh, a different, already-existing tool).
#
# Happy path (run from the deployment directory, live stack up):
#   ./tools/restore-drill.sh
#
# Test against a specific backup (compose exec discovery, or a local file):
#   ./tools/restore-drill.sh --file /app/data/backups/chronicle_manifest_<TS>.txt
#   ./tools/restore-drill.sh --file /path/to/chronicle_db_<TS>.sql.gz
#
# Exit codes: 0 PASS / 1 FAIL (verification or restore failed) /
#             2 precondition (docker missing, no backups found, bad --file).
#
# Cites: cordinator/decisions/2026-05-21-core-tenets.md §T-O1 (this script
# verifies rather than assumes a backup is good), §T-B1 (never widens the
# live DB's attack surface -- no host port, no shared network, no reused
# credentials).

set -euo pipefail

# ---- Paths ----
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose.yml"

# ---- Reserved names: this script must NEVER create, exec into, stop, or
# remove a container/volume matching the live compose service. The
# throwaway container name is always machine-generated below and can never
# collide, but this list is asserted against defensively so the guarantee
# is enforced in code, not just by construction. ----
RESERVED_NAMES=(chronicle chronicle-db chronicle-redis)

assert_not_reserved() {
    local name="$1"
    local r
    for r in "${RESERVED_NAMES[@]}"; do
        if [[ "$name" == "$r" ]]; then
            fail "refusing to operate on '$name' -- it matches the live compose service name" 2
        fi
    done
}

# ---- Output helpers ----
if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[0;33m'; C_RESET=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_RESET=""
fi

info()  { printf '%s\n' "$*"; }
note()  { printf '%snote:%s %s\n' "$C_YELLOW" "$C_RESET" "$*"; }

FAIL_EXIT=1
fail() {
    local code="${2:-$FAIL_EXIT}"
    printf '%sRESTORE DRILL: FAIL:%s %s\n' "$C_RED" "$C_RESET" "$1" >&2
    exit "$code"
}

# ---- Args ----
FILE_OVERRIDE=""
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        --file) FILE_OVERRIDE="${2:-}"; shift ;;
        -h|--help)
            sed -n '2,/^set -euo pipefail/p' "$0" | sed 's/^# \{0,1\}//;$d'
            exit 0 ;;
        *)
            fail "unknown argument '$1' (see --help)" 2 ;;
    esac
    shift
done

command -v docker >/dev/null 2>&1 || fail "docker not on PATH" 2
command -v gunzip >/dev/null 2>&1 || fail "gunzip not on PATH" 2
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum not on PATH" 2
docker info >/dev/null 2>&1 || fail "docker daemon not reachable (is it running? do you have permission?)" 2

# ---- Cleanup (always runs) ----
DRILL_TMPDIR=""
DRILL_NAME=""
cleanup() {
    local ec=$?
    # Inline reserved-name guard (not assert_not_reserved/fail) -- this runs
    # inside the EXIT trap itself, so it must never call exit again. A
    # reserved name here can't happen by construction (DRILL_NAME is always
    # machine-generated), but skipping rather than acting is the safe
    # failure mode if that invariant is ever broken.
    if [[ -n "$DRILL_NAME" ]]; then
        case "$DRILL_NAME" in
            chronicle|chronicle-db|chronicle-redis) : ;;
            *)
                docker stop -t 5 "$DRILL_NAME" >/dev/null 2>&1 || true
                docker rm -f "$DRILL_NAME" >/dev/null 2>&1 || true
                ;;
        esac
    fi
    [[ -n "$DRILL_TMPDIR" && -d "$DRILL_TMPDIR" ]] && rm -rf "$DRILL_TMPDIR"
    exit "$ec"
}
trap cleanup EXIT INT TERM

DRILL_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/chronicle-restore-drill.XXXXXX")"

# ============================================================================
# Step 1 -- locate the backup to test
# ============================================================================
DB_LOCAL=""       # path on host to the .sql / .sql.gz to restore
SOURCE_DESC=""    # human-readable description for the summary

parse_manifest_db_file() {
    # Reads db_file=<name> sha256=<sha> from a manifest line; echoes "name sha".
    local manifest="$1"
    awk '/^db_file=/{print}' "$manifest" | head -n1 | \
        sed -n 's/^db_file=\([^ ]*\).*sha256=\([^ ]*\).*/\1 \2/p'
}

if [[ -n "$FILE_OVERRIDE" ]]; then
    [[ -f "$FILE_OVERRIDE" ]] || fail "--file $FILE_OVERRIDE not found" 2
    case "$(basename "$FILE_OVERRIDE")" in
        chronicle_manifest_*|*manifest*.txt)
            read -r DB_NAME_ONLY DB_SHA < <(parse_manifest_db_file "$FILE_OVERRIDE") || true
            [[ -n "${DB_NAME_ONLY:-}" ]] || fail "manifest $FILE_OVERRIDE has no db_file= entry" 2
            DB_LOCAL="$(dirname "$FILE_OVERRIDE")/$DB_NAME_ONLY"
            [[ -f "$DB_LOCAL" ]] || fail "manifest references $DB_NAME_ONLY but it isn't next to the manifest ($DB_LOCAL not found -- copy both files together)" 2
            if [[ -n "${DB_SHA:-}" ]]; then
                ACTUAL_SHA="$(sha256sum "$DB_LOCAL" | awk '{print $1}')"
                [[ "$ACTUAL_SHA" == "$DB_SHA" ]] || fail "sha256 mismatch: $DB_LOCAL does not match the manifest (corrupt copy?)" 1
            fi
            SOURCE_DESC="$FILE_OVERRIDE (manifest, sha256 verified)"
            ;;
        *)
            DB_LOCAL="$FILE_OVERRIDE"
            SOURCE_DESC="$FILE_OVERRIDE (raw dump, no manifest -- sha256 verification skipped)"
            note "no manifest for this file; sha256 verification skipped"
            ;;
    esac
else
    # Default happy path: ask the live `chronicle` app container what the
    # newest backup is. This only ever reads from the chronicle service
    # (ls/cat/cp) -- it never touches chronicle-db.
    [[ -f "$COMPOSE_FILE" ]] || fail "docker-compose.yml not found at $COMPOSE_FILE (pass --file to test a backup directly)" 2

    NEWEST_MANIFEST_IN_CONTAINER="$( (cd "$REPO_ROOT" && docker compose -f "$COMPOSE_FILE" exec -T chronicle \
        sh -c 'ls -1t /app/data/backups/chronicle_manifest_*.txt 2>/dev/null | head -n1') 2>/dev/null || true)"

    if [[ -z "$NEWEST_MANIFEST_IN_CONTAINER" ]]; then
        fail "no backups found in /app/data/backups (or the chronicle service isn't running). Run 'make backup' first, confirm 'docker compose ps' shows chronicle up, or pass --file <path>." 2
    fi

    MANIFEST_LOCAL="$DRILL_TMPDIR/$(basename "$NEWEST_MANIFEST_IN_CONTAINER")"
    (cd "$REPO_ROOT" && docker compose -f "$COMPOSE_FILE" cp \
        "chronicle:$NEWEST_MANIFEST_IN_CONTAINER" "$MANIFEST_LOCAL") \
        || fail "could not copy $NEWEST_MANIFEST_IN_CONTAINER out of the chronicle container" 2

    read -r DB_NAME_ONLY DB_SHA < <(parse_manifest_db_file "$MANIFEST_LOCAL") || true
    [[ -n "${DB_NAME_ONLY:-}" ]] || fail "manifest $NEWEST_MANIFEST_IN_CONTAINER has no db_file= entry" 2

    DB_LOCAL="$DRILL_TMPDIR/$DB_NAME_ONLY"
    (cd "$REPO_ROOT" && docker compose -f "$COMPOSE_FILE" cp \
        "chronicle:/app/data/backups/$DB_NAME_ONLY" "$DB_LOCAL") \
        || fail "could not copy $DB_NAME_ONLY out of the chronicle container" 2

    if [[ -n "${DB_SHA:-}" ]]; then
        ACTUAL_SHA="$(sha256sum "$DB_LOCAL" | awk '{print $1}')"
        [[ "$ACTUAL_SHA" == "$DB_SHA" ]] || fail "sha256 mismatch: $DB_NAME_ONLY does not match its manifest" 1
    fi
    SOURCE_DESC="$NEWEST_MANIFEST_IN_CONTAINER (newest backup, sha256 verified)"
fi

info "source: $SOURCE_DESC"

# ============================================================================
# Step 2 -- spin up a throwaway MariaDB, never the live one
# ============================================================================
# Match the image the live compose file uses so restore behavior is
# representative; fall back to mariadb:latest (the compose default) if the
# file can't be parsed for any reason.
DRILL_IMAGE="$(awk '/^  chronicle-db:/{f=1} f&&/image:/{print $2; exit}' "$COMPOSE_FILE" 2>/dev/null || true)"
DRILL_IMAGE="${DRILL_IMAGE:-mariadb:latest}"

DRILL_NAME="chronicle-restore-drill-$(date -u +%Y%m%d%H%M%S)-$$"
assert_not_reserved "$DRILL_NAME"
DRILL_DB="chronicle_drill"
DRILL_PW="drill-$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 24)"

info "throwaway container: $DRILL_NAME (image $DRILL_IMAGE, no host port, no shared network/volume)"
docker run -d --rm \
    --name "$DRILL_NAME" \
    -e MYSQL_ROOT_PASSWORD="$DRILL_PW" \
    -e MYSQL_DATABASE="$DRILL_DB" \
    "$DRILL_IMAGE" >/dev/null \
    || fail "could not start throwaway MariaDB container (image $DRILL_IMAGE)" 3

drill_sql() {
    docker exec -i "$DRILL_NAME" env MYSQL_PWD="$DRILL_PW" mysql -uroot -N -B "$DRILL_DB" -e "$1" 2>/dev/null
}

READY=0
for _ in $(seq 1 30); do
    if docker exec "$DRILL_NAME" healthcheck.sh --connect --innodb_initialized >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 2
done
[[ "$READY" == "1" ]] || fail "throwaway MariaDB container did not become healthy within 60s" 2

# ============================================================================
# Step 3 -- load the dump
# ============================================================================
info "loading dump into throwaway container..."
case "$DB_LOCAL" in
    *.gz)
        gunzip -c "$DB_LOCAL" | docker exec -i "$DRILL_NAME" env MYSQL_PWD="$DRILL_PW" mysql -uroot "$DRILL_DB" \
            || fail "mysql import into throwaway container failed (dump may be corrupt)" 1
        ;;
    *)
        docker exec -i "$DRILL_NAME" env MYSQL_PWD="$DRILL_PW" mysql -uroot "$DRILL_DB" < "$DB_LOCAL" \
            || fail "mysql import into throwaway container failed (dump may be corrupt)" 1
        ;;
esac

# ============================================================================
# Step 4 -- verify
# ============================================================================

# --- 4a. Migrations table present and at a plausible version ---
# Delimiter is a literal '|', not a SQL '\t' escape: MariaDB's non-interactive
# client does not expand backslash escapes in -e query text (confirmed by
# direct testing -- CONCAT(...,'\t',...) round-trips as the literal two
# characters '\' 't', not a tab byte), so splitting on $'\t' here would
# silently produce garbage on every run.
MIGROW="$(drill_sql "SELECT CONCAT(version,'|',dirty) FROM schema_migrations LIMIT 1" || true)"
[[ -n "$MIGROW" ]] || fail "schema_migrations table missing or empty -- this doesn't look like a Chronicle database dump" 1
MIG_VERSION="${MIGROW%%|*}"
MIG_DIRTY="${MIGROW#*|}"
[[ "$MIG_VERSION" =~ ^[0-9]+$ ]] || fail "schema_migrations.version is not a plausible number ('$MIG_VERSION')" 1
[[ "$MIG_DIRTY" == "0" ]] || fail "schema_migrations.dirty=1 -- this backup was taken mid-migration, restore is unsafe" 1

EXPECTED_MAX="$(grep -oE 'ExpectedCoreMigrationVersion uint = [0-9]+' "$REPO_ROOT/internal/database/migrate_state.go" 2>/dev/null | grep -oE '[0-9]+$' || true)"
if [[ -z "$EXPECTED_MAX" ]]; then
    EXPECTED_MAX="$(find "$REPO_ROOT/db/migrations" -name '*.up.sql' 2>/dev/null | wc -l | tr -d ' ')"
fi
if [[ -n "$EXPECTED_MAX" && "$EXPECTED_MAX" =~ ^[0-9]+$ ]] && (( MIG_VERSION > EXPECTED_MAX )); then
    fail "schema_migrations.version=$MIG_VERSION is newer than any migration this checkout knows about (max $EXPECTED_MAX) -- git pull and retry" 1
fi
info "migrations: version=$MIG_VERSION dirty=$MIG_DIRTY (checkout max=${EXPECTED_MAX:-unknown}) -- OK"

# --- 4b. Row counts > 0 for core tables ---
CORE_TABLES=(users campaigns entities)
PLUGIN_TABLES=(calendar_events)

table_exists() {
    local t="$1"
    local n
    n="$(drill_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DRILL_DB' AND table_name='$t'")"
    [[ "$n" == "1" ]]
}

for t in "${CORE_TABLES[@]}"; do
    table_exists "$t" || fail "core table '$t' missing from restored dump" 1
    n="$(drill_sql "SELECT COUNT(*) FROM \`$t\`")"
    [[ "$n" =~ ^[0-9]+$ ]] || fail "could not read row count for '$t'" 1
    (( n > 0 )) || fail "'$t' has 0 rows -- dump may be empty or corrupt" 1
    info "rows: $t=$n"
done

for t in "${PLUGIN_TABLES[@]}"; do
    if ! table_exists "$t"; then
        note "'$t' not present in this dump (plugin migrations not applied) -- skipping row-count check for it"
        continue
    fi
    n="$(drill_sql "SELECT COUNT(*) FROM \`$t\`")"
    [[ "$n" =~ ^[0-9]+$ ]] || fail "could not read row count for '$t'" 1
    (( n > 0 )) || fail "'$t' has 0 rows -- dump may be empty or corrupt" 1
    info "rows: $t=$n"
done

# --- 4c. Spot FK check: entities.campaign_id -> campaigns.id ---
ORPHANS="$(drill_sql "SELECT COUNT(*) FROM entities e LEFT JOIN campaigns c ON e.campaign_id = c.id WHERE c.id IS NULL")"
[[ "$ORPHANS" =~ ^[0-9]+$ ]] || fail "could not run the entities->campaigns FK spot check" 1
[[ "$ORPHANS" == "0" ]] || fail "$ORPHANS entities row(s) reference a non-existent campaign_id -- referential integrity broken" 1
info "fk-check: entities.campaign_id -> campaigns.id -- OK (0 orphans)"

# ============================================================================
# PASS
# ============================================================================
trap - EXIT INT TERM
docker stop -t 5 "$DRILL_NAME" >/dev/null 2>&1 || true
rm -rf "$DRILL_TMPDIR"
printf '%sRESTORE DRILL: PASS%s (source: %s, migration version %s, %ss elapsed)\n' \
    "$C_GREEN" "$C_RESET" "$SOURCE_DESC" "$MIG_VERSION" "$SECONDS"
exit 0
