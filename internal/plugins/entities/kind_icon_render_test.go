package entities

import (
	"bytes"
	"context"
	"github.com/a-h/templ"
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

// The list's row and tile name a page by its words and its own picture; neither
// draws a category icon, so there is nothing for a category colour to tint.
func TestEntityRowAndTile_DrawNoCategoryIcon(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	e := &Entity{ID: "e1", Name: "Harbor Town", TypeName: "Location", TypeIcon: "fa-map-pin", TypeColor: "#ef4444"}

	parts := map[string]templ.Component{"row": EntityRow(e, cc), "tile": EntityCard(e, cc)}
	for name, part := range parts {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := part.Render(kindCtx("category"), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if strings.Contains(out, "fa-map-pin") || strings.Contains(out, "#ef4444") {
				t.Errorf("the %s draws its category's icon or colour: %s", name, out)
			}
		})
	}
}
