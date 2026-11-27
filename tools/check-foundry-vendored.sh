#!/usr/bin/env bash
# tools/check-foundry-vendored.sh [--manifest FILE] [--root DIR]
#
# The Foundry module keeps byte-for-byte copies of some Chronicle page
# scripts and stylesheets. Nothing else tells it when one changes, so the
# copy drifts silently. This recomputes each listed file's sha256 prefix and
# fails when a file is missing or its bytes no longer match the manifest.
#
# --manifest / --root point at a fixture instead of the repo (used by the
# self-test, tools/test-foundry-vendored.sh).
#
# Exit: 0 every file matches / 1 a file changed, is missing, or a line is
# malformed / 2 usage.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$ROOT/internal/plugins/fou""ndry_vtt/vendored-assets.txt"
while [ $# -gt 0 ]; do
  case "$1" in
    --manifest) manifest="$2"; shift 2 ;;
    --root)     ROOT="$2"; shift 2 ;;
    *) echo "usage: check-foundry-vendored.sh [--manifest FILE] [--root DIR]" >&2; exit 2 ;;
  esac
done

refresh="The Foundry module keeps a copy of this file. Update its hash in the manifest ($manifest) and open a module refresh: copy it into vendor/chronicle/ with tools/refresh-vendor.mjs on a module branch with the same name as this one, so the sync bench tests them together."

bad=0
total=0
n=0
while IFS= read -r line || [ -n "$line" ]; do
  n=$((n + 1))
  case "$line" in ''|'#'*) continue ;; esac
  read -r want path engine extra <<<"$line"
  if [ -z "${engine:-}" ] || [ -n "${extra:-}" ] || ! [[ "$want" =~ ^[0-9a-f]{12}$ ]]; then
    echo "check-foundry-vendored: manifest line $n is malformed (want <12-hex>  <path>  <engine>): $line" >&2
    bad=$((bad + 1))
    continue
  fi
  total=$((total + 1))
  if [ ! -f "$ROOT/$path" ]; then
    echo "check-foundry-vendored: $path is missing. $refresh" >&2
    bad=$((bad + 1))
    continue
  fi
  got="$(sha256sum "$ROOT/$path" | cut -c1-12)"
  if [ "$got" != "$want" ]; then
    echo "check-foundry-vendored: $path changed (hash $got, manifest says $want). $refresh" >&2
    bad=$((bad + 1))
  fi
done <"$manifest"

if [ "$bad" -gt 0 ]; then
  echo "check-foundry-vendored: $bad problem(s) in $total listed files" >&2
  exit 1
fi
echo "check-foundry-vendored: OK ($total files match)"
