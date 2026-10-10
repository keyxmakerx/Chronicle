// Pins the sanitization-on-write invariant (T-B1): every function in a
// plugin/widget service.go that accepts HTML-typed user input must reach
// sanitize.HTML, either in its own body or through a same-package helper
// (at most two call levels away).
//
// The check is per function, not per file, so a new Update method cannot
// hide behind a sibling that already sanitizes. It is name-based: a call
// to foo() or x.foo() resolves to every function named foo in the package,
// which over-approximates reachability slightly but never misses a real
// call. Functions that legitimately skip sanitizing (they delegate to a
// method that does, or never persist the value) are listed in
// unsanitizedAllowlist with the reason.
//
// A snapshot file (sanitize_invariant_snapshot.txt) lists every
// HTML-input function and how it is covered, so new sanitize surface is
// explicit at review time. Regenerate after an intentional change:
//
//	UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...
package sanitize

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

// maxHelperDepth bounds how far from the function a sanitize.HTML call may
// sit. Deeper chains are too indirect to audit at review time.
const maxHelperDepth = 2

// unsanitizedAllowlist maps "<repo-relative service.go>:<function>" to why
// that function takes HTML input yet need not call sanitize.HTML itself.
var unsanitizedAllowlist = map[string]string{
	"internal/plugins/entities/service.go:buildSearchText": "reduces the HTML to plain text for the search_text column; the result is never rendered as markup",
}

// funcInfo is what the analysis keeps per function declaration.
type funcInfo struct {
	Key        string   // "<rel service.go>:<Recv.>Name" for service files.
	Name       string   // Bare function name, used for call resolution.
	HTMLInputs []string // Parameter descriptions that carry HTML.
	Direct     bool     // Body calls sanitize.HTML.
	Callees    []string // Bare names of functions called in the body.
}

// pkgIndex holds every non-test function and HTML-bearing struct type of a
// package directory so calls and input types resolve across its files.
type pkgIndex struct {
	funcs     map[string][]*funcInfo // by bare name
	htmlTypes map[string]string      // type name -> first HTML field
}

// serviceFunc is a function declared in a service.go file.
type serviceFunc struct {
	Dir  string
	Info *funcInfo
}

// coverage reports how a function reaches sanitize.HTML: 0 direct, 1..max
// via helpers, -1 not at all.
func (p *pkgIndex) coverage(f *funcInfo) int {
	if f.Direct {
		return 0
	}
	frontier := f.Callees
	seen := map[string]bool{}
	for depth := 1; depth <= maxHelperDepth; depth++ {
		var next []string
		for _, name := range frontier {
			if seen[name] {
				continue
			}
			seen[name] = true
			for _, c := range p.funcs[name] {
				if c.Direct {
					return depth
				}
				next = append(next, c.Callees...)
			}
		}
		frontier = next
	}
	return -1
}

func (f *funcInfo) formatLine(cov int) string {
	how := "direct"
	switch {
	case cov < 0:
		how = "allowlisted"
	case cov > 0:
		how = fmt.Sprintf("helper_depth=%d", cov)
	}
	return fmt.Sprintf("%s\t%s\tinputs=%s", f.Key, how, strings.Join(f.HTMLInputs, ","))
}

// analyze walks every service.go and returns its HTML-input functions with
// the package indexes needed to judge them.
func analyze(t *testing.T) ([]serviceFunc, map[string]*pkgIndex) {
	t.Helper()
	root := repoRootForSanitize(t)

	var services []string
	for _, dir := range []string{"internal/plugins", "internal/widgets"} {
		base := filepath.Join(root, dir)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && info.Name() == "service.go" {
				services = append(services, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
	sort.Strings(services)

	indexes := map[string]*pkgIndex{}
	var out []serviceFunc
	for _, svc := range services {
		dir := filepath.Dir(svc)
		idx, ok := indexes[dir]
		if !ok {
			idx = indexPackage(t, dir)
			indexes[dir] = idx
		}
		rel, _ := filepath.Rel(root, svc)
		for _, f := range parseFuncs(t, svc, idx.htmlTypes) {
			f.Key = rel + ":" + f.Key
			if len(f.HTMLInputs) > 0 {
				out = append(out, serviceFunc{Dir: dir, Info: f})
			}
		}
	}
	return out, indexes
}

// indexPackage parses all non-test files of dir.
func indexPackage(t *testing.T, dir string) *pkgIndex {
	t.Helper()
	idx := &pkgIndex{funcs: map[string][]*funcInfo{}, htmlTypes: map[string]string{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var paths []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			paths = append(paths, filepath.Join(dir, n))
		}
	}
	// Types first: parameter detection needs the complete type table.
	for _, p := range paths {
		file := parseFile(t, p)
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil || !isInputTypeName(ts.Name.Name) {
					continue
				}
				for _, fld := range st.Fields.List {
					for _, n := range fld.Names {
						if endsHTML(n.Name) {
							if _, dup := idx.htmlTypes[ts.Name.Name]; !dup {
								idx.htmlTypes[ts.Name.Name] = n.Name
							}
						}
					}
				}
			}
		}
	}
	for _, p := range paths {
		for _, f := range parseFuncs(t, p, idx.htmlTypes) {
			idx.funcs[f.Name] = append(idx.funcs[f.Name], f)
		}
	}
	return idx
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

// parseFuncs extracts every function/method declaration of a file.
func parseFuncs(t *testing.T, path string, htmlTypes map[string]string) []*funcInfo {
	t.Helper()
	file := parseFile(t, path)
	var out []*funcInfo
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		f := &funcInfo{Name: fd.Name.Name, Key: fd.Name.Name}
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			f.Key = typeName(fd.Recv.List[0].Type) + "." + fd.Name.Name
		}
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				tn := typeName(field.Type)
				for _, name := range field.Names {
					switch {
					case endsHTML(name.Name):
						f.HTMLInputs = append(f.HTMLInputs, name.Name)
					case htmlTypes[tn] != "":
						f.HTMLInputs = append(f.HTMLInputs, name.Name+"("+tn+"."+htmlTypes[tn]+")")
					}
				}
			}
		}
		if fd.Body != nil {
			seen := map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := call.Fun.(type) {
				case *ast.SelectorExpr:
					if id, ok := fn.X.(*ast.Ident); ok && id.Name == "sanitize" && fn.Sel.Name == "HTML" {
						f.Direct = true
					}
					if !seen[fn.Sel.Name] {
						seen[fn.Sel.Name] = true
						f.Callees = append(f.Callees, fn.Sel.Name)
					}
				case *ast.Ident:
					if !seen[fn.Name] {
						seen[fn.Name] = true
						f.Callees = append(f.Callees, fn.Name)
					}
				}
				return true
			})
		}
		out = append(out, f)
	}
	return out
}

// isInputTypeName reports whether a struct is a write-path input (request,
// input or import payload). Read models such as Entity or Note also carry
// HTML fields, but they hold already-stored content, and treating every
// function that merely receives one as an HTML sink would bury real
// findings in noise.
func isInputTypeName(name string) bool {
	return strings.HasSuffix(name, "Input") || strings.HasSuffix(name, "Request") ||
		strings.HasSuffix(name, "Req") || strings.HasPrefix(name, "Export")
}

// typeName unwraps pointers and slices to the bare package-local type name,
// or "" for anything else (maps, selectors, funcs).
func typeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeName(x.X)
	case *ast.ArrayType:
		return typeName(x.Elt)
	}
	return ""
}

// TestSanitizeInvariant_HTMLInputFunctionsSanitize is the security
// invariant: each service.go function with an HTML-typed input reaches
// sanitize.HTML or is explicitly allowlisted.
func TestSanitizeInvariant_HTMLInputFunctionsSanitize(t *testing.T) {
	funcs, indexes := analyze(t)

	used := map[string]bool{}
	var violations []string
	for _, sf := range funcs {
		cov := indexes[sf.Dir].coverage(sf.Info)
		_, allowed := unsanitizedAllowlist[sf.Info.Key]
		if allowed {
			used[sf.Info.Key] = true
		}
		if cov < 0 && !allowed {
			violations = append(violations, fmt.Sprintf("  %s takes HTML input [%s] but never reaches sanitize.HTML",
				sf.Info.Key, strings.Join(sf.Info.HTMLInputs, ",")))
		}
	}
	// A stale entry would silently exempt a future function of that name.
	for key := range unsanitizedAllowlist {
		if !used[key] {
			violations = append(violations, "  stale allowlist entry (function gone or no longer takes HTML): "+key)
		}
	}
	sort.Strings(violations)

	if len(violations) > 0 {
		t.Errorf("sanitization-on-write invariant violated.\n"+
			"Each service function that accepts HTML-typed user input must call\n"+
			"sanitize.HTML directly or via a same-package helper (max %d levels).\n"+
			"Add the call, or allowlist the function with a reason if it\n"+
			"delegates to a method that sanitizes:\n\n%s",
			maxHelperDepth, strings.Join(violations, "\n"))
	}
}

// TestSanitizeInvariant_SnapshotConformance pins the per-function inventory
// so new HTML input surface, or lost coverage, shows up as snapshot drift.
// Regenerate after an intentional change:
//
//	UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...
func TestSanitizeInvariant_SnapshotConformance(t *testing.T) {
	funcs, indexes := analyze(t)

	var lines []string
	for _, sf := range funcs {
		lines = append(lines, sf.Info.formatLine(indexes[sf.Dir].coverage(sf.Info)))
	}
	current := strings.Join(lines, "\n") + "\n"

	snapPath := snapshotPath(t)

	if os.Getenv("UPDATE_SANITIZE_SNAPSHOT") == "1" {
		if err := os.WriteFile(snapPath, []byte(current), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		t.Logf("snapshot regenerated at %s (%d entries)", snapPath, len(lines))
		return
	}

	expectedBytes, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot %s: %v\n\nFirst-time setup: run UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...", snapPath, err)
	}
	expected := string(expectedBytes)

	if current != expected {
		t.Errorf("sanitize-invariant snapshot drift detected.\n\n"+
			"If the drift is intentional, regenerate:\n\n"+
			"  UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...\n\n"+
			"then commit the updated sanitize_invariant_snapshot.txt in the\n"+
			"same PR.\n\n"+
			"Diff (sample first 40 lines):\n%s",
			diffSample(expected, current, 40),
		)
	}
}

// endsHTML reports whether the identifier looks like an HTML-typed
// field/param. Matches "HTML" exactly or any camelCase identifier
// whose final two segments are <something>HTML (e.g. EntryHTML,
// descHTML, notesHTML).
func endsHTML(name string) bool {
	if name == "HTML" {
		return false // type itself, not a parameter
	}
	return strings.HasSuffix(name, "HTML")
}


// repoRootForSanitize finds the repository root by walking up from the
// test file's directory.
func repoRootForSanitize(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// cwd will be internal/sanitize when tests run; walk up two dirs.
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func snapshotPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "sanitize_invariant_snapshot.txt")
}

// diffSample returns a coarse unified-diff sample for the error message
// — first n lines of "expected" + first n lines of "got" tagged.
func diffSample(expected, got string, n int) string {
	expLines := strings.Split(expected, "\n")
	gotLines := strings.Split(got, "\n")
	if len(expLines) > n {
		expLines = expLines[:n]
	}
	if len(gotLines) > n {
		gotLines = gotLines[:n]
	}
	return "---EXPECTED (snapshot)---\n" + strings.Join(expLines, "\n") +
		"\n\n---GOT (current)---\n" + strings.Join(gotLines, "\n")
}
