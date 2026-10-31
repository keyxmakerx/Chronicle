#!/usr/bin/env bash
# tools/check-v2-motion-discipline.sh
#
# Enforces motion discipline in V2-scope plugins (calendar/timeline/
# ai_workspace/campaigns): no NEW `transition: all` / `transition-all`. Only
# `transform` and `opacity` may transition there; other properties are
# discouraged by the same rule but enforced via PR review instead (see each
# plugin's .ai.md), since they're harder to lint statically.
#
# Diff-scoped: only lines introduced vs origin/main; pre-existing violations
# are grandfathered. Forbidden tokens are fragment-joined so the script
# doesn't self-match scanning its own tree.
#
# Exit: 0 no new violations / 1 at least one new violation

set -euo pipefail

# Reconstruct the forbidden tokens (concatenated at runtime to keep
# the literal out of any tree-wide scan that would self-match).
ta1="trans""ition: all"        # raw CSS, with space after colon
ta2="trans""ition:all"          # raw CSS, no space
ta3="trans""ition-all"          # Tailwind utility (matches `transition-all` token,
                                 # including inside a longer class list because the
                                 # following char is whitespace or quote)

# V2-scope directories — only these are linted. Adding a directory here only
# affects code added by future PRs; pre-existing `transition: all` inside an
# in-scope directory is grandfathered.
scopes=(
  "internal/plugins/calendar"
  "internal/plugins/timeline"
  "internal/plugins/ai_workspace"
  # The Extensions hub (internal/plugins/campaigns/extensions_hub*.templ) is
  # V2-styled and the operator's first encounter with V2 chrome. New
  # `transition: all` in any campaigns/*.templ fails unless tagged
  # `/* OK exempt: ... */`.
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
  echo "Motion rules:"
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
