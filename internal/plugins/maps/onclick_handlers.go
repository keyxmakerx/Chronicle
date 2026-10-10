package maps

import (
	"fmt"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// scriptURL is the content-hashed URL of one of this plugin's scripts.
func scriptURL(name string) string {
	return layouts.AssetURL("/static/plugins/" + PluginSlug + "/js/" + name)
}

// mapPageURL is the map's own page.
func mapPageURL(campaignID, mapID string) string {
	return fmt.Sprintf("/campaigns/%s/maps/%s", campaignID, mapID)
}

// bindingsCreateURL is the widget-binding create-and-bind route the map
// picker's "Create a map" field posts to.
func bindingsCreateURL(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/bindings/create", campaignID)
}

// mapCountLabel is the picker's count before anything is typed.
func mapCountLabel(n int) string {
	if n == 1 {
		return "1 map"
	}
	return fmt.Sprintf("%d maps", n)
}

// mapViewerURL is the bare framed-viewer fragment the focus view fetches.
func mapViewerURL(campaignID, mapID string) string {
	return mapPageURL(campaignID, mapID) + "/viewer"
}

// unfoldOnClick is the preview's click and keyboard handler.
//
// Rule: every interactive element inside an HTMX-swapped fragment uses an inline
// IIFE in its on* attribute, never a templ `script` helper or a delegated
// document listener (a helper's sibling <script> is not reliably executed on an
// innerHTML swap). The body is built here and injected through a
// templ.ComponentScript with an empty Function, which templ writes into the
// attribute unescaped, so it must contain no double quote. Enter and Space
// open it from the keyboard; any other key is left alone.
func unfoldOnClick() templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "maps_unfoldPreview",
		Function: "",
		Call: `(function(el,e){` +
			`if(e&&e.type==='keydown'){if(e.key!=='Enter'&&e.key!==' ')return;e.preventDefault();}` +
			`if(window.ChronicleMapFocus)window.ChronicleMapFocus.open(el);` +
			`})(this,event)`,
	}
}

// pickerCall is one of the map picker's handlers in
// static/js/map_block_picker.js (filter, select, pick, create, cancelCreate),
// as an inline IIFE for the same swap-safety reason as unfoldOnClick. fn is
// always one of those fixed names, never input.
func pickerCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "maps_picker_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleMapPicker)window.ChronicleMapPicker.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}
