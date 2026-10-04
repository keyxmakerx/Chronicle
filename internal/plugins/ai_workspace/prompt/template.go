// template.go holds the prompt template — the wire format the
// "Copy AI Prompt" button emits.
//
// `text/template` (not `html/template`) — output is markdown, not
// HTML, so no template-side escaping is wanted. The `join` funcmap
// covers the Subcategories list inline-render.

package prompt

import (
	"strings"
	"text/template"
)

// templateData is the type promptTemplate renders against. Field
// names match the template's `{{ .Field }}` references exactly;
// reordering or renaming requires a template-source change.
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
	SampleEntities  []sampleEntityView // not yet populated; see builder.go
	TagsVocabulary  []tagView          // not yet populated; see builder.go

	// Content section.
	ContentMode     string
	ExportedContent string

	// Custom instruction (operator's textarea contents).
	OperatorInstruction string

	// RecordDocs is the "other things" section: one entry per record kind.
	RecordDocs string
}

// entityTypeView is the slim shape the template iterates.
type entityTypeView struct {
	Name           string
	Slug           string
	PresetCategory string
}

// categoryInUseView is the "Categories currently in use" row.
type categoryInUseView struct {
	TypeName      string
	Subcategories []string
	Count         int
}

// sampleEntityView is reserved — the picker doesn't expose the toggle yet.
type sampleEntityView struct {
	TypeName      string
	MarkdownBlock string
}

// tagView is reserved — same reason.
type tagView struct {
	Name   string
	DmOnly bool
}

// promptTemplate is the operator-visible wire format for the "Copy AI
// Prompt" button; changing it changes what operators paste into AI
// tools.
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

{{ if .RecordDocs }}
## Other things you can add, change or remove

Besides pages, a block can change other parts of the campaign. Give it
` + "`" + `kind:` + "`" + ` (below) and ` + "`" + `action:` + "`" + ` (` + "`" + `create` + "`" + `, ` + "`" + `update` + "`" + ` or ` + "`" + `delete` + "`" + `), matched by
` + "`" + `name` + "`" + `. I review every change, and confirm each removal myself, before
anything happens. For a page, ` + "`" + `action: update` + "`" + ` and ` + "`" + `action: delete` + "`" + ` work the
same way.

{{ .RecordDocs }}
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

Please output your response as one or more blocks in the format above.
Use front-matter for every block. Do not include any text outside the
blocks (no preamble, no commentary between blocks).
`

// tmpl is the parsed template, ready for Execute. Parsed once at
// import time; thread-safe.
var tmpl = template.Must(template.New("prompt").
	Funcs(template.FuncMap{
		"join": strings.Join,
	}).
	Parse(promptTemplate))
