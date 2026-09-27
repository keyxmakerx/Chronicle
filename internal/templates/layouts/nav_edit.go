package layouts

import (
	"context"
	"encoding/json"
)

// nav_edit.go carries what the owner's sidebar editor (sidebar_editor.js)
// starts from: every section and row of the owner's arrangement, including
// rows hidden from players and apps turned off in Extensions. Only the owner
// is ever given it, so it is the one place the page holds rows a player must
// not see; the template renders it only for the owner as well.

const keyNavEdit ctxKey = "layout_nav_edit"

// NavEditView is the owner's whole arrangement, as the editor draws it.
type NavEditView struct {
	Sections []NavEditSection `json:"sections"`
	// Off lists the apps turned off in Extensions, for the editor's tray.
	Off []NavEditRow `json:"off"`
}

// NavEditSection is one section of the arrangement. Kind is "pinned",
// "apps", "categories" or "custom".
type NavEditSection struct {
	ID    string       `json:"id"`
	Kind  string       `json:"kind"`
	Label string       `json:"label"`
	Items []NavEditRow `json:"items"`
}

// NavEditRow is one row of the arrangement.
type NavEditRow struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"` // "app", "category" or "link"
	Label  string `json:"label"`
	Icon   string `json:"icon,omitempty"`
	Color  string `json:"color,omitempty"`
	URL    string `json:"url,omitempty"` // a link's target, as the owner typed it
	Hidden bool   `json:"hidden,omitempty"`
	// Off marks an app turned off in Extensions. It keeps its place in the
	// arrangement, for when it is turned back on, but no sidebar shows it.
	Off bool `json:"off,omitempty"`
}

// SetNavEdit stores the owner's arrangement in context. The LayoutInjector
// calls it for the owner only.
func SetNavEdit(ctx context.Context, v *NavEditView) context.Context {
	return context.WithValue(ctx, keyNavEdit, v)
}

// NavEditJSON is the owner's arrangement as JSON for the editor, or "" for
// anyone else, including an owner viewing the campaign as a player, and on
// an archived campaign.
func NavEditJSON(ctx context.Context) string {
	v, _ := ctx.Value(keyNavEdit).(*NavEditView)
	if v == nil || !NavCanEdit(ctx) {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
