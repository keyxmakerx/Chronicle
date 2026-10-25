// rollers.go turns `{roll{Table name|3}}`, alone on its line, into the
// rolling-table roller the editor places with /roll, so an AI can keep, move
// or add the roller when it rewrites a page. The export writes a roller the
// same way. A name that matches no rolling table stays as its plain words,
// with a review warning, as an unknown @[Name] does.
//
// The built-in tables (the starters, the library and the name generator)
// live in the browser, not here, so the importer cannot check those names.
// The export writes such a roller by its table's id, and the id is accepted
// on the page that already has a roller on it: the AI can keep or move it,
// but not add a new one.

package importer

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// rollHTMLRe finds a roller line once goldmark has made it a paragraph of
// its own; rollAnyRe finds one anywhere, for the warning about a roller
// written inside other text.
var (
	rollHTMLRe = regexp.MustCompile(`<p>\{roll\{([^{}|<>]+)(?:\|\s*(\d+)\s*)?\}\}</p>`)
	rollAnyRe  = regexp.MustCompile(`\{roll\{[^{}<>]*\}\}`)
	codeHTMLRe = regexp.MustCompile(`(?s)<pre\b.*?</pre>|<code>.*?</code>`)
)

// rollerIDRe is the id shape the editor accepts for a roller's table.
var rollerIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// rollerTableRe reads a saved roller's table id from its classes.
var rollerTableRe = regexp.MustCompile(`<div\b[^>]*\bclass="[^"]*\bce-roll--t-([a-z][a-z0-9-]{0,63})\b[^"]*"`)

// SetRollTables tells the resolver the campaign's rolling tables: each key is
// a table's lower-cased name or its id, the value its id. Tables a paste
// makes are included by the caller, since records commit after pages.
func (p *PageLinks) SetRollTables(ids map[string]string) {
	if p != nil {
		p.rollTables = ids
	}
}

// rollTable resolves a roller line's name: a campaign table by name or id,
// else the id of a roller the page already has.
func (p *PageLinks) rollTable(name string, kept map[string]bool) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if p != nil {
		if id, ok := p.rollTables[key]; ok {
			return id, true
		}
	}
	if kept[key] {
		return key, true
	}
	return "", false
}

// WithKeptRollers lets a page's own rollers be written back by their table's
// id, for a body that replaces e's text.
func WithKeptRollers(e *entities.Entity) RenderOption {
	return func(o *renderOpts) { o.keptRollers = keptRollers(e) }
}

// keptRollers is the table ids of the rollers e's text holds.
func keptRollers(e *entities.Entity) map[string]bool {
	if e == nil || e.EntryHTML == nil || !strings.Contains(*e.EntryHTML, "ce-roll--t-") {
		return nil
	}
	out := map[string]bool{}
	for _, m := range rollerTableRe.FindAllStringSubmatch(*e.EntryHTML, -1) {
		out[m[1]] = true
	}
	return out
}

// rollCount is the roller's count as the editor allows it: 1, 3 or 5, the
// nearest one when another number is written, 3 when none is.
func rollCount(s string) int {
	n, err := strconv.Atoi(s)
	switch {
	case err != nil:
		return 3
	case n <= 1:
		return 1
	case n <= 3:
		return 3
	}
	return 5
}

// rollerDiv is the roller as the editor saves it: the table and count live
// in classes, which the sanitizer keeps.
func rollerDiv(id string, count int) string {
	return fmt.Sprintf(`<div class="ce-roll ce-roll--t-%s ce-roll--n-%d"></div>`, id, count)
}

// linkRollers replaces each roller line whose table resolves; the rest stay
// as written. Only the owner or a co-DM places a roller, as in the editor.
func linkRollers(htmlIn string, p *PageLinks, kept map[string]bool) string {
	if !strings.Contains(htmlIn, "{roll{") || p == nil || !secretWriter(p.viewer) {
		return htmlIn
	}
	return rollHTMLRe.ReplaceAllStringFunc(htmlIn, func(m string) string {
		sub := rollHTMLRe.FindStringSubmatch(m)
		id, ok := p.rollTable(html.UnescapeString(sub[1]), kept)
		if !ok || !rollerIDRe.MatchString(id) {
			return m
		}
		return rollerDiv(id, rollCount(sub[2]))
	})
}

// RollWarnings names each roller line whose table is not one of the
// campaign's rolling tables, the paste's own or one existing (the page it
// replaces, nil for a new page) already rolls, and any roller written inside
// other text. A warning, never a block: those stay as plain text.
func RollWarnings(p *PageLinks, markdown string, existing *entities.Entity) []string {
	alone, inline := rollerLines(markdown)
	if len(alone)+inline == 0 {
		return nil
	}
	if p == nil || !secretWriter(p.viewer) {
		return []string{"Only the campaign's owner or a co-DM can put a table roller in a page, so the {roll{…}} lines stay as plain text."}
	}
	kept := keptRollers(existing)
	var out []string
	seen := map[string]bool{}
	for _, name := range alone {
		if _, ok := p.rollTable(name, kept); ok || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, fmt.Sprintf("No rolling table called %q, so {roll{%s}} stays as plain text. Make the table with a `kind: table` block, or use one of the campaign's.", name, name))
	}
	if inline > 0 {
		out = append(out, "A {roll{…}} written inside other text stays as plain text; a table roller needs a line of its own, with a blank line before and after.")
	}
	return out
}

// rollerLines is the table names of the body's roller lines, outside code,
// and how many rollers are written inside other text.
func rollerLines(markdown string) (alone []string, inline int) {
	if !strings.Contains(markdown, "{roll{") {
		return nil, 0
	}
	out, err := MarkdownToHTML(markdown, func(o *renderOpts) { o.markers = true })
	if err != nil {
		return nil, 0
	}
	out = codeHTMLRe.ReplaceAllString(out, "")
	for _, m := range rollHTMLRe.FindAllStringSubmatch(out, -1) {
		alone = append(alone, strings.TrimSpace(html.UnescapeString(m[1])))
	}
	inline = len(rollAnyRe.FindAllString(rollHTMLRe.ReplaceAllString(out, ""), -1))
	return alone, inline
}
