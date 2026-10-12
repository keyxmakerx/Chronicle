#!/usr/bin/env bash
# tools/check-css-emissions.sh [--css FILE] [--list FILE]
#
# Builds the minified stylesheet to a temp file and fails if any selector in
# tools/css-emissions.txt is missing from it. Tailwind drops rules it thinks
# are unused (class names built in JS, rules in a layer), and the page then
# loses styling with no build error.
#
# --css checks an already-built file instead of building (used by the
# self-test, tools/test-css-emissions.sh).
#
# Exit: 0 every listed selector is emitted / 1 one or more missing / 2 usage.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
css=""
list="$ROOT/tools/css-emissions.txt"
while [ $# -gt 0 ]; do
  case "$1" in
    --css)  css="$2"; shift 2 ;;
    --list) list="$2"; shift 2 ;;
    *) echo "usage: check-css-emissions.sh [--css FILE] [--list FILE]" >&2; exit 2 ;;
  esac
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -z "$css" ]; then
  "$ROOT/tools/tailwind.sh" -i static/css/input.css -o "$tmp/app.css" --minify >/dev/null 2>&1 \
    || { echo "check-css-emissions: tailwind build failed" >&2; "$ROOT/tools/tailwind.sh" -i static/css/input.css -o "$tmp/app.css" --minify >&2 || true; exit 1; }
  css="$tmp/app.css"
fi

# One selector or declaration block per line, so a selector is a whole line
# ending in "{" or ",". Quotes go too: the minifier drops them in [attr="x"].
tr -d ' \t\r\n"'"'" <"$css" | sed 's/}/}\n/g; s/{/{\n/g; s/,/,\n/g' >"$tmp/flat"
sed 's/{$//; s/,$//' "$tmp/flat" | sort -u >"$tmp/emitted"

missing=0
total=0
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in ''|'#'*) continue ;; esac
  sel="$(printf '%s' "$line" | tr -d ' \t\r"'"'")"
  total=$((total + 1))
  if ! grep -Fxq -- "$sel" "$tmp/emitted"; then
    echo "check-css-emissions: not emitted: $line" >&2
    missing=$((missing + 1))
  fi
done <"$list"

if [ "$missing" -gt 0 ]; then
  echo "check-css-emissions: $missing of $total selectors missing from the build" >&2
  exit 1
fi
echo "check-css-emissions: OK ($total selectors emitted)"
