package layouts

import (
	"html"

	"github.com/a-h/templ"
)

// newPageOpenBody is the click handler every place that starts Create page
// carries. A plain click opens the small sheet over the current screen
// (new_page_sheet.js); a modified click, a middle click, or a page without the
// script follows the link to the full screen, which keeps working for direct
// links and no-JS. stopImmediatePropagation keeps htmx's boosted navigation,
// bound after this handler, from also following the link.
const newPageOpenBody = `(function(e,a){` +
	`if(e.defaultPrevented||e.button||e.metaKey||e.ctrlKey||e.shiftKey||e.altKey)return true;` +
	`var n=window.Chronicle&&window.Chronicle.newPage;if(!n||!n.open(a.getAttribute('href')))return true;` +
	`e.preventDefault();e.stopImmediatePropagation();return false;` +
	`})(event,this)`

// NewPageOpenJS is the handler as an onclick for a templ attribute.
func NewPageOpenJS() templ.ComponentScript {
	return templ.ComponentScript{Name: "newPageOpen", Function: "", Call: html.EscapeString(newPageOpenBody)}
}

// NewPageOpenAttrs is the same handler as attributes, for a component that
// takes them as a map (AppAction). templ replaces a plain string given to an
// event attribute with a refusal comment, so the value is the script itself.
func NewPageOpenAttrs() templ.Attributes {
	return templ.Attributes{"onclick": NewPageOpenJS()}
}
