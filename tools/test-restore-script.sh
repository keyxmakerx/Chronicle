#!/usr/bin/env bash
# tools/test-restore-script.sh -- self-test for scripts/restore.sh's refusal
# logic and safety backup, with stub mysql/mysqldump/wget on PATH so no
# database or running Chronicle is needed.
#
# Pins: a human running the script against a live site is always refused;
# --from-inside lifts that refusal only when the maintenance marker, the run
# token and /healthz all agree; a failed safety backup stops the restore
# before the database is touched; a by-hand restore clears a stale marker.
#
# Exit: 0 if every case behaved as expected, 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESTORE="$SCRIPT_DIR/../scripts/restore.sh"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/chronicle-restore-script-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

BIN="$WORK/bin"
BACKUPS="$WORK/backups"
SQL_LOG="$WORK/sql.log"
mkdir -p "$BIN" "$BACKUPS"

# wget: STUB_HEALTH=down fails like a closed port; otherwise prints
# STUB_HEALTH_BODY.
cat > "$BIN/wget" <<'EOF'
#!/bin/sh
[ "${STUB_HEALTH:-down}" = "down" ] && exit 1
printf '%s' "${STUB_HEALTH_BODY:-{\"status\":\"ok\"}}"
EOF

# mysql: logs every statement; answers the table count and the schema version.
cat > "$BIN/mysql" <<'EOF'
#!/bin/sh
sql=""
while [ "$#" -gt 0 ]; do
    if [ "$1" = "-e" ]; then sql="$2"; shift; fi
    shift
done
if [ -z "$sql" ]; then cat >/dev/null; printf 'load\n' >> "$SQL_LOG"; exit 0; fi
printf '%s\n' "$sql" >> "$SQL_LOG"
case "$sql" in
    *"COUNT(*)"*) [ "${STUB_COUNT_FAIL:-0}" = "1" ] && { echo "mysql: gone away" >&2; exit 1; }
        printf '%s\n' "${STUB_TABLES:-5}" ;;
    *schema_migrations*) printf '42/0\n' ;;
esac
exit 0
EOF

# mysqldump: succeeds with a tiny dump, or fails when STUB_DUMP_FAIL=1.
cat > "$BIN/mysqldump" <<'EOF'
#!/bin/sh
[ "${STUB_DUMP_FAIL:-0}" = "1" ] && { echo "mysqldump: boom" >&2; exit 2; }
echo "-- dump of the live site"
echo "-- Dump completed"
EOF
chmod +x "$BIN"/*

# A restorable artifact set: the manifest the tests restore from.
printf 'CREATE TABLE t (id int);\n' | gzip -c > "$BACKUPS/chronicle_db_20260101T000000Z.sql.gz"
SHA="$(sha256sum "$BACKUPS/chronicle_db_20260101T000000Z.sql.gz" | awk '{print $1}')"
MANIFEST="$BACKUPS/chronicle_manifest_20260101T000000Z.txt"
printf 'migration_version=7/0\ndb_file=chronicle_db_20260101T000000Z.sql.gz sha256=%s size=1\n' "$SHA" > "$MANIFEST"

MARKER="$BACKUPS/.chronicle-maintenance"
TOKEN="tok-123"

write_marker() { # state token manifest-basename
    printf 'state=%s\ntoken=%s\nmanifest=%s\n' "$1" "$2" "$3" > "$MARKER"
}

PASS=0
FAIL=0

# run_case NAME EXPECT_EXIT GREP_FOR [env/args via RUN_ENV and RUN_ARGS]
run_case() {
    local name="$1" expect_exit="$2" grep_for="$3" out ec
    : > "$SQL_LOG"
    set +e
    out="$(env -i PATH="$BIN:$PATH" HOME="$WORK" SQL_LOG="$SQL_LOG" \
        DB_USER=u DB_PASSWORD=p DB_NAME=chronicle BACKUP_DIR="$BACKUPS" \
        MEDIA_PATH="$WORK/media" CHRONICLE_MAINTENANCE_FILE="$MARKER" \
        "${RUN_ENV[@]}" \
        sh "$RESTORE" --manifest "$MANIFEST" --yes --force "${RUN_ARGS[@]}" 2>&1)"
    ec=$?
    set -e
    if [[ "$ec" != "$expect_exit" ]]; then
        echo "FAIL: $name -- exit $ec, want $expect_exit"; echo "$out"
        FAIL=$((FAIL + 1)); return 1
    fi
    if ! grep -qF -- "$grep_for" <<<"$out"; then
        echo "FAIL: $name -- output lacks '$grep_for'"; echo "$out"
        FAIL=$((FAIL + 1)); return 1
    fi
    echo "ok: $name"
    PASS=$((PASS + 1))
    LAST_OUT="$out"
}

dropped() { grep -q 'DROP DATABASE' "$SQL_LOG"; }

expect_untouched() { # name
    if dropped; then
        echo "FAIL: $1 -- the database was dropped"; FAIL=$((FAIL + 1))
    fi
}

MAINT_BODY='{"status":"maintenance"}'

# 1. By hand against a live site: refused, nothing touched.
rm -f "$MARKER"
RUN_ENV=(STUB_HEALTH=up); RUN_ARGS=()
run_case "live site, by hand" 2 "reason=server_running" && expect_untouched "live site, by hand"

# 2. By hand while Chronicle really is in maintenance: still refused.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=()
run_case "maintenance but no flag" 2 "reason=server_running" && expect_untouched "maintenance but no flag"

# 3. Flag, live site, no marker.
rm -f "$MARKER"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=(--from-inside)
run_case "flag without marker" 2 "reason=maintenance_not_confirmed" && expect_untouched "flag without marker"

# 4. Flag, wrong token.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=guess"); RUN_ARGS=(--from-inside)
run_case "flag with wrong token" 2 "reason=maintenance_not_confirmed" && expect_untouched "flag with wrong token"

# 5. Flag, marker for a different manifest.
write_marker running "$TOKEN" "chronicle_manifest_other.txt"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=(--from-inside)
run_case "flag with another manifest in the marker" 2 "reason=maintenance_not_confirmed" && expect_untouched "other manifest"

# 6. Flag, marker not in the running state (a failed restore's marker).
write_marker failed "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=(--from-inside)
run_case "flag with a failed marker" 2 "reason=maintenance_not_confirmed" && expect_untouched "failed marker"

# 7. Flag, everything agrees except /healthz says the site is serving.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up 'STUB_HEALTH_BODY={"status":"ok"}' "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=(--from-inside)
run_case "flag while healthz says ok" 2 "reason=maintenance_not_confirmed" && expect_untouched "healthz ok"

# 8. Flag with everything agreeing: runs, saves a safety copy first, keeps
#    the marker (Chronicle clears its own).
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN"); RUN_ARGS=(--from-inside)
if run_case "flag with marker, token and healthz" 0 "RESTORE=ok"; then
    grep -q '^safety_backup=' <<<"$LAST_OUT" || { echo "FAIL: no safety_backup line"; FAIL=$((FAIL + 1)); }
    first_step="$(grep -n 'step=safety_backup file' <<<"$LAST_OUT" | head -1 | cut -d: -f1)"
    drop_step="$(grep -n 'step=db_drop' <<<"$LAST_OUT" | head -1 | cut -d: -f1)"
    if [[ -z "$first_step" || -z "$drop_step" || "$first_step" -ge "$drop_step" ]]; then
        echo "FAIL: safety copy was not taken before the database drop"; FAIL=$((FAIL + 1))
    fi
    ls "$BACKUPS"/chronicle_pre_restore_manifest_*.txt >/dev/null 2>&1 || { echo "FAIL: no safety manifest written"; FAIL=$((FAIL + 1)); }
    grep -q 'chronicle_pre_restore=1' "$BACKUPS"/chronicle_pre_restore_manifest_*.txt || { echo "FAIL: safety manifest not flagged"; FAIL=$((FAIL + 1)); }
    [[ -f "$MARKER" ]] || { echo "FAIL: in-app run removed Chronicle's marker"; FAIL=$((FAIL + 1)); }
fi
rm -f "$BACKUPS"/chronicle_pre_restore_*

# 9. Env var instead of the flag behaves the same.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" CHRONICLE_RESTORE_IN_PROCESS=1); RUN_ARGS=()
run_case "env var form" 0 "RESTORE=ok"
rm -f "$BACKUPS"/chronicle_pre_restore_*

# 10. Env var with no marker: refused.
rm -f "$MARKER"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" CHRONICLE_RESTORE_IN_PROCESS=1); RUN_ARGS=()
run_case "env var without marker" 2 "reason=maintenance_not_confirmed" && expect_untouched "env var without marker"

# 11. A failed safety copy stops the restore before the drop.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" STUB_DUMP_FAIL=1); RUN_ARGS=(--from-inside)
run_case "safety copy fails" 2 "reason=safety_backup_failed" && expect_untouched "safety copy fails"
ls "$BACKUPS"/chronicle_pre_restore_* >/dev/null 2>&1 && { echo "FAIL: partial safety files left behind"; FAIL=$((FAIL + 1)); }

# 11b. A failed table count is not an empty database: stop before the drop.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" STUB_COUNT_FAIL=1); RUN_ARGS=(--from-inside)
run_case "table count fails" 2 "reason=safety_backup_failed" && expect_untouched "table count fails"
grep -q 'reason=db_empty' <<<"$LAST_OUT" && { echo "FAIL: a failed count read as an empty database"; FAIL=$((FAIL + 1)); }

# 11c. A non-numeric answer is refused the same way.
write_marker running "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" STUB_TABLES=oops); RUN_ARGS=(--from-inside)
run_case "table count unreadable" 2 "reason=safety_backup_failed" && expect_untouched "table count unreadable"

# 12. --no-safety-backup skips it.
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" STUB_DUMP_FAIL=1); RUN_ARGS=(--from-inside --no-safety-backup)
run_case "--no-safety-backup" 0 "RESTORE=ok"
grep -q 'step=safety_backup' <<<"$LAST_OUT" && { echo "FAIL: safety step ran despite the flag"; FAIL=$((FAIL + 1)); }

# 13. An empty target database has nothing worth saving.
RUN_ENV=(STUB_HEALTH=up "STUB_HEALTH_BODY=$MAINT_BODY" "CHRONICLE_RESTORE_TOKEN=$TOKEN" STUB_TABLES=0); RUN_ARGS=(--from-inside)
run_case "empty database skips the safety copy" 0 "step=safety_backup skipped reason=db_empty"

# 14. By hand with Chronicle stopped: works, and clears a stale marker.
write_marker failed "$TOKEN" "$(basename "$MANIFEST")"
RUN_ENV=(STUB_HEALTH=down); RUN_ARGS=()
if run_case "stopped site, by hand" 0 "RESTORE=ok"; then
    [[ ! -f "$MARKER" ]] || { echo "FAIL: stale marker survived a by-hand restore"; FAIL=$((FAIL + 1)); }
fi
rm -f "$BACKUPS"/chronicle_pre_restore_*

echo "restore.sh self-test: $PASS passed, $FAIL failed"
[[ "$FAIL" == 0 ]]
