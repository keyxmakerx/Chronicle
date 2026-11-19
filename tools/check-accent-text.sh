#!/usr/bin/env bash
# tools/check-accent-text.sh
#
# WORDS ON THE RAW ACCENT: the picked accent is one colour in both themes, so
# as text it falls under 4.5:1 on a dark page (and just short on the default in
# light). AppearanceCSS writes --color-accent-link, shaded per theme to read on
# the page and card; text must use it:
#   color: var(--color-accent-link, var(--color-accent));
# Fills, borders, outlines, focus rings and icons-as-art stay on the raw accent.
#
# Fails on a `color:` declaration (not background-color, border-color,
# outline-color, caret-color, accent-color, ...) that reads --color-accent,
# --color-accent-rgb, --color-accent-hover or --cpe-accent without
# --color-accent-link. Justified exceptions are listed in
# tools/accent-text-allowlist.txt as `path|exact declaration|reason`.
# Self-test: --self-test-only.
#
# Exit: 0 clean / 1 offending declaration / 2 self-test failed

set -euo pipefail

ALLOWLIST="${ALLOWLIST:-tools/accent-text-allowlist.txt}"

# scan <file>...: prints "file:line: declaration" for each offender.
scan() {
  grep -nHoE '(^|[^-A-Za-z_])color:[^;}"]*' "$@" 2>/dev/null \
    | grep -E 'var\(--color-accent(-rgb|-hover)?[,)]|var\(--color-accent-rgb[ ,)]|var\(--cpe-accent[,)]' \
    | grep -v -- '--color-accent-link' \
    | sed -E 's/^([^:]+:[0-9]+):[^c]*(color:.*)$/\1: \2/' || true
}

self_test() {
  local d; d="$(mktemp -d)"
  cat >"$d/bad.css" <<'CSS'
.a { color: var(--color-accent); }
.b { color: rgb(var(--color-accent-rgb)); }
.c { color: var(--color-accent-hover, #4f46e5); }
.d { color: var(--cpe-accent); }
CSS
  cat >"$d/good.css" <<'CSS'
.a { color: var(--color-accent-link, var(--color-accent)); }
.b { background-color: var(--color-accent); border-color: var(--color-accent); }
.c { outline-color: var(--color-accent); caret-color: var(--color-accent); accent-color: var(--color-accent); }
.d { color: var(--color-accent-on-fill, #fff); background: var(--color-accent-fill, var(--color-accent)); }
.e { background: rgb(var(--color-accent-rgb) / .12); }
CSS
  local n
  n="$(scan "$d/bad.css" | wc -l)"
  [ "$n" = 4 ] || { echo "self-test: expected 4 offenders in bad.css, got $n"; rm -rf "$d"; exit 2; }
  n="$(scan "$d/good.css" | wc -l)"
  [ "$n" = 0 ] || { echo "self-test: expected 0 offenders in good.css, got $n"; scan "$d/good.css"; rm -rf "$d"; exit 2; }
  rm -rf "$d"
}

self_test
[ "${1:-}" = "--self-test-only" ] && { echo "check-accent-text: self-test OK"; exit 0; }

mapfile -t files < <(find static internal -type f \( -name '*.css' -o -name '*.js' -o -name '*.templ' \) \
  -not -path 'static/vendor/*' -not -path 'static/css/app.css' -not -path '*/node_modules/*' | sort)

bad=""
while IFS= read -r line; do
  [ -z "$line" ] && continue
  file="${line%%:*}"
  decl="${line#*: }"
  allowed=0
  while IFS='|' read -r apath adecl _reason; do
    [[ -z "$apath" || "$apath" == \#* ]] && continue
    [ "$file" = "$apath" ] && [ "$decl" = "$adecl" ] && allowed=1
  done <"$ALLOWLIST"
  [ "$allowed" = 0 ] && bad+="$line"$'\n'
done < <(scan "${files[@]}")

if [ -n "$bad" ]; then
  echo "ERROR: text reads the raw accent; use var(--color-accent-link, var(--color-accent)) instead:"
  printf '%s' "$bad"
  exit 1
fi
echo "check-accent-text: OK"
