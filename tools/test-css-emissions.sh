#!/usr/bin/env bash
# tools/test-css-emissions.sh
#
# Self-test for check-css-emissions.sh against a tiny fixture stylesheet (no
# Tailwind build): the pass path, a missing selector, and the shapes the
# minifier produces (grouped selectors, @media nesting, quote-less attribute
# selectors, combinators).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
t="$(mktemp -d)"
trap 'rm -rf "$t"' EXIT

cat >"$t/app.css" <<'CSS'
.a{color:red}.b,.c>.d{margin:0}@media (pointer:coarse){.e{opacity:1}}.f[class~=g]{x:y;z:1px,2px}
.h:not(:last-child){border:0}
CSS

run() { "$ROOT/tools/check-css-emissions.sh" --css "$t/app.css" --list "$t/list" >/dev/null 2>&1; }

fail() { echo "test-css-emissions: FAIL: $1" >&2; exit 1; }

printf '# comment\n\n.a\n.b\n.c > .d\n.e\n.f[class~="g"]\n.h:not(:last-child)\n' >"$t/list"
run || fail "all-present list should pass"

printf '.a\n.zzz\n' >"$t/list"
run && fail "missing selector should fail"

# A declaration value must not satisfy a selector entry.
printf '2px\n' >"$t/list"
run && fail "declaration text is not a selector"

# A selector that only appears as the tail of another must not match.
printf '.d\n' >"$t/list"
run && fail "partial selector should fail"

echo "test-css-emissions: OK"
