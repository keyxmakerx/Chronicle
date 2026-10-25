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
	EntityTypes []entityTypeView
	// AnyLayouts says some category has more than one page layout, so the
	// prompt explains `layout:`.
	AnyLayouts      bool
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
	// Today and WorldToday are the real-world and in-world dates.
	Today, WorldToday string

	// InstructionsOnly renders only the standing part (formats, lookups,
	// rules) for an AI to keep; InstructionsSaved renders only the request,
	// for an AI that already has them. At most one is set.
	InstructionsOnly  bool
	InstructionsSaved bool
}

// entityTypeView is the slim shape the template iterates.
type entityTypeView struct {
	Name           string
	Slug           string
	PresetCategory string
	// Fields are the category's sheet fields, one line each: the key the
	// AI writes under `fields:` and what value it takes.
	Fields []string
	// Layouts names the category's page layouts, main one first, when it
	// has more than one; each page wears one.
	Layouts []string
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
const promptTemplate = `{{ if .InstructionsSaved }}This is a request for my Chronicle campaign. Follow the Chronicle instructions
I gave you earlier (saved in this chat, project or custom instructions): the
block formats, the lookups and the rules for what you write.
{{ else }}You are helping me extend my TTRPG campaign in Chronicle, an Obsidian-style
worldbuilding tool. Generate new content that conforms to my world's existing
structure so I can paste your output back into Chronicle's AI Import flow
and it will be accepted with minimal review.
{{ if .InstructionsOnly }}
These are standing instructions: keep them for the whole chat or project. Each
time, I will send a short request (what I want, and sometimes what my campaign
has now); answer it in the formats below.
{{ end }}{{ end }}{{ if .Today }}
Today is {{ .Today }} in the real world{{ if .WorldToday }}, and {{ .WorldToday }} on my campaign's main calendar{{ end }}.
{{ end }}{{ if .Capabilities }}
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
ask a ` + "`" + `what: format` + "`" + ` lookup with ` + "`" + `block:` + "`" + ` set to the kind, for example
` + "`" + `block: calendar` + "`" + `.{{ else }} If you need the
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
- **{{ .Name }}** (slug: ` + "`" + `{{ .Slug }}` + "`" + `){{ if .PresetCategory }} — preset: {{ .PresetCategory }}{{ end }}{{ if .Layouts }} — layouts: {{ join .Layouts ", " }}{{ end }}
{{- if .Fields }}{{ range .Fields }}
  - {{ . }}{{ end }}{{ end }}
{{ end }}

A page's sheet is set with a ` + "`" + `fields:` + "`" + ` map in its front matter, using the field keys
listed under its type, for example ` + "`" + `fields: {level: 3, stamina: 30}` + "`" + `. Only those keys
work, and each value must fit its field. On ` + "`" + `action: update` + "`" + ` only the fields you name
change, and ` + "`" + `null` + "`" + ` clears one. A field that holds a list (a stat block's traits or
abilities, say) takes a YAML list. To build a monster, give its page the creature type
and every stat block field, then name that page in an encounter's creatures.
{{- if .AnyLayouts }}

One category can hold different kinds of page, so a type listed with layouts gives each
page one of them: its heroes might wear one and its NPCs another. Pick it with
` + "`" + `layout:` + "`" + ` and the layout's name, for example ` + "`" + `layout: Hero` + "`" + `. Without it a new page wears
the type's first layout (a player's character, the one the type starts claimed pages on).
The list of my pages says which layout each existing page wears.
{{- end }}
{{ end }}

{{ if and .IncludeCategoriesInUse .CategoriesInUse }}
## Categories currently in use

{{ range .CategoriesInUse }}
- **{{ .TypeName }}**: {{ join .Subcategories ", " }} ({{ .Count }} pages)
{{ end }}
{{ end }}

{{ if .IncludeFrontMatterExample }}
## Format your output as markdown with YAML front-matter

Each page is one block: YAML front-matter between two ` + "`" + `---` + "`" + ` lines, then the
page's text in markdown. The page's name comes from ` + "`" + `name:` + "`" + `, and Chronicle shows it as
the title, so don't repeat it as a heading. Every block starts with its own ` + "`" + `---` + "`" + `
line, so after a block with no text write ` + "`" + `---` + "`" + ` twice: once to close it, once to
open the next. For example:

` + "```" + `
---
name: Example Page Name
type: location
subcategory: city
visibility: private
tags: [trade-hub, coastal]
reason: The players asked where the caravan was headed.
---
The body of the page in markdown.
` + "```" + `

Valid ` + "`" + `type` + "`" + ` values are the slugs listed above. ` + "`" + `visibility` + "`" + ` is ` + "`" + `public` + "`" + ` (everyone
sees it) or ` + "`" + `private` + "`" + ` (hidden from players; ` + "`" + `dm_only` + "`" + ` means the same). Events, pins and map
sketches take the same words, and also ` + "`" + `everyone` + "`" + `; a note says who it is shared with in
` + "`" + `share:` + "`" + ` instead (` + "`" + `private` + "`" + `, ` + "`" + `gm` + "`" + ` or ` + "`" + `party` + "`" + `). ` + "`" + `tags` + "`" + ` is optional; use the campaign's
existing tag vocabulary where possible; a tag that doesn't exist yet is made. On
` + "`" + `action: update` + "`" + `, ` + "`" + `tags` + "`" + ` replaces the page's tags, ` + "`" + `tags: []` + "`" + ` clears them, and leaving
the line out keeps them. A page's GM-only tags always stay.

` + "`" + `reason` + "`" + ` is one sentence saying why you are asking for this change. I see it
beside the change when I review, and it is never saved.

To put a page under another page, add ` + "`" + `parent: Name of the parent page` + "`" + `. The parent
is an existing page (an index line like "Name (in Parent)" means that page already sits
under Parent) or a page earlier in this same paste, so list parents first. With
` + "`" + `action: update` + "`" + `, ` + "`" + `parent:` + "`" + ` moves the page, ` + "`" + `parent: none` + "`" + ` lifts it to the top
level, and leaving the line out keeps it where it is. A page cannot be its own parent.

To file a page in a sidebar folder of its category (a folder shows as "Name (in folder Gods)"
in the index), add ` + "`" + `folder: Gods` + "`" + `. Make the folder first with a ` + "`" + `kind: folder` + "`" + ` block, or
use one that already exists; ` + "`" + `folder: none` + "`" + ` takes the page out of its folder, and
leaving the line out keeps it where it is. A page sits under a parent page or in a
folder, not both.

A page can read differently in each era of the main calendar. To write one era's
version of a page, give a separate block the page's ` + "`" + `name:` + "`" + ` plus
` + "`" + `era: Name of the era` + "`" + ` (matched ignoring case). The body is that era's text; leave it
empty to keep the version's text as it is. ` + "`" + `known_as: Name in that era` + "`" + ` sets the
page's name in the era and only works together with ` + "`" + `era:` + "`" + `. The usual page is never
changed by an era block, and its ` + "`" + `type` + "`" + `, ` + "`" + `visibility` + "`" + `, ` + "`" + `parent` + "`" + ` and ` + "`" + `tags` + "`" + ` lines
are ignored there. The page must already exist or come earlier in this paste in a block
without ` + "`" + `era:` + "`" + `. ` + "`" + `action: delete` + "`" + ` with ` + "`" + `era:` + "`" + ` removes only that era's version.

To link to another page inside a body, write ` + "`" + `@[Page Name]` + "`" + ` (shown as "@Page Name", with
a hover card and click-to-preview, like an @mention typed in the editor) or
` + "`" + `@[Page Name|words to show]` + "`" + `. The name is an existing page or a page anywhere in this
same paste. A name that matches no page I can see is saved as plain text, so use
the exact page name. Do not link inside code, and do not use a markdown link for a
page.

To hide part of a body from players, put it between ` + "`" + `{gm{` + "`" + ` and ` + "`" + `}gm}` + "`" + `, for example
` + "`" + `The mayor is {gm{secretly a cultist.}gm} He greets everyone.` + "`" + `. It becomes a GM-only
passage that only I (the director) can see. A passage cannot cross a blank line, so
open and close the markers again in each paragraph, and an unclosed ` + "`" + `{gm{` + "`" + ` hides
only the rest of its paragraph. When you edit a page my export shows with these
markers, keep them around the same words; text I exported without them is not
GM-only, and a page update that leaves out one of its existing GM-only passages needs
my extra confirmation. Do not use the markers for anything else.
{{ end }}

{{ if .RecordDocs }}
## Other things you can add, change or remove

Besides pages, a block can change other parts of the campaign. Give it
` + "`" + `kind:` + "`" + ` (below) and ` + "`" + `action:` + "`" + ` (` + "`" + `create` + "`" + `, ` + "`" + `update` + "`" + ` or ` + "`" + `delete` + "`" + `), matched by
` + "`" + `name` + "`" + `. I review every change, and confirm each removal myself, before
anything happens. For a page, ` + "`" + `action: update` + "`" + ` and ` + "`" + `action: delete` + "`" + ` work the
same way. Any block can add ` + "`" + `reason:` + "`" + ` with one sentence saying why, above all a
removal.

{{ .RecordDocs }}
{{ end }}
{{ if or .IncludeFrontMatterExample .RecordDocs }}
## If your answer comes in parts

Keep your answer in one reply when you can. If it is too long and you split it
over several replies, start each one with a part block, and put what a later
part relies on (a page, an event, a calendar change) in an earlier part:

` + "```" + `
---
kind: part
name: Lastwell contract
part: 2
of: 3
needs: [1]
---
` + "```" + `

` + "`" + `name` + "`" + ` is the same on every part of one answer, ` + "`" + `part` + "`" + ` counts from 1, ` + "`" + `of` + "`" + ` is how
many parts there are, and ` + "`" + `needs` + "`" + ` lists the earlier parts this one relies on (leave
it out when it relies on none). A part can only need earlier parts. Chronicle keeps
each part I paste in a tray until every part is in, then puts them in order and
reviews the whole answer at once, so the order I paste them in doesn't matter. Never
repeat a block from another part. A fix you send after a part failed is a new part
with the next number, and its ` + "`" + `of` + "`" + ` counts it too.
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
{{ if .InstructionsOnly }}
## How to answer my requests

Answer each request with one or more blocks in the format above. Use
front-matter for every block. Do not include any text outside the blocks (no
preamble, no commentary between blocks){{ if .Capabilities }}, unless you are asking for more with
{{ if .Lookups }}lookup blocks or {{ end }}` + "`" + `NEED:` + "`" + `{{ end }}.
{{ else }}
## What I want you to generate

{{ .OperatorInstruction }}

Please output your response as one or more blocks in the format {{ if .InstructionsSaved }}from my Chronicle
instructions{{ else }}above{{ end }}. Use front-matter for every block. Do not include any text outside the
blocks (no preamble, no commentary between blocks){{ if or .Capabilities .InstructionsSaved }}, unless you are asking
for more with {{ if .Lookups }}lookup blocks or {{ end }}` + "`" + `NEED:` + "`" + `{{ end }}.
{{ end }}`

// tmpl is the parsed template, ready for Execute. Parsed once at
// import time; thread-safe.
var tmpl = template.Must(template.New("prompt").
	Funcs(template.FuncMap{
		"join": strings.Join,
	}).
	Parse(promptTemplate))
