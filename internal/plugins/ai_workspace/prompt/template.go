// template.go holds the verbatim §3.5 prompt template per
// reports/chronicle/2026-05-26-c-ai-workspace-scoping.md. The
// template is locked operator-side; this is the wire format the
// "Copy AI Prompt" button emits.
//
// `text/template` (not `html/template`) — output is markdown, not
// HTML; we never want template-side escaping. The `join` funcmap
// covers the Subcategories list inline-render.

package prompt

import (
	"strings"
	"text/template"
)

// templateData is the type the §3.5 template renders against.
// Field names match the template's `{{ .Field }}` references
// exactly; reordering or renaming requires a template-source change.
type templateData struct {
	// Schema picker conditionals.
	IncludeEntityTypes        bool
	IncludeCategoriesInUse    bool
	IncludeFrontMatterExample bool
	IncludeSampleEntity       bool
	IncludeTagsVocabulary     bool

	// Section data — populated by builder.go when the matching
	// conditional is true.
	EntityTypes     []entityTypeView
	CategoriesInUse []categoryInUseView
	SampleEntities  []sampleEntityView // V2; never populated by V1
	TagsVocabulary  []tagView          // V2; never populated by V1

	// Content section.
	ContentMode     string
	ExportedContent string

	// Custom instruction (operator's textarea contents).
	OperatorInstruction string
}

// entityTypeView is the slim shape the template iterates.
type entityTypeView struct {
	Name           string
	Slug           string
	PresetCategory string
}

// categoryInUseView is the §3.5 "Categories currently in use" row.
type categoryInUseView struct {
	TypeName      string
	Subcategories []string
	Count         int
}

// sampleEntityView is reserved for V2 — the picker doesn't expose
// the toggle yet.
type sampleEntityView struct {
	TypeName      string
	MarkdownBlock string
}

// tagView is reserved for V2 — same reason.
type tagView struct {
	Name   string
	DmOnly bool
}

// promptTemplate is the verbatim §3.5 template text. Changing this
// updates the scoping report's locked operator-visible wire format —
// must be paired with a Cordinator decision-doc amendment.
const promptTemplate = `You are helping me extend my TTRPG campaign in Chronicle, an Obsidian-style
worldbuilding tool. Generate new content that conforms to my world's existing
structure so I can paste your output back into Chronicle's AI Import flow
and it will be accepted with minimal review.

{{ if .IncludeEntityTypes }}
## My campaign's entity types

{{ range .EntityTypes }}
- **{{ .Name }}** (slug: ` + "`" + `{{ .Slug }}` + "`" + `){{ if .PresetCategory }} — preset: {{ .PresetCategory }}{{ end }}
{{ end }}
{{ end }}

{{ if .IncludeCategoriesInUse }}
## Categories currently in use

{{ range .CategoriesInUse }}
- **{{ .TypeName }}**: {{ join .Subcategories ", " }} ({{ .Count }} pages)
{{ end }}
{{ end }}

{{ if .IncludeFrontMatterExample }}
## Format your output as markdown with YAML front-matter

Each page is one section starting with a ` + "`" + `#` + "`" + ` heading. Include YAML
front-matter above each heading like this:

` + "```" + `
---
name: Example Page Name
type: location
subcategory: city
visibility: private
tags: [trade-hub, coastal]
---

# Example Page Name

The body of the page in markdown.
` + "```" + `

Valid ` + "`" + `type` + "`" + ` values are the slugs listed above. ` + "`" + `visibility` + "`" + ` must be one of
` + "`" + `private` + "`" + `, ` + "`" + `dm_only` + "`" + `, or ` + "`" + `public` + "`" + `. ` + "`" + `tags` + "`" + ` is optional; use the campaign's
existing tag vocabulary where possible.
{{ end }}

{{ if .IncludeSampleEntity }}
## Sample entity (one per type, for shape reference)

{{ range .SampleEntities }}
### {{ .TypeName }} sample

{{ .MarkdownBlock }}
{{ end }}
{{ end }}

{{ if ne .ContentMode "none" }}
## Existing world context

{{ .ExportedContent }}
{{ end }}

## What I want you to generate

{{ .OperatorInstruction }}

Please output your response as one or more entity blocks in the format above.
Use front-matter for every entity. Do not include any text outside the entity
blocks (no preamble, no commentary between entities).
`

// tmpl is the parsed template, ready for Execute. Parsed once at
// import time; thread-safe.
var tmpl = template.Must(template.New("prompt").
	Funcs(template.FuncMap{
		"join": strings.Join,
	}).
	Parse(promptTemplate))
