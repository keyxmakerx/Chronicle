#!/usr/bin/env bash
# check-live-todos.sh: fail when the live checks hold more known-bug "todo"
# subtests than the budget below.
#
# Why: a todo subtest reports a bug without failing CI, so todos pile up
# unseen. The budget only goes down: when a fix lets a todo pass, drop the
# todo and lower LIVE_TODO_BUDGET to the new count in the same PR. A new
# todo means fixing an old one first, or the bug goes in an issue and the
# check waits until it is fixed. The open todos are listed in issue #2143.

set -euo pipefail
cd "$(dirname "$0")/.."

LIVE_TODO_BUDGET=11

count=$(grep -ho '{ *todo:' test/live/*.spec.mjs | wc -l | tr -d ' ')
if (( count > LIVE_TODO_BUDGET )); then
  echo "check-live-todos: FAIL: $count todo subtests in test/live, budget is $LIVE_TODO_BUDGET" >&2
  grep -n '{ *todo:' test/live/*.spec.mjs | cut -c1-160 >&2
  echo "Fix a known bug and drop its todo instead of adding one." >&2
  exit 1
fi
if (( count < LIVE_TODO_BUDGET )); then
  echo "check-live-todos: FAIL: only $count todo subtests now; lower LIVE_TODO_BUDGET in $0 to $count" >&2
  exit 1
fi
echo "check-live-todos: OK ($count of $LIVE_TODO_BUDGET)."
