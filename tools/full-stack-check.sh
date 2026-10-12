#!/usr/bin/env bash
# Build Chronicle from source, boot the whole docker-compose stack (app,
# MariaDB, Redis) and wait until the app answers /healthz, then tear it down.
#
# This is the only check that runs the real image: Dockerfile, entrypoint,
# migrations on an empty database and the health route together. `go test`
# covers none of that.
#
# The secrets below are throwaway values for a stack that lives for a minute
# and is deleted with its volumes. Anything already exported wins.
#
# USAGE
#   tools/full-stack-check.sh        (or: make full-stack-check)

set -euo pipefail

cd "$(dirname "$0")/.."

# A separate project name keeps the check's volumes apart from a real stack's,
# so `down -v` below can never touch an operator's data.
PROJECT="chronicle-fullstack-check"
COMPOSE=(docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.build.yml)
WAIT_TIMEOUT="${FULL_STACK_WAIT_TIMEOUT:-300}"
PORT="${PORT:-8080}"

# docker-compose.yml fixes container_name and the host port, so a running
# stack of the same names would clash with this one; stop before touching it.
for name in chronicle chronicle-db chronicle-redis; do
  if [ -n "$(docker ps -aq --filter "name=^${name}\$")" ]; then
    echo "error: a container named '${name}' already exists; stop that stack first" >&2
    exit 1
  fi
done

# ENV is left to the compose default (production) so the check boots what
# operators run; the throwaway values below satisfy production's checks.
export SECRET_KEY="${SECRET_KEY:-ci-only-secret-key-not-for-real-use-0123456789}"
export DB_PASSWORD="${DB_PASSWORD:-ci-only-db-password}"
export MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-ci-only-root-password}"

cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "==> stack failed; container state and logs follow" >&2
    "${COMPOSE[@]}" ps -a >&2 || true
    "${COMPOSE[@]}" logs --no-color --tail=200 >&2 || true
  fi
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

echo "==> docker compose up --build --wait (up to ${WAIT_TIMEOUT}s)"
# --wait blocks until every service with a healthcheck is healthy; the app's
# check is the Dockerfile HEALTHCHECK against /healthz.
"${COMPOSE[@]}" up -d --build --wait --wait-timeout "$WAIT_TIMEOUT"

echo "==> GET http://localhost:${PORT}/healthz"
# /healthz pings MariaDB and Redis, so a 200 means the app reached both.
body=$(curl -fsS --retry 5 --retry-delay 2 --retry-connrefused "http://localhost:${PORT}/healthz")
echo "$body"
case "$body" in
  *'"status":"ok"'*) ;;
  *) echo "error: /healthz did not report ok" >&2; exit 1 ;;
esac

echo "==> full-stack-check: OK"
