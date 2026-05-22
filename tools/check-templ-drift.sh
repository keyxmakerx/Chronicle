#!/usr/bin/env bash
# check-templ-drift.sh — fail if any *.templ.go file is stale relative to
# its *.templ source.
#
# Mechanism per cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D6
# ("fail-PR-on-drift templ guard"). Catches the "forgot to run templ generate
# before committing" scenario where the .templ source changed but the
# generated .templ.go file is out of date.
#
# Approach: run `templ generate`, then compare the working tree to its
# pre-generation state via git diff. A clean exit = no drift; any diff = drift.

set -euo pipefail

# Use the pinned templ version from ci.yml (kept in sync with go.mod).
# If templ isn't on PATH, attempt to install the pinned version.
TEMPL_VERSION="${TEMPL_VERSION:-v0.3.1001}"

if ! command -v templ >/dev/null 2>&1; then
  echo "check-templ-drift: installing templ ${TEMPL_VERSION}..."
  go install "github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}"
  export PATH="$(go env GOPATH)/bin:${PATH}"
fi

# Generate against the current source tree.
templ generate

# Any diff against the index means the generated output drifted from what's
# checked in. `git diff --exit-code` returns 1 on diff, 0 on clean.
if ! git diff --exit-code -- '*.templ.go' >/dev/null 2>&1; then
  echo
  cat <<MSG
ERROR — generated .templ.go files are out of date.

The following .templ.go files differ from what 'templ generate' produces:

MSG
  git diff --stat -- '*.templ.go'
  cat <<MSG

Run 'templ generate' locally and commit the regenerated files. The CI generator
is pinned to ${TEMPL_VERSION} (matching go.mod's templ runtime); make sure your
local install matches:
  go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}

This guard exists because forgetting to regenerate produces a build that diverges
from the .templ source in subtle ways (missing component changes, stale signatures);
catching it at PR time is cheaper than chasing the symptom in production.
MSG
  exit 1
fi

echo "check-templ-drift: OK — generated .templ.go files match their sources."
exit 0
