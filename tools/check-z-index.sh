#!/usr/bin/env bash
# tools/check-z-index.sh
#
# LAYER SCALE RATCHET: stacking order comes from the --z-* tokens at the top
# of static/css/input.css (.ai/ui-standard.md, "Layers"). A raw z-index above
# 10 is a number picked by hand; this counts them across CSS, templates and
# scripts and fails only if the count grows past tools/z-index-ceiling.txt.
# Moving a layer onto a token lowers the count; then lower the ceiling to
# match, so it can only shrink.
#
# Counted: `z-index: N`, `zIndex = N`, and Tailwind `z-[N]` / `z-N`, for N
# above 10. `var(--z-*)` never matches.
#
# Exit: 0 count within the ceiling / 1 count grew

set -euo pipefail

CEILING_FILE="${CEILING_FILE:-tools/z-index-ceiling.txt}"

count=$(
  { grep -rEoh --include='*.css' --include='*.templ' --include='*.js' --include='*.mjs' \
      --exclude='app.css' --exclude='*.min.js' --exclude='*.min.css' \
      -e 'z-index:[[:space:]]*[0-9]+' \
      -e 'zIndex[[:space:]]*[=:][[:space:]]*.?[0-9]+' \
      -e '(^|[^A-Za-z0-9_-])z-\[?[0-9]+\]?([^A-Za-z0-9_-]|$)' \
      static/css static/js internal 2>/dev/null || true; } \
  | grep -oE '[0-9]+' | awk '$1 > 10' | wc -l
)

ceiling=$(tr -d '[:space:]' < "$CEILING_FILE")

if (( count > ceiling )); then
  echo "ERROR: ${count} raw z-index values above 10; the ceiling is ${ceiling}."
  echo
  echo "Take the band for your surface from the --z-* tokens in"
  echo "static/css/input.css (.ai/ui-standard.md, \"Layers\") instead of a number."
  exit 1
fi
if (( count < ceiling )); then
  echo "note: ${count} raw z-index values above 10, under the ceiling of ${ceiling}; lower ${CEILING_FILE} to ${count}."
fi
echo "z-index ratchet OK (${count} raw values above 10, ceiling ${ceiling})"
