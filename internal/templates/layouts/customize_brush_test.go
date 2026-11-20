package layouts

import (
	"context"
	"strings"
	"testing"
)

// The brush is for whoever may open Customize (the campaign's owner), in
// their own view of the campaign. A player, a co-DM, a guest, an owner
// looking as a player and a signed-out visitor get none; an owner who put it
// away gets only the inert template Campaign settings brings it back from.
func TestTopbarBrush_PerViewer(t *testing.T) {
	hidden := SetViewPrefs(viewer(3, true, false, false, false), &ViewPrefsData{Brush: "hidden"})
	shown := SetViewPrefs(viewer(3, true, false, false, false), &ViewPrefsData{Brush: "shown"})
	guestPreview := SetPreviewAs(viewer(3, true, false, false, false), "guest")
	signedOut := SetCampaignID(context.Background(), "camp-1")
	tests := []struct {
		name     string
		ctx      context.Context
		live     bool // the brush is on the bar
		template bool // kept as an inert template
	}{
		{"owner", viewer(3, true, false, false, false), true, false},
		{"owner who chose shown", shown, true, false},
		{"owner who hid it", hidden, false, true},
		{"owner viewing as a player", viewer(1, true, false, true, false), false, false},
		{"owner previewing as a guest", guestPreview, false, false},
		{"co-DM", viewer(1, false, true, false, false), false, false},
		{"player", viewer(1, false, false, false, false), false, false},
		{"guest", viewer(1, false, false, false, true), false, false},
		{"signed out", signedOut, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBar(t, tc.ctx)
			tpl := strings.Contains(html, "data-brush-kept")
			live := strings.Contains(html, `data-widget="customize-brush"`) && !tpl
			if live != tc.live || tpl != tc.template {
				t.Fatalf("brush live = %v, template = %v; want %v, %v", live, tpl, tc.live, tc.template)
			}
		})
	}
}

// The menu's fixed rows lead into this campaign's Customize, are words with
// no icon, and the first row waits hidden for the page's own place.
func TestTopbarBrush_Menu(t *testing.T) {
	html := renderBar(t, viewer(3, true, false, false, false))
	start := strings.Index(html, `id="tb-brush-menu"`)
	if start < 0 {
		t.Fatal("no brush menu")
	}
	menu := html[start:]
	menu = menu[:strings.Index(menu, "</div></div>")]
	for _, want := range []string{
		`role="menu"`, `hidden`, `data-brush-here hidden`,
		`href="/campaigns/camp-1/customize/look"`, "This campaign’s look", "Colours, fonts, style kit",
		`href="/campaigns/camp-1/customize"`, "Everything in Customize",
		"Hide this brush", "Campaign settings brings it back",
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("menu misses %q", want)
		}
	}
	if strings.Contains(menu, "<i ") {
		t.Errorf("a menu row carries an icon: %s", menu)
	}
	btn := html[strings.Index(html, `id="tb-brush-btn"`)-200:]
	for _, want := range []string{`aria-haspopup="menu"`, `aria-expanded="false"`, `aria-controls="tb-brush-menu"`, `aria-label="Customize this campaign"`} {
		if !strings.Contains(btn[:600], want) {
			t.Errorf("brush button misses %s", want)
		}
	}
}
