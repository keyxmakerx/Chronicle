#!/usr/bin/env bash
# tools/check-comment-clutter.sh
#
# Keeps new code comments to "why, briefly, pointing at something stable"
# (the comment rules in CLAUDE.md and Chronicle's .ai/conventions.md). Fails
# when a line this branch ADDS carries, inside a comment:
#   - an old tracking or dispatch ID (C-…, FM-…)
#   - a path into the Cordinator archive (cordinator/…)
#   - "this PR", "this slice" or "this dispatch"
#   - a file:line pointer (they drift)
#   - a date outside tests (dates are provenance; it belongs in the commit
#     or PR; tests may name the fixture dates they use)
#   - a pointer to the retired .ai/todo.md
#
# Diff-scoped: existing comments are grandfathered, so nobody has to rewrite
# old files; fix a comment when you touch its code. A line that genuinely
# needs one of these (a date-format example, say) ends with `clutter-ok`.
#
# Shared verbatim by Chronicle, the Foundry module and the Draw Steel package;
# change all three together. Self-test: tools/test-comment-clutter.sh.
#
# Exit: 0 nothing new / 1 at least one new violation

set -euo pipefail

base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

if ! git rev-parse --verify --quiet "${base}" >/dev/null; then
  echo "check-comment-clutter: base ${base} not found; nothing to check."
  exit 0
fi

# Source files only. Docs (.md) describe history on purpose, and CI configs
# talk about "this PR" at run time, so neither is checked.
exts=(go templ js mjs cjs ts css sql sh)
pathspecs=()
for e in "${exts[@]}"; do pathspecs+=("*.${e}"); done

self_re='^tools/(check|test)-comment-clutter\.sh$'
skip_re='(^|/)(vendor|node_modules|static/vendor)/|_templ\.go$|\.min\.(js|css)$'

# Each pattern is matched against the comment part of an added line only.
patterns=(
  '\b(C|FM)-[A-Z][A-Z0-9]+(-[A-Z0-9]+)*\b'
  '\b[Cc]ordinator/'
  '\b[Tt]his (PR|slice|dispatch)\b'
  '[A-Za-z0-9_./-]+\.(go|templ|js|mjs|ts|css|sql|sh|py|md):[0-9]+'
  '\b20[0-9]{2}-[01][0-9]-[0-3][0-9]\b'
  'todo\.md'
)
labels=(
  "tracking/dispatch ID"
  "Cordinator archive path"
  "\"this PR/slice/dispatch\""
  "file:line pointer"
  "date"
  "pointer to .ai/todo.md"
)

# One awk pass over the diff prints "file<TAB>comment text" for every added
# line that is (or ends in) a comment. Whole-line comments start with
# // # /* * -- or <!--; a trailing comment is " // " or " # " after code (a
# URL's "://" has no space before it, so it never counts).
comments() {
  local files
  files=$(git diff --name-only --diff-filter=AM "${base}"...HEAD -- "${pathspecs[@]}" 2>/dev/null \
    | grep -Ev "${self_re}" | grep -Ev "${skip_re}" || true)
  [[ -z "${files}" ]] && return 0
  # shellcheck disable=SC2086
  git diff -U0 "${base}"...HEAD -- ${files} | awk '
    /^\+\+\+ / { f = substr($0, 7); next }
    /^\+/ {
      l = substr($0, 2)
      if (l ~ /clutter-ok/) next
      if (l ~ /^[ \t]*(\/\/|#|\/\*|\*|--|<!--)/) { print f "\t" l; next }
      if (match(l, /[ \t](\/\/|#)[ \t]/)) print f "\t" substr(l, RSTART + 1)
    }'
}

found="$(comments)"
violations=0
# Tests describe fixture dates on purpose, so the date rule skips them.
test_re='(_test\.go|\.test\.m?js|(^|/)test/|(^|/)tools/test-)'
for i in "${!patterns[@]}"; do
  scan="${found}"
  if [[ "${labels[$i]}" == "date" ]]; then
    scan=$(awk -F'\t' -v re="${test_re}" '$1 ~ re { print ""; next } { print }' <<<"${found}")
  fi
  hits=$(cut -f2- <<<"${scan}" | grep -nE -- "${patterns[$i]}" || true)
  [[ -z "${hits}" ]] && continue
  while IFS= read -r hit; do
    n="${hit%%:*}"
    row="$(sed -n "${n}p" <<<"${found}")"
    echo "${row%%$'\t'*}: ${labels[$i]} in a new comment: ${row#*$'\t'}"
    violations=$((violations + 1))
  done <<<"${hits}"
done

if (( violations > 0 )); then
  echo
  echo "check-comment-clutter: ${violations} new comment(s) carry history or a drifting pointer."
  echo "Comments say why and may point at an ADR, a test or an issue (#123); history goes in the PR."
  echo "If a line really needs it (e.g. a date-format example), end it with clutter-ok."
  exit 1
fi
echo "check-comment-clutter: OK (no new clutter vs ${base})."
