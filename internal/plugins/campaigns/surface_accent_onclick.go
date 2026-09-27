package campaigns

import (
	"fmt"
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
// The handler no longer PUTs to the server itself: it sets the CSS variable
// for an instant local preview, then dispatches a CustomEvent so the
// appearance-editor widget's own JS (which owns campaignId/csrfToken and the
// Save/Discard flow) can fold the slot into its one staged draft instead of
// saving on every click.

// jsStr returns a single-quoted JS string literal for embedding in an inline
// attribute handler (which templ delimits with double quotes).
func jsStr(s string) string {
	return "'" + template.JSEscapeString(s) + "'"
}

// inlineHandler wraps a JS body in a ComponentScript with an empty Function
// (no <script> tag emitted) so the body renders directly into the attribute.
func inlineHandler(name, jsBody string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: jsBody}
}

// applySurfaceAccentJS is the shared body for the preset/reset buttons and
// the custom picker: local CSS-variable preview, then a
// chronicle:surface-accent-change event the widget listens for. colorExpr is
// a JS expression yielding the chosen color ("" clears the slot).
func applySurfaceAccentJS(slot int, colorExpr string) string {
	return fmt.Sprintf(
		`(function(){`+
			`var color=%s;`+
			`var prop='--color-accent-surface-%d';`+
			`if(color){document.documentElement.style.setProperty(prop,color);}`+
			`else{document.documentElement.style.removeProperty(prop);}`+
			`var w=document.querySelector('[data-widget="appearance-editor"]');`+
			`if(w){w.dispatchEvent(new CustomEvent('chronicle:surface-accent-change',{detail:{slot:%d,color:color}}));}`+
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
