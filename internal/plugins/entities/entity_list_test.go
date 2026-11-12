package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderEntityList(t *testing.T, cc *campaigns.CampaignContext, et *EntityType, pages []Entity) string {
	t.Helper()
	var buf bytes.Buffer
	if err := EntityListWidget(cc, et, pages).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestEntityListWidget(t *testing.T) {
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	player := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	et := &EntityType{ID: 3, NamePlural: "Locations"}
	pages := []Entity{{ID: "e1", Name: "Harbor Town"}}

	tests := []struct {
		name    string
		cc      *campaigns.CampaignContext
		et      *EntityType
		pages   []Entity
		want    string
		notWant string
	}{
		{"lists pages with a link", player, et, pages, "/campaigns/c1/entities/e1", ""},
		{"names the category", player, et, pages, "Locations", ""},
		{"no category: owner is told", owner, nil, nil, "Pick a category", ""},
		{"no category: player sees nothing", player, nil, nil, "", "Pick a category"},
		{"empty category: player sees nothing", player, et, nil, "", "No Locations"},
		{"empty category: owner is told", owner, et, nil, "No Locations yet", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderEntityList(t, tt.cc, tt.et, tt.pages)
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Errorf("output missing %q:\n%s", tt.want, out)
			}
			if tt.notWant != "" && strings.Contains(out, tt.notWant) {
				t.Errorf("output has %q:\n%s", tt.notWant, out)
			}
		})
	}
}
