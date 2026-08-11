package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The operator diagnostics in internal/systems reach app-layer state through
// dependency inversion: systems declares a `SetXProvider(fn)` setter and the app
// injects the real implementation at startup. systems cannot import the app, so
// the compiler cannot check that the injection ever happens — an unwired
// provider builds, vets and unit-tests perfectly and then prints "Provider not
// wired" forever in production.
//
// That failure is worse than a missing diagnostic, because it looks like an
// answer. host.plugins would report no plugins, host.errors no errors, and each
// render is one sentence away from being read as a finding. The renderers all
// deny the misreading explicitly, but the real fix is to notice at CI time that
// a setter exists with nobody calling it.
//
// This is a source-level check on purpose. Executing the wiring instead would
// mean standing up RegisterRoutes, which needs a database, Redis and every
// plugin's service — so it would be an integration test, skipped in exactly the
// CI run where a forgotten wiring line would be introduced.
//
// It walks the AST rather than grepping the bytes. That is not fastidiousness:
// the first version of this test used a regexp over raw source, and when the
// wiring line was commented out to check the test could fail, IT STILL PASSED —
// the commented-out call matched. Commenting out a line is the likeliest way
// this wiring ever gets disabled, so the guard has to be blind to comments.

// setProviderRe matches the provider-setter naming convention and captures the
// middle: `SetSyncMappingProvider` → "SyncMapping".
var setProviderRe = regexp.MustCompile(`^Set([A-Za-z0-9_]+)Provider$`)

// TestEveryDiagnosticProviderIsWiredInAppSource asserts that every provider
// setter internal/systems declares is really called from non-test app source.
//
// It deliberately does NOT assert the call sits in a particular function: the
// point is that a human wired it somewhere real, and pinning the location would
// break on an ordinary refactor while catching nothing extra. The boot-path
// question is the separate test below.
func TestEveryDiagnosticProviderIsWiredInAppSource(t *testing.T) {
	declared := declaredProviders(t, filepath.Join("..", "systems"))
	if len(declared) == 0 {
		t.Fatal("found no Set*Provider declarations in internal/systems — this test would pass vacuously")
	}

	called := calledProviders(t, ".")

	for name := range declared {
		if !called[name] {
			t.Errorf("internal/systems declares Set%sProvider but no non-test file in internal/app calls systems.Set%sProvider.\n"+
				"An unwired provider ships as a diagnostic that permanently reports 'provider not wired', which reads like a real answer.\n"+
				"Wire it in RegisterRoutes (or, if it is genuinely wired outside internal/app, widen this test's search to that package).",
				name, name)
		}
	}
}

// TestDiagnosticProviderCallsAreOnTheBootPath checks the wiring is reached from
// RegisterRoutes, which cmd/server/main.go calls at startup.
//
// Separate from the test above because it fails for a different reason: a call
// can exist in app source and still never run, sitting in a helper nothing
// invokes. This resolves the call's enclosing function and requires it to be
// RegisterRoutes itself or a method RegisterRoutes calls, so moving the wiring
// into a helper stays legal as long as the helper is actually invoked.
func TestDiagnosticProviderCallsAreOnTheBootPath(t *testing.T) {
	fset := token.NewFileSet()
	files := parseAppFiles(t, fset, ".")

	// Methods called from RegisterRoutes' body, plus RegisterRoutes itself.
	// One level is enough today (all wiring is inline in RegisterRoutes) and a
	// deeper walk would assert less clearly than it appears to.
	bootFuncs := map[string]bool{"RegisterRoutes": true}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "RegisterRoutes" || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "a" {
						bootFuncs[sel.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	if !bootFuncs["RegisterRoutes"] || len(bootFuncs) == 1 {
		t.Fatal("could not find RegisterRoutes in internal/app — this test's assumption about the boot path is stale, re-derive it before deleting")
	}

	// Where each provider call actually sits. `found` records calls in the body
	// of a named function; `inLiteral` records calls buried inside a function
	// literal, kept apart only so the failure can say which of the two shapes
	// it saw instead of claiming the call does not exist.
	found := map[string]string{}
	inLiteral := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				// Do NOT descend into a function literal. A provider call inside
				// one only runs if something invokes the literal, and the AST
				// cannot tell us that anything does: `neverRun := func() { …
				// systems.SetXProvider(…) … }` is indistinguishable from a
				// callback the router really runs. Recording it as boot-path
				// wiring would certify a call that never executes — the exact
				// defect this guard exists to catch, and a mutation it was
				// measured passing before this skip was added.
				//
				// This does not hide the shipped wiring, which PASSES literals
				// as ARGUMENTS (the route-table closure, the package lister).
				// ast.Inspect visits the CallExpr before its arguments, so the
				// provider call is recorded on the way in and only its argument
				// subtree is skipped.
				if lit, ok := n.(*ast.FuncLit); ok {
					ast.Inspect(lit.Body, func(m ast.Node) bool {
						if name, ok := systemsProviderCall(m); ok {
							inLiteral[name] = fn.Name.Name
						}
						return true
					})
					return false
				}
				if name, ok := systemsProviderCall(n); ok {
					found[name] = fn.Name.Name
				}
				return true
			})
		}
	}

	for name := range declaredProviders(t, filepath.Join("..", "systems")) {
		host, ok := found[name]
		if !ok {
			if lit, buried := inLiteral[name]; buried {
				t.Errorf("systems.Set%sProvider is only called from inside a function literal in %s. "+
					"Nothing here proves that literal is ever invoked, so the provider may never be set at runtime — "+
					"move the call into the function body itself.", name, lit)
				continue
			}
			t.Errorf("systems.Set%sProvider is never called from internal/app, so it is not on any boot path", name)
			continue
		}
		if !bootFuncs[host] {
			t.Errorf("systems.Set%sProvider is called from %s, which RegisterRoutes does not call — so it is not on the boot path cmd/server/main.go runs", name, host)
		}
	}
}

// systemsProviderCall reports whether n is a `systems.SetXProvider(...)` call,
// returning the captured X.
func systemsProviderCall(n ast.Node) (string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "systems" {
		return "", false
	}
	m := setProviderRe.FindStringSubmatch(sel.Sel.Name)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// declaredProviders returns the provider names declared by func decls in dir.
func declaredProviders(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, f := range parseAppFiles(t, fset, dir) {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if m := setProviderRe.FindStringSubmatch(fn.Name.Name); m != nil {
				out[m[1]] = true
			}
		}
	}
	return out
}

// calledProviders returns the provider names called as systems.SetXProvider in dir.
func calledProviders(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, f := range parseAppFiles(t, fset, dir) {
		ast.Inspect(f, func(n ast.Node) bool {
			if name, ok := systemsProviderCall(n); ok {
				out[name] = true
			}
			return true
		})
	}
	return out
}

// parseAppFiles parses the non-test .go files directly inside dir.
//
// Test files are excluded deliberately: a provider wired only from a test is
// precisely the bug being hunted, so counting a _test.go call site would make
// this test certify the defect it exists to catch.
func parseAppFiles(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var out []*ast.File
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		path := filepath.Join(dir, n)
		// Parsing without ParseComments: comments are not part of the AST the
		// walks above see, which is the whole point (see the file comment).
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		out = append(out, f)
	}
	return out
}
