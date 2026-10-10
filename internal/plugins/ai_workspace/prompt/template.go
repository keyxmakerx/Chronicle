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
	// Capabilities is the opening "what you can do" list.
	Capabilities string
	// Lookups lists what a lookup block can ask for; "" keeps the older
	// NEED:-only wording.
	Lookups string
	// PageIndex is the compact list of page names per type.
	PageIndex string
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
{{ if .Capabilities }}
## What you can do

Each block you write adds, changes or removes one thing in my campaign. I
review every change before anything happens, and I confirm each removal
myself. A block can be:

- a page (a character, place, item and so on)
{{ .Capabilities }}

## If you need more

Don't guess a format, a name or what is already there.{{ if .Lookups }} Ask Chronicle
instead: reply with only lookup blocks, and I will paste back the answer. A
lookup only reads; it never changes anything. For example:

` + "```" + `
---
kind: lookup
what: events
from: <month> 1 <year>
to: <month> 30 <year>
---
---
kind: lookup
what: players
---
` + "```" + `

A lookup can ask for:

{{ .Lookups }}
If you need the exact format for one of the kinds above{{ if .RecordDocs }} beyond what is below{{ end }},
reply with a short list that starts with ` + "`" + `NEED:` + "`" + `, for example
` + "`" + `NEED: format for kind: table` + "`" + `.{{ else }} If you need the
exact format for one of these{{ if .RecordDocs }} beyond what is below{{ end }}, or need to know what already exists
(a table's entries, a map's pins, the date of an event), reply with only a
short list that starts with ` + "`" + `NEED:` + "`" + `, for example
` + "`" + `NEED: format for kind: table; the pins on the Grimvale map` + "`" + `. I will paste it
back, and then you answer in full.{{ end }}
{{ end }}
{{ if .PageIndex }}
## What already exists

The pages in my campaign, by type. Ask a ` + "`" + `what: page` + "`" + ` lookup to read one.

{{ .PageIndex }}
{{ end }}
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

To put a page under another page, add ` + "`" + `parent: Name of the parent page` + "`" + `. The parent
is an existing page (an index line like "Name (in Parent)" means that page already sits
under Parent) or a page earlier in this same paste, so list parents first. With
` + "`" + `action: update` + "`" + `, ` + "`" + `parent:` + "`" + ` moves the page, ` + "`" + `parent: none` + "`" + ` lifts it to the top
level, and leaving the line out keeps it where it is. A page cannot be its own parent.
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
blocks (no preamble, no commentary between blocks){{ if .Capabilities }}, unless you are asking
for more with {{ if .Lookups }}lookup blocks or {{ end }}` + "`" + `NEED:` + "`" + `{{ end }}.
`

// tmpl is the parsed template, ready for Execute. Parsed once at
// import time; thread-safe.
var tmpl = template.Must(template.New("prompt").
	Funcs(template.FuncMap{
		"join": strings.Join,
	}).
	Parse(promptTemplate))
