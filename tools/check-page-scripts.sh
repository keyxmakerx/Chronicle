#!/usr/bin/env bash
# tools/check-page-scripts.sh
#
# PAGE-SIDE <script src> RATCHET.
#
# THE RULE, AND WHY IT IS A RULE. The App layout renders `{children...}` inside
# `<main id="main-content">`. Every sidebar link in internal/templates/layouts/app.templ
# is `hx-boost="true" hx-target="#main-content" hx-select="#main-content"
# hx-swap="innerHTML"`, and static/js/boot.js sets `htmx.config.allowScriptTags = false`.
# At that setting htmx's makeFragment does not merely decline to EXECUTE script
# tags in the swapped fragment — it REMOVES them from the DOM before the swap.
#
# So a `<script src>` inside a page templ is delivered on a direct page load and
# is NOT delivered when the same page is reached through the sidebar. The failure
# is invisible: `<link rel=stylesheet>` in the same region is untouched by that
# code path, so the page renders pixel-identically and simply does nothing. This
# is the defect that shipped the Calendar Bench with a dead day card and a dead
# Permissions button for anyone who navigated to it instead of typing the URL.
#
# THE FIX IS ALWAYS THE SAME: contribute the script to the plugin BODY-SCRIPT
# REGISTRY (internal/app/routes.go's `pluginBodyScripts` →
# layouts.SetPluginBodyScripts → internal/templates/layouts/base.templ), which
# emits after `{children...}` — outside the swapped region, and therefore
# identical on both navigation paths. Do NOT flip allowScriptTags (it is a
# deliberate security posture) and do NOT put hx-boost="false" on the link (that
# hides the defect on one route and leaves it on every other).
#
# THIS GUARD IS A RATCHET, NOT AN AUDIT. It does not claim the survivors in
# tools/page-script-allowlist.txt are all broken — whether a given one is
# depends on where its templ renders and whether its module re-inits on
# htmx:afterSettle. It claims only that the count may never grow. The sweep of
# the survivors is booked as C-HTMX-SCRIPT-SWEEP in .ai/todo.md.
#
# WHOLE-TREE, NOT DIFF-SCOPED, and deliberately so. The sibling guards
# (check-plugin-isolation.sh, check-v2-motion-discipline.sh) scope to the PR diff
# because their baselines are unwritten. This one has a written baseline, so it
# can do the stronger thing: it fails on a file that is not in the allowlist at
# all, on a count above its allowlisted number, AND on a count BELOW it — because
# a stale-high entry is slack in the ratchet, i.e. a hole a script can be put
# back through without anything noticing.
#
# SELF-TEST: every run first executes the comparison core against fixtures in a
# temp dir, so "OK" always means the rule can actually fire — a guard that
# cannot fail is worse than no guard, because it reads as coverage. Run just the
# self-test with: --self-test-only
#
# Exit codes:
#   0 — the tree matches the allowlist exactly
#   1 — a new/grown page-side script, a stale allowlist entry, or slack
#   2 — the guard's own self-test failed (the guard is broken, not the code)

set -euo pipefail

# The layout itself is the OUTSIDE of the swapped region: it is where a
# page-side script should move TO. Exempt by construction, by exact path.
EXEMPT="internal/templates/layouts/base.templ"

ALLOWLIST="${ALLOWLIST:-tools/page-script-allowlist.txt}"

# The tag name is joined at runtime so the guard's own source and its allowlist —
# both of which have to spell it out to be readable — can never be swept up by a
# future widening of the scan beyond *.templ. Same trick as
# tools/check-no-instance-hostname.sh.
stag='<scr''ipt'
marker="${stag} src="

# count_src_tags <file>
#   Prints the number of script OPEN TAGS in <file> that carry a `src`
#   attribute.
#
# ATTRIBUTE-ORDER-BLIND, BY NAME (C-SWEEP-R3, page-script-ratchet-attr-order).
# This used to be `grep -o` for the literal `${marker}`, i.e. for `<script`
# followed by exactly one space followed by `src=`. Measured, `<script defer
# src={ … }>`, `<script type="module" src={ … }>` and a newline-split `<script\n
# src={ … }>` all walked straight through it — the file did not even enter the
# inventory, so NEW/GREW could not fire — and all three are valid templ that
# `templ fmt` leaves alone, so the evasion survived `make templ` and CI. The harm
# is order-blind: htmx's makeFragment removes scripts BY TAG NAME
# (allowScriptTags=false), so a guard that is order-sensitive is weaker than the
# failure it is guarding.
#
# It walks the open tag a character at a time rather than matching a line,
# because the tag may span lines and because a `>` inside a quoted value or a
# templ `{ … }` expression must not close it early — the same walk
# tools/check-calendar-v4-lints.sh does for B3/B4. Bodies are never inspected, so
# an inline `<script>…</script>` (a different rule, policed elsewhere) is not
# counted.
count_src_tags() {
  awk -v stag="${stag}" '
    BEGIN { SQ = sprintf("%c", 39); tag = tolower(substr(stag, 2)); TL = length(tag); n = 0 }
    { LINE[NR] = $0 }
    # hasSrc — exact attribute-name match on the buffered open tag, so
    # `data-src=` and `srcset=` are not mistaken for `src=`.
    function hasSrc(buf) { return buf ~ /(^|[^-[:alnum:]])src[[:space:]]*=/ }
    END {
      inTag = 0; buf = ""; q = ""; brace = 0
      for (ln = 1; ln <= NR; ln++) {
        s = LINE[ln]; L = length(s)
        for (i = 1; i <= L; i++) {
          c = substr(s, i, 1)
          if (!inTag) {
            # Enter only on the tag name itself, on a name boundary: `<script`
            # followed by whitespace, `>`, `/` or end of line. `</script>` and
            # `<scripts>` therefore do not enter.
            if (c == "<" && tolower(substr(s, i + 1, TL)) == tag) {
              nx = substr(s, i + 1 + TL, 1)
              if (nx == "" || nx ~ /[[:space:]>\/]/) { inTag = 1; buf = "<"; q = ""; brace = 0 }
            }
            continue
          }
          if (q != "")              { buf = buf c; if (c == q) q = "";           continue }
          if (brace > 0)            { buf = buf c
                                      if (c == "{") brace++
                                      else if (c == "}") brace--              ; continue }
          if (c == "\"" || c == SQ) { q = c; buf = buf c;                        continue }
          if (c == "{")             { brace = 1; buf = buf c;                    continue }
          if (c == ">")             { if (hasSrc(buf)) n++; inTag = 0; buf = ""; continue }
          buf = buf c
        }
        # A tag continuing onto the next line: keep the attributes separated, or
        # `<script\nsrc=` would buffer as `<scriptsrc=`.
        if (inTag) buf = buf " "
      }
      print n
    }
  ' "$1"
}

# inventory <root> [exempt-relative-path]
#   Prints "<count> <path>" for every *.templ under <root> holding at least one
#   page-side script tag with a `src`, paths relative to <root>, sorted. Counts
#   OCCURRENCES, not lines: two tags on one line are two violations, and
#   `grep -c` would have said one.
inventory() {
  local root="$1" exempt="${2:-}"
  ( cd "${root}" && \
    find . -name '*.templ' -not -path './node_modules/*' -print \
    | sed 's|^\./||' | sort \
    | while IFS= read -r f; do
        [[ -n "${exempt}" && "${f}" == "${exempt}" ]] && continue
        n=$(count_src_tags "${f}" 2>/dev/null)
        [[ "${n}" == "0" ]] && continue
        echo "${n} ${f}"
      done )
}

# read_allowlist <file> — strips comments and blank lines, normalises spacing.
read_allowlist() {
  sed -e 's/#.*$//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$1" \
    | grep -v '^$' \
    | awk '{print $1" "$2}' \
    | sort -k2
}

# compare <actual-text> <expected-text> — prints findings, returns 1 on any.
# Both arguments are "<count> <path>" lines. Kept pure so the self-test can
# exercise the real comparison rather than a re-implementation of it.
compare() {
  local actual="$1" expected="$2" findings=""

  # New or grown.
  while IFS=' ' read -r n f; do
    [[ -z "${f:-}" ]] && continue
    local want
    want=$(echo "${expected}" | awk -v p="${f}" '$2==p{print $1}')
    if [[ -z "${want}" ]]; then
      findings+="  NEW      ${f} (${n}) — not in the allowlist"$'\n'
    elif (( n > want )); then
      findings+="  GREW     ${f} (${n}, allowed ${want})"$'\n'
    elif (( n < want )); then
      findings+="  SLACK    ${f} (${n}, allowlist still says ${want}) — lower it"$'\n'
    fi
  done <<< "${actual}"

  # Gone entirely: the allowlist may only shrink, so a fixed file must be
  # DELETED from it, not left behind as a licence to re-add.
  while IFS=' ' read -r n f; do
    [[ -z "${f:-}" ]] && continue
    if ! echo "${actual}" | awk -v p="${f}" '$2==p{found=1} END{exit !found}'; then
      findings+="  STALE    ${f} (allowlist says ${n}, tree has none) — delete the line"$'\n'
    fi
  done <<< "${expected}"

  if [[ -n "${findings}" ]]; then
    printf '%s' "${findings}"
    return 1
  fi
  return 0
}

# --- self-test --------------------------------------------------------------
#
# Six fixtures, proving each verdict fires and that a clean tree stays quiet.
# Exercises inventory() (including the exemption, the occurrences-not-lines
# count, and — since C-SWEEP-R3 — that the scan is ATTRIBUTE-ORDER-BLIND and
# still ignores inline script bodies) and compare() as they actually run.
self_test() {
  local tmp fail=0
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN

  mkdir -p "${tmp}/tree/internal/templates/layouts" "${tmp}/tree/pkg"
  # Exempt file, deliberately noisy: must never appear in the inventory.
  printf '%s\n%s\n' "${marker}\"a.js\">" "${marker}\"b.js\">" \
    > "${tmp}/tree/internal/templates/layouts/base.templ"
  # Two markers on ONE line — the case `grep -c` gets wrong.
  printf '%s"x.js"> %s"y.js">\n' "${marker}" "${marker}" > "${tmp}/tree/pkg/two.templ"
  printf '%s"z.js">\n' "${marker}" > "${tmp}/tree/pkg/one.templ"
  printf '<div>no scripts here</div>\n' > "${tmp}/tree/pkg/clean.templ"
  # THE THREE FORMS THAT USED TO WALK THROUGH: an attribute before `src`, an
  # attribute before `src` with a quoted value holding a `/`, and an open tag
  # split across lines with `src` on neither the first nor the last. All three
  # are valid templ that `templ fmt` does not normalise to src-first.
  { printf '%s defer src={ layouts.AssetURL("/static/js/a.js") }>\n' "${stag}"
    printf '%s type="module" src="/static/js/b.js">\n' "${stag}"
    printf '%s\n\t\tsrc={ u }\n\t\tasync\n\t>\n' "${stag}"
  } > "${tmp}/tree/pkg/attrs.templ"
  # An INLINE script is a different rule, policed elsewhere: the body must not
  # be inspected and the file must stay out of the inventory entirely — even
  # though its body says `src=`.
  printf '%s>var s = "src=/static/js/x.js";</scr%s>\n' "${stag}" "ipt" \
    > "${tmp}/tree/pkg/inline.templ"

  local got want
  got="$(inventory "${tmp}/tree" "${EXEMPT}")"
  want=$'3 pkg/attrs.templ\n1 pkg/one.templ\n2 pkg/two.templ'
  if [[ "${got}" != "${want}" ]]; then
    echo "  self-test FAILED: inventory = [${got}]; expected [${want}]" >&2
    fail=1
  fi

  # Quiet when the allowlist matches exactly.
  if ! compare "${got}" "${want}" >/dev/null; then
    echo "  self-test FAILED: an exactly-matching allowlist did not pass" >&2
    fail=1
  fi
  # Each verdict fires.
  local out
  out="$(compare "${got}" "$(printf '3 pkg/attrs.templ\n2 pkg/two.templ')" || true)"
  [[ "${out}" == *"NEW      pkg/one.templ"* ]] || { echo "  self-test FAILED: NEW did not fire" >&2; fail=1; }
  out="$(compare "${got}" "$(printf '3 pkg/attrs.templ\n1 pkg/one.templ\n1 pkg/two.templ')" || true)"
  [[ "${out}" == *"GREW     pkg/two.templ"* ]] || { echo "  self-test FAILED: GREW did not fire" >&2; fail=1; }
  out="$(compare "${got}" "$(printf '3 pkg/attrs.templ\n1 pkg/one.templ\n3 pkg/two.templ')" || true)"
  [[ "${out}" == *"SLACK    pkg/two.templ"* ]] || { echo "  self-test FAILED: SLACK did not fire" >&2; fail=1; }
  out="$(compare "${got}" "$(printf '%s\n1 pkg/gone.templ' "${want}")" || true)"
  [[ "${out}" == *"STALE    pkg/gone.templ"* ]] || { echo "  self-test FAILED: STALE did not fire" >&2; fail=1; }

  return "${fail}"
}

if ! self_test; then
  echo "check-page-scripts: SELF-TEST FAILED — the guard is broken; fix it before trusting a pass." >&2
  exit 2
fi

if [[ "${1:-}" == "--self-test-only" ]]; then
  echo "check-page-scripts: self-test OK (NEW / GREW / SLACK / STALE each fire, clean stays quiet)."
  exit 0
fi

# --- the real run -----------------------------------------------------------

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

if [[ ! -f "${ALLOWLIST}" ]]; then
  echo "check-page-scripts: allowlist ${ALLOWLIST} is missing — a ratchet without its baseline cannot fail, which is worse than no ratchet." >&2
  exit 2
fi

actual="$(inventory "${repo_root}" "${EXEMPT}")"
expected="$(read_allowlist "${ALLOWLIST}")"

if findings="$(compare "${actual}" "${expected}")"; then
  count=$(echo "${expected}" | grep -c . || true)
  echo "check-page-scripts: OK — page-side <script src> matches the allowlist exactly (${count} file(s) remaining). (self-test OK)"
  exit 0
fi

echo "ERROR: the page-side <script src> ratchet slipped."
echo
printf '%s' "${findings}"
echo
echo "WHY THIS IS BLOCKED. A <script src> in a page templ lands inside"
echo "<main id=\"main-content\">. The sidebar navigates with hx-boost +"
echo "hx-select=\"#main-content\", and htmx REMOVES script tags from a swapped"
echo "fragment because boot.js sets allowScriptTags=false. Your script will load"
echo "when someone types the URL and will silently NOT load when they click"
echo "through the sidebar — and the page will look identical either way, because"
echo "stylesheets in the same region survive."
echo
echo "WHAT TO DO INSTEAD: add it to the plugin body-script registry —"
echo "  internal/app/routes.go   pluginBodyScripts := []string{ ... }"
echo "which base.templ emits after {children...}, outside the swapped region."
echo "Deferred entries execute in slice order, so a dependency may be listed"
echo "first. Make sure the module bails cleanly when its mount is absent, since"
echo "the registry mounts on every page."
echo
echo "If you are FIXING a survivor rather than adding one, lower or delete its"
echo "line in ${ALLOWLIST} — that is the ratchet turning, and it is welcome."
echo "The remaining survivors are booked as C-HTMX-SCRIPT-SWEEP in .ai/todo.md."
exit 1
