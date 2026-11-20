package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
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

// "Try a style kit here" is drawn for the owner only while the pages wear
// the Material page look, the one kits dress; a player gets no brush and so
// never the row.
func TestTopbarBrush_KitRow(t *testing.T) {
	annals := func(ctx context.Context) context.Context {
		return SetAppearance(ctx, &AppearanceData{PageLook: "annals", StyleKit: "ornate"})
	}
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"owner, Material page look", annals(viewer(3, true, false, false, false)), true},
		{"owner, standard page look", SetAppearance(viewer(3, true, false, false, false), &AppearanceData{PageLook: "standard"}), false},
		{"owner, no look saved", viewer(3, true, false, false, false), false},
		{"player, Material page look", annals(viewer(1, false, false, false, false)), false},
		{"owner viewing as a player", annals(viewer(1, true, false, true, false)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBar(t, tc.ctx)
			row := strings.Contains(html, "data-brush-kit") && strings.Contains(html, "Try a style kit here")
			url := strings.Contains(html, `data-kit-url="/campaigns/camp-1/customize/kit-try"`)
			if row != tc.want || url != tc.want {
				t.Fatalf("kit row = %v, kit url = %v; want %v", row, url, tc.want)
			}
			if tc.want && !strings.Contains(html, "See this page in another kit. Nothing is saved.") {
				t.Error("kit row misses its second line")
			}
		})
	}
}

// An open try-out goes back on before the first paint only where the
// owner's brush is drawn and pages wear the Material look: the head loads
// kit_try_early.js and <html> carries the mark it acts on.
func TestBase_KitTryEarly(t *testing.T) {
	annals := func(ctx context.Context) context.Context {
		return SetAppearance(ctx, &AppearanceData{PageLook: "annals"})
	}
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"owner, Material page look", annals(viewer(3, true, false, false, false)), true},
		{"owner who hid the brush", annals(SetViewPrefs(viewer(3, true, false, false, false), &ViewPrefsData{Brush: "hidden"})), false},
		{"owner, standard page look", SetAppearance(viewer(3, true, false, false, false), &AppearanceData{PageLook: "standard"}), false},
		{"player", annals(viewer(1, false, false, false, false)), false},
		{"owner viewing as a player", annals(viewer(1, true, false, true, false)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Base("t").Render(templ.WithChildren(tc.ctx, templ.Raw("")), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			script := strings.Contains(html, "/static/js/kit_try_early.js")
			mark := strings.Contains(html, "data-kit-try-ok")
			if script != tc.want || mark != tc.want {
				t.Fatalf("early script = %v, mark = %v; want %v", script, mark, tc.want)
			}
		})
	}
}
