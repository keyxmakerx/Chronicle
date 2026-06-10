#!/bin/bash
# SessionStart hook: prepare the Go/templ toolchain so tests and linters
# run immediately in Claude Code web sessions. Idempotent; web-only.
set -euo pipefail
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi
cd "$CLAUDE_PROJECT_DIR"
# templ generator pinned to the go.mod version (needed for `templ generate`).
if ! command -v templ >/dev/null 2>&1; then
  go install github.com/a-h/templ/cmd/templ@v0.3.1001
fi
echo "export PATH=\"\$PATH:$(go env GOPATH)/bin\"" >> "$CLAUDE_ENV_FILE"
# Warm the module cache so the first go build/test doesn't stall.
go mod download
