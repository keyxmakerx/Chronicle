// C-FMC-DRIFT-GUARD: drift tests pinning errors.go ↔ .ai.md ↔
// error-catalog.json alignment.
//
// Four checks:
//
//   1. TestErrorCatalog_NoDriftFromConstructors — every Err*
//      constructor in errors.go has a row in .ai.md's catalog.
//   2. TestErrorCatalog_NoOrphanedRows — every row in .ai.md's
//      catalog has a backing constructor in errors.go.
//   3. TestErrorCatalog_CategoriesMatchConstructorBodies — the
//      Category column in .ai.md matches what the constructor's
//      &Error{...} body actually sets.
//   4. TestErrorCatalog_JSONArtifactMatchesSource — the committed
//      error-catalog.json equals what BuildJSONArtifact produces
//      from the live errors.go.
//
// If any test fails, the failure message tells the operator the
// exact file to fix + the regeneration command. The drift guard
// IS the verification — reviewers don't need to remember to
// check three files manually.
//
// Adding a new error code workflow: see .ai.md "Adding a new
// error code" runbook.
package foundry_vtt

import (
	"fmt"
	"os"
	"testing"
)

// loadErrorsSource reads errors.go from the package directory.
// The test runs from the package dir so the relative path works.
func loadErrorsSource(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("errors.go")
	if err != nil {
		t.Fatalf("read errors.go: %v", err)
	}
	return b
}

// loadAIMarkdown reads .ai.md from the package directory.
func loadAIMarkdown(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(".ai.md")
	if err != nil {
		t.Fatalf("read .ai.md: %v", err)
	}
	return string(b)
}

// TestErrorCatalog_NoDriftFromConstructors asserts every Err*
// constructor parsed from errors.go has a matching row in .ai.md's
// catalog table. Missing rows fail with the exact line to add.
func TestErrorCatalog_NoDriftFromConstructors(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse constructors: %v", err)
	}
	rows, err := ParseCatalogMarkdown(loadAIMarkdown(t))
	if err != nil {
		t.Fatalf("parse .ai.md catalog: %v", err)
	}

	rowByName := map[string]CatalogEntry{}
	for _, r := range rows {
		rowByName[r.Constructor] = r
	}

	for _, c := range constructors {
		if _, ok := rowByName[c.Constructor]; !ok {
			suggestedRow := fmt.Sprintf("| `%s` | %s | %d | <one-line summary> |",
				c.Constructor, c.Category, c.HTTPStatus)
			t.Errorf("Constructor %s (in errors.go) has no row in .ai.md catalog.\n"+
				"Add this row between the <!-- foundry-vtt-error-catalog-start --> and end markers:\n  %s",
				c.Constructor, suggestedRow)
		}
	}
}

// TestErrorCatalog_NoOrphanedRows asserts every row in .ai.md's
// catalog has a backing constructor in errors.go. Orphan rows fail
// with the exact action: remove the row OR add the constructor.
func TestErrorCatalog_NoOrphanedRows(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse constructors: %v", err)
	}
	rows, err := ParseCatalogMarkdown(loadAIMarkdown(t))
	if err != nil {
		t.Fatalf("parse .ai.md catalog: %v", err)
	}

	consByName := map[string]ConstructorEntry{}
	for _, c := range constructors {
		consByName[c.Constructor] = c
	}

	for _, r := range rows {
		if _, ok := consByName[r.Constructor]; !ok {
			t.Errorf("Catalog row for `%s` has no constructor in errors.go.\n"+
				"Either remove the row from .ai.md (if the constructor was deleted) "+
				"or implement func %s() *Error in errors.go.",
				r.Constructor, r.Constructor)
		}
	}
}

// TestErrorCatalog_CategoriesMatchConstructorBodies asserts the
// Category column in .ai.md matches what the constructor's
// &Error{...} body actually sets. A drift here means someone
// changed errors.go without updating .ai.md (or vice versa).
func TestErrorCatalog_CategoriesMatchConstructorBodies(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse constructors: %v", err)
	}
	rows, err := ParseCatalogMarkdown(loadAIMarkdown(t))
	if err != nil {
		t.Fatalf("parse .ai.md catalog: %v", err)
	}

	rowByName := map[string]CatalogEntry{}
	for _, r := range rows {
		rowByName[r.Constructor] = r
	}

	for _, c := range constructors {
		row, ok := rowByName[c.Constructor]
		if !ok {
			// Already reported by TestErrorCatalog_NoDriftFromConstructors.
			continue
		}
		if row.Category != c.Category {
			t.Errorf("Category mismatch for %s: errors.go body says %q, .ai.md catalog says %q.\n"+
				"Update one to match the other.",
				c.Constructor, c.Category, row.Category)
		}
		if row.HTTPStatus != c.HTTPStatus {
			t.Errorf("HTTP Status mismatch for %s: errors.go body implies %d (via category %q), .ai.md catalog says %d.\n"+
				"The category-to-status mapping is hardcoded in errors_catalog.go's categoryHTTPStatus; if you intended to change it, update both files together.",
				c.Constructor, c.HTTPStatus, c.Category, row.HTTPStatus)
		}
	}
}

// TestErrorCatalog_JSONArtifactMatchesSource asserts the committed
// error-catalog.json equals what BuildJSONArtifact produces from
// the live errors.go. A mismatch means someone changed errors.go
// without running the regeneration command.
func TestErrorCatalog_JSONArtifactMatchesSource(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse constructors: %v", err)
	}
	freshJSON, err := BuildJSONArtifact(constructors)
	if err != nil {
		t.Fatalf("build JSON: %v", err)
	}

	committedJSON, err := os.ReadFile("error-catalog.json")
	if err != nil {
		t.Fatalf("read error-catalog.json: %v (regenerate via `make foundry-error-catalog`)", err)
	}

	if string(freshJSON) != string(committedJSON) {
		t.Errorf("error-catalog.json is out of date.\n"+
			"Run `make foundry-error-catalog` from the repo root and commit the regenerated file.\n"+
			"\nDiff hint — committed length: %d bytes, fresh length: %d bytes.",
			len(committedJSON), len(freshJSON))
	}
}

// TestParseConstructors_RecognizesWildcardErrInternal pins the
// special-case behavior: ErrInternal's Code is a parameter, not a
// literal. The parser must recognize this and emit Wildcard=true
// rather than failing with "Code value should be a string literal".
//
// This is a focused unit test on the parser itself; the other
// tests are integration-level.
func TestParseConstructors_RecognizesWildcardErrInternal(t *testing.T) {
	constructors, err := ParseConstructors(loadErrorsSource(t), "errors.go")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var internal *ConstructorEntry
	for i := range constructors {
		if constructors[i].Constructor == "ErrInternal" {
			internal = &constructors[i]
			break
		}
	}
	if internal == nil {
		t.Fatal("ErrInternal not found among parsed constructors")
	}
	if !internal.Wildcard {
		t.Error("ErrInternal should be marked Wildcard=true (its Code is a function parameter)")
	}
	if internal.Code != "<dynamic>" {
		t.Errorf("ErrInternal.Code should be '<dynamic>' for wildcard constructors, got %q", internal.Code)
	}
}
