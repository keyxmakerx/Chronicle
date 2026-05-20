#!/usr/bin/env bash
# tools/check-no-instance-hostname.sh
#
# Operator-security guard: scan tracked source for the operator's
# production hostname and fail if found. Per C-SCRUB-INSTANCE-URLS
# (2026-05-20). Once-and-done scrub plus this grep makes the
# requirement permanent — a future PR that reintroduces the
# hostname trips the CI lint job.
#
# The pattern is built from a fragment join so this script itself
# doesn't contain the literal token it's scanning for (otherwise the
# guard would fail-self). Anyone adding more secret-shaped tokens
# should follow the same pattern.

set -euo pipefail

# Build the forbidden pattern via fragment join. Same trick the
# upstream secret-scanner guidance recommends — keep the literal
# out of the file that's looking for it.
forbidden="bnu""uy"

if grep -rn --exclude-dir=.git --exclude-dir=vendor --exclude-dir=node_modules --exclude-dir=tmp --exclude-dir=bin --exclude="$(basename "$0")" "$forbidden" .; then
  echo
  echo "ERROR: operator's production hostname must not appear in tracked source."
  echo "See cordinator/dispatches/chronicle/C-SCRUB-INSTANCE-URLS.md for context."
  exit 1
fi
echo "OK: no instance hostname references in tracked source."
