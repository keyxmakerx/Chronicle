package campaigns

import (
	"fmt"
	"html"
	"text/template"

	"github.com/a-h/templ"
)

// surface_accent_onclick.go builds the Surface Accents card's click/change
// handlers as inline IIFEs in onclick/onchange attributes, never a <script>
// tag: boot.js sets htmx.config.allowScriptTags=false, so htmx deletes any
// <script> in a swapped-in fragment, and this card is reached by a boosted
// sidebar swap (.ai/conventions.md, HTMX swap-safety). Same pattern as
// internal/plugins/foundry_vtt/onclick_handlers.go. Only server-controlled
// values (the slot, preset colors, the custom input's own id) are
// interpolated; the custom color is read from the input when the handler
// runs.
//
// The handler previews locally on the appearance-editor widget element (so
// the sample card inherits it, without leaking to the rest of the page) and
// dispatches a CustomEvent so the widget's own JS — which owns
// campaignId/csrfToken and the Save/Discard flow — folds the slot into its
// one staged draft.

// jsStr returns a single-quoted JS string literal for embedding in an inline
// attribute handler (which templ delimits with double quotes).
func jsStr(s string) string {
	return "'" + template.JSEscapeString(s) + "'"
}

// inlineHandler wraps a JS body in a ComponentScript with an empty Function
// (no <script> tag emitted) so the body renders directly into the attribute.
// templ writes ComponentScript.Call into onclick="..."/onchange="..."
// verbatim, with no HTML-escaping of its own (generator.go's
// writeExpressionAttributeValueScript writes .Call raw) — so a literal `"`
// anywhere in the body would close the attribute early and truncate every
// handler after it. html.EscapeString makes the body opaque to the HTML
// parser regardless of what characters the JS needs, so jsStr (and any
// future caller building a body from non-constant input) doesn't have to
// get that right on its own.
func inlineHandler(name, jsBody string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: html.EscapeString(jsBody)}
}

// applySurfaceAccentJS is the shared body for the preset/reset buttons and
// the custom picker: an instant local preview on the appearance-editor
// widget element (its style, not <html> — the "Sample character header"
// card inherits the custom property through the normal CSS cascade since
// it's a descendant, while the rest of the page, outside the widget, never
// sees it), then a chronicle:surface-accent-change event the widget listens
// for. colorExpr is a JS expression yielding the chosen color ("" clears
// the slot). The selector's quotes are backslash-escaped single quotes, not
// literal double quotes, so the body stays readable without leaning on
// html.EscapeString alone to keep the attribute intact.
func applySurfaceAccentJS(slot int, colorExpr string) string {
	return fmt.Sprintf(
		`(function(){`+
			`var color=%s;`+
			`var prop='--color-accent-surface-%d';`+
			`var w=document.querySelector('[data-widget=\'appearance-editor\']');`+
			`if(w){`+
			`if(color){w.style.setProperty(prop,color);}else{w.style.removeProperty(prop);}`+
			`w.dispatchEvent(new CustomEvent('chronicle:surface-accent-change',{detail:{slot:%d,color:color}}));`+
			`}`+
			`})()`,
		colorExpr, slot, slot,
	)
}

// surfaceAccentApplyOnClick returns the onclick handler for a preset swatch
// (or the reset button, with color=""): the slot and color are both known at
// render time, so they're baked in as literals.
func surfaceAccentApplyOnClick(slot int, color string) templ.ComponentScript {
	body := applySurfaceAccentJS(slot, jsStr(color))
	return inlineHandler(fmt.Sprintf("surfaceAccentApply_%d_%s", slot, color), body)
}

// surfaceAccentCustomOnChange returns the onchange handler for the custom
// color <input>: the color isn't known until the user picks one, so it reads
// the input's own current value at fire time via its id.
func surfaceAccentCustomOnChange(slot int, inputID string) templ.ComponentScript {
	body := applySurfaceAccentJS(slot,
		fmt.Sprintf(`document.getElementById(%s).value`, jsStr(inputID)))
	return inlineHandler(fmt.Sprintf("surfaceAccentCustom_%d", slot), body)
}
