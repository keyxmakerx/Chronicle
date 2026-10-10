#!/usr/bin/env bash
# check-migration-numbers.sh — fail when two migrations in one directory share
# a version number.
#
# Why: golang-migrate's file:// source refuses a directory holding two files
# for the same version, so migrations fail at boot. Two PRs can each add the
# next number and pass CI alone; merged together they collide.
#
# Checks the union of this tree and the base branch's current tip, so a PR is
# also caught against migrations that merged after it branched.
#
# Per .ai/conventions.md §"Migration Safety Rules".

set -euo pipefail

base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

list_tree() {
  git ls-tree -r --name-only "$1" -- db/migrations internal/plugins 2>/dev/null \
    | grep -E '(^db/migrations/|^internal/plugins/[^/]+/migrations/)[^/]+\.(up|down)\.sql$' || true
}

files=$(
  {
    # The working tree, so uncommitted files count when run by hand.
    find db/migrations internal/plugins/*/migrations -maxdepth 1 -type f \
      -name '*.sql' 2>/dev/null | grep -E '\.(up|down)\.sql$' || true
    if git rev-parse --verify --quiet "${base}" >/dev/null; then
      list_tree "${base}"
    else
      echo "check-migration-numbers: base ${base} not found; checking this tree only" >&2
    fi
  } | sort -u
)

# One line per (directory, version, direction) with the distinct names using it.
dupes=$(
  printf '%s\n' "${files}" | awk -F/ 'NF {
    name = $NF
    dir = substr($0, 1, length($0) - length(name) - 1)
    if (match(name, /^[0-9]+/) == 0) next
    ver = substr(name, 1, RLENGTH) + 0
    dirn = (name ~ /\.up\.sql$/) ? "up" : "down"
    key = dir " " ver " " dirn
    if (!(key SUBSEP name in seen)) { seen[key SUBSEP name] = 1; n[key]++; names[key] = names[key] " " name }
  } END { for (k in n) if (n[k] > 1) print k ":" names[k] }' | sort
)

if [[ -n "${dupes}" ]]; then
  echo "FAIL: migration version numbers used more than once:" >&2
  printf '%s\n' "${dupes}" | sed 's/^/  /' >&2
  cat >&2 <<'MSG'

Renumber this PR's migration to the next free number in that directory
(rename both the .up.sql and .down.sql). Only renumber a migration that is
not on the base branch yet; applied migrations are immutable.
MSG
  exit 1
fi

echo "OK: every migration version number is unique per directory."
