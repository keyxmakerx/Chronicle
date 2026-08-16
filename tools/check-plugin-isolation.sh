#!/usr/bin/env bash
# check-plugin-isolation.sh — fail PRs introducing new plugin-name string
# literals OUTSIDE the owning plugin directory.
#
# Mechanism M-B2.1 per cordinator/decisions/2026-05-21-core-tenets.md §T-B2.
# Diff-scoped FAIL: only checks lines INTRODUCED by the PR vs origin/main.
# Existing violations in main are grandfathered — this guard only catches
# NEW additions.
#
# Extended (M-B2.1-extend, 2026-06-21): now covers all plugin slugs, not
# just foundry-vtt. Per cordinator/reports/chronicle/2026-06-20-plugin-
# isolation-modularity-audit.md — each slug has a per-slug allowlist to
# suppress English-word collisions, URL segments, PackageType enum values,
# addon-slug lookups, and the @layer CSS reservation line.
#
# Forbidden tokens are reconstructed via fragment join so this script can
# scan its own directory tree without matching itself (same technique as
# tools/check-no-instance-hostname.sh).

set -euo pipefail

# ---------------------------------------------------------------------------
# Token reconstruction — adjacent quoted strings concatenate at shell-runtime
# so these literals never appear in this file and can't self-trigger.
# ---------------------------------------------------------------------------

# foundry_vtt tokens (original M-B2.1 set)
fv1="fou""ndry-vtt"
fv2="fou""ndry-module"
fv3="fou""ndry_vtt"

# calendar
cal1="cal""endar"

# maps — skip: too common an English word; false-positive rate too high.
# maps_tok="ma""ps"

# media — skip: common English word (media picker, media upload, etc.)
# media_tok="me""dia"

# settings — skip: extremely common English word.
# settings_tok="set""tings"

# syncapi
sync1="syn""capi"

# ai_workspace
ai1="ai""_workspace"

# bestiary
best1="bes""tiary"

# armory
arm1="ar""mory"

# designlab
dl1="des""ignlab"

# sessions — skip: common word.
# sess_tok="ses""sions"

# packages
pkg1="pac""kages"

# npcs — skip: very short; appears in entity names, comments, etc.
# npcs_tok="np""cs"

# widgetbindings
wb1="wid""getbindings"

# timeline
tl1="tim""eline"

# backup / restore — skip: common English words.

# addons — skip: short common word; addon references are integral to most plugins.

# smtp — skip: common abbreviation used in comments everywhere.

# ---------------------------------------------------------------------------
# Per-plugin owned prefixes
# ---------------------------------------------------------------------------

fv_prefix="internal/plugins/fou""ndry_vtt/"
cal_prefix="internal/plugins/cal""endar/"
sync_prefix="internal/plugins/syn""capi/"
ai_prefix="internal/plugins/ai""_workspace/"
best_prefix="internal/plugins/bes""tiary/"
arm_prefix="internal/plugins/ar""mory/"
dl_prefix="internal/plugins/des""ignlab/"
pkg_prefix="internal/plugins/pac""kages/"
wb_prefix="internal/plugins/wid""getbindings/"
tl_prefix="internal/plugins/tim""eline/"

# ---------------------------------------------------------------------------
# Allowlisted files that may legitimately reference any plugin slug
# (e.g. the plugin registry, CI tools, docs)
# ---------------------------------------------------------------------------
# ---------------------------------------------------------------------------
# Amendment HOSTDIAG-1 (host.* operator diagnostics, 2026-08-11) — the four
# operator-diagnostic test files listed at the end of the array below.
#
# They are the unit tests for host.embedded / host.widgets / host.plugins /
# host.deploy-check, whose whole job is to REPORT ON plugins: which registered,
# which mounted an embedded static FS, and what is inside it. Their fixtures
# therefore carry plugin slugs ("calendar", "foundry-vtt") as DATA — a URL
# segment, a map key, an expected output string — which is exactly the "URL
# segment in a test fixture" case this guard's own remedy text names as
# legitimate. None of the four imports a plugin package or calls into one, so
# there is no cross-plugin COUPLING here, which is what the guard exists to stop.
#
# Renaming the fixtures to neutral slugs was considered and rejected: the
# 2026-08-11 incident these diagnostics exist to prevent was specifically a grep
# for the CALENDAR plugin's assets returning empty, because those assets are
# //go:embed-ed into the binary rather than on disk. A fixture called
# "fakeplugin" would still exercise the code path but would stop documenting the
# case it was built from.
#
# Listed as full filenames. Matching here is by PREFIX, so these are effectively
# exact — deliberately not the directory, which would exempt every future test
# in internal/systems.
# ---------------------------------------------------------------------------
always_allowed_prefixes=(
  "internal/app/routes.go"
  "internal/app/plugins.go"
  "internal/app/app.go"
  "tools/"
  ".github/"
  "cmd/"
  "internal/systems/operator_diag_assets_test.go"
  "internal/systems/operator_diag_deploy_test.go"
  "internal/systems/operator_diag_plugins_test.go"
  "internal/app/operator_diag_plugins_test.go"
  # The campaign diagnostics' fixtures. Same allowance as the four test files
  # above: these tests assert what `campaign.config` / `campaign.surfaces` print
  # about the CALENDAR addon specifically — a disabled-addon row, and the sidebar
  # item that links to it. Substituting a non-colliding slug would leave the test
  # asserting nothing about the case it is named for.
  "internal/systems/operator_diag_campaign_test.go"
  # The sessions plugin's real-MariaDB test harness. sessions/migrations/001
  # carries `FOREIGN KEY (calendar_id) REFERENCES calendars(id)`, so a sessions
  # row-level test cannot get a schema at all without the CALENDAR plugin's
  # migrations applied first. It loads them OFF DISK with os.DirFS precisely to
  # avoid the import edge — internal/wire/plugin_import_guard_test.go forbids a
  # sessions→calendar import outright — so the slug here names a migrations
  # DIRECTORY in a test fixture, not a runtime dependency. Substituting a
  # non-colliding name would simply fail to find the migrations.
  "internal/plugins/sessions/dbtest_support_test.go"
)

# ---------------------------------------------------------------------------
# Amendment R4-S26-A (sweep R4 stage 26) — const-registry files.
#
# STRICTLY NARROWER than always_allowed_prefixes, and the correct tool when a
# file needs to DEFINE a colliding label rather than USE a plugin. Matching is
# by EXACT path (not prefix), and inside a listed file only a bare
# const-assignment line — `Name = "slug"` and nothing else on it — is exempt.
# Every other added line in that file is checked exactly as if it were not
# listed, so a `report.Fail("<slug>", …)` smuggled into a registry file still
# fails, and so does a trailing-code line dressed up to look like an
# assignment. Blanket-allowlisting the whole file would have exempted both.
#
# Listed:
#   internal/plugins/campaigns/import_report.go — import-report section/kind
#   vocabulary. Two report headings and two singular nouns happen to spell
#   plugin slugs; see the const block there for why they are labels and not
#   references. Introduced when sweep R4 stage 16 threaded ImportReport through
#   the export adapters and left this guard red for nine commits.
#
#   internal/systems/operator_diag_campaign.go — one LAYOUT BLOCK TYPE that
#   spells the calendar plugin's slug (`blockTypeCalendar`). The diagnostic
#   annotates block types it is HANDED by the app layer; internal/systems may
#   not import a plugin, so it cannot borrow calendar.PluginSlug the way
#   internal/app/operator_diag_campaign_adapter.go does. See the const block
#   there. The app-layer half of the same diagnostic needs no exemption — it
#   was fixed by aliasing the plugin's own const instead.
#
# Pinned by tools/test-plugin-isolation.sh, which mutation-tests the narrowness
# in both directions (a const line passes; the same slug on any other line of
# the same file still fails).
# ---------------------------------------------------------------------------
const_registry_files=(
  "internal/plugins/campaigns/import_report.go"
  "internal/systems/operator_diag_campaign.go"
)

# ---------------------------------------------------------------------------
# Diff base detection
# ---------------------------------------------------------------------------
base="${DIFF_BASE:-}"
if [[ -z "${base}" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    base="origin/${GITHUB_BASE_REF}"
  else
    base="origin/main"
  fi
fi

# Files changed in the PR.
changed_files=$(git diff --name-only "${base}"...HEAD -- '*.go' '*.templ' '*.css' 2>/dev/null || true)
if [[ -z "${changed_files}" ]]; then
  echo "check-plugin-isolation: no changed Go/templ/CSS files vs ${base}; nothing to check."
  exit 0
fi

# ---------------------------------------------------------------------------
# check_token <file> <added_lines> <token_regex> <owned_prefix>
# Returns 0 (clean) or emits a finding to stdout.
# Callers accumulate findings into $violations.
# ---------------------------------------------------------------------------
check_one() {
  local file="$1"
  local added="$2"
  local tok_re="$3"
  local owned="$4"

  # Skip if this file is owned by the plugin.
  case "${file}" in
    "${owned}"*) return 0 ;;
  esac

  # Skip always-allowed paths (app wiring, tools, CI, cmd).
  for ap in "${always_allowed_prefixes[@]}"; do
    case "${file}" in
      "${ap}"*) return 0 ;;
    esac
  done

  # Strip whole-line `//` comments before scanning: a full-line comment that
  # quotes a plugin slug is documentation, not a cross-plugin code reference, and
  # shouldn't trip the guard (RC-15.2 — it flagged doc comments in #530). ONLY
  # lines that START with `//` (after the diff `+` and indentation) are dropped —
  # the entire line is then a comment, so no code can hide behind the skip.
  # Block comments (`/* */`), block-comment continuations (` * `), inline/trailing
  # comments, and CSS `#id` / `*{}` / `* x` selectors are deliberately NOT skipped:
  # dropping a line that merely BEGINS with a comment token would delete the real
  # code after it and open a bypass (e.g. `/* x */ f("calendar")`).
  local code_added
  code_added=$(echo "${added}" | grep -vE '^\+[[:space:]]*//' || true)
  [[ -z "${code_added}" ]] && return 0

  # Amendment R4-S26-A — const-registry files: drop bare const-assignment
  # lines, and ONLY those. The anchored `$` is what keeps this narrow; without
  # it, `X = "calendar"; report.Fail("calendar", …)` would slip through. An
  # inline trailing comment is not permitted either — the declaration must be
  # the whole line, so nothing can hide to the right of it.
  local crf
  for crf in "${const_registry_files[@]}"; do
    if [[ "${file}" == "${crf}" ]]; then
      code_added=$(echo "${code_added}" | grep -vE '^\+[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=[[:space:]]*"[^"]*"[[:space:]]*$' || true)
      break
    fi
  done
  [[ -z "${code_added}" ]] && return 0

  if echo "${code_added}" | grep -qE "\"${tok_re}\""; then
    echo "${file}:"
    echo "${code_added}" | grep -nE "\"${tok_re}\"" || true
    echo ""
  fi
}

violations=""
while IFS= read -r file; do
  [[ -z "${file}" ]] && continue
  [[ -f "${file}" ]] || continue  # skip deleted files

  added=$(git diff -U0 "${base}"...HEAD -- "${file}" | grep -E '^\+' | grep -vE '^\+\+\+' || true)
  [[ -z "${added}" ]] && continue

  # Check each plugin token
  hit=$(check_one "${file}" "${added}" "(${fv1}|${fv2}|${fv3})" "${fv_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${cal1}" "${cal_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${sync1}" "${sync_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${ai1}" "${ai_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${best1}" "${best_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${arm1}" "${arm_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${dl1}" "${dl_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${pkg1}" "${pkg_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${wb1}" "${wb_prefix}")
  violations+="${hit}"

  hit=$(check_one "${file}" "${added}" "${tl1}" "${tl_prefix}")
  violations+="${hit}"

done <<< "${changed_files}"

if [[ -n "${violations}" ]]; then
  cat <<MSG
ERROR — new plugin-isolation violations introduced (T-B2 / M-B2.1):

${violations}

Plugin-name string literals must live inside the owning plugin's directory
or pass through the plugin-registration interface (PluginSlug const, service
interfaces, etc.). Per cordinator/decisions/2026-05-21-core-tenets.md §T-B2.

Existing violations in main are grandfathered. This guard only catches NEW
additions. If your change is a legitimate cross-plugin reference (e.g. the
plugin registry wiring in internal/app/routes.go, a CI tool, or a URL
segment in a test fixture), add the file prefix to always_allowed_prefixes
in this script with a comment citing the allowance.
MSG
  exit 1
fi

echo "check-plugin-isolation: OK — no new plugin-isolation violations introduced."
exit 0
