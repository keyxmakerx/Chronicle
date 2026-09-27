package campaigns

import "encoding/json"

// topbar_json.go builds the data-topbar-style / data-topbar-content
// attribute payloads that appearance_editor.js JSON.parse()s at init. Kept
// as a plain .go file, not inline in branding.templ: a templ file hand-
// assembling JSON via fmt.Sprintf is exactly the class of bug this file
// replaces (a value carrying a stray quote or backslash broke the attribute
// instead of just being escaped), and encoding/json can't make that mistake.
// TopbarStyle/TopbarContent already carry the right json tags (model.go), so
// marshaling the struct directly reproduces the same field names the widget
// reads; a nil pointer marshals to "null", not "{}", so each keeps its own
// nil guard to match what the widget expects when nothing is set.

// topbarStyleJSON serializes a TopbarStyle to a JSON data attribute value.
func topbarStyleJSON(style *TopbarStyle) string {
	if style == nil {
		return "{}"
	}
	b, err := json.Marshal(style)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// topbarContentJSON serializes TopbarContent to a JSON data attribute value.
func topbarContentJSON(content *TopbarContent) string {
	if content == nil {
		return `{"mode":"none","links":[],"quote":""}`
	}
	b, err := json.Marshal(content)
	if err != nil {
		return `{"mode":"none","links":[],"quote":""}`
	}
	return string(b)
}
