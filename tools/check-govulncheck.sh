#!/usr/bin/env bash
# tools/check-govulncheck.sh
#
# Runs govulncheck and fails on any vulnerability Chronicle's code calls,
# except an entry in tools/govulncheck-allow.txt at or above the version it
# names. The allow-list exists for one case: a module has shipped a fix the
# Go vulnerability database does not record yet, so plain govulncheck would
# stay red on every PR until the database catches up.
#
# Usage: tools/check-govulncheck.sh [govulncheck-json-file]
#   With no file it runs `govulncheck -format json ./...` itself.
#   --self-test-only runs the self-test and exits.
# Exit: 0 clean / 1 a called vulnerability is not allowed / 2 self-test or
# tool failure.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
allow_file="${ALLOW_FILE:-$repo_root/tools/govulncheck-allow.txt}"

# called <json>: prints "<id> <module> <version>" for each vulnerability the
# code reaches. Only symbol-level findings carry a function in trace[0];
# module- and package-level ones are not called and do not fail the check,
# matching govulncheck's own exit code.
called() {
  jq -r 'select(.finding != null) | .finding
         | select(.trace[0].function != null)
         | "\(.osv) \(.trace[0].module) \(.trace[0].version)"' "$1" | sort -u
}

# version_at_least <have> <min>: true when have >= min (semver, leading v ok).
version_at_least() {
  [ "$(printf '%s\n%s\n' "${2#v}" "${1#v}" | sort -V | head -n1)" = "${2#v}" ]
}

# gate <json> <allow-file>: prints the verdict, returns 1 on a disallowed find.
gate() {
  local json="$1" allow="$2" id mod ver a_id a_mod a_min a_issue ok bad=0
  local -A seen=()
  while read -r id mod ver; do
    [ -n "$id" ] || continue
    seen[$id]=1
    ok=0
    while read -r a_id a_mod a_min a_issue; do
      case "$a_id" in ''|'#'*) continue ;; esac
      if [ "$a_id" = "$id" ] && [ "$a_mod" = "$mod" ] && version_at_least "$ver" "$a_min"; then
        ok=1
        echo "allowed: $id in $mod@$ver (fixed upstream, database not updated; $a_issue)"
      fi
    done < "$allow"
    if [ "$ok" = 0 ]; then
      echo "::error::$id in $mod@$ver is called by Chronicle code; see https://pkg.go.dev/vuln/$id"
      bad=1
    fi
  done < <(called "$json")
  while read -r a_id _ _ a_issue; do
    case "$a_id" in ''|'#'*) continue ;; esac
    [ -n "${seen[$a_id]:-}" ] || echo "::warning::$a_id no longer reported; remove it from the allow-list ($a_issue)"
  done < "$allow"
  return "$bad"
}

self_test() {
  local t
  t="$(mktemp -d)"
  trap 'rm -rf "$t"' RETURN
  printf 'GO-1 example.com/m v1.2.3 #1\nGO-9 example.com/m v1.0.0 #9\n' > "$t/allow"
  f() { printf '{"finding":{"osv":"%s","trace":[{"module":"example.com/m","version":"%s"%s}]}}\n' "$1" "$2" "$3"; }
  fn=',"function":"F"'
  f GO-1 v1.2.3 "$fn" > "$t/ok.json"
  f GO-1 v1.10.0 "$fn" > "$t/newer.json"
  f GO-1 v1.2.2 "$fn" > "$t/older.json"
  f GO-2 v1.2.3 "$fn" > "$t/other.json"
  f GO-2 v1.2.3 "" > "$t/uncalled.json"
  gate "$t/ok.json" "$t/allow" >/dev/null || { echo "self-test: allowed entry failed" >&2; return 1; }
  gate "$t/newer.json" "$t/allow" >/dev/null || { echo "self-test: newer version failed" >&2; return 1; }
  ! gate "$t/older.json" "$t/allow" >/dev/null || { echo "self-test: older version passed" >&2; return 1; }
  ! gate "$t/other.json" "$t/allow" >/dev/null || { echo "self-test: unlisted id passed" >&2; return 1; }
  gate "$t/uncalled.json" "$t/allow" >/dev/null || { echo "self-test: uncalled finding failed" >&2; return 1; }
  gate "$t/ok.json" "$t/allow" | grep -q 'warning::GO-9' || { echo "self-test: stale entry not warned" >&2; return 1; }
}

if ! self_test; then exit 2; fi
[ "${1:-}" = "--self-test-only" ] && { echo "check-govulncheck: self-test OK"; exit 0; }

if [ -n "${1:-}" ]; then
  json="$1"
else
  json="$(mktemp)"
  trap 'rm -f "$json"' EXIT
  # -format json exits 0 with findings; a non-zero exit is a scan failure.
  govulncheck -format json ./... > "$json" || { echo "::error::govulncheck failed to run" >&2; exit 2; }
fi

gate "$json" "$allow_file"
