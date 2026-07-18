#!/usr/bin/env bash
# tools/test-restore-drill.sh -- CI-safe self-test for tools/restore-drill.sh.
#
# Runs the real script against tiny fixtures in testdata/restore-drill/,
# spinning real (but disposable) MariaDB containers -- no chronicle app
# stack required, since --file bypasses the docker-compose discovery step
# entirely. This is what keeps the drill kit from rotting silently: every
# check the script makes (migrations plausibility, row counts, the FK spot
# check) has at least one fixture proving both its PASS and its FAIL path
# actually fire.
#
# Requires: docker (present on GitHub Actions ubuntu-latest runners by
# default -- no setup needed). Not run as part of `make test`; wired into
# CI as its own step (see .github/workflows/ci.yml).
#
# Exit: 0 if every fixture produced the expected verdict, 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DRILL="$REPO_ROOT/tools/restore-drill.sh"
FIXTURES="$REPO_ROOT/testdata/restore-drill"

PASS_COUNT=0
FAIL_COUNT=0

TMPDIR_SELFTEST="$(mktemp -d "${TMPDIR:-/tmp}/chronicle-restore-drill-selftest.XXXXXX")"
trap 'rm -rf "$TMPDIR_SELFTEST"' EXIT

# expect_case NAME FIXTURE_FILE EXPECT_EXIT GREP_FOR
expect_case() {
    local name="$1" fixture="$2" expect_exit="$3" grep_for="$4"
    local out ec

    set +e
    out="$("$DRILL" --file "$fixture" 2>&1)"
    ec=$?
    set -e

    if [[ "$ec" != "$expect_exit" ]]; then
        echo "FAIL: $name -- expected exit $expect_exit, got $ec"
        echo "--- output ---"; echo "$out"; echo "--------------"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return
    fi
    if ! grep -qF "$grep_for" <<<"$out"; then
        echo "FAIL: $name -- expected output to contain '$grep_for'"
        echo "--- output ---"; echo "$out"; echo "--------------"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return
    fi
    echo "ok: $name"
    PASS_COUNT=$((PASS_COUNT + 1))
}

# --- PASS path, gzipped (the real-world shape: scripts/backup.sh always
# gzips) ---
GOOD_GZ="$TMPDIR_SELFTEST/good.sql.gz"
gzip -c "$FIXTURES/good.sql" > "$GOOD_GZ"
expect_case "good fixture (gzipped)" "$GOOD_GZ" 0 "RESTORE DRILL: PASS"

# --- PASS path, plain .sql (covers the non-gz branch) ---
expect_case "good fixture (plain sql)" "$FIXTURES/good.sql" 0 "RESTORE DRILL: PASS"

# --- FAIL paths ---
expect_case "dirty migrations flag" "$FIXTURES/bad_dirty.sql" 1 "dirty=1"
expect_case "orphaned FK" "$FIXTURES/bad_orphan_fk.sql" 1 "campaign_id"
expect_case "empty core table" "$FIXTURES/bad_zero_rows.sql" 1 "0 rows"

echo "---"
echo "restore-drill self-test: $PASS_COUNT passed, $FAIL_COUNT failed"
[[ "$FAIL_COUNT" -eq 0 ]]
