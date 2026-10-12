#!/usr/bin/env bash
# tools/tailwind.sh [tailwindcss args...]
#
# Runs the pinned standalone Tailwind CLI, downloading it into bin/ on first
# use (checksum-verified). Same version as the Dockerfile's tailwind stage, so
# a local build, CI and the image produce the same CSS; the standalone build
# bundles the forms and typography plugins tailwind.config.js requires, so no
# Node install is needed. Keep VERSION in step with the Dockerfile.
#
# TAILWINDCSS=/path/to/binary overrides the download (offline machines).
# `tools/tailwind.sh --print-path` prints the binary path without running it.

set -euo pipefail

VERSION="3.4.17"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ -n "${TAILWINDCSS:-}" ]; then
  bin="$TAILWINDCSS"
else
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)                platform="linux-x64";   sha="7d24f7fa191d2193b78cd5f5a42a6093e14409521908529f42d80b11fde1f1d4" ;;
    Linux-aarch64|Linux-arm64)   platform="linux-arm64"; sha="69b1378b8133192d7d2feb12a116fa12d035594f58db3eff215879e4ad8cf39b" ;;
    Darwin-arm64)                platform="macos-arm64"; sha="a1d0c7985759accca0bf12e51ac1dcbf0f6cf2fffb62e6e0f62d091c477a10a3" ;;
    Darwin-x86_64)               platform="macos-x64";   sha="6cbdad74be776c087ffa5e9a057512c54898f9fe8828d3362212dfe32fc933a3" ;;
    *) echo "tailwind.sh: no standalone build for $(uname -s)-$(uname -m); set TAILWINDCSS=/path/to/tailwindcss" >&2; exit 1 ;;
  esac
  bin="$ROOT/bin/tailwindcss-$VERSION-$platform"
  if [ ! -x "$bin" ]; then
    mkdir -p "$ROOT/bin"
    tmp="$(mktemp "$ROOT/bin/.tailwindcss.XXXXXX")"
    trap 'rm -f "$tmp"' EXIT
    echo "tailwind.sh: downloading tailwindcss v$VERSION ($platform)" >&2
    curl -fsSL -o "$tmp" "https://github.com/tailwindlabs/tailwindcss/releases/download/v$VERSION/tailwindcss-$platform"
    if command -v sha256sum >/dev/null 2>&1; then got="$(sha256sum "$tmp" | cut -d' ' -f1)"; else got="$(shasum -a 256 "$tmp" | cut -d' ' -f1)"; fi
    if [ "$got" != "$sha" ]; then
      echo "tailwind.sh: checksum mismatch for $platform (got $got)" >&2
      exit 1
    fi
    chmod +x "$tmp"
    mv "$tmp" "$bin"
    trap - EXIT
  fi
fi

if [ "${1:-}" = "--print-path" ]; then echo "$bin"; exit 0; fi
cd "$ROOT"
exec "$bin" "$@"
