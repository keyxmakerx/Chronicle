// system_viewer_allowlist_test.go guards ADR-049: permissions.SystemViewer
// skips every per-user visibility rule, so only reviewed, request-less call
// sites may use it. A call outside the allowlist below fails CI; adding one
// means editing that list in review.
package wire

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// systemViewerAllowlist is the reviewed set of repo-relative files allowed
// to call permissions.SystemViewer. Each is request-less or owner-only by
// its own route: the campaign and AI exports, a widget picker already
// authorized at its route, and the operator's health check (see each call
// site's comment for why it's trusted).
var systemViewerAllowlist = map[string]bool{
	"internal/app/export_adapters.go": true,
	// A quest's due-date event is the DM's own record, written after the
	// quests route has checked DM access; it must see and author dm_only
	// events whatever the acting user's per-user rules.
	"internal/app/quests_adapters.go":                   true,
	"internal/plugins/timeline/timeline_widget_type.go": true,
	// The operator's calendar health diagnostic counts rows for the server
	// admin; no campaign member's view is behind it.
	"internal/app/operator_diag_calendar_adapter.go": true,
	// Same shape as timeline_widget_type.go above: InstanceExists and
	// DefaultInstance are the binding framework's own orphan/scope and
	// unbound-default checks (not a viewer's content read — RenderBlock
	// re-checks visibility against the real request viewer), and
	// ListInstances backs the Scribe+-gated binding picker route, which
	// carries a role but no per-request user id.
	"internal/plugins/calendar/calendar_widget_type.go": true,
}

// TestSystemViewerAllowlist scans internal/ for calls to
// permissions.SystemViewer and fails if any occur outside
// systemViewerAllowlist. Test files are excluded — tests legitimately
// construct a SystemViewer to exercise system-viewer behavior.
func TestSystemViewerAllowlist(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")

	var offenders []string
	seen := map[string]bool{}

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The definition site is not a call site.
		if filepath.Base(path) == "viewer.go" {
			return nil
		}

		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return nil // build errors are caught by `go build`/`go vet`, not this test
		}

		repoRel, _ := filepath.Rel(root, path)

		callsHere := false
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "SystemViewer" {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "permissions" {
				return true
			}
			callsHere = true
			return false
		})

		if callsHere && !systemViewerAllowlist[repoRel] {
			if !seen[repoRel] {
				seen[repoRel] = true
				offenders = append(offenders, repoRel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalDir, err)
	}

	if len(offenders) == 0 {
		return
	}

	sort.Strings(offenders)
	var msg strings.Builder
	fmt.Fprintf(&msg, "permissions.SystemViewer called from %d file(s) outside the reviewed allowlist:\n\n", len(offenders))
	for _, f := range offenders {
		msg.WriteString("  " + f + "\n")
	}
	msg.WriteString("\nSystemViewer bypasses every per-user visibility filter (ADR-049); a new call site\n")
	msg.WriteString("must be reviewed for genuinely having no request identity behind it, then added\n")
	msg.WriteString("to systemViewerAllowlist in system_viewer_allowlist_test.go.\n")
	t.Fatal(msg.String())
}
