#!/usr/bin/env bash
# tools/test-foundry-vendored.sh
#
# Self-test for check-foundry-vendored.sh against a fixture tree: a file that
# matches passes, a changed file fails, a missing file fails, and a malformed
# manifest line fails rather than being skipped.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
t="$(mktemp -d)"
trap 'rm -rf "$t"' EXIT

mkdir -p "$t/static/js"
printf 'one\n' >"$t/static/js/a.js"
printf 'two\n' >"$t/static/js/b.js"
ha="$(sha256sum "$t/static/js/a.js" | cut -c1-12)"
hb="$(sha256sum "$t/static/js/b.js" | cut -c1-12)"

run() { "$ROOT/tools/check-foundry-vendored.sh" --manifest "$t/list" --root "$t" >"$t/out" 2>&1; }

fail() { echo "test-foundry-vendored: FAIL: $1" >&2; cat "$t/out" >&2; exit 1; }

printf '# header\n\n%s  static/js/a.js  sky\n%s  static/js/b.js  shop\n' "$ha" "$hb" >"$t/list"
run || fail "matching files should pass"

printf 'changed\n' >"$t/static/js/b.js"
run && fail "a changed file should fail"
grep -q 'static/js/b.js changed' "$t/out" || fail "a changed file should be named"
grep -q 'keeps a copy of this file' "$t/out" || fail "the message should say the module keeps a copy"
printf 'two\n' >"$t/static/js/b.js"

rm "$t/static/js/a.js"
run && fail "a missing file should fail"
grep -q 'static/js/a.js is missing' "$t/out" || fail "a missing file should be named"
printf 'one\n' >"$t/static/js/a.js"

printf '%s  static/js/a.js\n' "$ha" >"$t/list"
run && fail "a malformed line should fail"

echo "test-foundry-vendored: OK"
