#!/usr/bin/env bash
# check-plugin-isolation.sh — fail PRs introducing new foundry-vtt plugin-name
# string literals OUTSIDE the foundry_vtt plugin directory.
#
# Mechanism M-B2.1 per cordinator/decisions/2026-05-21-core-tenets.md §T-B2.
# Diff-scoped FAIL: only checks lines INTRODUCED by the PR vs origin/main.
# Existing 161 violations (per cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md
# §1.D) are grandfathered until NW-2.2 plugin-isolation refactor lands.
#
# Forbidden tokens are reconstructed via fragment join so this script can scan
# its own directory tree without matching itself (same pattern as
# tools/check-no-instance-hostname.sh).

set -euo pipefail

# Reconstruct the forbidden tokens. Adjacent quoted strings concatenate at
# shell-runtime; the source-level tokens never appear as a literal.
fv1="fou""ndry-vtt"
fv2="fou""ndry-module"
fv3="fou""ndry_vtt"

# Path that owns these tokens — every other path is in violation.
owned_prefix="internal/plugins/fou""ndry_vtt/"

# Determine the diff base. In CI, GITHUB_BASE_REF is set on PRs; locally,
# fall back to origin/main. Allow override via DIFF_BASE env var.
base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

# Files changed in the PR. Empty diff = no-op exit 0.
changed_files=$(git diff --name-only "${base}"...HEAD -- '*.go' '*.templ' '*.css' 2>/dev/null || true)
if [[ -z "${changed_files}" ]]; then
  echo "check-plugin-isolation: no changed Go/templ/CSS files vs ${base}; nothing to check."
  exit 0
fi

# For each changed file outside the owned directory, grep ONLY the lines
# introduced by the PR (added in the diff, starting with '+') for any
# forbidden token. We use git diff with -U0 to keep context minimal.
violations=""
while IFS= read -r file; do
  [[ -z "${file}" ]] && continue
  case "${file}" in
    "${owned_prefix}"*) continue ;;  # owned by the foundry_vtt plugin
  esac
  # Skip deleted files (no introduced content to check).
  [[ -f "${file}" ]] || continue

  # `git diff -U0` shows only changed lines. `^\+` filters to additions; `^\+\+\+` is the file header, excluded.
  added=$(git diff -U0 "${base}"...HEAD -- "${file}" | grep -E '^\+' | grep -vE '^\+\+\+' || true)
  [[ -z "${added}" ]] && continue

  if echo "${added}" | grep -qE "\"(${fv1}|${fv2}|${fv3})\""; then
    line_hits=$(echo "${added}" | grep -nE "\"(${fv1}|${fv2}|${fv3})\"" || true)
    violations+="${file}:"$'\n'"${line_hits}"$'\n'
  fi
done <<< "${changed_files}"

if [[ -n "${violations}" ]]; then
  cat <<MSG
ERROR — new plugin-isolation violations introduced (T-B2):

${violations}

These literal strings must live inside ${owned_prefix} or pass through the
plugin-registration interface. Per cordinator/decisions/2026-05-21-core-tenets.md
§T-B2, plugin identifiers don't appear outside the owning plugin's directory.

Existing violations in main are grandfathered (NW-2.2 will clean them up).
This guard only catches NEW additions.
MSG
  exit 1
fi

echo "check-plugin-isolation: OK — no new foundry-vtt plugin-isolation violations introduced."
exit 0
