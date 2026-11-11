#!/usr/bin/env bash
# Run the live checks in test/live against a scratch Chronicle: the real
# binary on an empty database, driven by a browser the way the operator's
# click-path checks are.
#
# If LIVE_BASE already answers /healthz, the checks run against it and nothing
# is started (it must be a fresh scratch instance: _setup.mjs needs to be the
# first account). Otherwise this script empties the database DB_NAME, starts
# the binary on LIVE_PORT, runs the checks, and stops it again.
#
# USAGE
#   tools/live-checks.sh                       every check
#   tools/live-checks.sh test/live/x.spec.mjs  some checks
#
# ENVIRONMENT (defaults suit tools/start-test-db.sh and a local redis-server)
#   DB_HOST            MariaDB host:port            127.0.0.1:13306
#   DB_ROOT_PASSWORD   root password, if it has one (none)
#   DB_NAME            scratch database, EMPTIED    chronicle_live
#   REDIS_URL          Redis                        redis://localhost:6379
#   CHRONICLE_BIN      a built binary               builds ./bin/chronicle-live
#   LIVE_PORT          port for the scratch server  18080
#   LIVE_BASELINE      folder of pictures to compare against (none)
#   LIVE_OUT           where pictures and the report go  test-results/live

set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${LIVE_PORT:-18080}"
export LIVE_BASE="${LIVE_BASE:-http://localhost:${PORT}}"
export LIVE_OUT="${LIVE_OUT:-$PWD/test-results/live}"
export NODE_PATH="${NODE_PATH:-$(npm root -g)}"
DB_HOST="${DB_HOST:-127.0.0.1:13306}"
DB_NAME="${DB_NAME:-chronicle_live}"
case "$DB_NAME" in
  *live*|*test*|*scratch*) ;;
  *) echo "error: DB_NAME=$DB_NAME is emptied by this script; use a name with live, test or scratch in it" >&2; exit 1 ;;
esac

rm -rf "$LIVE_OUT"
mkdir -p "$LIVE_OUT"
server_pid=""
quiet_log=""

stop() {
  status=$?
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
    if [ "$status" -ne 0 ] && [ -z "$quiet_log" ]; then
      echo "==> last server log lines" >&2
      tail -n 60 "$LIVE_OUT/server.log" >&2 || true
    fi
  fi
  exit "$status"
}
trap stop EXIT

if curl -fsS "$LIVE_BASE/healthz" >/dev/null 2>&1; then
  echo "==> using the instance already running at $LIVE_BASE"
else
  host="${DB_HOST%:*}"; port="${DB_HOST##*:}"
  pw=(); [ -n "${DB_ROOT_PASSWORD:-}" ] && pw=(-p"$DB_ROOT_PASSWORD")
  echo "==> emptying database $DB_NAME on $DB_HOST"
  mysql -h "$host" -P "$port" --protocol=tcp -uroot "${pw[@]}" \
    -e "DROP DATABASE IF EXISTS \`$DB_NAME\`; CREATE DATABASE \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

  bin="${CHRONICLE_BIN:-}"
  if [ -z "$bin" ]; then
    bin="bin/chronicle-live"
    echo "==> building $bin"
    go build -o "$bin" ./cmd/server
  fi
  [ -f static/css/app.css ] || { echo "error: static/css/app.css is missing; run make tailwind" >&2; exit 1; }

  scratch="$LIVE_OUT/data"
  mkdir -p "$scratch/media" "$scratch/backups"
  echo "==> starting Chronicle on $LIVE_BASE"
  # Throwaway secrets for a server that lives for the length of the run.
  ENV=development PORT="$PORT" BASE_URL="$LIVE_BASE" LOG_LEVEL=info \
  DB_HOST="$DB_HOST" DB_NAME="$DB_NAME" DB_USER="${DB_USER:-root}" DB_PASSWORD="${DB_PASSWORD:-${DB_ROOT_PASSWORD:-}}" \
  REDIS_URL="${REDIS_URL:-redis://localhost:6379}" \
  SECRET_KEY="live-checks-only-secret-key-0123456789abcdef" \
  MEDIA_PATH="$scratch/media" MEDIA_SIGNING_SECRET_FILE="$scratch/.signing-secret" \
  BACKUP_DIR="$scratch/backups" BACKUP_SCRIPT_PATH="$PWD/scripts/backup.sh" RESTORE_SCRIPT_PATH="$PWD/scripts/restore.sh" \
    "$bin" >"$LIVE_OUT/server.log" 2>&1 &
  server_pid=$!
  for _ in $(seq 1 120); do
    curl -fsS "$LIVE_BASE/healthz" >/dev/null 2>&1 && break
    kill -0 "$server_pid" 2>/dev/null || { echo "error: Chronicle exited while starting" >&2; exit 1; }
    sleep 1
  done
  curl -fsS "$LIVE_BASE/healthz" >/dev/null || { echo "error: Chronicle did not answer /healthz in 120s" >&2; exit 1; }
fi

echo "==> setting up the site admin"
node test/live/_setup.mjs

specs=("$@")
[ ${#specs[@]} -eq 0 ] && specs=(test/live/*.spec.mjs)
echo "==> running ${#specs[@]} live check file(s)"
checks=0
node --test --test-concurrency=4 "${specs[@]}" || checks=$?

echo "==> pictures"
pictures=0
node tools/live-pictures.mjs || pictures=$?

[ "$checks" -eq 0 ] || { echo "error: live checks failed" >&2; exit "$checks"; }
# A changed picture is for a person to approve; the server log is no help.
quiet_log=1
exit "$pictures"
