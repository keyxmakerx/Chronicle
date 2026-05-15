package packages

import "strings"

// sanitizeForID returns a string safe to use in HTML element IDs.
// Version strings can contain dots and dashes that some CSS selectors
// or HTMX hx-target attributes won't escape correctly. Replace dots
// with hyphens; pass-through everything else.
//
// Used by packages.templ to build per-version DOM IDs for foundry_vtt's
// "campaigns using v0.1.5" expandable target divs.
func sanitizeForID(s string) string {
	return strings.NewReplacer(".", "-", "+", "-", "/", "-").Replace(s)
}
