#!/usr/bin/env bash
# tools/js-test-files.sh
#
# Prints the JS test files in one CI group, so the slow browser tests run on
# runners of their own. The dice tests are CPU-bound on a software GPU, so
# they gain nothing from sharing a runner; every other file, a new one
# included, lands in `general`.
#
# Usage: js-test-files.sh general|dice|crits-succ|crits-fail|all
#        js-test-files.sh --check   every file is in exactly one group
#
# Exit: 0 / 1 a file is in no group or in two / 2 bad usage.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

GROUPS_ALL=(general dice crits-succ crits-fail)

group() {
  local f
  case "$1" in
    general)    for f in test/js/*.test.mjs; do [[ $f == test/js/dice_*browser.test.mjs ]] || echo "$f"; done ;;
    dice)       echo test/js/dice_browser.test.mjs ;;
    crits-succ) echo test/js/dice_crits_succ_browser.test.mjs ;;
    crits-fail) echo test/js/dice_crits_fail_browser.test.mjs ;;
    all)        ls test/js/*.test.mjs ;;
    *)          echo "unknown group: $1" >&2; exit 2 ;;
  esac
}

if [[ "${1:-}" == "--check" ]]; then
  want="$(group all | sort)"
  got="$(for g in "${GROUPS_ALL[@]}"; do group "$g"; done | sort)"
  if [[ "$want" != "$got" ]]; then
    echo "JS test files missing from a group, or in two:" >&2
    diff <(echo "$want") <(echo "$got") >&2 || true
    exit 1
  fi
  for f in $got; do [[ -f $f ]] || { echo "group names a missing file: $f" >&2; exit 1; }; done
  echo "OK: $(echo "$got" | wc -l) JS test files, each in one group"
  exit 0
fi

[[ $# -eq 1 ]] || { sed -n '9,10p' "${BASH_SOURCE[0]}" >&2; exit 2; }
group "$1"
