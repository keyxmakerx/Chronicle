package entities

import "strings"

// itemLookType is the words the Look block uses to pick how the item moves on
// its stage (a blade lunges, a potion pours): the page's subtype, else its
// sheet's "type" field. Empty is fine; the widget falls back to the picture.
func itemLookType(e *Entity) string {
	if e.TypeLabel != nil && strings.TrimSpace(*e.TypeLabel) != "" {
		return *e.TypeLabel
	}
	if t, ok := e.FieldsData["type"].(string); ok {
		return t
	}
	return ""
}
