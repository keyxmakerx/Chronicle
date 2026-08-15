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
#     REQUIRES a TOP-LEVEL `--- PASS: <name> (` line for every probe by name,
#     AND a clean exit from `go test`. A probe that skipped, failed, was
#     renamed, was deleted or never ran fails this guard. That is the half
#     that makes "never a silent pass" true: once the machine can run a probe,
#     not running it is an error. The anchoring is load-bearing — see
#     probe_verdict() for the two log shapes an unanchored match let through.
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
# guard covers the probes that FAIL when the rendered result is wrong. The list
# is now a CENSUS rather than a recollection — see PROBES_CALENDAR for how it
# was taken and how to retake it when a lane adds a probe.
#
# SELF-TEST: every run first exercises the PASS/SKIP/FAIL/ABSENT assertion
# against synthetic `go test` logs, so "OK" always means the rule can actually
# fire — a guard that cannot fail is worse than no guard. The fixtures are the
# real shape `go test -v` emits, copied from a run: the previous ones were
# hand-written to match the code, which is how a matcher that credited a FAILED
# probe as PASS sat under a green self-test. Run just the self-test with:
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
# THE #590 MOONS/SKY PROBES ARE REGISTERED HERE, and their absence was itself a
# finding (2026-08-11 observations report §7.4): six probes were written to catch
# exactly the class of regression that then shipped — a container query that hid
# the per-day moon discs from every phone and most desktops — and not one of them
# ran in CI, because every one self-skips under `-short` and none was on this
# list. The investigators had to run them by hand. A probe nobody runs is not
# coverage, and this file exists to say so.
#
# TestMoonReachProbe_* are that regression's own guards, added by the fix: the
# first measures whether the discs are on the screen at real viewport widths,
# the second drives a genuinely coarse pointer and measures the touch target.
#
# THE LEDGER DAY-PANEL PAIR WERE UNREGISTERED FROM THE DAY THEY LANDED (stage 3,
# 3a23667b) and the census caught them — which is the census doing its job and
# also the reason it exists: they drove a browser, asserted on what they
# measured, and CI ran neither. Note what an unregistered probe costs: the guard
# aborts on the census BEFORE running a single probe, so one forgotten name
# takes down the whole file's coverage, not just its own.
#
# TestCellProbe_* are C-CALV4-SPEC §1's own guards, added with the day cell's
# corner fixes: the first measures that each corner PAINTS and that the moon and
# the GM fold do not share one, the second that the era tint carries the era and
# keeps the cell above its ground in both themes. Both replace string assertions
# that were green while the surface they described was blank.
#
# TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid is the third, and it was
# added because the SECOND one's shape was the lesson and nobody applied it: the
# era tint got a floor AND a ceiling, and in the same change the tile's own 1px
# edge shipped at --rule-structural with neither — 36.1 from the ground on light
# and 39.7 on dark, the loudest step in a grid whose stated purpose that change
# was to soften. It bands the tile edge in both themes and asserts the loudness
# ORDER of the grid's three rules. A step with a floor and no ceiling is how the
# last two loud things shipped.
#
# TestTilePaintProbe_TheTileIsOnTheScreen is the fourth, and it exists because
# the THIRD one passed on a grid that was drawing nothing. A stray `*/` orphaned
# a comment paragraph, the orphan parsed as a selector prelude and swallowed
# `.cal-block-host .cell` whole; `isolation: isolate` went with it, the
# z-index:-1 tile fell behind the grid ground, `container-type` reverted to
# `normal` and every @container query died — the moon silhouette off at every
# width, thirty day cells painting flat ground. The edge probe reported a
# healthy band throughout, because getComputedStyle answers what the CASCADE
# resolved and not what the compositor DREW, and its one plausible reading came
# from `.interc`, the single surface still painting. This one decodes a PNG:
# three flat runs through a tile's top edge, and the gutter's ground between two
# neighbours. Proven red against exactly that regression before it was
# registered here.
PROBES_CALENDAR_BLOCK="TestProbe_ContainerQuerySizingInRealBrowser \
TestProbe_ShelfGeometryIsInvariant \
TestProbe_StdTierFilledShelfDoesNotCollideWithTheFilledLedger \
TestProbe_LedgerHeightIsInvariantUnderSelection \
TestProbe_StdTierFilledLedgerDoesNotCollide \
TestProbe_AtThePhoneWidthTheLedgerStillSitsBelowTheMonth \
TestProbe_DayPanelAppearsOnTapAndCostsTheLedgerNothing \
TestCellProbe_EveryCornerPaintsAndNoTwoShareOne \
TestCellProbe_TheEraTintCarriesTheEraAndKeepsThePop \
TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid \
TestTilePaintProbe_TheTileIsOnTheScreen \
TestMoonReachProbe_TheDiscsAreOnTheScreenAtAPhoneWidth \
TestMoonReachProbe_TheOpenerOnATouchDevice \
TestMoonPanelProbe_TheFoldCostsNothingAndObeysTheRegister \
TestMoonPanelProbe_ReducedMotionIsInstantAndComplete \
TestSkyMeasureProbe_TheThreeCounts \
TestSkyPaneProbe_TheStillsTwoShapes \
TestSkyDiscPaintProbe_TheDiscsAreInkAndNotOnlyWrappers \
TestSkySceneryProbe_TheBandsMoonsAreNotAControl \
TestSkyCloseProbe_TheCloseRendersFrames"

PKG_CALENDAR="./internal/plugins/calendar/"
# C-CALV4-MOBILE registers its six phone probes here. Every one of them is
# browser-gated ONLY — no env var hides them — because a registered probe that
# skips itself by default is precisely the silent pass this guard exists to
# kill, and because the mobile lane's findings are all RENDERED numbers that no
# string assertion can see.
#
# THE REMAINING SIX WERE FOUND BY CENSUS, NOT BY MEMORY. Registering the #590
# probes by name left the list still incomplete, because nobody had ever
# enumerated the browser probes — each lane added its own and registered (or
# forgot) them by hand. The census: every top-level `func Test…` whose body
# reaches a Chromium finder (findProbeChromium / findChromium /
# benchFindChromium / mobileNeedChromium / builderShotChromium) is a browser
# probe. That is 39 tests. Ten are the opt-in generators and explorers excluded
# above by policy — each gated behind its own env var (BENCH_SCREENSHOTS,
# BUILDER_SCREENSHOTS, BUILDER_CLIPS, DAYCARD_SHOTS ×2, DAYCARD_GEOMETRY,
# DAYCARD_FLOORS, DAYCARD_MORPH_TRACE, MOON_SHOTS, BLOCK_SCREENSHOTS). The other
# 29 assert, so all 29 belong here.
#
#   TestBuilderProbe_*        the wizard's narrow lane, its step ladder, and
#                             that switching preset does not move the month.
#   TestDaycardLedgerDoor_*   the Ledger door renders only where it does
#                             something — docked vs stacked, measured.
#   TestYearProbe_*           the emptied-year refusal in calendar_daycard.js
#                             and gm_panel.js. NOTE these two are the only
#                             browser probes in the repo with NO
#                             `testing.Short()` gate: they run under CI's
#                             `-short` job and skip there for want of a
#                             browser, which is the silent pass in its purest
#                             form — a green "Build & Test" that measured
#                             nothing and said so to no one.
#
# THE CENSUS IS NOW RETAKEN ON EVERY RUN — see census_check() below. Adding a
# browser probe and forgetting this file is the mistake that produced the gap in
# the first place, and asking the next person to remember is how it recurs. The
# check is source-only, so it fires on a machine with no browser too.
PROBES_CALENDAR="TestDayCardReducedMotionAnchorsToItsDay \
TestDayCardMorphInterpolates \
TestMobileProbe_TheLedgerShowsThreeRowsOnAPhone \
TestMobileProbe_TheSheetStaysReachableWhenTheViewportShrinks \
TestMobileProbe_ThePageIsLockedBehindASheetAndReleasedOnEveryExit \
TestMobileProbe_TheScrollerCensusAndTheLongWeek \
TestMobileProbe_TheRSVPOverviewAndTheScheduleFitThePhone \
TestMobileProbe_TheTapFloorAtAPhoneWidth \
TestBuilderProbe_TheMonthDidNotMove \
TestBuilderProbe_TheLadderActuallyRuns \
TestBuilderProbe_NarrowLaneHoldsItsGate \
TestDaycardLedgerDoor_ItOnlyRendersWhereItDoesSomething \
TestYearProbe_BenchDateVerbRow \
TestYearProbe_V2GMConsole"

# Needs the Tailwind CLI on top of the browser.
PROBES_CALENDAR_TAILWIND="TestProbe_MobileBreakpointSwapInRealBrowser"

# --- the census --------------------------------------------------------------
#
# THE LISTS ABOVE ARE CHECKED AGAINST THE TREE, NOT TRUSTED.
#
# The gap this guard was extended to close was not a wrong list, it was an
# UNKNOWN one: probes accumulated lane by lane and nobody had ever enumerated
# them, so "is this complete?" had no answer and the six that were missing had
# been missing for as long as they had existed. A list maintained by memory
# drifts the moment someone adds a probe in a hurry; a list checked against the
# tree cannot.
#
# THE RULE. A browser probe is a top-level `func Test…` whose body reaches one
# of the five Chromium finders. Of those, the ones that ALSO read `os.Getenv(`
# are the opt-in generators and explorers — that is the shape of their gate
# (`if os.Getenv("MOON_SHOTS") == "" { t.Skip(…) }`) — and they are excluded by
# the policy stated at the top of this file. Everything else asserts and must be
# registered.
#
# TWO HONEST LIMITS, because a guard that oversells itself is the thing this
# file exists to prevent:
#   * It keys on the five finder names. A probe that launches Chromium through
#     some sixth helper is invisible to the census AND absent from the lists, so
#     it produces no error — it is simply not seen. Add new finders here.
#   * A probe that reads an env var for any reason other than an opt-in gate
#     would be misclassified as a generator and silently excluded. Don't; if a
#     probe needs configuration, give it a default rather than a gate.
census_check() {
  local found registered unregistered vanished
  found="$(
    grep -rl 'findProbeChromium\|findChromium\|benchFindChromium\|mobileNeedChromium\|builderShotChromium' \
      --include='*_test.go' . 2>/dev/null |
    while IFS= read -r f; do
      awk '
        function emit() {
          if (body !~ /findProbeChromium|findChromium|benchFindChromium|mobileNeedChromium|builderShotChromium/) return
          if (body ~ /os\.Getenv\(/) return   # opt-in generator/explorer
          print cur
        }
        /^func /{ if (cur != "") emit()
                  n = $2; sub(/\(.*/, "", n)
                  if ($0 ~ /^func Test/) { cur = n; body = "" } else { cur = "" }
                  next }
                { if (cur != "") body = body "\n" $0 }
        /^}/    { if (cur != "") { emit(); cur = "" } }
      ' "$f"
    done | sort -u
  )"
  registered="$(printf '%s\n' ${PROBES_CALENDAR_BLOCK} ${PROBES_CALENDAR} ${PROBES_CALENDAR_TAILWIND} | sort -u)"

  unregistered="$(comm -23 <(printf '%s\n' "${found}") <(printf '%s\n' "${registered}"))"
  vanished="$(comm -13 <(printf '%s\n' "${found}") <(printf '%s\n' "${registered}"))"

  local bad=0 n
  if [[ -n "${unregistered}" ]]; then
    bad=1
    echo
    echo "############################################################"
    echo "# A BROWSER PROBE EXISTS THAT THIS GUARD DOES NOT RUN       #"
    echo "############################################################"
    for n in ${unregistered}; do
      echo "  UNREGISTERED  ${n}"
    done
    echo
    echo "  It drives a real browser and asserts on what it measures, but it is"
    echo "  on none of this file's lists, so CI never runs it: every probe here"
    echo "  self-skips under \`-short\`, which is the mode \"Build & Test\" uses."
    echo "  Add it to the list for its package — in the commit that adds it."
  fi
  if [[ -n "${vanished}" ]]; then
    bad=1
    echo
    echo "############################################################"
    echo "# THIS GUARD DEMANDS A PROBE THE TREE NO LONGER HAS         #"
    echo "############################################################"
    for n in ${vanished}; do
      echo "  NOT IN TREE   ${n}"
    done
    echo
    echo "  Renamed or deleted. Update the list in the same commit, or fix the"
    echo "  rename — a demand for a probe that cannot exist is not coverage."
  fi
  return "${bad}"
}

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

# probe_verdict <logfile> <PASS|SKIP|FAIL> <probe name>
#
# A PROBE'S VERDICT IS ITS TOP-LEVEL RESULT LINE AND NOTHING ELSE.
#
# `go test -v` writes a top-level verdict flush against the left margin and
# indents each subtest's verdict by four spaces per level, always following the
# name with ` (duration)`. Anchoring on both — `^--- PASS: <name> (` — is what
# separates the probe's own verdict from two lines that merely contain its name.
# The unanchored `grep -- "--- PASS: ${name}"` this replaces could not, and
# credited a red probe as green in both of these real shapes:
#
#   * A SUBTEST OF A FAILED PROBE. A table-driven probe that fails prints
#     `--- FAIL: TestX (3.10s)` at the margin and `    --- PASS: TestX/w1024 …`
#     beneath it. That second line CONTAINS the substring "--- PASS: TestX", so
#     the old grep matched, the `elif` for FAIL was never reached, and the guard
#     printed `PASS  TestX` for a probe the browser had just measured as broken.
#     This is not a hypothetical shape: every #590 moons/sky probe and stage 1's
#     own reachability probe are `t.Run`-per-viewport tables, and a real
#     regression fails SOME widths, never all of them — precisely the case the
#     old matcher waved through. The guard existed to end silent passes and had
#     one of its own.
#   * A LONGER NAME THAT STARTS WITH THIS ONE. `TestMoonReachProbe_TheOpener`
#     was satisfied by `TestMoonReachProbe_TheOpenerOnATouchDevice`, so deleting
#     the shorter probe would have taken its coverage with it in silence — the
#     exact failure the ABSENT branch exists to catch.
#
# BRE is used deliberately: `(` and `-` are literal in it, and Go test names are
# `[A-Za-z0-9_/]` so they carry no BRE metacharacter to escape.
probe_verdict() {
  local log="$1" verdict="$2" name="$3"
  grep -q -- "^--- ${verdict}: ${name} (" "${log}"
}

# HARNESS_MARKER — the one literal that separates "a pixel was measured and it
# was wrong" from "the browser child died and nothing was measured at all".
#
# WHY IT EXISTS. This guard used to print, for every failing probe without
# exception, "the rendered result is wrong. This is the guard working." That is
# a claim about pixels, and the mobile rig has a failure mode in which no pixel
# is ever measured: under parallel load Chromium is starved of its
# `--virtual-time-budget`, the frame's transcript comes back short, and
# mobileDrive fatals on the reply COUNT. Reported as a rendering verdict, the
# first person to see a red Browser Probes job is told the moons regressed —
# and, because this job only started gating in the moon arc, that person has no
# history with the flake to correct the guard with.
#
# Go writes it (mobile_probe_test.go's browserHarnessMarker), this file greps
# for it, and marker_check() below fails the run if the two drift apart.
HARNESS_MARKER='BROWSER HARNESS FAILURE'

# probe_fail_region <logfile> <name>
#
# The lines a probe printed between its own `=== RUN` and its own `--- FAIL`
# verdict — i.e. its reason, in its own words. Anchored on both ends for
# probe_verdict's reasons: an unanchored scan would pick up a longer probe's
# name or a subtest's indented verdict. Go test names are `[A-Za-z0-9_/]`, so
# the name carries no regex metacharacter.
probe_fail_region() {
  local log="$1" name="$2"
  awk -v n="${name}" '
    $0 == "=== RUN   " n { inside = 1; next }
    inside && index($0, "--- FAIL: " n " (") == 1 { exit }
    inside { print }
  ' "${log}"
}

# marker_check — the Go literal and the shell literal are the same string.
#
# A guard whose grep silently stops matching is worse than no guard: it does not
# error, it just quietly reclassifies every starvation as a rendering verdict
# again. Source-only, so it runs on a machine with no browser, next to the
# census.
marker_check() {
  local src="internal/plugins/calendar/mobile_probe_test.go"
  if [[ ! -f "${src}" ]]; then
    echo "check-browser-probes: ${src} is gone, so the harness-marker pairing cannot" >&2
    echo "  be checked. The FAILED branch below would report a dead browser child as" >&2
    echo "  a rendering regression. Re-point marker_check at the rig's new home." >&2
    return 1
  fi
  if ! grep -q -- "${HARNESS_MARKER}" "${src}"; then
    echo "check-browser-probes: ${src} no longer contains \"${HARNESS_MARKER}\"." >&2
    echo "  This guard greps for that literal to tell a browser child that DIED from" >&2
    echo "  a rendering result that is WRONG. With the two out of step every" >&2
    echo "  starvation is announced as \"the rendered result is wrong\" — the exact" >&2
    echo "  false verdict the pairing exists to prevent. Re-align both in one commit." >&2
    return 1
  fi
  return 0
}

# assert_probes_passed <logfile> <probe names…>
#
# Prints one diagnostic line per probe that did not PASS and returns 1 if any
# did. Kept pure — it reads a log and nothing else — so the self-test exercises
# the real rule rather than a copy of it.
assert_probes_passed() {
  local log="$1"; shift
  local missing=0 name
  for name in "$@"; do
    if probe_verdict "${log}" PASS "${name}"; then
      continue
    fi
    missing=1
    if probe_verdict "${log}" SKIP "${name}"; then
      echo "  SKIPPED  ${name} — the machine can drive a browser, so a skip here is a"
      echo "           real gap, not an absent dependency. Its skip reason:"
      # PRECEDING line, not the following one. `go test -v` writes t.Skip's
      # message from inside the test, so it lands BEFORE the verdict:
      #     === RUN   TestX
      #         x_test.go:699: browser probe: skipped under -short
      #     --- SKIP: TestX (0.00s)
      # The `,+1p` this replaces read the line AFTER the verdict, which in real
      # output is `PASS`, `ok …` or the next `=== RUN` — so the guard reported
      # a reason that was never the reason. It went unnoticed because the
      # self-test's fixture had been hand-written to match the sed rather than
      # copied from `go test`; the fixtures below are now the real shape.
      grep -B 1 -- "^--- SKIP: ${name} (" "${log}" | head -n 1 | sed 's/^/           /'
    elif probe_verdict "${log}" FAIL "${name}"; then
      # THE DEFAULT LINE NO LONGER ASSERTS A CAUSE IT CANNOT SEE. It states the
      # verdict and then QUOTES the probe's own words, so the reader gets the
      # reason rather than this script's guess at it. The harness branch above is
      # the one case the script can positively identify.
      local region
      region="$(probe_fail_region "${log}" "${name}")"
      if printf '%s\n' "${region}" | grep -q -- "${HARNESS_MARKER}"; then
        echo "  FAILED   ${name} — THE BROWSER CHILD DIED OR STOPPED ANSWERING."
        echo "           NO PIXEL WAS MEASURED, so this says nothing about whether the"
        echo "           rendering regressed — it is not evidence for or against a"
        echo "           layout change. Known cause: Chromium starved of its"
        echo "           virtual-time budget under parallel load. Re-run this probe on"
        echo "           its own before concluding anything. Its own words:"
      else
        echo "  FAILED   ${name} — an assertion about the RENDERED result failed. This"
        echo "           is the guard working. Its own words:"
      fi
      printf '%s\n' "${region}" | grep -m 3 -- '_test\.go:[0-9]' | cut -c1-200 | sed 's/^ */           /'
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

  # EVERY FIXTURE BELOW IS THE REAL SHAPE `go test -v` EMITS, copied from a run,
  # not hand-written to match the code. The previous skip fixture put t.Skip's
  # message after the verdict; go puts it before, so the guard's own self-test
  # was certifying a diagnostic that could never fire correctly in production.
  printf -- '=== RUN   TestA\n--- PASS: TestA (1.00s)\n=== RUN   TestB\n--- PASS: TestB (1.00s)\nPASS\nok\n' \
    > "${dir}/green.log"
  if ! assert_probes_passed "${dir}/green.log" TestA TestB > /dev/null; then
    echo "  self-test FAILED: two passing probes were reported as a problem" >&2
    fail=1
  fi

  printf -- '=== RUN   TestA\n--- PASS: TestA (1.00s)\n=== RUN   TestB\n    x_test.go:1: browser probe: no Chromium\n--- SKIP: TestB (0.00s)\nPASS\nok\n' \
    > "${dir}/skip.log"
  local out
  out="$(assert_probes_passed "${dir}/skip.log" TestA TestB)" && {
    echo "  self-test FAILED: a SKIPPED probe passed the guard — this is the exact" >&2
    echo "                    silent pass the guard exists to end" >&2
    fail=1
  }
  [[ "${out}" == *"SKIPPED  TestB"* ]] || {
    echo "  self-test FAILED: the SKIPPED diagnostic did not name the probe" >&2; fail=1; }
  [[ "${out}" == *"browser probe: no Chromium"* ]] || {
    echo "  self-test FAILED: the SKIPPED diagnostic did not quote t.Skip's own" >&2
    echo "                    message — it reads the line BEFORE the verdict" >&2; fail=1; }

  printf -- '=== RUN   TestA\n--- PASS: TestA (1.00s)\n=== RUN   TestB\n    b_test.go:551: discs 0, want 3\n--- FAIL: TestB (1.00s)\nFAIL\n' \
    > "${dir}/fail.log"
  out="$(assert_probes_passed "${dir}/fail.log" TestA TestB)" && {
    echo "  self-test FAILED: a FAILING probe passed the guard" >&2; fail=1; }
  [[ "${out}" == *"FAILED   TestB"* ]] || {
    echo "  self-test FAILED: the FAILED diagnostic did not name the probe" >&2; fail=1; }
  [[ "${out}" == *"assertion about the RENDERED result failed"* ]] || {
    echo "  self-test FAILED: a probe that failed an ASSERTION was not reported as" >&2
    echo "                    one — the rendering branch of the verdict is dead" >&2; fail=1; }
  [[ "${out}" == *"discs 0, want 3"* ]] || {
    echo "  self-test FAILED: the FAILED diagnostic did not quote the probe's own" >&2
    echo "                    words, so the reader gets this script's guess at the" >&2
    echo "                    cause instead of the probe's statement of it" >&2; fail=1; }

  # THE STARVATION CASE, WHICH IS NOT A RENDERING VERDICT.
  #
  # This fixture is the real shape `go test -v` emits when mobileDrive's child is
  # starved of its virtual-time budget: a t.Fatalf carrying browserHarnessMarker,
  # then the probe's own FAIL. Before this branch existed the guard announced it
  # as "the rendered result is wrong. This is the guard working." — a claim about
  # pixels made when the browser never answered and no pixel was read. The two
  # assertions below are the whole point: the harness line must fire, AND the
  # rendering claim must NOT.
  printf -- '=== RUN   TestB\n    mobile_probe_test.go:196: BROWSER HARNESS FAILURE (no pixel was measured): asked for 3 steps and got 2 replies — the child stopped answering.\n--- FAIL: TestB (12.00s)\nFAIL\n' \
    > "${dir}/harness.log"
  out="$(assert_probes_passed "${dir}/harness.log" TestB)" && {
    echo "  self-test FAILED: a harness failure passed the guard — a probe that never" >&2
    echo "                    measured anything must not count as a measurement" >&2; fail=1; }
  [[ "${out}" == *"BROWSER CHILD DIED OR STOPPED ANSWERING"* ]] || {
    echo "  self-test FAILED: a starved browser child was not identified as one" >&2; fail=1; }
  [[ "${out}" == *"assertion about the RENDERED result failed"* ]] && {
    echo "  self-test FAILED: a starved browser child was reported as a RENDERING" >&2
    echo "                    verdict. Nothing was measured, so that is an assertion" >&2
    echo "                    about pixels nobody looked at — the exact false verdict" >&2
    echo "                    this branch exists to end" >&2; fail=1; }

  # …and the marker is not a phrase this file invented for its own fixture: the
  # Go rig must still carry it, or the grep above matches nothing in production
  # while this self-test stays green off its own hand-written string.
  marker_check || {
    echo "  self-test FAILED: the harness marker is not in the Go rig, so the branch" >&2
    echo "                    proved above can never fire on a real log" >&2; fail=1; }

  # THE SUBTEST SHADOW. A table-driven probe that fails SOME of its cases — what
  # a real rendering regression actually looks like — prints its own FAIL at the
  # margin and an indented PASS for each case that still worked. The guard used
  # to grep for the unanchored substring "--- PASS: TestB", find it inside
  # "    --- PASS: TestB/w1024_week7", and report the probe green. Stage 1's
  # reachability probe fails 11 of 20 subtests when its fix is reverted, so this
  # fixture is that revert's exact log shape.
  printf -- '=== RUN   TestB\n=== RUN   TestB/w1024_week7\n=== RUN   TestB/w390_week7\n    b_test.go:551: discs 0, want 3\n--- FAIL: TestB (3.10s)\n    --- PASS: TestB/w1024_week7 (0.20s)\n    --- FAIL: TestB/w390_week7 (0.20s)\nFAIL\n' \
    > "${dir}/subtest.log"
  out="$(assert_probes_passed "${dir}/subtest.log" TestB)" && {
    echo "  self-test FAILED: a probe whose OWN verdict was FAIL passed the guard" >&2
    echo "                    on the strength of one passing subtest — a red probe" >&2
    echo "                    reported green is worse than no probe" >&2
    fail=1; }
  [[ "${out}" == *"FAILED   TestB"* ]] || {
    echo "  self-test FAILED: the subtest-shadow case was caught but misreported" >&2; fail=1; }

  # THE PREFIX COLLISION. A registered probe must not be satisfied by a
  # DIFFERENT probe whose name merely starts with it, or deleting the short one
  # takes its coverage with it and the ABSENT branch never fires.
  printf -- '=== RUN   TestBOnATouchDevice\n--- PASS: TestBOnATouchDevice (2.00s)\nPASS\nok\n' \
    > "${dir}/prefix.log"
  out="$(assert_probes_passed "${dir}/prefix.log" TestB)" && {
    echo "  self-test FAILED: a probe that never ran was credited to a longer" >&2
    echo "                    probe name that happens to start with it" >&2
    fail=1; }
  [[ "${out}" == *"ABSENT   TestB"* ]] || {
    echo "  self-test FAILED: the prefix-collision case was caught but misreported" >&2; fail=1; }

  printf -- '=== RUN   TestA\n--- PASS: TestA (1.00s)\nPASS\nok\n' > "${dir}/gone.log"
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

# THE CENSUS RUNS FIRST, AND ON EVERY MACHINE. It reads source only, so it costs
# nothing and — unlike everything below it — does not need a browser. A probe
# added without being registered is caught on the developer's laptop rather than
# discovered months later by an operator who cannot see the moons.
census_check || exit 1

# AND SO DOES THE MARKER PAIRING, for the same reason: source-only, no browser,
# and a drift here silently turns every starvation back into a false rendering
# verdict rather than erroring.
marker_check || exit 1

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
  local gostatus=$?
  if ! assert_probes_passed "${LOG}" ${names}; then
    echo "  (full output below)"
    sed 's/^/  | /' "${LOG}"
    return 1
  fi
  # AND the run itself has to have been clean. `go test`'s exit code was
  # previously discarded outright, so the guard's verdict rested entirely on the
  # per-probe lines. With `-run` narrowed to the covered probes, the ways this
  # can be non-zero while every one of them still printed PASS are a panic that
  # took the binary down after the last verdict, a TestMain/teardown fault, or a
  # build failure in the package — none of which is the clean measurement the
  # PASS lines appear to report. Report the run, not just the lines.
  if (( gostatus != 0 )); then
    echo "  UNCLEAN  ${pkg} — every covered probe printed PASS, but \`go test\`"
    echo "           exited ${gostatus}. A panic, a teardown fault or a build"
    echo "           failure can end a run after its last verdict was printed."
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
