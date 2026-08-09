#!/usr/bin/env bash
# tools/check-calendar-v4-lints.sh
#
# calendar-v4 guards B1–B4, per
# cordinator/decisions/2026-07-26-calendar-v4-canon-amendments.md §B.
# Landed by C-CALV4-FOUNDATION-P0.
#
#   B1  No --dash / --gap in a transition / transition-property / animation
#       property list. Morphing a dash pattern destroys the GREYSCALE identity
#       channel the signed greyscale render promises — and it is the obvious
#       "improvement" the next hand reaches for.
#
#   B2  No `font:` shorthand carrying `inherit` as the FAMILY. Invalid CSS: it
#       silently dropped 81 declarations in the v4 mockup and nothing failed
#       loudly. Bare `font: inherit` is the CSS-wide keyword and is LEGAL —
#       only the shorthand-with-family form is flagged.
#
#   B3  No interactive control reusing an <html> state-marker attribute name.
#       Convention: state markers are bare nouns; controls end in `-pick`.
#       Reusing one made every click on the mockup re-navigate, twice.
#
#   B4  Every dated cell/row in a calendar_block / bench template carries
#       `data-day` — the ANSWER key. A surface that forgets it simply stops
#       answering, which reads as "the calendar feels dead over here" and is
#       invisible in code review.
#
# ---------------------------------------------------------------------------
# DIFF-SCOPED, like tools/check-plugin-isolation.sh and
# tools/check-v2-motion-discipline.sh: only lines INTRODUCED vs the merge base
# are judged. A lint that fires on pre-existing code would block the whole
# calendar-v4 wave on surfaces its slices do not own.
#
# Needs full git history for the diff base (ci.yml sets fetch-depth: 0). In a
# shallow clone the diff resolves empty and the guard reports OK — same
# limitation the other two diff-scoped guards carry.
#
# ---------------------------------------------------------------------------
# SELF-TEST: every run first executes the lint core against embedded fixtures
# and asserts each rule both FIRES on a violation and STAYS QUIET on the clean
# equivalent. This exists because C-CALV4-FOUNDATION-P0 also DELETES
# tools/check-templ-drift.sh — a guard that was structurally incapable of
# failing and therefore read as coverage it never provided. B4 in particular
# has no subjects on main yet (no calendar_block templates exist until
# C-CALV4-BLOCK-P1 lands), so without the self-test there would be no evidence
# it can fire at all. Run just the self-test with: --self-test-only
#
# Exit codes:
#   0 — no new violations (or advisory-only findings; see CALV4_B4_ADVISORY)
#   1 — at least one new blocking violation; CI fails
#   2 — the guard's own self-test failed (the guard is broken, not the code)

set -euo pipefail

# B4 can be demoted to warn-and-pass via CALV4_B4_ADVISORY=1. Default is
# blocking: the self-test proves the rule fires, so it is not a guard that
# cannot fail.
B4_ADVISORY="${CALV4_B4_ADVISORY:-0}"

# The <html> state-marker attribute names, taken from the SIGNED contract
# (cordinator mockups/calendar-v4.html:1166-1169 — `R.dataset.theme/view/role/
# colour/moonstyle`, plus `R.dataset.ready` at :2690). None of these names
# exists anywhere in this repo today, so there is nothing to grandfather.
# Extend this list when a new <html>-level marker is introduced; the matching
# control attribute must then end in `-pick` (mockup: `data-colour-pick`,
# `data-layer-pick`).
STATE_MARKERS="data-theme,data-view,data-role,data-colour,data-moonstyle,data-ready"

# ---------------------------------------------------------------------------
# The lint core.
#
# lint_file <path> <added-line-numbers|ALL> <b4_in_scope:0|1>
#
# Emits one `RULE|line|detail` record per violation on stdout. Empty stdout
# means clean. Scanning is ELEMENT-scoped (B3/B4) and DECLARATION-scoped (B1),
# never line-scoped and never structure-scoped: an attribute may sit five lines
# below its tag name, which is the normal shape of a templ element, and a
# regex anchored on surrounding structure is exactly the brittleness the wave
# COMMON brief §3 warns about. B1–B4 all key on the marker itself.
# ---------------------------------------------------------------------------
# b4_scope_for decides whether guard B4 applies to a file, BY FILENAME.
#
# B4 is the calendar-v4 surfaces' own rule (every dated node carries data-day);
# B1-B3 are universal CSS/markup correctness and run repo-wide. The glob is the
# whole scope, so a surface whose filename it does not match is silently
# unguarded — which is why `*schedule*` is here BEFORE the file exists
# (C-CALV4-RSVP-P8 §11: Part B's /schedule page is drawn on the coordinator
# lane, and a missing glob is invisible in review). Extracted from the main loop
# so the self-test can exercise the RESOLUTION and not just the rules.
#
# AMENDED 2026-07-31 — coverage WIDENING, C-CALV4-DAYCARD/R2-2a, ruling on
# DC2-B4-GLOB-4. `*daycard*` joins the list. The day card and the event editor
# (internal/plugins/calendar/daycard.templ, static/css/calendar-daycard.css) are
# new DATED surfaces — the card head, the editor head and every module-built row
# name a day — and their filenames matched none of the five existing globs, so
# B4 resolved to 0 and read them not at all. The keys were in fact correct and
# are pinned elsewhere (TestDayCard_KeysAgreeWithTheRenderedBlock goes red on a
# separator change), but a surface that is silently OUT of scope is the exact
# invisible coverage hole this function's header says it exists to prevent, and
# `*schedule*` is already here for a file that does not exist yet on the same
# reasoning. Nothing is loosened: no glob is removed and no rule is relaxed.
#
# AMENDED 2026-08-08 — coverage WIDENING, C-CALV4-THEATER/R2-3, acceptance item
# under §12's guards table ("B4, and its GLOB"). `*theater*` and `*Theater*`
# join the list. The Block theater is a DATED surface by construction: it
# renders a second calendar-v4 Block, so every cell, row and Ledger line it
# carries names a day, and its filenames
# (internal/plugins/calendar/theater.templ, static/css/calendar-theater.css)
# matched none of the six existing globs — B4 would have resolved to 0 and read
# them not at all. That is the same invisible coverage hole DC2-B4-GLOB-4 closed
# for the day card.
#
# THE GLOB LANDS AHEAD OF THE SURFACE, DELIBERATELY, AND THAT IS THE PRECEDENT
# RATHER THAN AN EXCEPTION. `*schedule*` was added before schedule.templ
# existed, for the reason this function's header states: a missing glob is
# invisible in review, and it is invisible precisely at the moment the new file
# arrives. R2-3's mount is BLOCKED on [TH-14] (the entity calendar block renders
# entirely inside widgetbindings.BlockHost, which picker.templ swaps with
# hx-swap="outerHTML" — so no position available to that slice is outside an
# HTMX-swappable region, and the placement is a host restructure the slice was
# not signed to take). The scope is landed anyway so that whichever slice
# unblocks the theater inherits B4 coverage rather than having to remember it.
# Nothing is loosened: no glob is removed and no rule is relaxed.
b4_scope_for() {
  case "$1" in
    *calendar_block*|*bench*|*Bench*|*daycard*|*schedule*|*Schedule*|*theater*|*Theater*) echo 1 ;;
    *) echo 0 ;;
  esac
}

lint_file() {
  local file="$1" added="$2" b4scope="$3"

  awk -v added="${added}" \
      -v b4scope="${b4scope}" \
      -v markers="${STATE_MARKERS}" \
      '
    BEGIN {
      allLines = (added == "ALL")
      if (!allLines) {
        n = split(added, a, ",")
        for (i = 1; i <= n; i++) if (a[i] != "") ADD[a[i] + 0] = 1
      }
      nmk = split(markers, MK, ",")

      # A literal apostrophe, built by code point: this whole program lives
      # inside a single-quoted shell string, so it cannot be written directly.
      SQ = sprintf("%c", 39)

      # Tags that are interactive by nature. Anything else counts as
      # interactive only if it carries a handler/hx attribute or an ARIA
      # button role (see isInteractive).
      split("a button input select textarea label summary option form", it, " ")
      for (i in it) INTERACTIVE[it[i]] = 1
    }

    { LINE[NR] = $0 }

    # isAdded — true when any line of a span was introduced by this diff.
    function isAdded(from, to,   n) {
      if (allLines) return 1
      for (n = from; n <= to; n++) if (n in ADD) return 1
      return 0
    }

    # hasAttr — exact attribute-name match. Guards both ends so `data-day`
    # does not match `data-daynum` and `data-cell` does not match
    # `x-data-cell`.
    function hasAttr(buf, name) {
      return buf ~ ("(^|[^-[:alnum:]])" name "([=[:space:]>/]|$)")
    }

    # ---------------- B1: declaration-scoped -------------------------------
    # Walks each line with a cursor, entering on a transition/animation
    # property name and leaving on the terminating `;` or `}`. State carries
    # across lines, so a multi-line `transition:` list is judged whole.
    function scanB1(s, ln,   seg) {
      while (length(s) > 0) {
        if (!inDecl) {
          if (match(s, /(^|[^-[:alnum:]])(transition|transition-property|animation|animation-name)[[:space:]]*:/)) {
            inDecl = 1
            s = substr(s, RSTART + RLENGTH)
          } else return
        }
        if (match(s, /[;}]/)) {
          seg = substr(s, 1, RSTART - 1)
          s   = substr(s, RSTART + 1)
          inDecl = 0
        } else {
          seg = s
          s = ""
        }
        if (seg ~ /--dash|--gap/ && isAdded(ln, ln))
          print "B1|" ln "|animating a greyscale-identity token in: " trim(seg)
      }
    }

    # ---------------- B2: value-scoped -------------------------------------
    # `font: inherit` alone is the CSS-wide keyword and is valid. `font: 600
    # 14px/1.2 inherit` names `inherit` as the FAMILY and is not.
    function scanB2(s, ln,   rest, val, cut) {
      rest = s
      while (match(rest, /(^|[^-[:alnum:]])font[[:space:]]*:/)) {
        rest = substr(rest, RSTART + RLENGTH)
        cut = match(rest, /[;}]/) ? RSTART - 1 : length(rest)
        val = trim(substr(rest, 1, cut))
        if (val ~ /(^|[[:space:],\/])inherit([[:space:];]|$)/ && val != "inherit" && isAdded(ln, ln))
          print "B2|" ln "|font shorthand names `inherit` as the family: font: " val
        rest = substr(rest, cut + 1)
      }
    }

    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }

    # stripComments — blanks out /* ... */ spans (multi-line included) while
    # preserving line numbering, so B1/B2 judge declarations and never prose.
    # Without it, a comment explaining WHY --dash must not be animated would
    # itself trip B1.
    function stripComments(s,   out, i, c, c2) {
      out = ""
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1); c2 = substr(s, i, 2)
        if (inComment) {
          if (c2 == "*/") { inComment = 0; out = out "  "; i++ } else out = out " "
        } else if (c2 == "/*") {
          inComment = 1; out = out "  "; i++
        } else out = out c
      }
      return out
    }

    # ---------------- B3 + B4: element-scoped ------------------------------
    # Evaluates one complete OPEN TAG (`<button ... >`), however many lines it
    # spans. Quoted values, templ `{ ... }` expressions AND the Go expression
    # of a templ control-flow attribute header (`if c.Day > 0 {`) are skipped
    # so a `>` inside any of them does not close the tag early.
    function evalTag(buf, from, to,   name, i, m) {
      if (!isAdded(from, to)) return

      match(buf, /^<[A-Za-z][A-Za-z0-9]*/)
      name = tolower(substr(buf, RSTART + 1, RLENGTH - 1))

      if (isInteractive(buf, name)) {
        for (i = 1; i <= nmk; i++) {
          m = MK[i]
          if (m != "" && hasAttr(buf, m))
            print "B3|" from "|<" name "> is an interactive control but carries the <html> state marker `" m "`; controls end in `-pick` (e.g. " m "-pick)"
        }
      }

      if (b4scope && (hasAttr(buf, "data-cell") || hasAttr(buf, "data-row")) && !hasAttr(buf, "data-day"))
        print "B4|" from "|dated <" name "> carries a data-cell/data-row marker but no `data-day` ANSWER key"
    }

    function isInteractive(buf, name) {
      if (name in INTERACTIVE) return 1
      if (buf ~ /(^|[^-[:alnum:]])(onclick|hx-get|hx-post|hx-put|hx-patch|hx-delete|tabindex)([=[:space:]>\/]|$)/) return 1
      if (buf ~ /role[[:space:]]*=[[:space:]]*.?(button|tab|switch|menuitem|option|link)/) return 1
      return 0
    }

    END {
      inDecl = 0; inComment = 0
      for (ln = 1; ln <= NR; ln++) {
        code = stripComments(LINE[ln])
        scanB1(code, ln); scanB2(code, ln)
      }

      inTag = 0; buf = ""; from = 0; q = ""; brace = 0; ctrl = 0
      for (ln = 1; ln <= NR; ln++) {
        s = LINE[ln]; L = length(s)
        for (i = 1; i <= L; i++) {
          c = substr(s, i, 1)
          if (!inTag) {
            if (c == "<" && substr(s, i + 1) ~ /^[A-Za-z]/) {
              inTag = 1; buf = "<"; from = ln; q = ""; brace = 0; ctrl = 0
            }
            continue
          }
          if (q != "")        { buf = buf c; if (c == q) q = "";            continue }
          if (brace > 0)      { buf = buf c
                                if (c == "{") brace++
                                else if (c == "}") brace--                 ; continue }
          if (c == "\"" || c == SQ)  { q = c; buf = buf c;                   continue }
          # A templ control-flow attribute header (`if c.Day > 0 {`, and the
          # else/for/switch forms) puts a Go expression BEFORE the `{` that
          # engages brace-skip — and a bare `>` in that expression must not
          # close the tag, or everything after the conditional falls off the
          # buffer: B4 false-positives on cells whose data-day sits behind the
          # conditional, B3 false-negatives on controls whose state marker
          # does. Keyword must sit on a word boundary — followed by
          # space/paren/brace, never `=` — so attribute names like `data-if`
          # or `<label for="x">` cannot trigger it.
          if (!ctrl && (i == 1 || substr(s, i - 1, 1) ~ /[[:space:]]/) && \
              substr(s, i) ~ /^(if|else|for|switch)([[:space:]({]|$)/) {
                                ctrl = 1; buf = buf c;                      continue }
          if (ctrl)           { buf = buf c
                                if (c == "{") { brace = 1; ctrl = 0 }      ; continue }
          if (c == "{")       { brace = 1; buf = buf c;                     continue }
          if (c == ">")       { evalTag(buf ">", from, ln); inTag = 0; buf = ""; continue }
          buf = buf c
        }
        # A tag continuing onto the next line: keep the attributes separated.
        if (inTag) buf = buf " "
      }
    }
  ' "${file}"
}

# ---------------------------------------------------------------------------
# Self-test — proves each rule fires on a violation and stays quiet on the
# clean equivalent. Runs on every invocation (it costs milliseconds).
# ---------------------------------------------------------------------------
self_test() {
  local dir rc=0
  dir="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '${dir}'" RETURN

  cat > "${dir}/dirty.css" <<'FIXTURE'
.cell {
  transition: --dash 200ms ease-out,
              opacity 150ms ease-out;
  animation-name: --gap;
  font: 600 14px/1.2 inherit;
}
FIXTURE

  cat > "${dir}/clean.css" <<'FIXTURE'
/* Prose is not code: this comment says `transition: --dash 200ms` and
   `font: 600 14px/1.2 inherit` and must not trip B1 or B2. */
.cell {
  --dash: 4 2;
  --gap: 3px;
  transition: opacity 150ms ease-out, transform 150ms ease-out;
  animation: cell-in 200ms ease-out both;
  font: inherit;          /* the CSS-wide keyword — legal, not the shorthand */
  font-family: inherit;
  transition-property: opacity;
  --transition: 1;
}
FIXTURE

  cat > "${dir}/dirty.templ" <<'FIXTURE'
templ Dirty() {
	<button
		type="button"
		data-colour="type"
		class="pick"
	>Colour</button>
	<div data-cell="1" class="cell">9</div>
	<tr data-row="2"><td>x</td></tr>
}
FIXTURE

  cat > "${dir}/clean.templ" <<'FIXTURE'
templ Clean() {
	<button
		type="button"
		data-colour-pick="type"
		class="pick"
	>Colour</button>
	<div data-cell="1" data-day="harptos-9" class="cell">9</div>
	<tr data-row="2" data-day="harptos-w1"><td>x</td></tr>
	<div data-colour="type">a non-interactive state marker is fine</div>
	<div data-daynum="9" data-cell="2" data-day="harptos-2">near-miss attr names</div>
	<div
		data-cell="3"
		if c.Day > 0 {
			data-day={ dayKey(c) }
		}
		class="cell"
	>9</div>
	<button
		type="button"
		if open > 0 {
			data-colour-pick="type"
		}
		class="pick"
	>Colour</button>
}
FIXTURE

  # Conditional-attribute VIOLATIONS: the marker the rule keys on sits inside
  # a templ conditional whose Go expression carries a bare `>`. A scanner that
  # closes the tag on that `>` never sees the marker — B3/B4 false-negatives.
  cat > "${dir}/cond-dirty.templ" <<'FIXTURE'
templ CondDirty() {
	<button
		type="button"
		if open > 0 {
			data-colour="type"
		}
		class="pick"
	>Colour</button>
	<div
		if c.Day > 0 {
			data-cell="1"
		}
		class="cell"
	>9</div>
}
FIXTURE

  # B4's scope is resolved BY FILENAME, and the resolution is now self-tested
  # too. It was not before, and that was the gap: a Part B file named
  # schedule.templ would have been silently OUT of scope, which is precisely the
  # invisible coverage hole this guard's header says it exists to prevent. A
  # missing glob is invisible; the glob is free (C-CALV4-RSVP-P8 §11).
  expect_scope() { # expect_scope <path> <want 0|1>
    local f="$1" want="$2" got
    got="$(b4_scope_for "${f}")"
    if [[ "${got}" != "${want}" ]]; then
      echo "  self-test FAILED: B4 scope for ${f} = ${got}; expected ${want}" >&2
      rc=1
    fi
  }
  expect_scope "internal/widgets/calendar_block/block.templ"   1
  expect_scope "internal/plugins/calendar/bench.templ"         1
  expect_scope "internal/plugins/calendar/Bench.templ"         1
  # Part B's file, which lands in a later slice. In scope BEFORE it exists.
  expect_scope "internal/plugins/calendar/schedule.templ"      1
  expect_scope "static/css/calendar-schedule.css"              1
  # R2-2a's dated surfaces (DC2-B4-GLOB-4). Both matched nothing before the
  # glob was widened, so the resolution is self-tested rather than assumed.
  expect_scope "internal/plugins/calendar/daycard.templ"       1
  expect_scope "static/css/calendar-daycard.css"               1
  # R2-3's surfaces (C-CALV4-THEATER §12). In scope BEFORE they exist, exactly
  # as schedule.templ was — the theater renders a second calendar-v4 Block, so
  # every cell it carries is a dated node, and a glob that arrives with the file
  # arrives too late to be noticed missing.
  expect_scope "internal/plugins/calendar/theater.templ"       1
  expect_scope "static/css/calendar-theater.css"               1
  expect_scope "internal/plugins/calendar/Theater.templ"       1
  # The negative that keeps the two new globs from being a prefix sweep: a
  # filename that merely CONTAINS neither token stays out, and calendar_v2 is
  # the frozen shell R2-4 retires.
  expect_scope "internal/plugins/calendar/calendar_v2.templ"   0

  expect() { # expect <label> <file> <b4scope> <want-rules|NONE>
    local label="$1" f="$2" scope="$3" want="$4" got
    got="$(lint_file "${f}" ALL "${scope}" | cut -d'|' -f1 | sort -u | paste -sd, -)"
    [[ -z "${got}" ]] && got="NONE"
    if [[ "${got}" != "${want}" ]]; then
      echo "  self-test FAILED: ${label} — rules fired: ${got}; expected: ${want}" >&2
      rc=1
    fi
  }

  expect "dirty.css fires B1+B2"        "${dir}/dirty.css"   0 "B1,B2"
  expect "clean.css stays quiet"        "${dir}/clean.css"   0 "NONE"
  expect "dirty.templ fires B3+B4"      "${dir}/dirty.templ" 1 "B3,B4"
  expect "clean.templ stays quiet"      "${dir}/clean.templ" 1 "NONE"
  # B4 must not fire outside its scope, even on the same markup.
  expect "dirty.templ out of B4 scope"  "${dir}/dirty.templ" 0 "B3"
  # Markers behind templ conditional attributes must still be seen: the whole
  # tag — conditional blocks included — is one open tag to B3/B4.
  expect "cond-dirty.templ fires B3+B4" "${dir}/cond-dirty.templ" 1 "B3,B4"
  expect "cond-dirty.templ out of B4 scope" "${dir}/cond-dirty.templ" 0 "B3"

  if (( rc != 0 )); then
    echo "check-calendar-v4-lints: SELF-TEST FAILED — the guard is broken, fix the guard." >&2
    exit 2
  fi
}

self_test
if [[ "${1:-}" == "--self-test-only" ]]; then
  echo "check-calendar-v4-lints: self-test OK (B1–B4 each fire and each stay quiet)."
  exit 0
fi

# ---------------------------------------------------------------------------
# Diff base detection — identical resolution order to check-plugin-isolation.sh.
# ---------------------------------------------------------------------------
base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

changed_files=$(git diff --name-only "${base}"...HEAD -- '*.css' '*.templ' '*.html' 2>/dev/null || true)
if [[ -z "${changed_files}" ]]; then
  echo "check-calendar-v4-lints: no changed CSS/templ/HTML vs ${base}; nothing to check. (self-test OK)"
  exit 0
fi

# added_lines — NEW-file line numbers introduced by the diff, from the `@@`
# hunk headers of a zero-context diff.
added_lines() {
  git diff -U0 "${base}"...HEAD -- "$1" 2>/dev/null | awk '
    /^@@/ {
      split($3, p, ",")
      start = substr(p[1], 2) + 0
      count = (length(p) > 1) ? p[2] + 0 : 1
      for (i = 0; i < count; i++) printf "%d,", start + i
    }'
}

blocking=""
advisory=""

while IFS= read -r file; do
  [[ -z "${file}" ]] && continue
  [[ -f "${file}" ]] || continue   # deleted files have no introduced content

  add="$(added_lines "${file}")"
  [[ -z "${add}" ]] && continue

  # B4 scopes to the calendar-v4 Block and Bench surfaces only. B1–B3 are
  # universal CSS/markup correctness and run repo-wide (diff-scoped, so they
  # only judge new code).
  b4scope="$(b4_scope_for "${file}")"

  while IFS='|' read -r rule ln detail; do
    [[ -z "${rule}" ]] && continue
    entry="  ${file}:${ln}  [${rule}] ${detail}"$'\n'
    if [[ "${rule}" == "B4" && "${B4_ADVISORY}" == "1" ]]; then
      advisory+="${entry}"
    else
      blocking+="${entry}"
    fi
  done < <(lint_file "${file}" "${add%,}" "${b4scope}")
done <<< "${changed_files}"

if [[ -n "${advisory}" ]]; then
  echo "WARNING: calendar-v4 guard B4 findings (advisory — CALV4_B4_ADVISORY=1):"
  echo
  printf '%s' "${advisory}"
  echo
fi

if [[ -n "${blocking}" ]]; then
  echo "ERROR: calendar-v4 guard violations (B1–B4, new code only vs ${base}):"
  echo
  printf '%s' "${blocking}"
  echo
  cat <<'MSG'
Per cordinator/decisions/2026-07-26-calendar-v4-canon-amendments.md §B:
  B1 — never put --dash / --gap in a transition/animation property list;
       the dash pattern IS the greyscale identity channel.
  B2 — `font:` shorthand may not name `inherit` as the family (invalid CSS,
       silently drops the declaration). Bare `font: inherit` is fine.
  B3 — an interactive control may not reuse an <html> state-marker name.
       State markers are bare nouns; controls end in `-pick`.
  B4 — every element carrying a data-cell / data-row marker also carries
       `data-day`, the ANSWER key consumed at W-B.
MSG
  exit 1
fi

echo "check-calendar-v4-lints: OK — no new B1–B4 violations vs ${base}. (self-test OK)"
