package layouts

import (
	"context"
	"strconv"
	"strings"
)

// nav_drawer.go carries the app drawer's tiles into the templates. The drawer
// lists every app the viewer may open, grouped, whether or not it is in their
// sidebar; the owner also sees the apps turned off. The tiles are built from
// the same rows the sidebar is drawn from (ViewNav and NavDrawerRows), so the
// drawer never offers a page the sidebar would withhold.

const keyNavDrawer ctxKey = "layout_nav_drawer"

// NavDrawerView is the viewer's app drawer.
type NavDrawerView struct {
	Groups []NavDrawerGroup
	// Off lists the apps turned off for the campaign. Only the owner, who can
	// turn them on, is ever given any.
	Off []NavDrawerTile
}

// NavDrawerGroup is one titled group of tiles.
type NavDrawerGroup struct {
	ID     string
	Label  string
	DMOnly bool // only Directors see it
	Tiles  []NavDrawerTile
}

// NavDrawerTile is one app in the drawer.
type NavDrawerTile struct {
	Key   string // "app:<slug>", the same key its sidebar row has
	Label string
	Icon  string
	URL   string
	Line  string // one line on what the app is for
	// InSidebar is set when the viewer's sidebar has the app's row; Pinned
	// when that row is in Pinned, and Personal when the viewer pinned it.
	InSidebar bool
	Pinned    bool
	Personal  bool
	Hidden    bool // hidden from players; only the owner is ever given one
	// TurnOn and TurnOff are the campaign addons that turn the app on or
	// off, for the owner only. Empty means the drawer cannot switch it and
	// sends the owner to Game & features instead.
	TurnOn  []int
	TurnOff []int
}

// SetNavDrawer stores the viewer's app drawer in context.
func SetNavDrawer(ctx context.Context, v *NavDrawerView) context.Context {
	return context.WithValue(ctx, keyNavDrawer, v)
}

// GetNavDrawer returns the viewer's app drawer, or nil outside a campaign.
func GetNavDrawer(ctx context.Context) *NavDrawerView {
	v, _ := ctx.Value(keyNavDrawer).(*NavDrawerView)
	return v
}

// NavDrawerCount is how many apps the viewer's drawer lists, turned-off ones
// left out.
func NavDrawerCount(ctx context.Context) int {
	v := GetNavDrawer(ctx)
	if v == nil {
		return 0
	}
	n := 0
	for _, g := range v.Groups {
		n += len(g.Tiles)
	}
	return n
}

// NavHasDrawer reports whether the viewer gets the drawer at all: it lists an
// app, or the owner has one to turn on.
func NavHasDrawer(ctx context.Context) bool {
	v := GetNavDrawer(ctx)
	return v != nil && (NavDrawerCount(ctx) > 0 || (len(v.Off) > 0 && NavCanEdit(ctx)))
}

// navDrawerIDs writes addon ids as the comma list the drawer's menu reads.
func navDrawerIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// navDrawerFind is the text the drawer's find line matches a tile against.
func navDrawerFind(t NavDrawerTile) string {
	return strings.ToLower(t.Label + " " + t.Line)
}
