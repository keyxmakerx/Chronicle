#!/usr/bin/env bash
# test-plugin-isolation.sh — self-tests for tools/check-plugin-isolation.sh.
#
# The guard went unnoticed-red for nine consecutive commits during sweep R4
# (stages 16..25) because nothing ran it and nothing tested it. Two things
# close that: CI already invokes the guard, and this script proves the guard
# still bites — in particular that amendment R4-S26-A (const-registry files)
# is an exemption for DECLARATIONS ONLY and not a back door for call sites.
#
# Each case builds a throwaway git repo whose paths mirror the real ones the
# guard's allowlists key on, commits a base on `main`, adds lines on a branch,
# and runs the real guard with DIFF_BASE=main.
#
# Usage: tools/test-plugin-isolation.sh   (exit 0 = all self-tests pass)

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="${repo_root}/tools/check-plugin-isolation.sh"

if [[ ! -x "${guard}" ]]; then
  echo "FATAL: guard not found or not executable: ${guard}" >&2
  exit 1
fi

pass=0
fail=0

# The forbidden token is reconstructed the same way the guard does it, so this
# test file never trips the guard it tests.
slug="cal""endar"

# ---------------------------------------------------------------------------
# run_case <name> <expected_exit> <file_path> <added_line>
# ---------------------------------------------------------------------------
run_case() {
  local name="$1" expected="$2" path="$3" line="$4"

  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN

  (
    cd "${tmp}" || exit 1
    git init -q -b main .
    git config user.email t@t
    git config user.name t
    mkdir -p "$(dirname "${path}")" tools
    # Base version of the file: no forbidden token anywhere.
    printf 'package p\n\n// base\n' > "${path}"
    cp "${guard}" tools/check-plugin-isolation.sh
    git add -A
    git commit -qm base
    git checkout -q -b feature
    printf '%s\n' "${line}" >> "${path}"
    git add -A
    git commit -qm change
  ) >/dev/null 2>&1

  local actual
  ( cd "${tmp}" && DIFF_BASE=main ./tools/check-plugin-isolation.sh ) >/dev/null 2>&1
  actual=$?

  if [[ "${actual}" == "${expected}" ]]; then
    echo "  PASS  ${name} (exit ${actual})"
    pass=$((pass + 1))
  else
    echo "  FAIL  ${name} — expected exit ${expected}, got ${actual}"
    fail=$((fail + 1))
  fi
}

registry="internal/plugins/campaigns/import_report.go"
ordinary="internal/plugins/campaigns/export_service.go"

echo "test-plugin-isolation: running self-tests"

# 1. The allowance works at all: a bare const declaration in a registry file.
run_case "const declaration in a const-registry file is allowed" \
  0 "${registry}" "	SectionCalendar = \"${slug}\""

# 2. THE ANTI-BYPASS. The same slug on a call-site line in the SAME file must
#    still fail. A blanket always_allowed_prefixes entry would pass this, which
#    is exactly why amendment R4-S26-A exists instead.
run_case "call site in a const-registry file still fails" \
  1 "${registry}" "	report.Fail(\"${slug}\", \"x\", n, e)"

# 3. Anchoring: code hiding to the right of a declaration must still fail.
run_case "code trailing a declaration in a registry file still fails" \
  1 "${registry}" "	X = \"${slug}\"; report.Fail(\"${slug}\", \"x\", n, e)"

# 4. The allowance is file-scoped: a declaration-shaped line elsewhere fails.
run_case "declaration-shaped line outside the registry still fails" \
  1 "${ordinary}" "	SectionCalendar = \"${slug}\""

# 5. The guard still bites on the ordinary case it has always caught.
run_case "call site in an ordinary plugin file fails" \
  1 "${ordinary}" "	report.Fail(\"${slug}\", \"x\", n, e)"

# 6. No false positive on unrelated added code.
run_case "unrelated added line passes" \
  0 "${ordinary}" "	report.Fail(\"notes\", \"note\", n, e)"

# 7. A whole-line comment naming the slug is documentation, not a reference
#    (RC-15.2). Kept here so a future edit to the comment-strip can't silently
#    regress it.
run_case "whole-line comment naming a slug passes" \
  0 "${ordinary}" "	// the ${slug} plugin owns this"

echo
echo "test-plugin-isolation: ${pass} passed, ${fail} failed"
[[ "${fail}" -eq 0 ]] || exit 1
exit 0
