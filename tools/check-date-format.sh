#!/usr/bin/env bash
# tools/check-date-format.sh
#
# A real-world date a person reads is written the way their own device writes
# it (static/js/dates.js), so a template never spells a date layout itself:
# `Format("Jan 2, 2006")` is month first and `Format("2 Jan 2006")` is day
# first, and the two together are the clash this guard keeps out. Use
# components.Date, or timeutil.FixedDate where a plain string is needed.
#
# Scans *.templ for a Format layout that starts with a month or weekday name
# or with a day then a month name. Machine layouts (2006-01-02, RFC3339) are
# not dates a person reads and pass. The helper itself lives in
# internal/timeutil/date.go, which is Go, so there is no exemption.
#
# Exit: 0 clean / 1 a template writes a date layout / 2 self-test failed

set -euo pipefail

PATTERN='Format\("(Mon|Monday|Jan|January|2 Jan|2 January)\b'

# scan <file>... prints the offending lines, "file:line:text".
scan() { grep -nHE "$PATTERN" "$@" 2>/dev/null || true; }

self_test() {
  local d; d=$(mktemp -d)
  printf '%s\n' '{ t.Format("Jan 2, 2006") }' > "$d/a.templ"
  printf '%s\n' '{ t.Format("2 Jan 2006") }' > "$d/b.templ"
  printf '%s\n' '{ t.Format("Monday 2 January") }' > "$d/c.templ"
  printf '%s\n' '{ t.Format("2006-01-02") } { t.Format(time.RFC3339) } { timeutil.FixedDate(t, timeutil.DateShort) }' > "$d/ok.templ"
  local bad ok
  bad=$(scan "$d/a.templ" "$d/b.templ" "$d/c.templ" | wc -l)
  ok=$(scan "$d/ok.templ" | wc -l)
  rm -rf "$d"
  if [ "$bad" -ne 3 ] || [ "$ok" -ne 0 ]; then
    echo "check-date-format: self-test failed (bad=$bad ok=$ok)" >&2
    exit 2
  fi
}

self_test
[ "${1:-}" = "--self-test-only" ] && { echo "check-date-format: self-test OK"; exit 0; }

hits=$(git ls-files '*.templ' | xargs -r grep -nHE "$PATTERN" 2>/dev/null || true)
if [ -n "$hits" ]; then
  echo "ERROR: these templates write a date layout; use components.Date (or timeutil.FixedDate for a string):"
  printf '%s\n' "$hits"
  exit 1
fi
echo "check-date-format: OK"
