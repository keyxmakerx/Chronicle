package entities

import (
	"github.com/a-h/templ"
)

// contents_block.go registers the "Contents" page block: a margin rail (a
// labelled button on a narrow page) that lists the page's own headings as jump
// links. The headings are read in the browser from the rendered page text
// (static/js/page_contents.js), never from the stored document: the server has
// already left a player's Director-only text out of that DOM, so a heading a
// viewer may not read is never offered to them, and the block needs no route
// and stores nothing.

// ContentsBlockType is the layout block type for the Contents block.
const ContentsBlockType = "contents"

// ContentsWidgetName is the data-widget the block mounts so boot.js loads its
// script on sight (ADR-063).
const ContentsWidgetName = "page-contents"

// registerContentsBlock adds the Contents block. It is one per page because
// the rail is a single strip down the page margin and its panel is one sheet.
// It declares no tile sizes: in the rail it takes no room of its own, so a
// tile would leave an empty cell.
func registerContentsBlock(r *BlockRegistry) {
	r.Register(BlockMeta{
		Type: ContentsBlockType, Label: "Contents", Icon: "fa-list-ul",
		Description: "The page's headings as a rail in the margin, each a jump link",
		Contexts:    []string{"template"},
		Singleton:   true,
	}, func(ctx BlockRenderContext) templ.Component {
		return blockContents(ctx.Entity)
	})
}

// contentsCall is one of the block's handlers in static/js/page_contents.js,
// written as an inline IIFE so it works after an HTMX swap of the page (a templ
// script helper or a delegated listener may never run there). fn is always one
// of that file's fixed names, never input.
func contentsCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "entities_contents_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleContents)window.ChronicleContents.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// contentsPanelID is the id the icon's aria-controls points at. The block is a
// singleton, but the id carries the entity so a page opened in a side peek
// beside another never shares it.
func contentsPanelID(e *Entity) string {
	if e == nil {
		return "ct-panel"
	}
	return "ct-panel-" + e.ID
}
