#!/usr/bin/env bash
# check-decision-citations.sh — warn (never fail) on orphan decision docs.
#
# Mechanism M-O3.1 per cordinator/decisions/2026-05-21-core-tenets.md §T-O3,
# implementing the Phase 2 ask from cordinator/reports/coordinator/2026-05-21-c-meta-audit.md.
#
# A decision doc is an orphan when its basename appears NOWHERE outside the
# decision file itself across:
#   - Chronicle source code (.go, .templ, .sh, .md, Makefile, etc.)
#   - chronicle-foundry-module source code (if present locally)
#   - Cordinator dispatches/, reports/, decisions/ (other decisions can cite)
#
# Run mode: WARN. Always exits 0. The OUTPUT is the value — surfaces orphans
# without blocking PRs. CI can publish the output as a job annotation.

set -euo pipefail

# Repo roots. Override via env if checked out elsewhere.
CHRONICLE_ROOT="${CHRONICLE_ROOT:-/home/user/Chronicle}"
FOUNDRY_ROOT="${FOUNDRY_ROOT:-/home/user/Chronicle-Foundry-Module}"
CORDINATOR_ROOT="${CORDINATOR_ROOT:-/home/user/Cordinator}"

DECISIONS_DIR="${CORDINATOR_ROOT}/decisions"

if [[ ! -d "${DECISIONS_DIR}" ]]; then
  echo "check-decision-citations: ${DECISIONS_DIR} not found; nothing to check."
  echo "(Set CORDINATOR_ROOT if Cordinator is checked out elsewhere.)"
  exit 0
fi

orphans=()
checked=0

# Build the list of search roots that exist. Missing roots are silently skipped
# (e.g., Foundry module not checked out in this CI environment).
search_roots=()
[[ -d "${CHRONICLE_ROOT}" ]]   && search_roots+=("${CHRONICLE_ROOT}")
[[ -d "${FOUNDRY_ROOT}" ]]     && search_roots+=("${FOUNDRY_ROOT}")
[[ -d "${CORDINATOR_ROOT}/dispatches" ]] && search_roots+=("${CORDINATOR_ROOT}/dispatches")
[[ -d "${CORDINATOR_ROOT}/reports" ]]    && search_roots+=("${CORDINATOR_ROOT}/reports")
[[ -d "${CORDINATOR_ROOT}/decisions" ]]  && search_roots+=("${CORDINATOR_ROOT}/decisions")
[[ -d "${CORDINATOR_ROOT}/plans" ]]      && search_roots+=("${CORDINATOR_ROOT}/plans")

if [[ ${#search_roots[@]} -eq 0 ]]; then
  echo "check-decision-citations: no search roots present; nothing to check."
  exit 0
fi

# Iterate every decision file. Use a stable sort for deterministic output.
while IFS= read -r decision_path; do
  basename=$(basename "${decision_path}" .md)
  checked=$((checked + 1))

  # Search every root for any file other than the decision itself referencing
  # the basename. grep -l prints filenames with matches; -r recursive; --include
  # could narrow but we want broad coverage. Exclude the decision file itself
  # by absolute path.
  found=$(grep -rln --binary-files=without-match "${basename}" "${search_roots[@]}" 2>/dev/null \
    | grep -v "^${decision_path}$" \
    || true)

  if [[ -z "${found}" ]]; then
    orphans+=("${basename}")
  fi
done < <(find "${DECISIONS_DIR}" -maxdepth 1 -name '*.md' -type f | sort)

echo "check-decision-citations: scanned ${checked} decision docs in ${DECISIONS_DIR}."

if [[ ${#orphans[@]} -eq 0 ]]; then
  echo "check-decision-citations: OK — all decisions are cited from somewhere."
  exit 0
fi

cat <<MSG

WARN — ${#orphans[@]} orphan decision doc(s) found (no citations from code,
dispatches, reports, or other decisions):

MSG

for o in "${orphans[@]}"; do
  echo "  - ${DECISIONS_DIR}/${o}.md"
done

cat <<MSG

Orphan decisions violate T-O3 (decision discipline): if a decision isn't cited
from code or workflow artifacts, it's not load-bearing — it's orphan aspiration.
Either:
  - Add a code/dispatch/report citation that references the decision basename, OR
  - Retire the decision (move to an archive) if it's been superseded, OR
  - Mark process-only decisions explicitly (some ADRs are intentionally process-only
    and don't expect code citations — see hygiene audit §6 for the canonical examples).

This is a WARN, not a FAIL. The output is the value.

MSG

# Always exit 0 — this guard never blocks CI.
exit 0
