#!/usr/bin/env bash
# tools/check-customize-discipline.sh
#
# Three checks against a fixed file list on the Customize -> Appearance
# surface: JS constructing a topbar mode/upload control that belongs in
# server-rendered templ markup instead; a setTimeout(...) whose body reloads
# the page rather than swapping the affected fragment; and a hand-built
# multi-field JSON string literal in a templ file, in place of encoding/json.
# These are heuristics on the three concrete shapes named above, not a
# general "no bad JS/JSON" scan.
#
# Whole-tree, not diff-scoped, against a FIXED file list: this guard exists
# specifically so a returning regression fails even when nobody remembers it
# was already fixed once, not only a new violation on a diff. Needs no git
# history. Self-test: --self-test-only.
#
# Exit: 0 clean / 1 a rule fired (including a listed file that's missing) /
# 2 self-test failed
set -euo pipefail

# The Customize surface this guard polices. A new Customize file joins this
# list by hand — the guard cannot discover "is this part of Customize" on its
# own, and a silent miss is worse than a short list that has to be kept
# honest.
TEMPL_FILES=(
  "internal/plugins/campaigns/branding.templ"
  "internal/plugins/campaigns/customize.templ"
)
JS_FILES=(
  "static/js/widgets/appearance_editor.js"
)

# require_file_exists <file>
#   Prints an ERROR and fails if <file> doesn't exist. A listed file that's
#   gone means every check below silently runs against nothing for it — a
#   guard reporting clean while checking less than it claims. Missing is a
#   failure, not something to skip past.
require_file_exists() {
  if [[ ! -f "$1" ]]; then
    echo "ERROR: $1 is listed here but does not exist — update the file list, or restore the file."
    return 1
  fi
  return 0
}

# setTimeout_reload_hits <file>
#   Prints one "line N" hit per setTimeout(...) call whose argument list
#   contains a "reload(" call, and returns non-zero if it found any. A
#   paren-depth scan, not a line-bounded regex, because a
#   `setTimeout(function(){ location.reload(); }, 600)` and its fix (an
#   HTMX-style swap of the affected fragment) both wrap the callback across
#   several lines.
setTimeout_reload_hits() {
  awk '
    BEGIN { armed = 0; depth = 0; buf = ""; startLine = 0; hits = 0 }
    {
      line = $0
      if (!armed && index(line, "setTimeout(") > 0) {
        armed = 1; depth = 0; buf = ""; startLine = NR
        line = substr(line, index(line, "setTimeout("))
      }
      if (armed) {
        buf = buf line "\n"
        for (i = 1; i <= length(line); i++) {
          c = substr(line, i, 1)
          if (c == "(") depth++
          else if (c == ")") {
            depth--
            if (depth == 0) {
              if (index(buf, "reload(") > 0) {
                hits++
                print "  line " startLine ": setTimeout(...) reloads the page"
              }
              armed = 0; buf = ""
              break
            }
          }
        }
      }
    }
    END { exit (hits > 0 ? 1 : 0) }
  ' "$1"
}

# manual_json_hits <file>
#   Flags the hand-built-JSON signature: a backtick string literal opening on
#   `{"` with a SECOND quoted key following a comma — e.g.
#   `{"mode":"%s","color":"%s",...}`. The comma is what tells a multi-field
#   config blob (the anti-pattern) apart from this same codebase's pervasive
#   single-field wire idiom, `hx-headers={ fmt.Sprintf(`{"X-CSRF-Token":"%s"}`,
#   ...) }` and the equivalent one-key hx-vals, neither of which has a second
#   field to comma-separate. An Alpine.js x-data object literal — the other
#   accepted pattern in these same templ files — always opens its backtick on
#   whitespace/newline before a bare key, never immediately on `{"`, so it
#   can't false-positive here either.
manual_json_hits() {
  grep -nE '`\{"[^`]+","' "$1" || true
}

# js_injected_ui_hits <file>
#   Flags JS constructing a `data-mode="..."` control or a `type="file"`
#   input as a string/DOM-build: the topbar Image mode's button and upload
#   panel belong in branding.templ's server-rendered markup (TopbarImageSection),
#   never assembled here. This file only ever READS data-mode
#   (`querySelectorAll('button[data-mode]')`, `getAttribute('data-mode')` —
#   neither has `=` immediately after `data-mode`), so a write signature here
#   is unambiguous.
js_injected_ui_hits() {
  grep -nE "data-mode=[\"']|type=[\"']file[\"']" "$1" || true
}

# --- self-test --------------------------------------------------------------

self_test() {
  local fail=0 tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN

  # setTimeout+reload: single-line.
  printf 'x();\nsetTimeout(() => location.reload(), 600);\ny();\n' > "${tmp}/bad1.js"
  if setTimeout_reload_hits "${tmp}/bad1.js" >/dev/null; then
    echo "  self-test FAILED: single-line setTimeout(reload) not caught" >&2; fail=1
  fi
  # setTimeout+reload: multi-line, the shape a swapped-in fragment callback
  # actually takes.
  printf 'setTimeout(function () {\n  window.location.reload();\n}, 600);\n' > "${tmp}/bad2.js"
  if setTimeout_reload_hits "${tmp}/bad2.js" >/dev/null; then
    echo "  self-test FAILED: multi-line setTimeout(reload) not caught" >&2; fail=1
  fi
  # setTimeout without reload: must stay quiet, including a later, unrelated
  # setTimeout call after the first one closes.
  printf 'setTimeout(function () {\n  doThing();\n}, 300);\nsetTimeout(other, 10);\n' > "${tmp}/good1.js"
  if ! setTimeout_reload_hits "${tmp}/good1.js" >/dev/null; then
    echo "  self-test FAILED: an unrelated setTimeout tripped the guard" >&2; fail=1
  fi

  # Manual JSON in a templ file: a multi-field literal is what tells the
  # anti-pattern apart from the single-field wire idiom below.
  printf 'func f() string { return fmt.Sprintf(`{"mode":"%%s","color":"%%s"}`, x, y) }\n' > "${tmp}/bad.templ"
  if [[ -z "$(manual_json_hits "${tmp}/bad.templ")" ]]; then
    echo "  self-test FAILED: hand-built JSON literal not caught" >&2; fail=1
  fi
  # An Alpine x-data object literal must NOT be mistaken for hand-built JSON.
  printf 'x-data={ fmt.Sprintf(`{\n\ttab: %%s,\n}`, y) }\n' > "${tmp}/good.templ"
  if [[ -n "$(manual_json_hits "${tmp}/good.templ")" ]]; then
    echo "  self-test FAILED: an Alpine x-data literal false-positived as hand-built JSON" >&2; fail=1
  fi
  # The pervasive single-field hx-headers/hx-vals CSRF/form-value idiom, used
  # throughout this codebase (not just Customize), must NOT be mistaken for a
  # hand-built config blob — it has no second field to comma-separate.
  printf 'hx-headers={ fmt.Sprintf(`{"X-CSRF-Token":"%%s"}`, csrfToken) }\n' > "${tmp}/good3.templ"
  if [[ -n "$(manual_json_hits "${tmp}/good3.templ")" ]]; then
    echo "  self-test FAILED: the single-field hx-headers CSRF idiom false-positived" >&2; fail=1
  fi

  # JS-injected UI control.
  printf 'el.innerHTML = %s;\n' "'<button data-mode=\"image\">Image</button>'" > "${tmp}/bad.js"
  if [[ -z "$(js_injected_ui_hits "${tmp}/bad.js")" ]]; then
    echo "  self-test FAILED: a JS-built data-mode control not caught" >&2; fail=1
  fi
  printf "modeContainer.querySelectorAll('button[data-mode]');\n" > "${tmp}/good2.js"
  if [[ -n "$(js_injected_ui_hits "${tmp}/good2.js")" ]]; then
    echo "  self-test FAILED: reading data-mode via a selector false-positived" >&2; fail=1
  fi
  printf "this.getAttribute('data-mode');\n" > "${tmp}/good3.js"
  if [[ -n "$(js_injected_ui_hits "${tmp}/good3.js")" ]]; then
    echo "  self-test FAILED: reading data-mode via getAttribute false-positived" >&2; fail=1
  fi

  # A missing listed file must fail, not be silently skipped.
  if require_file_exists "${tmp}/does-not-exist.js" >/dev/null; then
    echo "  self-test FAILED: a missing listed file did not fail" >&2; fail=1
  fi
  if ! require_file_exists "${tmp}/bad1.js" >/dev/null; then
    echo "  self-test FAILED: an existing file was reported as missing" >&2; fail=1
  fi

  return "${fail}"
}

if ! self_test; then
  echo "check-customize-discipline: SELF-TEST FAILED — the guard is broken; fix it before trusting a pass." >&2
  exit 2
fi

if [[ "${1:-}" == "--self-test-only" ]]; then
  echo "check-customize-discipline: self-test OK (missing-file / setTimeout-reload / manual-JSON / JS-injected-UI each fire, clean stays quiet)."
  exit 0
fi

# --- the real run -----------------------------------------------------------

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

problems=0

for f in "${JS_FILES[@]}"; do
  if ! require_file_exists "${f}"; then
    problems=1
    continue
  fi
  if out="$(setTimeout_reload_hits "${f}")"; then
    :
  else
    echo "ERROR: ${f} reloads the page from a setTimeout after a save (no full page reloads — swap the affected fragment/preview instead):"
    printf '%s\n' "${out}"
    problems=1
  fi
  out="$(js_injected_ui_hits "${f}")"
  if [[ -n "${out}" ]]; then
    echo "ERROR: ${f} constructs a data-mode or file-upload control from JS; that UI belongs in a .templ file as first-class markup:"
    printf '%s\n' "${out}"
    problems=1
  fi
done

for f in "${TEMPL_FILES[@]}"; do
  if ! require_file_exists "${f}"; then
    problems=1
    continue
  fi
  out="$(manual_json_hits "${f}")"
  if [[ -n "${out}" ]]; then
    echo "ERROR: ${f} hand-assembles a JSON string literal; build it with encoding/json from a small .go helper instead:"
    printf '%s\n' "${out}"
    problems=1
  fi
done

if [[ "${problems}" -ne 0 ]]; then
  echo
  echo "Each of these is a regression this guard exists to reject: a mode or"
  echo "upload control built in JS instead of shipped as templ markup, a"
  echo "setTimeout that reloads the page instead of swapping a fragment, or a"
  echo "hand-built multi-field JSON literal instead of encoding/json."
  exit 1
fi

echo "check-customize-discipline: OK — no JS-built mode/upload control, no setTimeout reload, no hand-built JSON in the Customize files. (self-test OK)"
exit 0
