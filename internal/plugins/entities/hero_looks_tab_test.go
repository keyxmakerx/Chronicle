package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// attrOf reads an attribute's value out of rendered markup, undoing the
// escaping a browser would undo.
func attrOf(t *testing.T, markup, name string) string {
	t.Helper()
	m := regexp.MustCompile(` ` + regexp.QuoteMeta(name) + `="([^"]*)"`).FindStringSubmatch(markup)
	if m == nil {
		t.Fatalf("no %s attribute in:\n%s", name, markup)
	}
	return html.UnescapeString(m[1])
}

// The Customize hook: a "Hero creator" tab that saves into the herolooks
// draft section and whose body is the workshop mount, pointed at this
// campaign's own plan.
func TestHeroLooksTab(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	tab := CustomizeTabFactory()(cc)
	if tab.ID != "hero-creator" || tab.Label != "Hero creator" || tab.Section != "herolooks" || tab.Content == nil {
		t.Fatalf("tab = %+v", tab)
	}
	if len(tab.Paths) != 1 || tab.Paths[0] != "/characters/new" || tab.Here == "" || tab.Blurb == "" {
		t.Errorf("the brush needs the creator's page and a row's words: paths %v, here %q, blurb %q", tab.Paths, tab.Here, tab.Blurb)
	}
	if len(tab.Finds) != 1 || tab.Finds[0].Label != "Card borders" || !strings.Contains(tab.Finds[0].Words, "colour") {
		t.Errorf("finds = %+v", tab.Finds)
	}

	var buf bytes.Buffer
	if err := tab.Content.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`data-widget="hero-looks"`, `data-campaign-id="camp-1"`, `data-hl-frame`, `data-hl-insp`, `aria-label="Card border"`, `data-hl-dock`,
		"hero_creator.css", "hero_stage.css", "hero_borders.css", "hero_border_picker.css", "hero_looks.css"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab body is missing %s", want)
		}
	}
	if strings.Contains(out, "<script") {
		t.Error("a page template carries no script tag")
	}
	if strings.Contains(out, `data-widget="hero-creator"`) {
		t.Error("the creator is mounted by the page script, after the workshop's own scripts are loaded")
	}
	if got := attrOf(t, out, "data-plan-url"); got != "/campaigns/camp-1/characters/new/plan" {
		t.Errorf("plan url = %q (the owner's own plan, no ?as=)", got)
	}
	if got := attrOf(t, out, "data-tab-url"); got != "/campaigns/camp-1/customize/x/hero-creator" {
		t.Errorf("tab url = %q", got)
	}
	var lib map[string]map[string]any
	if err := json.Unmarshal([]byte(attrOf(t, out, "data-borders")), &lib); err != nil || len(lib) == 0 {
		t.Fatalf("data-borders is not the library: %v", err)
	}
	var pick struct {
		Groups []struct {
			Name string
			Keys []string
		} `json:"groups"`
		Borders map[string]struct{ Name, Colour string } `json:"borders"`
	}
	if err := json.Unmarshal([]byte(attrOf(t, out, "data-border-picker")), &pick); err != nil || len(pick.Groups) == 0 || len(pick.Borders) == 0 {
		t.Fatalf("data-border-picker is not the picker's JSON: %v (%+v)", err, pick)
	}
	for key := range pick.Borders {
		if _, ok := lib[key]; !ok {
			t.Errorf("picker border %q is not in the library", key)
		}
	}
}
