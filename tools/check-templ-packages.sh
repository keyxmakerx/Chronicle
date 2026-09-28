#!/usr/bin/env bash
# tools/check-templ-packages.sh
#
# Every directory of .templ sources carries at least one hand-written .go
# file (a doc.go is enough). Generated *_templ.go files are gitignored, so a
# checkout that never runs `templ generate` (Dependabot's) sees a
# templ-only package as empty, `go mod tidy` fails looking for it as a
# module, and every Go dependency update fails with it.
#
# Exit: 0 every templ package has a hand-written .go file / 1 one lacks it /
# 2 self-test failed. Self-test only: --self-test-only.

set -euo pipefail

# missing <root>: prints each directory under <root> holding .templ files
# but no .go file other than generated or test files.
missing() {
  local root="$1" d
  find "$root" -name '*.templ' -not -path '*/node_modules/*' -printf '%h\n' | sort -u | while IFS= read -r d; do
    if ! find "$d" -maxdepth 1 -name '*.go' ! -name '*_templ.go' ! -name '*_test.go' | grep -q .; then
      echo "${d#"$root"/}"
    fi
  done
}

self_test() {
  local t
  t="$(mktemp -d)"
  trap 'rm -rf "$t"' RETURN
  mkdir -p "$t/a" "$t/b"
  touch "$t/a/page.templ" "$t/a/page_templ.go" "$t/a/page_test.go" "$t/b/page.templ" "$t/b/doc.go"
  if [ "$(missing "$t")" != "a" ]; then
    echo "check-templ-packages: self-test failed (expected only 'a' flagged)" >&2
    return 1
  fi
}

if ! self_test; then exit 2; fi
[ "${1:-}" = "--self-test-only" ] && { echo "check-templ-packages: self-test OK"; exit 0; }

cd "$(dirname "$0")/.."
bad="$(missing .)"
if [ -n "$bad" ]; then
  echo "check-templ-packages: these packages hold only .templ sources; add a hand-written doc.go to each (see internal/templates/pages/doc.go):" >&2
  echo "$bad" | sed 's/^/  /' >&2
  exit 1
fi
echo "check-templ-packages: OK"
