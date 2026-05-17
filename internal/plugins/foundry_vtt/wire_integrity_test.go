// C-WIRE-INTEGRITY: tests that pin two surfaces the
// 2026-05-17 error-catalog wire-contract decision names but
// the existing drift guard does not enforce.
//
//  1. Response shape: handler.go:respondError must emit exactly
//     the JSON keys declared by ResponseShape in errors_catalog.go.
//     A renamed, dropped, or added field would silently break
//     the Chronicle ↔ Foundry contract; today's drift tests
//     would not catch it.
//
//  2. Category enum reconciliation: the category enum is
//     declared at four sites — the ErrCategory* constants in
//     errors.go, the categoryGoToString map, the
//     categoryHTTPStatus map, and the Categories slice (all in
//     errors_catalog.go). A new category added to one site but
//     not the others would produce an internally-inconsistent
//     error-catalog.json that the existing drift guard cannot
//     detect.
//
// Both surfaces matter for FM-DRIFT-GUARD: Foundry-side CI will
// trust error-catalog.json as the wire contract; this file
// makes that trust earned mechanically on the Chronicle side.
//
// Scope per C-WIRE-INTEGRITY: tests only. The AST helper for
// ErrCategory* constants lives in this file rather than in
// errors_catalog.go to keep the runtime surface unchanged. If a
// future contract evolution wants the helper in production code,
// it can be promoted then.
package foundry_vtt

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
)

// --- Item 1: response shape ---

// constructorSample is a constructor name paired with a synthetic
// invocation suitable for feeding to respondError. The list is
// hand-maintained; TestWireResponseShape_AllConstructorsCovered
// fails if a new Err* constructor is added without an entry here.
type constructorSample struct {
	Name string
	Err  *Error
}

// allConstructorSamples enumerates every Err* constructor with
// stub arguments. Stubs are deliberately recognizable so test
// failures point at the right constructor. ErrInternal's first
// arg is the dynamic code; treat it as opaque per the wire
// contract's wildcard-codes amendment.
func allConstructorSamples() []constructorSample {
	stub := fmt.Errorf("stub cause")
	return []constructorSample{
		{"ErrInvalidToken", ErrInvalidToken(stub)},
		{"ErrNoPackageRegistered", ErrNoPackageRegistered()},
		{"ErrPinnedVersionNotInstalled", ErrPinnedVersionNotInstalled("v0.0.0-stub")},
		{"ErrNoVersionAvailable", ErrNoVersionAvailable()},
		{"ErrTokenNotInitialized", ErrTokenNotInitialized()},
		{"ErrDescriptorInvalid", ErrDescriptorInvalid(stub)},
		{"ErrModuleJSONMissing", ErrModuleJSONMissing("module.json", stub)},
		{"ErrCampaignNotFound", ErrCampaignNotFound("stub-campaign-id")},
		{"ErrInternal", ErrInternal("stub_code", stub)},
	}
}

// TestWireResponseShape_MatchesResponseShapeDeclaration asserts
// that handler.go:respondError emits a JSON body with exactly
// the key set declared by ResponseShape (errors_catalog.go) for
// every constructor in errors.go. All fields must also be JSON
// strings — ResponseShape's placeholder values (`<code>`,
// `<4-clause>`, `<category>`) declare string semantics.
//
// A silent refactor of respondError that renames `category` to
// `chronicleCategory`, drops a field, or adds a discriminator
// (e.g. `requestID`) fails this test with the offending
// constructor's name.
func TestWireResponseShape_MatchesResponseShapeDeclaration(t *testing.T) {
	expectedKeys := sortedKeys(ResponseShape)

	h := &Handler{}
	for _, sample := range allConstructorSamples() {
		t.Run(sample.Name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := h.respondError(c, sample.Err); err != nil {
				t.Fatalf("respondError returned error: %v", err)
			}

			// Unmarshal into map[string]string — ResponseShape
			// declares every field as a string. A non-string
			// value would fail this unmarshal with the offending
			// key surfaced in the body literal.
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body did not decode as map[string]string: %v\n"+
					"body: %s\n"+
					"ResponseShape declares all fields as strings; "+
					"a non-string value in respondError breaks that contract.",
					err, rec.Body.String())
			}

			gotKeys := sortedKeys(body)
			if !reflect.DeepEqual(gotKeys, expectedKeys) {
				t.Errorf("response-body key set mismatch for %s:\n"+
					"  got:      %v\n"+
					"  expected: %v\n"+
					"\n"+
					"handler.go:respondError must emit exactly the keys declared "+
					"in ResponseShape (errors_catalog.go). If the shape change is "+
					"intentional, update ResponseShape AND amend "+
					"decisions/2026-05-17-error-catalog-wire-contract.md — "+
					"this is a cross-repo wire contract.",
					sample.Name, gotKeys, expectedKeys)
			}
		})
	}
}

// TestWireResponseShape_AllConstructorsCovered guards
// allConstructorSamples against drift. If a new Err* constructor
// is added to errors.go without a matching entry here, this test
// fails with the missing name and the file/line to fix.
func TestWireResponseShape_AllConstructorsCovered(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse constructors: %v", err)
	}

	sampleNames := map[string]bool{}
	for _, s := range allConstructorSamples() {
		sampleNames[s.Name] = true
	}
	constructorNames := map[string]bool{}
	for _, c := range constructors {
		constructorNames[c.Constructor] = true
	}

	for name := range constructorNames {
		if !sampleNames[name] {
			t.Errorf("Constructor %s exists in errors.go but is missing "+
				"from allConstructorSamples in wire_integrity_test.go.\n"+
				"Add an entry calling %s with stub arguments so the "+
				"response-shape test covers it.",
				name, name)
		}
	}
	for name := range sampleNames {
		if !constructorNames[name] {
			t.Errorf("allConstructorSamples references %s but no "+
				"matching Err* constructor exists in errors.go.\n"+
				"Either remove the sample (if the constructor was "+
				"deleted) or fix the name.",
				name)
		}
	}
}

// --- Item 2: category enum reconciliation ---

// parseErrCategoryConstants walks errors.go via go/ast and
// returns every package-level constant of declared type
// ErrCategory as a map of Go identifier → wire string.
//
// Mirrors ParseConstructors's AST approach in errors_catalog.go
// but limited to the const declarations rather than the func
// declarations. Lives here (not in errors_catalog.go) to keep
// C-WIRE-INTEGRITY test-only.
func parseErrCategoryConstants(t *testing.T) map[string]string {
	t.Helper()
	src := loadErrorsSource(t)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "errors.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse errors.go: %v", err)
	}

	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// Only ErrCategory-typed constants. Untyped or
			// otherwise-typed consts are ignored.
			typeIdent, ok := vs.Type.(*ast.Ident)
			if !ok || typeIdent.Name != "ErrCategory" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					t.Fatalf("ErrCategory constant %s has no explicit value "+
						"(iota / implicit-carry not supported by this parser)",
						name.Name)
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("ErrCategory constant %s value is not a string literal",
						name.Name)
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote ErrCategory constant %s: %v", name.Name, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// TestCategoryEnum_ConstantsAlignWithStringMap asserts every
// ErrCategory* constant in errors.go has a matching entry in
// categoryGoToString (errors_catalog.go) with the same wire
// string, and vice versa. A new constant without a map entry
// would crash ParseConstructors at PR time with "unknown
// category"; this test surfaces the gap earlier with an
// actionable message naming both sites.
func TestCategoryEnum_ConstantsAlignWithStringMap(t *testing.T) {
	constants := parseErrCategoryConstants(t)

	for name, wireValue := range constants {
		mapped, ok := categoryGoToString[name]
		if !ok {
			t.Errorf("ErrCategory constant %s (value %q) is declared in "+
				"errors.go but missing from categoryGoToString in "+
				"errors_catalog.go.\n"+
				"Add this entry:\n"+
				"  %q: %q,\n"+
				"The AST parser uses categoryGoToString to translate Go "+
				"identifiers to wire strings; without it, every constructor "+
				"using %s will fail to parse.",
				name, wireValue, name, wireValue, name)
			continue
		}
		if mapped != wireValue {
			t.Errorf("ErrCategory constant %s has wire value %q in "+
				"errors.go but maps to %q in categoryGoToString.\n"+
				"Reconcile: pick one wire value and update the other site.",
				name, wireValue, mapped)
		}
	}
	for name := range categoryGoToString {
		if _, ok := constants[name]; !ok {
			t.Errorf("categoryGoToString in errors_catalog.go has an entry "+
				"for %s but no ErrCategory constant by that name exists in "+
				"errors.go.\n"+
				"Either remove the categoryGoToString entry (if the "+
				"constant was deleted) or add the missing const declaration.",
				name)
		}
	}
}

// TestCategoryEnum_StringMapAlignsWithCategoriesSlice asserts the
// Categories slice — embedded verbatim into error-catalog.json's
// top-level "categories" field by BuildJSONArtifact — matches
// the set of wire strings in categoryGoToString. A category in
// the map but missing from the slice produces a JSON artifact
// where codes[].category may reference a value absent from the
// top-level enum: internally inconsistent, and the existing
// drift guard does not catch it.
func TestCategoryEnum_StringMapAlignsWithCategoriesSlice(t *testing.T) {
	inMap := map[string]bool{}
	for _, v := range categoryGoToString {
		inMap[v] = true
	}
	inSlice := map[string]bool{}
	for _, v := range Categories {
		inSlice[v] = true
	}

	for v := range inMap {
		if !inSlice[v] {
			t.Errorf("Category %q is in categoryGoToString but missing "+
				"from the Categories slice (both in errors_catalog.go).\n"+
				"Add %q to Categories. The slice is embedded into "+
				"error-catalog.json's top-level \"categories\" field; "+
				"a missing value produces an internally-inconsistent "+
				"artifact.",
				v, v)
		}
	}
	for v := range inSlice {
		if !inMap[v] {
			t.Errorf("Category %q is in the Categories slice but has no "+
				"mapping in categoryGoToString (errors_catalog.go).\n"+
				"Either remove %q from Categories or add the matching "+
				"ErrCategory constant + categoryGoToString entry.",
				v, v)
		}
	}
}

// TestCategoryEnum_HTTPStatusMapCoversAllCategories asserts every
// category in Categories has a defined HTTP status in
// categoryHTTPStatus, and that every status in categoryHTTPStatus
// refers to a known category. Missing entries silently produce
// httpStatus: 0 in the JSON artifact (map zero-value) and
// respondError defaults to 500 — both failure modes are quiet
// without a focused test.
func TestCategoryEnum_HTTPStatusMapCoversAllCategories(t *testing.T) {
	inHTTP := map[string]int{}
	for k, v := range categoryHTTPStatus {
		inHTTP[k] = v
	}
	inSlice := map[string]bool{}
	for _, v := range Categories {
		inSlice[v] = true
	}

	for _, cat := range Categories {
		status, ok := inHTTP[cat]
		if !ok {
			t.Errorf("Category %q is in the Categories slice but missing "+
				"from categoryHTTPStatus (errors_catalog.go).\n"+
				"Add a status mapping so BuildJSONArtifact emits the "+
				"correct httpStatus for codes in this category.",
				cat)
			continue
		}
		if status < 100 || status > 599 {
			t.Errorf("Category %q has HTTP status %d in "+
				"categoryHTTPStatus — outside the valid 100-599 range.\n"+
				"Pick a status that matches the category semantic.",
				cat, status)
		}
	}
	for cat := range inHTTP {
		if !inSlice[cat] {
			t.Errorf("categoryHTTPStatus has an entry for %q but no "+
				"matching value in the Categories slice.\n"+
				"Either remove the categoryHTTPStatus entry or add %q "+
				"to Categories.",
				cat, cat)
		}
	}
}

// --- helpers ---

// sortedKeys returns the keys of any map[string]V in sorted
// order. Used so key-set comparisons produce deterministic
// failure messages.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
