package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A type tab's href is what opens in a new tab or without JS, so it must be
// the category route, which resolves the bare type slug. A pluralised slug
// never matches a type and, for "characters", lands on the Cast page instead.
func TestEntityListContent_TypeTabHrefUsesTypeSlug(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	types := []EntityType{
		{ID: 1, Slug: "character", NamePlural: "Characters", Enabled: true},
		{ID: 2, Slug: "location", NamePlural: "Locations", Enabled: true},
	}
	var b strings.Builder
	if err := EntityListContent(cc, nil, types, map[int]int{}, 0, DefaultListOptions(), 0, "", "tok").Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()
	tests := []struct{ want, notWant string }{
		{`href="/campaigns/c1/character"`, `href="/campaigns/c1/characters"`},
		{`href="/campaigns/c1/location"`, `href="/campaigns/c1/locations"`},
	}
	for _, tt := range tests {
		if !strings.Contains(out, tt.want) {
			t.Errorf("missing %s", tt.want)
		}
		if strings.Contains(out, tt.notWant) {
			t.Errorf("still links %s", tt.notWant)
		}
	}
}
