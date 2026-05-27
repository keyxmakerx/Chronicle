// Package importer parses AI-generated markdown into per-page
// ParsedPage structs, classifies each (full / defaults / conflict /
// new category / parse error), and prepares the review-screen data
// the operator inspects before committing.
//
// V1 Phase 4 ships parse + review only. The commit handler (which
// actually creates entities + categories) lands in Phase 5
// (C-AI-WORKSPACE-V1-D's sibling). This package's public surface is
// designed so Phase 5 consumes ParsedPage + ReviewRow without
// re-parsing.
//
// SEC-6-AMENDED is inherited at the markdown→HTML boundary (see
// markdown_html.go's MarkdownToHTML; pipes goldmark output through
// sanitize.HTML before any storage hand-off). The AST structural
// pin enforcing the funnel lands in Phase 5 alongside the committer.
//
// Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §1.4 (HTML→ProseMirror — recommendation A), §2.2 (review screen),
// §3.3 (front-matter schema), §3.8 (per-category create-once).
package importer

import "time"

// FrontMatter is the YAML preamble that AI tools emit between
// `---` fences above each page. Matches scoping §3.3 schema exactly;
// every field is optional — missing fields fall back to bulk
// defaults or the H1 / filename for Name.
//
// Unknown YAML keys are tolerated by yaml.v3's loose unmarshalling
// + surfaced as a warning chip on the review row (handled by the
// parser; does not block import).
type FrontMatter struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`        // entity type slug (e.g. "location")
	Subcategory string   `yaml:"subcategory"` // TypeLabel value
	Visibility  string   `yaml:"visibility"`  // "private" | "dm_only" | "public"
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"` // V2 candidate; tolerated in V1
}

// ParseStatus classifies how the parser fared on a single page.
// Drives the review row's right-most "Status" column.
type ParseStatus string

const (
	// StatusNew means the page parses, has a name, and no slug
	// conflict exists. Default-include.
	StatusNew ParseStatus = "new"
	// StatusConflict means a campaign entity with the same slug
	// already exists. Operator picks Skip / Rename / Overwrite per
	// row. Default-include with default-mode = Rename.
	StatusConflict ParseStatus = "conflict"
	// StatusNewCategory means the page parses BUT references an
	// entity-type slug that doesn't exist in the campaign. Operator
	// picks Create new or Map to existing. Default-include.
	StatusNewCategory ParseStatus = "new_category"
	// StatusParseError means the page failed to parse — bad YAML,
	// invalid enum value, missing required field with no default.
	// Default-EXCLUDE; the operator must fix the source markdown
	// or skip the row.
	StatusParseError ParseStatus = "parse_error"
)

// ParsedPage is one page detected in the multi-page input. The
// parser produces a slice of these; the review screen renders one
// row per ParsedPage; Phase 5's committer iterates the operator-
// confirmed selection to create entities.
type ParsedPage struct {
	// SourceIndex is the page's position in the input (0-based),
	// used for stable React-style keys on the review row.
	SourceIndex int

	// Name is the resolved page name. Resolution priority:
	//   1. FrontMatter.Name (if non-empty)
	//   2. First H1 in the body markdown
	//   3. Filename without extension (when uploaded as a file)
	//   4. ""  →  marks the row as a parse error
	Name string

	// FrontMatter is the parsed YAML (may be zero-value if the page
	// had no front-matter block; bulk defaults fill the gaps at
	// review time).
	FrontMatter FrontMatter

	// HasFrontMatter is true when a `---...---` block was present
	// (even if it parsed to an empty struct). Distinguishes "I
	// didn't write FM" from "I wrote FM with an empty body".
	HasFrontMatter bool

	// Body is the markdown body (everything after the front-matter
	// fence, or the whole input if no FM). Phase 5 converts this
	// to HTML via MarkdownToHTML + then to ProseMirror JSON.
	Body string

	// Status classifies the page; see ParseStatus.
	Status ParseStatus

	// Warnings is a slice of human-readable strings shown in the
	// review row's expandable detail. Examples:
	//   - "Unknown front-matter key 'description' (will be ignored)"
	//   - "No H1 heading found; using filename as name"
	// Errors that block import live in ParseError, not Warnings.
	Warnings []string

	// ParseError is the load-bearing failure reason when
	// Status==StatusParseError. Human-readable; rendered verbatim
	// in the review row. Empty for non-error statuses.
	ParseError string

	// ParsedAt timestamps the parse for downstream audit /
	// debugging. Not surfaced to the operator UI.
	ParsedAt time.Time
}

// HasName returns true when the page resolved to a non-empty name.
// Used by the review-screen render to decide whether to show the
// name-input vs an error placeholder.
func (p ParsedPage) HasName() bool {
	return p.Name != ""
}
