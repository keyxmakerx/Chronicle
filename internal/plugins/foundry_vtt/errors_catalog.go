package foundry_vtt

// C-FMC-DRIFT-GUARD: error-catalog parser + JSON artifact generator.
//
// This file is the load-bearing piece of the drift guard: it walks
// errors.go via go/ast, extracts every constructor's Code +
// Category, and assembles them into a canonical structure. The
// test (errors_test.go) asserts this structure aligns with both
// .ai.md's catalog table AND the committed error-catalog.json
// artifact. The cmd binary (cmd/foundry-error-catalog/main.go)
// regenerates the JSON when the operator runs `make
// foundry-error-catalog`.
//
// Three coupled sources must stay in sync; this file makes the
// coupling mechanical:
//
//   1. errors.go — the constructors (source of truth)
//   2. .ai.md — the human-authored catalog table (cross-references
//      + ships in operator-facing docs)
//   3. error-catalog.json — machine-readable artifact (consumed by
//      Foundry-side docs + potentially Foundry-side CI in a future
//      FM-DRIFT-GUARD)
//
// If any two drift apart, the test fails loudly with an actionable
// message telling the operator which file to update.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrorCatalog is the canonical JSON shape. The schemaVersion is
// for the consumer-side compatibility check — bump only on a
// breaking layout change.
type ErrorCatalog struct {
	SchemaVersion int                `json:"schemaVersion"`
	GeneratedFrom string             `json:"generatedFrom"`
	Categories    []string           `json:"categories"`
	ResponseShape map[string]string  `json:"responseShape"`
	Codes         []ConstructorEntry `json:"codes"`
}

// ConstructorEntry is one row in the catalog. For named constructors
// (most), Code is the literal string from the &Error{...} body.
// For ErrInternal (and any future wildcard constructor that takes
// Code as a parameter), Wildcard=true and Code is the placeholder
// "<dynamic>".
type ConstructorEntry struct {
	Constructor string `json:"constructor"`
	Code        string `json:"code"`
	Category    string `json:"category"`
	HTTPStatus  int    `json:"httpStatus"`
	Wildcard    bool   `json:"wildcard,omitempty"`
}

// CatalogEntry is one row parsed from .ai.md's markdown table.
// Used only for the drift test; not part of the JSON artifact.
type CatalogEntry struct {
	Constructor string
	Category    string
	HTTPStatus  int
	Message     string
}

// errorCatalogSchemaVersion is the JSON output's schemaVersion.
// Bump when the JSON shape changes in a backward-incompatible way.
const errorCatalogSchemaVersion = 1

// errorCatalogGeneratedFromPath identifies the source file in the
// JSON output. Hardcoded — if errors.go ever moves, update this
// and the cmd's read path together.
const errorCatalogGeneratedFromPath = "internal/plugins/foundry_vtt/errors.go"

// categoryGoToString maps the Go-side ErrCategory* identifiers to
// the lowercase/snake_case strings that appear in the .ai.md table
// + the JSON artifact + the on-wire `category` field of error
// responses. Hardcoded by design: if errors.go gains a new
// category constant, this map must be updated in lock-step (the
// parser fails loudly otherwise).
var categoryGoToString = map[string]string{
	"ErrCategoryAuth":       "auth",
	"ErrCategoryConfig":     "config",
	"ErrCategoryNotFound":   "not_found",
	"ErrCategoryValidation": "validation",
	"ErrCategoryInternal":   "internal",
}

// categoryHTTPStatus mirrors (*Error).HTTPStatus(). Duplicated
// rather than imported because the parser runs against source-text,
// not the runtime types — keeping the mapping in one map of strings
// makes the drift test self-contained.
var categoryHTTPStatus = map[string]int{
	"auth":       403,
	"config":     503,
	"not_found":  404,
	"validation": 422,
	"internal":   500,
}

// Categories lists the catalog's enum values in canonical order.
// Same order as the (*Error).HTTPStatus switch in errors.go for
// readability when diffing.
var Categories = []string{"auth", "config", "not_found", "validation", "internal"}

// ResponseShape documents the on-wire JSON shape callers will see.
// Embedded in the JSON artifact so consumers (Foundry-side docs)
// have a single source of truth for both the codes AND the shape.
var ResponseShape = map[string]string{
	"error":    "<code>",
	"message":  "<4-clause>",
	"category": "<category>",
}

// ParseConstructors walks the given Go source for every `func Err*`
// that returns `*Error`, extracts its Code + Category from the
// `&Error{...}` composite literal in the body, and returns the
// catalog entries in sorted-by-constructor order. The filename arg
// is used only for parser-error messages.
//
// Wildcard constructors (those whose Code is a function parameter
// rather than a string literal — currently only ErrInternal) are
// recognized: their Code becomes "<dynamic>" + Wildcard=true.
func ParseConstructors(sourceCode []byte, filename string) ([]ConstructorEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, sourceCode, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	var out []ConstructorEntry
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !strings.HasPrefix(fn.Name.Name, "Err") {
			continue
		}
		if fn.Recv != nil {
			// Skip methods (Err* methods on *Error like Error(),
			// Unwrap()). Constructors are package-level funcs.
			continue
		}
		if !returnsErrorPointer(fn.Type.Results) {
			continue
		}
		entry, err := extractConstructorEntry(fn)
		if err != nil {
			return nil, fmt.Errorf("constructor %s: %w", fn.Name.Name, err)
		}
		entry.HTTPStatus = categoryHTTPStatus[entry.Category]
		out = append(out, entry)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Constructor < out[j].Constructor })
	return out, nil
}

// returnsErrorPointer reports whether the function's return list is
// a single *Error.
func returnsErrorPointer(results *ast.FieldList) bool {
	if results == nil || len(results.List) != 1 {
		return false
	}
	star, ok := results.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "Error"
}

// extractConstructorEntry finds the first `return &Error{...}` in
// the function body and pulls Code + Category from the composite
// literal's fields. Returns an error if the function doesn't match
// the expected shape (would indicate a constructor was added
// without following the established pattern).
func extractConstructorEntry(fn *ast.FuncDecl) (ConstructorEntry, error) {
	entry := ConstructorEntry{Constructor: fn.Name.Name}
	if fn.Body == nil {
		return entry, fmt.Errorf("function has no body")
	}

	var composite *ast.CompositeLit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if composite != nil {
			return false // already found, stop walking
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		unary, ok := ret.Results[0].(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			return true
		}
		cl, ok := unary.X.(*ast.CompositeLit)
		if !ok {
			return true
		}
		ident, ok := cl.Type.(*ast.Ident)
		if !ok || ident.Name != "Error" {
			return true
		}
		composite = cl
		return false
	})

	if composite == nil {
		return entry, fmt.Errorf("no `return &Error{...}` found")
	}

	for _, elt := range composite.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Category":
			catIdent, ok := kv.Value.(*ast.Ident)
			if !ok {
				return entry, fmt.Errorf("category value should be an identifier like ErrCategoryAuth, got %T", kv.Value)
			}
			catStr, known := categoryGoToString[catIdent.Name]
			if !known {
				return entry, fmt.Errorf(
					"unknown category %q — add it to categoryGoToString in errors_catalog.go",
					catIdent.Name)
			}
			entry.Category = catStr
		case "Code":
			switch v := kv.Value.(type) {
			case *ast.BasicLit:
				if v.Kind != token.STRING {
					return entry, fmt.Errorf("code literal should be a string, got %v", v.Kind)
				}
				// Strip surrounding quotes from the raw token value.
				unquoted, err := strconv.Unquote(v.Value)
				if err != nil {
					return entry, fmt.Errorf("unquote Code: %w", err)
				}
				entry.Code = unquoted
			case *ast.Ident:
				// Wildcard constructor — Code is a function parameter.
				entry.Code = "<dynamic>"
				entry.Wildcard = true
			default:
				return entry, fmt.Errorf("code value should be a string literal or identifier, got %T", kv.Value)
			}
		}
	}

	if entry.Category == "" {
		return entry, fmt.Errorf("no Category field found in &Error{...}")
	}
	if entry.Code == "" {
		return entry, fmt.Errorf("no Code field found in &Error{...}")
	}
	return entry, nil
}

// catalogTableRegex matches one markdown table row. The cells are
// captured (constructor, category, http status, message).
// Constructor has its backticks stripped at the caller.
var catalogTableRegex = regexp.MustCompile(`^\|\s*(.+?)\s*\|\s*(.+?)\s*\|\s*(.+?)\s*\|\s*(.+?)\s*\|\s*$`)

// catalogMarkerStart / catalogMarkerEnd delimit the catalog table in
// .ai.md. Tests + the regenerator both target this region; prose
// outside the markers is human-authored and the drift guard leaves
// it alone.
const (
	catalogMarkerStart = "<!-- foundry-vtt-error-catalog-start -->"
	catalogMarkerEnd   = "<!-- foundry-vtt-error-catalog-end -->"
)

// ParseCatalogMarkdown reads the .ai.md content and returns the
// rows found between catalogMarkerStart and catalogMarkerEnd.
// Header + separator rows (containing `---`) are skipped. The
// constructor cell is expected to be backtick-wrapped; ParseCatalog
// strips the backticks.
func ParseCatalogMarkdown(content string) ([]CatalogEntry, error) {
	startIdx := strings.Index(content, catalogMarkerStart)
	if startIdx < 0 {
		return nil, fmt.Errorf("%s marker not found in .ai.md", catalogMarkerStart)
	}
	endIdx := strings.Index(content, catalogMarkerEnd)
	if endIdx < 0 {
		return nil, fmt.Errorf("%s marker not found in .ai.md", catalogMarkerEnd)
	}
	if endIdx < startIdx {
		return nil, fmt.Errorf("end marker appears before start marker")
	}

	region := content[startIdx+len(catalogMarkerStart) : endIdx]

	var out []CatalogEntry
	for _, line := range strings.Split(region, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "|") {
			continue
		}
		// Skip the separator row (|---|---|...|).
		if strings.Contains(line, "---") {
			continue
		}
		m := catalogTableRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		constructor := strings.Trim(m[1], "`")
		// Skip the header row.
		if constructor == "Constructor" {
			continue
		}
		category := strings.TrimSpace(m[2])
		httpStatusStr := strings.TrimSpace(m[3])
		httpStatus, err := strconv.Atoi(httpStatusStr)
		if err != nil {
			return nil, fmt.Errorf("row %q: HTTP Status %q is not an int", constructor, httpStatusStr)
		}
		out = append(out, CatalogEntry{
			Constructor: constructor,
			Category:    category,
			HTTPStatus:  httpStatus,
			Message:     strings.TrimSpace(m[4]),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Constructor < out[j].Constructor })
	return out, nil
}

// BuildJSONArtifact returns the canonical JSON byte slice for the
// given constructor list. Two-space indent + trailing newline so
// `git diff` shows readable hunks on changes.
func BuildJSONArtifact(constructors []ConstructorEntry) ([]byte, error) {
	catalog := ErrorCatalog{
		SchemaVersion: errorCatalogSchemaVersion,
		GeneratedFrom: errorCatalogGeneratedFromPath,
		Categories:    Categories,
		ResponseShape: ResponseShape,
		Codes:         constructors,
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	// SetEscapeHTML(false) — the message field never contains <>&,
	// but disable HTML escaping anyway so the JSON is byte-stable
	// across runs.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(catalog); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
