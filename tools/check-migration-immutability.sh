#!/usr/bin/env bash
# check-migration-immutability.sh — fail PRs that DELETE or MODIFY a migration
# file which already exists on the base branch. Migrations are APPEND-ONLY.
#
# Why: a live database may have already applied a migration, and golang-migrate's
# file:// source must contain a file for EVERY version up to the database's
# recorded version. Deleting or editing an applied migration crash-loops boot
# ("no migration found for version N: read down for version N: file does not
# exist"). New migration files (added) are always fine.
#
# Diff-scoped: only inspects what the PR changes vs the base branch; existing
# files are the immutable baseline. --no-renames makes a renamed/renumbered
# migration show up as a Delete of the old version (caught) + an Add of the
# new one (ignored).
#
# Per ADR-044/045 and .ai/conventions.md §"Migration Safety Rules".

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

# The one exception: a migration whose version number appears twice in its
# directory on the base may be renumbered. golang-migrate refuses to load a
# source with a duplicate version, so no database can have applied either file
# from that base. Only a Delete qualifies, and only when HEAD adds the same
# content under another name in the same directory.
duplicate_renumbered() {
  local status="$1" path="$2" dir name ver kind blob added
  [[ "${status}" == "D" ]] || return 1
  dir=$(dirname "${path}")
  name=$(basename "${path}")
  ver="${name%%_*}"
  kind="${name##*.}"; kind="${name%."${kind}"}"; kind="${kind##*.}"
  git ls-tree --name-only "${base}" -- "${dir}/" | xargs -n1 basename \
    | grep -E "^${ver}_.*\.${kind}\.sql$" | grep -qvxF "${name}" || return 1
  blob=$(git rev-parse "${base}:${path}")
  while IFS= read -r added; do
    [[ -n "${added}" ]] || continue
    [[ "$(git rev-parse "HEAD:${added}")" == "${blob}" ]] && return 0
  done < <(git diff --name-only --no-renames --diff-filter=A "${base}"...HEAD -- "${dir}/")
  return 1
}

blocked=""
while IFS=$'\t' read -r status path; do
  [[ -n "${path:-}" ]] || continue
  if duplicate_renumbered "${status}" "${path}"; then
    echo "check-migration-immutability: allowed — ${path} had a duplicate version on ${base} and is renumbered unchanged."
  else
    blocked+="${status}"$'\t'"${path}"$'\n'
  fi
done <<< "${changes}"
changes="${blocked%$'\n'}"

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
