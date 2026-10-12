package importer

import (
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// GMOnlyParts counts the GM-only parts of a page's current text. The AI never
// receives them in a form it can return (the export strips them or flattens
// them to ordinary words), so an update that replaces the whole text would
// delete them or publish the AI's copy to players. The review screen and the
// committer both measure it from the live entity so the two cannot disagree.
type GMOnlyParts struct {
	Passages int
	Pictures int
	Diagrams int
}

// GMOnlyPartsOf measures an existing page; a page with no text has none.
func GMOnlyPartsOf(e *entities.Entity) GMOnlyParts {
	if e == nil || e.EntryHTML == nil {
		return GMOnlyParts{}
	}
	p, pic, d := sanitize.CountSecrets(*e.EntryHTML)
	return GMOnlyParts{Passages: p, Pictures: pic, Diagrams: d}
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
