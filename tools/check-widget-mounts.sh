#!/usr/bin/env bash
# tools/check-widget-mounts.sh
#
# DEAD WIDGET MOUNT RATCHET.
#
# THE RULE, AND WHY IT IS A RULE. A widget in Chronicle is a `data-widget="name"`
# div in a templ plus a JS module that calls `Chronicle.register('name', …)`.
# static/js/boot.js binds the two at DOMContentLoaded — and when it finds a mount
# whose name is not in its registry it RETURNS SILENTLY (mountElement: `var impl =
# widgets[name]; if (!impl) { return; }`). There is no lazy loader: boot.js never
# injects a script tag, never dynamic-imports, never fetches
# /static/js/widgets/<name>.js. There is no bundler either — the only esbuild call
# in the Makefile builds the tiptap vendor bundle.
#
# So a widget whose JS file is in NO `<script src>` anywhere renders as an empty
# div, forever, on every page that mounts it — with an empty console, no network
# error, and a page that looks finished. That is how `aliases`, `inventory` and
# `transaction_log` shipped: complete table + repo + service + REST routes + a
# working 170-to-380-line widget each, mounted on real entity pages, permanently
# blank. `aliases` sat inside the `title` CORE block, i.e. effectively every
# entity page in the product.
#
# THE FIX IS ALWAYS ONE OF TWO. Add the file to the layout's script list
# (internal/templates/layouts/base.templ), which emits outside the hx-boost
# swapped region; or, for a plugin-owned script, contribute it to the plugin
# BODY-SCRIPT REGISTRY (internal/app/routes.go's `pluginBodyScripts`). Do NOT put
# it in a page templ — tools/check-page-scripts.sh explains at length why that
# loads on a typed URL and not through the sidebar.
#
# WHAT COUNTS AS A LOAD PATH. A `<script src=` line naming `<file>.js` in any
# *.templ, or any mention of it in internal/app/routes.go (the body-script
# registry). Mount names are kebab-case in the DOM and snake_case on disk
# (`data-widget="tag-picker"` → tag_picker.js), so the name is translated before
# the lookup. A mount whose `data-widget` value is a templ EXPRESSION rather than
# a literal (blockExtWidget's `data-widget={ widgetSlug }`) is out of scope by
# construction: extension widgets ship their own scripts per campaign.
#
# THIS GUARD IS A RATCHET, NOT AN AUDIT — same shape as check-page-scripts.sh.
# tools/widget-mount-allowlist.txt names the mounts that are known-dead and NOT
# fixable by a script tag alone. The set may only shrink: a name in the tree but
# not the allowlist fails, and a name in the allowlist that no longer has a dead
# mount ALSO fails, because a stale entry is a hole a dead mount can be put back
# through.
#
# SELF-TEST: every run first executes the resolver and the comparison against
# fixtures in a temp dir, so "OK" always means the rule can actually fire — a
# guard that cannot fail is worse than no guard, because it reads as coverage.
# Run just the self-test with: --self-test-only
#
# Exit codes:
#   0 — every literal widget mount in the tree has a load path (or is allowlisted)
#   1 — a mount with no load path, or a stale allowlist entry
#   2 — the guard's own self-test failed (the guard is broken, not the code)

set -euo pipefail

ALLOWLIST="${ALLOWLIST:-tools/widget-mount-allowlist.txt}"

# Joined at runtime so this guard's own source and its allowlist — both of which
# must spell the attribute out to be readable — can never be swept up by a future
# widening of the scan beyond *.templ. Same trick as check-page-scripts.sh.
attr='data-wid''get='

# mount_names <root>
#   Prints every distinct literal `data-widget` value under <root>'s *.templ
#   files, sorted. Non-literal (templ expression) mounts do not match and are
#   deliberately skipped.
mount_names() {
  local root="$1"
  ( cd "${root}" && \
    find . -name '*.templ' -not -path './node_modules/*' -print0 \
    | xargs -0 grep -hoE "${attr}\"[a-z0-9_-]+\"" 2>/dev/null \
    | sed -e "s/^${attr}\"//" -e 's/"$//' \
    | sort -u )
}

# has_load_path <root> <mount-name>
#   True when the widget's JS file is named by a `<script src=` line in some
#   *.templ, or anywhere in the plugin body-script registry (routes.go).
has_load_path() {
  local root="$1" name="$2" file
  file="$(echo "${name}" | tr '-' '_')"
  # The script-src lines are collected ONCE per tree by script_src_lines() and
  # cached, because re-walking every *.templ per widget name is O(names × tree).
  if grep -qE "[\"/]${file}\.js" <<< "$(script_src_lines "${root}")"; then
    return 0
  fi
  if [[ -f "${root}/internal/app/routes.go" ]] \
     && grep -qE "[\"/]${file}\.js" "${root}/internal/app/routes.go"; then
    return 0
  fi
  return 1
}

# script_src_lines <root> — every `<script src=` line across the tree's *.templ
# files, memoised in _SCRIPT_SRC_CACHE for the life of the process.
_SCRIPT_SRC_ROOT=""
_SCRIPT_SRC_CACHE=""
script_src_lines() {
  local root="$1"
  if [[ "${_SCRIPT_SRC_ROOT}" != "${root}" ]]; then
    _SCRIPT_SRC_ROOT="${root}"
    _SCRIPT_SRC_CACHE="$( cd "${root}" && \
      find . -name '*.templ' -not -path './node_modules/*' -print0 \
      | xargs -0 grep -hE "scr""ipt src=" 2>/dev/null || true )"
  fi
  printf '%s' "${_SCRIPT_SRC_CACHE}"
}

# dead_mounts <root> — prints the mount names with no load path, sorted.
dead_mounts() {
  local root="$1" n
  while IFS= read -r n; do
    [[ -z "${n}" ]] && continue
    has_load_path "${root}" "${n}" || echo "${n}"
  done < <(mount_names "${root}")
}

# read_allowlist <file> — strips comments and blank lines, one name per line.
read_allowlist() {
  # `|| true`: an allowlist that is entirely comments is legitimate (it is what
  # this file looks like once the last dead mount is fixed), and grep's
  # no-match exit 1 would otherwise abort the run under `set -e`.
  { sed -e 's/#.*$//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$1" \
    | grep -v '^$' || true; } | sort -u
}

# compare <actual-names> <allowed-names> — prints findings, returns 1 on any.
# Kept pure so the self-test exercises the real comparison, not a copy of it.
compare() {
  local actual="$1" allowed="$2" findings="" n

  while IFS= read -r n; do
    [[ -z "${n}" ]] && continue
    if ! echo "${allowed}" | grep -qxF "${n}"; then
      findings+="  DEAD     ${attr}\"${n}\" — mounted in a templ, but its JS is loaded nowhere"$'\n'
    fi
  done <<< "${actual}"

  while IFS= read -r n; do
    [[ -z "${n}" ]] && continue
    if ! echo "${actual}" | grep -qxF "${n}"; then
      findings+="  STALE    ${n} (allowlisted, but no longer a dead mount) — delete the line"$'\n'
    fi
  done <<< "${allowed}"

  if [[ -n "${findings}" ]]; then
    printf '%s' "${findings}"
    return 1
  fi
  return 0
}

# --- self-test --------------------------------------------------------------
#
# A fixture tree with one loaded mount, one registry-loaded mount, one kebab
# mount whose file is snake_case, one expression mount, and one dead mount —
# then each verdict is driven through the real compare().
self_test() {
  local tmp fail=0
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN

  mkdir -p "${tmp}/tree/internal/app" "${tmp}/tree/pkg"
  # The layout: loads editor.js and tag_picker.js via script src.
  {
    printf '<scr%s"/static/js/widgets/editor.js" defer></scr%s>\n' 'ipt src=' 'ipt'
    printf '<scr%s"/static/js/widgets/tag_picker.js" defer></scr%s>\n' 'ipt src=' 'ipt'
  } > "${tmp}/tree/pkg/layout.templ"
  # The body-script registry: loads calendar_widget.js.
  printf '"/static/plugins/calendar/js/calendar_widget.js",\n' \
    > "${tmp}/tree/internal/app/routes.go"
  # Mounts: editor (loaded), tag-picker (kebab→snake, loaded), calendar-widget
  # (registry), ghost (dead), and an expression mount that must not be scanned.
  {
    printf '<div %s"editor"></div>\n' "${attr}"
    printf '<div %s"tag-picker"></div>\n' "${attr}"
    printf '<div %s"calendar-widget"></div>\n' "${attr}"
    printf '<div %s"ghost"></div>\n' "${attr}"
    printf '<div %s{ widgetSlug }></div>\n' "${attr}"
  } > "${tmp}/tree/pkg/page.templ"

  local got
  got="$(mount_names "${tmp}/tree" | tr '\n' ',')"
  if [[ "${got}" != "calendar-widget,editor,ghost,tag-picker," ]]; then
    echo "  self-test FAILED: mount_names = [${got}]" >&2
    fail=1
  fi

  got="$(dead_mounts "${tmp}/tree")"
  if [[ "${got}" != "ghost" ]]; then
    echo "  self-test FAILED: dead_mounts = [${got}]; expected [ghost]" >&2
    fail=1
  fi

  # Quiet when the allowlist matches exactly.
  if ! compare "${got}" "ghost" >/dev/null; then
    echo "  self-test FAILED: an exactly-matching allowlist did not pass" >&2
    fail=1
  fi
  # Each verdict fires.
  local out
  out="$(compare "${got}" "" || true)"
  [[ "${out}" == *"DEAD"*"ghost"* ]] || { echo "  self-test FAILED: DEAD did not fire" >&2; fail=1; }
  out="$(compare "${got}" "$(printf 'ghost\nvanished')" || true)"
  [[ "${out}" == *"STALE    vanished"* ]] || { echo "  self-test FAILED: STALE did not fire" >&2; fail=1; }

  return "${fail}"
}

if ! self_test; then
  echo "check-widget-mounts: SELF-TEST FAILED — the guard is broken; fix it before trusting a pass." >&2
  exit 2
fi

if [[ "${1:-}" == "--self-test-only" ]]; then
  echo "check-widget-mounts: self-test OK (DEAD / STALE each fire, clean stays quiet)."
  exit 0
fi

# --- the real run -----------------------------------------------------------

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

if [[ ! -f "${ALLOWLIST}" ]]; then
  echo "check-widget-mounts: allowlist ${ALLOWLIST} is missing — a ratchet without its baseline cannot fail, which is worse than no ratchet." >&2
  exit 2
fi

actual="$(dead_mounts "${repo_root}")"
allowed="$(read_allowlist "${ALLOWLIST}")"

if findings="$(compare "${actual}" "${allowed}")"; then
  count=$(echo "${allowed}" | grep -c . || true)
  echo "check-widget-mounts: OK — every literal widget mount has a load path (${count} known-dead mount(s) allowlisted). (self-test OK)"
  exit 0
fi

echo "ERROR: a widget mount has no way to load its implementation."
echo
printf '%s' "${findings}"
echo
echo "WHY THIS IS BLOCKED. boot.js mounts a widget by looking its data-widget name"
echo "up in the registry that Chronicle.register() fills, and RETURNS SILENTLY when"
echo "the name is absent. Nothing lazy-loads the file: no script injection, no"
echo "dynamic import, no bundler. A mount whose JS is in no <script src> anywhere"
echo "is a permanently empty div — no console output, no failed request, and a page"
echo "that looks finished."
echo
echo "WHAT TO DO: add the file to the layout's script list —"
echo "  internal/templates/layouts/base.templ"
echo "or, for a plugin-owned script, to the body-script registry —"
echo "  internal/app/routes.go   pluginBodyScripts := []string{ ... }"
echo "Do NOT add a <script src> to a page templ; tools/check-page-scripts.sh"
echo "explains why that loads on a typed URL and not through the sidebar."
echo
echo "If a mount is dead for a reason a script tag cannot fix, say so in"
echo "${ALLOWLIST} with the reason. If you are FIXING an allowlisted"
echo "one, delete its line — that is the ratchet turning, and it is welcome."
exit 1
