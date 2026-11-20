package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A category's icon is drawn in front of a page's name in a page list row,
// hidden by the stylesheet until the campaign asks for icons beside names, and
// a card's own icon takes the campaign's icon colour theme instead of colouring
// itself by category. The category's colour reaches the markup only under the
// Category theme, and only as a plain #RRGGBB custom property.

func kindCtx(colour string) context.Context {
	return layouts.SetAppearance(context.Background(), &layouts.AppearanceData{IconColour: colour})
}

func TestEntityListWidget_CategoryIconInFrontOfEachName(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	pages := []Entity{{ID: "e1", Name: "Harbor Town"}, {ID: "e2", Name: "Saltmere"}}

	tests := []struct {
		name   string
		colour string
		et     *EntityType
		want   []string
		never  []string
		count  int // how many icons stand before names
	}{
		{"ink by default: the icon is there, hidden by CSS, with no colour", "",
			&EntityType{ID: 3, NamePlural: "Locations", Icon: "fa-map-pin", Color: "#ef4444"},
			[]string{`<i class="fa-solid fa-map-pin kind-ic" aria-hidden="true"></i>`}, []string{"--kind-colour", "#ef4444"}, 2},
		{"category theme passes the colour", "category",
			&EntityType{ID: 3, NamePlural: "Locations", Icon: "fa-map-pin", Color: "#EF4444"},
			[]string{`kind-ic" style="--kind-colour:#ef4444;"`}, nil, 2},
		{"a colour that is not plain is dropped", "category",
			&EntityType{ID: 3, NamePlural: "Locations", Icon: "fa-map-pin", Color: "red;background:url(x)"},
			[]string{`<i class="fa-solid fa-map-pin kind-ic" aria-hidden="true"></i>`}, []string{"url(", "red;"}, 2},
		{"a category with no icon draws none", "category",
			&EntityType{ID: 3, NamePlural: "Locations", Color: "#ef4444"}, nil, []string{"kind-ic", "--kind-colour"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := EntityListWidget(cc, tt.et, pages).Render(kindCtx(tt.colour), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			for _, n := range tt.never {
				if strings.Contains(out, n) {
					t.Errorf("has %q in %s", n, out)
				}
			}
			if got := strings.Count(out, "kind-ic"); got != tt.count {
				t.Errorf("%d icons before names, want %d", got, tt.count)
			}
		})
	}
}

func TestEntityCard_IconTakesTheThemeNotItsCategoryColour(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	e := &Entity{ID: "e1", Name: "Harbor Town", TypeName: "Location", TypeIcon: "fa-map-pin", TypeColor: "#ef4444"}

	for _, colour := range []string{"", "accent", "category"} {
		t.Run("theme "+colour, func(t *testing.T) {
			var buf bytes.Buffer
			if err := EntityCard(e, cc).Render(kindCtx(colour), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if strings.Count(out, "kind-themed") != 2 {
				t.Errorf("the band's icon and the pill's icon both take the theme: %s", out)
			}
			for rest := out; ; {
				i := strings.Index(rest, "<i ")
				if i < 0 {
					break
				}
				tag := rest[i : i+strings.Index(rest[i:], ">")]
				if strings.Contains(tag, "color:") && !strings.Contains(tag, "--kind-colour") {
					t.Errorf("an icon is still coloured by its category directly: %s", tag)
				}
				rest = rest[i+len(tag):]
			}
			if has := strings.Contains(out, "--kind-colour:#ef4444;"); has != (colour == "category") {
				t.Errorf("--kind-colour present = %v under theme %q", has, colour)
			}
		})
	}
}

func TestEntityTableRow_StandInIconIsUnboxedAndThemed(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	e := &Entity{ID: "e1", Name: "Harbor Town", TypeName: "Location", TypeIcon: "fa-map-pin", TypeColor: "#ef4444"}
	var buf bytes.Buffer
	if err := EntityTableRow(e, cc).Render(kindCtx("category"), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	i := strings.Index(out, `<span class="w-8 h-8 flex`)
	if i < 0 {
		t.Fatalf("the name's icon slot is missing: %s", out)
	}
	slot := out[i : i+strings.Index(out[i:], "</span>")]
	if strings.Contains(slot, "background") || strings.Contains(slot, "rounded") {
		t.Errorf("something is drawn behind the icon: %s", slot)
	}
	if !strings.Contains(slot, `kind-themed" style="--kind-colour:#ef4444;"`) {
		t.Errorf("the icon does not take the theme: %s", slot)
	}
}
