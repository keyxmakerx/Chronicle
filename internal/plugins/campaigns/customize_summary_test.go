package campaigns

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// campaignWithLook is a campaign seeded from a starter look, then changed by
// edit, the way Customize would save it.
func campaignWithLook(t *testing.T, look string, edit func(*CampaignSettings)) *Campaign {
	t.Helper()
	var s CampaignSettings
	if look != "" && !seedLook(&s, look) {
		t.Fatalf("seed %s failed", look)
	}
	if edit != nil {
		edit(&s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return &Campaign{ID: "c1", Settings: string(b)}
}

// The look is named the way the Look editor names it: the starter look while
// the style is still exactly that look's, "Custom" once the owner changed it;
// the colour by its name, none for a colour of the owner's own; the pages by
// the page look and its kit.
func TestLookSummaryOf(t *testing.T) {
	annals := func(kit string) func(*CampaignSettings) {
		return func(s *CampaignSettings) {
			if s.Appearance == nil {
				s.Appearance = &Appearance{}
			}
			s.Appearance.PageLook, s.Appearance.StyleKit = "annals", kit
		}
	}
	tests := []struct {
		name string
		c    *Campaign
		want LookSummary
	}{
		{"never customized", &Campaign{ID: "c1", Settings: "{}"}, LookSummary{"Classic", "Indigo", "Standard pages"}},
		{"Parchment in the Sketchbook kit", campaignWithLook(t, "parchment", annals("sketchbook")), LookSummary{"Parchment", "Sienna", "Sketchbook pages"}},
		{"Material with its own kit", campaignWithLook(t, "midnight", annals("")), LookSummary{"Midnight", "Moonlit blue", "Material pages"}},
		{"every starter is recognised", campaignWithLook(t, "starship", nil), LookSummary{"Starship", "Console teal", "Standard pages"}},
		{"a changed font makes it the owner's own", campaignWithLook(t, "forest", func(s *CampaignSettings) { s.Appearance.HeadingFont = "cinzel" }), LookSummary{"Custom", "Moss green", "Standard pages"}},
		{"a mixed colour has no name", campaignWithLook(t, "ember", func(s *CampaignSettings) { s.AccentColor = "#123456" }), LookSummary{"Custom", "", "Standard pages"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lookSummaryOf(context.Background(), tt.c); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The colour names are the Look editor's own, so the settings row and the
// editor never call one colour two things.
func TestLookColourNamesMatchEditor(t *testing.T) {
	src, err := os.ReadFile("../../../static/js/widgets/customize_look.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	pair := regexp.MustCompile(`\['([^']+)', '(#[0-9a-f]{6})'\]`)
	named := regexp.MustCompile(`'(#[0-9a-f]{6})':'([^']+)'`)
	var want [][2]string
	for _, list := range []string{"PRESETS", "ACCENT_PRESETS", "HDR_COLOURS"} {
		m := regexp.MustCompile(`var ` + list + ` = \[(.*?)\];`).FindStringSubmatch(js)
		if m == nil {
			t.Fatalf("customize_look.js has no %s", list)
		}
		for _, p := range pair.FindAllStringSubmatch(m[1], -1) {
			want = append(want, [2]string{p[2], p[1]})
		}
	}
	m := regexp.MustCompile(`var LOOK_COLOUR_NAMES = \{(.*?)\};`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal("customize_look.js has no LOOK_COLOUR_NAMES")
	}
	for _, p := range named.FindAllStringSubmatch(m[1], -1) {
		want = append(want, [2]string{p[1], p[2]})
	}
	if len(want) != len(lookColourNames) {
		t.Fatalf("editor names %d colours, server %d", len(want), len(lookColourNames))
	}
	for i := range want {
		if want[i] != lookColourNames[i] {
			t.Errorf("colour %d: editor %v, server %v", i, want[i], lookColourNames[i])
		}
	}
}

func TestCategoriesAndHomesSummary(t *testing.T) {
	p := 1
	n, hidden := categoriesSummaryOf([]CustomizeCategory{{ID: 1}, {ID: 2, ParentID: &p}, {ID: 3, DMOnly: true}})
	if n != 2 || hidden != 1 {
		t.Errorf("categories = %d, %d hidden; want 2, 1", n, hidden)
	}
	c := &Campaign{ID: "c1"}
	if text, sub := homesSummaryOf(c, "GM"); text != "Everyone" || sub != "no player home yet" {
		t.Errorf("no homes of their own: %q %q", text, sub)
	}
	full, err := c.SetRoleDashboardJSON(HomePlayer, DefaultDashboardLayout())
	if err != nil {
		t.Fatal(err)
	}
	c.DashboardLayout = full
	if _, sub := homesSummaryOf(c, "GM"); !strings.Contains(sub, "Players") {
		t.Errorf("a players' home of its own is named: %q", sub)
	}
}
