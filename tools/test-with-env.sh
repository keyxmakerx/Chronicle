#!/usr/bin/env bash
# tools/test-with-env.sh -- self-test for tools/with-env.sh: .env values reach
# the command, the shell's own values win, comments and bad keys are skipped,
# and surrounding quotes come off.
#
# Exit: 0 if every case behaved as expected, 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WITH_ENV="$SCRIPT_DIR/with-env.sh"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/chronicle-with-env-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/.env" <<'ENV'
# a comment
DB_HOST=from-file:3306

DB_NAME="quoted name"
REDIS_URL='redis://single:6379'
export LOG_LEVEL=info
SECRET_KEY=abc=def
DB_PASSWORD=from-file
bad-key=nope
NO_EQUALS_LINE
ENV

PASS=0
FAIL=0
check() { # name want got
    if [[ "$2" == "$3" ]]; then echo "ok: $1"; PASS=$((PASS + 1))
    else echo "FAIL: $1 -- want '$2', got '$3'"; FAIL=$((FAIL + 1)); fi
}

run() { # VAR [env assignments...]
    local var="$1"; shift
    (cd "$WORK" && env -i PATH="$PATH" "$@" "$WITH_ENV" sh -c "printf '%s' \"\${$var-<unset>}\"")
}

check "value from .env" "from-file:3306" "$(run DB_HOST)"
check "shell value wins" "127.0.0.1:13306" "$(run DB_HOST DB_HOST=127.0.0.1:13306)"
check "empty shell value wins" "" "$(run DB_PASSWORD DB_PASSWORD=)"
check "double quotes removed" "quoted name" "$(run DB_NAME)"
check "single quotes removed" "redis://single:6379" "$(run REDIS_URL)"
check "export prefix" "info" "$(run LOG_LEVEL)"
check "= inside a value" "abc=def" "$(run SECRET_KEY)"
check "no .env changes nothing" "<unset>" "$(cd "$WORK" && env -i PATH="$PATH" ENV_FILE=missing "$WITH_ENV" sh -c 'printf "%s" "${DB_HOST-<unset>}"')"
check "exit code passes through" "3" "$(cd "$WORK" && { "$WITH_ENV" sh -c 'exit 3' || echo $?; })"

echo "with-env.sh self-test: $PASS passed, $FAIL failed"
[[ "$FAIL" == 0 ]]
