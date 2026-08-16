#!/usr/bin/env bash
# Start a local MariaDB for integration tests WITHOUT Docker.
#
# WHY THIS EXISTS. Every integration test in this repo skips when no database
# answers, and the skip message says "run `make docker-up`". In sandboxes and CI
# images that have no Docker daemon, that advice is a dead end, and the project
# concluded — in `.ai/status.md`, in several sweep reports, and in dozens of
# "proven against fakes, not the database" caveats — that those environments
# simply cannot test against a real database.
#
# THAT CONCLUSION WAS WRONG, and it cost real coverage. The absent thing is the
# Docker *daemon*; the MariaDB *server binary* is installed
# (/usr/sbin/mariadbd). It can be run directly against a scratch datadir. The
# only non-obvious part is that mariadbd refuses to start as root unless you
# say --user=root, and that a Unix socket path has a ~107 character limit, which
# a long scratch path silently exceeds. Both are handled below.
#
# Measured 2026-08-08: MariaDB 10.11.14 starts this way in ~3s and round-trips
# DDL and DML normally.
#
# USAGE
#   tools/start-test-db.sh          # start (idempotent); prints the DSN to export
#   tools/start-test-db.sh --stop   # stop it and leave the datadir
#   tools/start-test-db.sh --clean  # stop it and delete the datadir
#
# The tests discover it through CHRONICLE_TEST_DB_DSN, which this script prints.
# It listens on 13306, NOT 3306, so it can never collide with a real dev server
# or be mistaken for one.

set -euo pipefail

PORT="${CHRONICLE_TEST_DB_PORT:-13306}"
DATADIR="${CHRONICLE_TEST_DB_DATADIR:-/tmp/chronicle-testdb/data}"
# Short by construction: a socket path over ~107 chars fails with a confusing
# truncated-path error rather than a length error.
SOCKET="${CHRONICLE_TEST_DB_SOCKET:-/tmp/chronicle-testdb.sock}"
PIDFILE="/tmp/chronicle-testdb.pid"
ERRLOG="/tmp/chronicle-testdb.err"

die() { echo "error: $*" >&2; exit 1; }

running() { [ -S "${SOCKET}" ] && mysql --socket="${SOCKET}" -uroot -e "SELECT 1" >/dev/null 2>&1; }

stop_server() {
  if [ -f "${PIDFILE}" ]; then
    local pid; pid="$(cat "${PIDFILE}" 2>/dev/null || true)"
    [ -n "${pid}" ] && kill "${pid}" 2>/dev/null || true
    for _ in $(seq 1 20); do running || break; sleep 0.5; done
  fi
  pkill -f "mariadbd .*${DATADIR}" 2>/dev/null || true
  rm -f "${PIDFILE}"
  echo "stopped"
}

case "${1:-start}" in
  --stop) stop_server; exit 0 ;;
  --clean) stop_server; rm -rf "${DATADIR}"; echo "datadir removed: ${DATADIR}"; exit 0 ;;
  start|"") ;;
  *) die "unknown argument: $1 (expected --stop, --clean, or nothing)" ;;
esac

command -v mariadbd >/dev/null 2>&1 || command -v /usr/sbin/mariadbd >/dev/null 2>&1 \
  || die "mariadbd not installed — this script is for images that ship the server binary without a Docker daemon"
MARIADBD="$(command -v mariadbd 2>/dev/null || echo /usr/sbin/mariadbd)"

if running; then
  echo "already running on socket ${SOCKET} (port ${PORT})"
else
  if [ ! -d "${DATADIR}/mysql" ]; then
    echo "initializing datadir at ${DATADIR} ..."
    mkdir -p "${DATADIR}"
    mysql_install_db --datadir="${DATADIR}" --auth-root-authentication-method=normal >/dev/null 2>&1 \
      || die "mysql_install_db failed"
  fi

  # --user=root is REQUIRED when running as root; without it mariadbd aborts with
  # "Please consult the Knowledge Base to find out how to run mysqld as root!",
  # which reads like a permissions problem and is really a missing flag.
  RUN_AS=()
  [ "$(id -u)" -eq 0 ] && RUN_AS=(--user=root)

  # --skip-grant-tables keeps this a zero-credential scratch server. It is bound
  # to loopback on a non-default port and holds nothing but disposable schemas.
  nohup "${MARIADBD}" "${RUN_AS[@]}" \
    --datadir="${DATADIR}" \
    --socket="${SOCKET}" \
    --port="${PORT}" \
    --bind-address=127.0.0.1 \
    --skip-grant-tables \
    --pid-file="${PIDFILE}" \
    >"${ERRLOG}" 2>&1 &

  # ── THE WAIT IS 120s, NOT 20s, AND THE DIFFERENCE IS A REAL FALSE ALARM ────
  #
  # This loop was `seq 1 40` × 0.5s. A COLD InnoDB start here takes ~24 seconds —
  # measured: server launched 05:46:35, "ready for connections" 05:46:59 — so the
  # loop expired FOUR SECONDS BEFORE the server was ready and the script exited
  # non-zero while mariadbd sat happily listening on the port.
  #
  # THAT FAILURE MODE IS WORSE THAN A SLOW START. Every integration test in this
  # repo SKIPS when it cannot reach a database, and a skipped test reports as a
  # passing package. So a false "did not come up" does not stop anyone — it
  # quietly converts the entire integration suite into green nothing, which is
  # the exact "a skipped run is NOT a pass" trap the browser probes have their
  # own census to prevent.
  #
  # The budget is generous on purpose: this waits on a first-run buffer-pool
  # load, which varies with disk and with how much the container is doing. Being
  # slow costs seconds; being wrong costs a suite.
  # NOT `local` — this block runs at top level, not inside a function, and `local`
  # there is a RUNTIME error that `bash -n` does not catch.
  waited=0
  for _ in $(seq 1 240); do
    running && break
    sleep 0.5
    waited=$((waited + 1))
    # Say something at 20s so a genuinely stuck start is distinguishable from a
    # slow one WHILE it is happening, rather than only in the post-mortem.
    [ "${waited}" -eq 40 ] && echo "still waiting for mariadbd (cold InnoDB start can take ~30s) ..." >&2
  done
  if ! running; then
    tail -8 "${ERRLOG}" >&2
    die "server did not come up after $((waited / 2))s — see ${ERRLOG}. If that log ends in
'ready for connections', the server IS up and this probe is what failed: check that
${SOCKET} is writable and that the mysql client can reach it."
  fi
  [ "${waited}" -gt 40 ] && echo "mariadbd ready after $((waited / 2))s" >&2
  echo "started MariaDB $(mysql --socket="${SOCKET}" -uroot -sN -e 'SELECT VERSION()')"
fi

cat <<EOF

  export CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:${PORT})/'

Then: go test ./... -count=1        (integration tests stop skipping)
      make test-int
Stop:  tools/start-test-db.sh --stop
EOF
