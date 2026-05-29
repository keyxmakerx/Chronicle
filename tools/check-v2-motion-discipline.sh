#!/usr/bin/env bash
# tools/check-v2-motion-discipline.sh
#
# V2 motion-discipline guard: enforces the locked rules from
# cordinator/decisions/2026-05-28-cal-timeline-v2-design.md §B2 in
# the V2-scope plugin directories. Specifically: no NEW `transition: all`
# (raw CSS) and no NEW `transition-all` (Tailwind utility) introduced
# in calendar / timeline / ai_workspace plugin sources.
#
# Diff-scoped FAIL (matches tools/check-plugin-isolation.sh pattern):
# only checks lines INTRODUCED by the PR vs origin/main. Pre-existing
# violations (5 known at PR-1 land: calendar.templ × 3, calendar_list.templ,
# timeline.templ) are grandfathered. Per the dispatch text:
# "Existing surfaces not in V2 scope are exempt for now."
#
# Pre-existing in-scope survivors are tracked in
# cordinator/plans/BACKLOG.md as Wave 1 cleanup (deferred so PR 1 stays
# small + visual-feedback gate ships first).
#
# Animations on properties other than `transform` + `opacity` are
# discouraged by the same decision but harder to lint statically —
# documented in the plugins' .ai.md instead and enforced via PR review.
#
# Forbidden tokens reconstructed via fragment join so the script can
# scan its own directory tree without self-matching (same trick as
# tools/check-no-instance-hostname.sh + check-plugin-isolation.sh).
#
# Exit codes:
#   0 — no new violations introduced by the diff vs the merge base
#   1 — at least one new violation; CI fails

set -euo pipefail

# Reconstruct the forbidden tokens (concatenated at runtime to keep
# the literal out of any tree-wide scan that would self-match).
ta1="trans""ition: all"        # raw CSS, with space after colon
ta2="trans""ition:all"          # raw CSS, no space
ta3="trans""ition-all"          # Tailwind utility (matches `transition-all` token,
                                 # including inside a longer class list because the
                                 # following char is whitespace or quote)

# V2-scope directories — only these are linted.
#
# Note: the guard checks NEW lines introduced by the PR (via the
# diff-scoped `^[+]` filter below), so adding a directory here only
# affects code added by future PRs. Pre-existing `transition: all`
# inside an in-scope directory is grandfathered.
scopes=(
  "internal/plugins/calendar"
  "internal/plugins/timeline"
  "internal/plugins/ai_workspace"
  # C-EXT-HUB Phase 1 (2026-05-29): the top-level Extensions hub at
  # internal/plugins/campaigns/extensions_hub*.templ is V2-styled and
  # the operator's first encounter with V2 chrome. The campaigns
  # plugin is added in full because the scope-glob below only walks
  # one level (campaigns/*.templ); the diff-scoped `^[+]` filter then
  # bounds enforcement to lines this PR (and future PRs) introduce.
  # Pre-existing campaigns templs are not re-scanned. New `transition:
  # all` introduced in any campaigns/*.templ will fail unless tagged
  # `/* OK exempt: ... */` — the rare legacy-chrome edit can opt out.
  "internal/plugins/campaigns"
)

# Determine the diff base. In CI, GITHUB_BASE_REF is set on PRs;
# locally, fall back to origin/main. Allow override via DIFF_BASE env.
base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

# Files changed in the PR, restricted to V2-scope + .templ/.css extensions.
# .go files may legitimately reference 'transition: all' inside string
# literals (test fixtures, etc.); excluding .go avoids false positives.
changed_files=""
for scope in "${scopes[@]}"; do
  in_scope=$(git diff --name-only "${base}"...HEAD -- "${scope}/*.templ" "${scope}/*.css" 2>/dev/null || true)
  if [[ -n "${in_scope}" ]]; then
    changed_files+="${in_scope}"$'\n'
  fi
done

if [[ -z "${changed_files}" ]]; then
  echo "check-v2-motion-discipline: no V2-scope templ/CSS changes vs ${base}; nothing to check."
  exit 0
fi

# For each changed file, grep only the lines INTRODUCED by the PR
# (additions in git diff -U0 output) for any forbidden token.
violations=""
while IFS= read -r file; do
  [[ -z "${file}" ]] && continue
  # Skip deleted files (no introduced content to check).
  [[ -f "${file}" ]] || continue

  # `git diff -U0` shows only changed lines; `^[+]` filters to
  # additions; `^[+][+][+]` is the file-header marker (excluded).
  # Character-class syntax for `+` avoids the regex-engine ambiguity
  # some grep variants (ugrep, busybox) raise on bare backslash-plus.
  added_lines=$(git diff -U0 "${base}"...HEAD -- "${file}" 2>/dev/null \
    | grep -E '^[+]' | grep -v '^[+][+][+]' || true)
  if [[ -z "${added_lines}" ]]; then
    continue
  fi

  # Lines explicitly tagged `/* OK exempt: ... */` are deliberate;
  # let them through. Useful for the rare V2 surface that genuinely
  # needs a multi-property transition; surfacing in PR review.
  filtered=$(echo "${added_lines}" | grep -v "OK exempt:" || true)
  if [[ -z "${filtered}" ]]; then
    continue
  fi

  if echo "${filtered}" | grep -E "${ta1}|${ta2}|${ta3}" >/dev/null 2>&1; then
    offending=$(echo "${filtered}" | grep -nE "${ta1}|${ta2}|${ta3}" || true)
    violations+="${file}:"$'\n'"${offending}"$'\n'
  fi
done <<< "${changed_files}"

if [[ -n "${violations}" ]]; then
  echo "ERROR: V2 motion-discipline violations (new transition:all / transition-all banned in V2-scope):"
  echo
  echo "${violations}"
  echo
  echo "Per cordinator/decisions/2026-05-28-cal-timeline-v2-design.md §B2:"
  echo "  - Only \`transform\` + \`opacity\` ever animated"
  echo "  - NEVER \`transition: all\` (explicitly list properties)"
  echo "  - Reach for \`transition-transform\`, \`transition-opacity\`,"
  echo "    \`transition-colors\`, or \`transition-shadow\` instead."
  echo
  echo "If you're aware of the violation and intend it (rare; surface to"
  echo "coordinator), append \`/* OK exempt: <reason> */\` to the line."
  exit 1
fi

echo "check-v2-motion-discipline: OK — no new transition:all/transition-all in V2-scope."
