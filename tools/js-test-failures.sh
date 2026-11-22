#!/usr/bin/env bash
# tools/js-test-failures.sh
#
# Prints each failed test of a `node --test` TAP stream again, with the block
# that says why and where. The stream names a failure only where it happens,
# and a group's log is thousands of lines long, so that place can be far above
# the last lines a reader can always reach. The JS tests job ends a failed run
# with this, so the names are in those last lines.
#
# Usage: js-test-failures.sh <log>
#
# Exit: 0 / 2 bad usage.

set -euo pipefail

log="${1:-}"
[ -n "$log" ] && [ -r "$log" ] || { echo "usage: js-test-failures.sh <log>" >&2; exit 2; }

# A block ends at its "..." line; the cap keeps one long diff from pushing the
# next name out of reach.
echo "Failed tests, named again from the stream above:"
awk '
  /^[[:space:]]*not ok / { left = 40; found++; print "" }
  left > 0 { print; left--; if ($0 ~ /^[[:space:]]*\.\.\.[[:space:]]*$/) left = 0 }
  END { if (!found) print "\nThe stream names none: the run ended without reporting a test as failed." }
' "$log"
