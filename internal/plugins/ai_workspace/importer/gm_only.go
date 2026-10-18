package importer

import (
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// GMOnlyParts counts the GM-only parts of a page's current text that an update
// would lose. Pictures and diagrams never come back through markdown. A
// passage does, as a {gm{…}gm} marker, but only when the export was made in a
// mode that wrote it; the AI's text may come from a safe-mode export, or the
// AI may have dropped or unmarked it. So a passage counts as kept only if the
// incoming text carries a marked passage with the same words. The review
// screen and the committer both measure it from the live entity so the two
// cannot disagree.
type GMOnlyParts struct {
	Passages int
	Pictures int
	Diagrams int
}

// GMOnlyPartsOf measures an existing page against a replacement that carries
// no GM-only passages at all; a page with no text has none.
func GMOnlyPartsOf(e *entities.Entity) GMOnlyParts {
	return GMOnlyPartsAfter(e, "")
}

// GMOnlyPartsAfter measures what replacing the page's text with incomingHTML
// (already converted from the AI's markdown) would lose. Passages are matched
// as a multiset on their words, so a repeated line is not covered by one copy.
func GMOnlyPartsAfter(e *entities.Entity, incomingHTML string) GMOnlyParts {
	if e == nil || e.EntryHTML == nil {
		return GMOnlyParts{}
	}
	_, pic, d := sanitize.CountSecrets(*e.EntryHTML)
	have := map[string]int{}
	for _, s := range secretPassages(incomingHTML) {
		have[s]++
	}
	missing := 0
	for _, s := range secretPassages(*e.EntryHTML) {
		if have[s] > 0 {
			have[s]--
			continue
		}
		missing++
	}
	// A passage with no words has nothing to lose, so it never blocks an
	// update.
	return GMOnlyParts{Passages: missing, Pictures: pic, Diagrams: d}
}

// GMOnlyPartsForMarkdown is GMOnlyPartsAfter for the AI's raw page body, used
// where the review screen has no converted HTML yet.
func GMOnlyPartsForMarkdown(e *entities.Entity, body string) GMOnlyParts {
	html, err := MarkdownToHTML(body, func(o *renderOpts) { o.markers = true })
	if err != nil {
		html = ""
	}
	return GMOnlyPartsAfter(e, html)
}

// Any reports whether the page has GM-only parts a text replace would lose.
func (g GMOnlyParts) Any() bool { return g.Passages+g.Pictures+g.Diagrams > 0 }

// Phrase is the counts in plain words, e.g. "2 GM-only passages and 1
// GM-only picture". Empty when there are none.
func (g GMOnlyParts) Phrase() string {
	var parts []string
	add := func(n int, one, many string) {
		switch {
		case n == 1:
			parts = append(parts, "1 GM-only "+one)
		case n > 1:
			parts = append(parts, strconv.Itoa(n)+" GM-only "+many)
		}
	}
	add(g.Passages, "passage", "passages")
	add(g.Pictures, "picture", "pictures")
	add(g.Diagrams, "diagram", "diagrams")
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// Warning is the review-screen sentence shown when a row would replace a page
// that has GM-only parts.
func (g GMOnlyParts) Warning() string {
	return "The AI's version leaves out this page's " + g.Phrase() +
		". Importing it removes them. Page history can bring the old text back."
}

// SkipReason is the committer's outcome text for an update that was not
// ticked.
func (g GMOnlyParts) SkipReason() string {
	return "Skipped: the AI's version leaves out this page's " + g.Phrase() +
		". Tick \"Replace the text anyway\" on this row to import it."
}
