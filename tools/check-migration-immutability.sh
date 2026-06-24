#!/usr/bin/env bash
# check-migration-immutability.sh — fail PRs that DELETE or MODIFY a migration
# file which already exists on the base branch. Migrations are APPEND-ONLY.
#
# Why: a live database may have already applied a migration, and golang-migrate's
# file:// source must contain a file for EVERY version up to the database's
# recorded version. Deleting or editing an applied migration crash-loops boot
# ("no migration found for version N: read down for version N: file does not
# exist") — this is the 2026-06-24 `000030` production incident. New migration
# files (added) are always fine.
#
# Diff-scoped, modelled on tools/check-plugin-isolation.sh: it only inspects what
# the PR changes vs the base branch; existing files are the immutable baseline.
# --no-renames makes a renamed/renumbered migration show up as a Delete of the
# old version (caught) + an Add of the new one (ignored).
#
# Per ADR-044 and .ai/conventions.md §"Migration Safety Rules".

set -euo pipefail

base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

# Deleted (D) or Modified (M) migration files (core + per-plugin) vs the base.
changes=$(git diff --name-status --no-renames --diff-filter=DM "${base}"...HEAD -- \
  'db/migrations' 'internal/plugins' 2>/dev/null \
  | grep -E 'migrations/[0-9]+_.*\.(up|down)\.sql$' || true)

if [[ -z "${changes}" ]]; then
  echo "check-migration-immutability: OK — no existing migration deleted or modified vs ${base}."
  exit 0
fi

cat <<MSG
ERROR — a migration that already exists on ${base} was DELETED or MODIFIED:

${changes}

Migrations are APPEND-ONLY. A live database may have already applied them, and
golang-migrate's file:// source must contain a file for every version up to the
database's recorded version. Deleting or editing an applied migration crash-loops
boot ("no migration found for version N") — the 2026-06-24 000030 incident.

Do this instead:
  - Changing schema?  Add a NEW migration with the next number (db/migrations or
                      the owning plugin's migrations/ dir).
  - Correcting data?  Use an idempotent reconciler (an EnsureX/MergeX service
                      method, or a SetupProvider), NOT a migration edit.

See .ai/conventions.md §"Migration Safety Rules" and ADR-044.
MSG
exit 1
