#!/usr/bin/env bash
# test-comment-clutter.sh: self-tests for tools/check-comment-clutter.sh.
#
# Each case builds a throwaway git repo, commits a base on `main`, adds one
# line on a branch, and runs the real guard with DIFF_BASE=main.
#
# Usage: tools/test-comment-clutter.sh   (exit 0 = all self-tests pass)

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="${repo_root}/tools/check-comment-clutter.sh"
[[ -x "${guard}" ]] || { echo "FATAL: guard not found or not executable: ${guard}" >&2; exit 1; }

pass=0
fail=0

# Tokens are split so this file never reads as clutter to anything scanning it.
id="C""-CAL""-7"
arch="cord""inator/"
day="2026""-10""-09"

# run_case <name> <expected_exit> <file_path> <added_line>
run_case() {
  local name="$1" expected="$2" path="$3" line="$4" tmp actual
  tmp="$(mktemp -d)"
  (
    cd "${tmp}" || exit 1
    git init -q -b main .
    git config user.email t@t
    git config user.name t
    mkdir -p "$(dirname "${path}")" tools
    printf 'x := 1\n' > "${path}"
    cp "${guard}" tools/check-comment-clutter.sh
    git add -A && git commit -qm base
    git checkout -q -b feature
    printf '%s\n' "${line}" >> "${path}"
    git add -A && git commit -qm change
  ) >/dev/null 2>&1
  ( cd "${tmp}" && DIFF_BASE=main ./tools/check-comment-clutter.sh ) >/dev/null 2>&1
  actual=$?
  rm -rf "${tmp}"
  if [[ "${actual}" == "${expected}" ]]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    echo "FAIL: ${name} (expected exit ${expected}, got ${actual})"
  fi
}

run_case "dispatch ID in a Go comment"        1 internal/a/a.go   "// Added for ${id}."
run_case "archive path in a JS comment"       1 static/js/a.js    "// See ${arch}decisions/x.md"
run_case "this PR in a block comment"         1 internal/a/a.go   " * Moved here in this PR."
run_case "file:line pointer"                  1 internal/a/a.go   "// Mirrors handler.go:120"
run_case "date in a trailing comment"         1 internal/a/a.go   "y := 2 // changed ${day}"
run_case "todo pointer in a shell comment"    1 tools/x.sh        "# booked in .ai/todo.md"
run_case "date in a SQL comment"              1 db/m/001.sql      "-- added ${day}"
run_case "fixture date in a Go test passes"   0 internal/a/a_test.go "// Saturday ${day} 18:00 UTC."
run_case "fixture date in a JS test passes"   0 test/js/a.test.mjs  "// ${day} is a Sunday."
run_case "dispatch ID in a test still fails"  1 internal/a/a_test.go "// For ${id}."
run_case "plain why comment passes"           0 internal/a/a.go   "// Ownership is checked first so a forged id can't reach another campaign."
run_case "issue pointer passes"               0 internal/a/a.go   "// TODO(#613): stop echoing untouched fields."
run_case "date in code, not a comment"        0 internal/a/a.go   "d := \"${day}\""
run_case "URL is not a comment"               0 static/js/a.js    "var u = 'https://example.com/a.go:12';"
run_case "clutter-ok escape"                  0 internal/a/a.go   "// Layout example: ${day} clutter-ok"
run_case "Markdown is not checked"            0 docs/notes.md     "Shipped on ${day} for ${id}."
run_case "security tag is not a dispatch ID"  0 internal/a/a.go   "// Same check as SEC-IDOR-2."

echo "test-comment-clutter: ${pass} passed, ${fail} failed"
(( fail == 0 ))
