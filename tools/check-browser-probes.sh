#!/usr/bin/env bash
# tools/check-browser-probes.sh
#
# THE BROWSER PROBES ACTUALLY RUN.
#
# Fix id: guards/probes-never-run-in-ci (C-SWEEP-R4).
#
# THE RULE, AND WHY IT IS A RULE. Chronicle's browser probes are the only tests
# in the repo that look at the RENDERED result: they build a page, drive a real
# headless Chromium over it, read geometry back out of the live layout and
# assert on it. Everything else in the suite asserts on strings — the HTML we
# emitted, the CSS we wrote — which cannot tell you whether a container query
# resolved, whether a shelf collided with the ledger, whether a card anchored to
# its day, or whether the phone breakpoint actually swapped the assembly.
#
# Every one of them was a silent pass, twice over:
#
#   1. They all begin `if testing.Short() { t.Skip(...) }`, and `-short` is the
#      mode CI's "Build & Test" job runs (`go test ./... -v -short`) AND the
#      mode `make verify` runs. So no probe had ever executed in either.
#   2. They then skip when no Chromium binary is present — and a `go test` SKIP
#      is reported inside an `ok` package line. A machine with no browser
#      produced a green run indistinguishable from one that measured everything.
#
# A test that cannot run is not coverage, but it reads as coverage, which is
# strictly worse than having none: it is the reason a rendering regression could
# land on main with a fully green build. C-SWEEP-R4 / guards/probes-never-run-in-ci.
#
# WHAT THIS GUARD DOES.
#
#   * It finds a browser the same way the Go probes do — CHROMIUM_BIN, then the
#     usual names on PATH, then the Playwright cache layouts
#     ($PLAYWRIGHT_BROWSERS_PATH, /opt/pw-browsers, ~/.cache/ms-playwright).
#   * With a browser present it runs the probes WITHOUT `-short` and then
#     REQUIRES a `--- PASS:` line for every probe by name. A probe that skipped,
#     was renamed, was deleted, or never ran fails this guard. That is the half
#     that makes "never a silent pass" true: once the machine can run a probe,
#     not running it is an error.
#   * With no browser it prints a LOUD banner NAMING every probe that did not
#     run and why, and exits 0 — a named skip, not a silent one — unless
#     BROWSER_PROBES_REQUIRED=1, which CI sets so that a runner whose browser
#     install broke reddens the build instead of quietly testing nothing.
#
# TWO DEPENDENCIES, NAMED SEPARATELY. TestProbe_MobileBreakpointSwapInRealBrowser
# also needs the Tailwind CLI, because the breakpoint it measures only exists in
# the BUILT stylesheet. It is tracked as its own group so a missing Tailwind
# cannot silently take the Chromium-only probes down with it, and so the banner
# names the dependency that is actually missing.
#
# WHAT IS DELIBERATELY NOT HERE. The opt-in explorers and screenshot generators
# (DAYCARD_GEOMETRY, DAYCARD_FLOORS, DAYCARD_MORPH_TRACE, the *_screenshot_gen
# tests). Those are measurement tools that emit artefacts for a human to look
# at; they assert nothing, so requiring them would be requiring noise. This
# guard covers the probes that FAIL when the rendered result is wrong.
#
# SELF-TEST: every run first exercises the PASS/SKIP assertion against synthetic
# `go test` logs, so "OK" always means the rule can actually fire — a guard that
# cannot fail is worse than no guard. Run just the self-test with:
# --self-test-only
#
# Exit codes:
#   0 — every covered probe PASSED, or no browser and the loud named skip
#   1 — a covered probe skipped/failed/vanished, or no browser under
#       BROWSER_PROBES_REQUIRED=1
#   2 — the guard's own self-test failed (the guard is broken, not the code)

set -uo pipefail

# --- what is covered ---------------------------------------------------------
#
# Package → probes that need only a browser. Space-separated, one line each.

PKG_CALENDAR_BLOCK="./internal/widgets/calendar_block/"
PROBES_CALENDAR_BLOCK="TestProbe_ContainerQuerySizingInRealBrowser \
TestProbe_ShelfGeometryIsInvariant \
TestProbe_StdTierFilledShelfDoesNotCollideWithTheFilledLedger \
TestProbe_LedgerHeightIsInvariantUnderSelection \
TestProbe_StdTierFilledLedgerDoesNotCollide"

PKG_CALENDAR="./internal/plugins/calendar/"
PROBES_CALENDAR="TestDayCardReducedMotionAnchorsToItsDay \
TestDayCardMorphInterpolates"

# Needs the Tailwind CLI on top of the browser.
PROBES_CALENDAR_TAILWIND="TestProbe_MobileBreakpointSwapInRealBrowser"

# --- dependency discovery ----------------------------------------------------
#
# Mirrors findProbeChromium/findChromium in the Go probes. Kept in step
# deliberately: if this script found a browser the tests could not, it would
# demand a PASS from a probe that had no way to run.

find_chromium() {
  if [[ -n "${CHROMIUM_BIN:-}" && -x "${CHROMIUM_BIN}" ]]; then
    echo "${CHROMIUM_BIN}"; return 0
  fi
  local name
  for name in chromium chromium-browser google-chrome google-chrome-stable; do
    local p
    p="$(command -v "${name}" 2>/dev/null)" && { echo "${p}"; return 0; }
  done
  local pattern
  for pattern in \
    "${PLAYWRIGHT_BROWSERS_PATH:-/opt/pw-browsers}"/chromium-*/chrome-linux/chrome \
    /opt/pw-browsers/chromium-*/chrome-linux/chrome \
    "${HOME}"/.cache/ms-playwright/chromium-*/chrome-linux/chrome; do
    local m
    for m in ${pattern}; do
      [[ -x "${m}" ]] && { echo "${m}"; return 0; }
    done
  done
  return 1
}

find_tailwind() {
  if [[ -n "${TAILWIND_BIN:-}" && -x "${TAILWIND_BIN}" ]]; then
    echo "${TAILWIND_BIN}"; return 0
  fi
  [[ -x node_modules/.bin/tailwindcss ]] && { echo "node_modules/.bin/tailwindcss"; return 0; }
  local p
  p="$(command -v tailwindcss 2>/dev/null)" && { echo "${p}"; return 0; }
  return 1
}

# --- the comparison core -----------------------------------------------------

# assert_probes_passed <logfile> <probe names…>
#
# Prints one diagnostic line per probe that did not PASS and returns 1 if any
# did. Kept pure — it reads a log and nothing else — so the self-test exercises
# the real rule rather than a copy of it.
assert_probes_passed() {
  local log="$1"; shift
  local missing=0 name
  for name in "$@"; do
    if grep -q -- "--- PASS: ${name}" "${log}"; then
      continue
    fi
    missing=1
    if grep -q -- "--- SKIP: ${name}" "${log}"; then
      echo "  SKIPPED  ${name} — the machine can drive a browser, so a skip here is a"
      echo "           real gap, not an absent dependency. Its skip reason:"
      sed -n "/--- SKIP: ${name}/,+1p" "${log}" | tail -n 1 | sed 's/^/           /'
    elif grep -q -- "--- FAIL: ${name}" "${log}"; then
      echo "  FAILED   ${name} — the rendered result is wrong. This is the guard working."
    else
      echo "  ABSENT   ${name} — never ran. Renamed or deleted? Update this guard's list"
      echo "           in the same commit, or the coverage disappears silently."
    fi
  done
  return "${missing}"
}

# --- self-test ---------------------------------------------------------------

self_test() {
  local dir fail=0
  dir="$(mktemp -d)"
  trap 'rm -rf "${dir}"' RETURN

  printf -- '--- PASS: TestA (1.00s)\n--- PASS: TestB (1.00s)\nok\n' > "${dir}/green.log"
  if ! assert_probes_passed "${dir}/green.log" TestA TestB > /dev/null; then
    echo "  self-test FAILED: two passing probes were reported as a problem" >&2
    fail=1
  fi

  printf -- '--- PASS: TestA (1.00s)\n--- SKIP: TestB (0.00s)\n    x.go:1: browser probe: no Chromium\n' \
    > "${dir}/skip.log"
  local out
  out="$(assert_probes_passed "${dir}/skip.log" TestA TestB)" && {
    echo "  self-test FAILED: a SKIPPED probe passed the guard — this is the exact" >&2
    echo "                    silent pass the guard exists to end" >&2
    fail=1
  }
  [[ "${out}" == *"SKIPPED  TestB"* ]] || {
    echo "  self-test FAILED: the SKIPPED diagnostic did not name the probe" >&2; fail=1; }

  printf -- '--- PASS: TestA (1.00s)\n--- FAIL: TestB (1.00s)\n' > "${dir}/fail.log"
  out="$(assert_probes_passed "${dir}/fail.log" TestA TestB)" && {
    echo "  self-test FAILED: a FAILING probe passed the guard" >&2; fail=1; }
  [[ "${out}" == *"FAILED   TestB"* ]] || {
    echo "  self-test FAILED: the FAILED diagnostic did not name the probe" >&2; fail=1; }

  printf -- '--- PASS: TestA (1.00s)\nok\n' > "${dir}/gone.log"
  out="$(assert_probes_passed "${dir}/gone.log" TestA TestB)" && {
    echo "  self-test FAILED: a probe that never ran passed the guard — a renamed" >&2
    echo "                    probe would take its coverage with it unnoticed" >&2
    fail=1; }
  [[ "${out}" == *"ABSENT   TestB"* ]] || {
    echo "  self-test FAILED: the ABSENT diagnostic did not name the probe" >&2; fail=1; }

  if (( fail )); then
    echo "check-browser-probes: SELF-TEST FAILED — the guard is broken; fix it before trusting a pass." >&2
    return 1
  fi
  return 0
}

self_test || exit 2

if [[ "${1:-}" == "--self-test-only" ]]; then
  echo "check-browser-probes: self-test OK (SKIP / FAIL / ABSENT each fire, green stays quiet)."
  exit 0
fi

# --- run ---------------------------------------------------------------------

CHROME="$(find_chromium)" || CHROME=""
TAILWIND="$(find_tailwind)" || TAILWIND=""

if [[ -z "${CHROME}" ]]; then
  echo "############################################################"
  echo "# BROWSER PROBES DID NOT RUN — NO CHROMIUM ON THIS MACHINE  #"
  echo "############################################################"
  echo
  echo "These are the only tests that look at the RENDERED result. None of them"
  echo "ran, so nothing below has been measured on this machine:"
  echo
  for name in ${PROBES_CALENDAR_BLOCK} ${PROBES_CALENDAR} ${PROBES_CALENDAR_TAILWIND}; do
    echo "  NOT RUN  ${name}"
  done
  echo
  echo "Install one, or point CHROMIUM_BIN at it:"
  echo "  npx playwright install chromium     (PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers)"
  echo "  apt-get install -y chromium-browser"
  echo
  if [[ "${BROWSER_PROBES_REQUIRED:-}" == "1" ]]; then
    echo "BROWSER_PROBES_REQUIRED=1 — this runner is supposed to have a browser."
    echo "Reporting success here would be a green build that rendered nothing."
    exit 1
  fi
  echo "check-browser-probes: SKIPPED, loudly and by name (set BROWSER_PROBES_REQUIRED=1 to make this fatal)."
  exit 0
fi

echo "check-browser-probes: driving ${CHROME}"

LOG="$(mktemp)"
trap 'rm -f "${LOG}"' EXIT
status=0

run_group() {
  local pkg="$1"; shift
  local names="$*"
  local pattern
  pattern="$(echo "${names}" | tr -s ' ' '|')"
  # NOT -short: skipping under it is exactly how these probes stayed unmeasured.
  # -count=1 so a cached PASS from a previous tree can never stand in for a run.
  go test "${pkg}" -count=1 -v -run "^(${pattern})$" > "${LOG}" 2>&1
  if ! assert_probes_passed "${LOG}" ${names}; then
    echo "  (full output below)"
    sed 's/^/  | /' "${LOG}"
    return 1
  fi
  local n
  for n in ${names}; do echo "  PASS     ${n}"; done
  return 0
}

run_group "${PKG_CALENDAR_BLOCK}" ${PROBES_CALENDAR_BLOCK} || status=1
run_group "${PKG_CALENDAR}" ${PROBES_CALENDAR} || status=1

if [[ -n "${TAILWIND}" ]]; then
  echo "check-browser-probes: building CSS with ${TAILWIND}"
  run_group "${PKG_CALENDAR}" ${PROBES_CALENDAR_TAILWIND} || status=1
else
  echo
  echo "########################################################"
  echo "# ONE BROWSER PROBE DID NOT RUN — NO TAILWIND CLI       #"
  echo "########################################################"
  for name in ${PROBES_CALENDAR_TAILWIND}; do
    echo "  NOT RUN  ${name} — needs the BUILT stylesheet; the breakpoint it"
    echo "           measures does not exist in the source CSS."
  done
  echo "  Fix: npm install tailwindcss@3.4.17, or set TAILWIND_BIN."
  echo
  if [[ "${BROWSER_PROBES_REQUIRED:-}" == "1" ]]; then
    echo "BROWSER_PROBES_REQUIRED=1 — this runner is supposed to have it."
    status=1
  fi
fi

if (( status )); then
  echo
  echo "check-browser-probes: FAILED — see above. (self-test OK, so the rule fired for a reason.)" >&2
  exit 1
fi

echo "check-browser-probes: OK — every covered browser probe ran and passed. (self-test OK)"
