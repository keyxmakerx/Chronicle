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
// to call permissions.SystemViewer. Both are genuinely request-less: an
// export walking its own rows, and a widget picker already authorized at
// its own route (see each call site's comment for why it's trusted).
var systemViewerAllowlist = map[string]bool{
	"internal/app/export_adapters.go":                   true,
	"internal/plugins/timeline/timeline_widget_type.go": true,
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
