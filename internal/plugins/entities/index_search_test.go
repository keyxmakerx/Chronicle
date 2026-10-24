package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The All Pages search box must filter #entity-list in place: a plain form
// submit to the search endpoint rendered its bare fragment as a whole page.
func TestEntityListContent_SearchFiltersInPlace(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe}
	tests := []struct {
		name    string
		query   string
		want    []string
		notWant []string
	}{
		{
			name:  "no query",
			query: "",
			want: []string{
				`action="/campaigns/c1/entities"`,
				`hx-get="/campaigns/c1/entities"`,
				`hx-target="#entity-list"`,
				`id="entity-list-search"`,
				`Create Your First Page`,
			},
			notWant: []string{`/entities/search`},
		},
		{
			name:    "query echoed and empty state explains",
			query:   "harbor",
			want:    []string{`value="harbor"`, `0 pages match`, `Nothing matches that search`},
			notWant: []string{`Create Your First Page`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultListOptions()
			opts.Query = tt.query
			var b strings.Builder
			if err := EntityListContent(cc, nil, nil, map[int]int{}, 0, opts, 0, "", "tok").Render(context.Background(), &b); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := b.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %s", w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(out, w) {
					t.Errorf("unexpected %s", w)
				}
			}
		})
	}
}

func TestEntityListParams(t *testing.T) {
	tests := []struct {
		typeID int
		query  string
		want   string
	}{
		{0, "", "type=0"},
		{3, "old harbor", "q=old+harbor&type=3"},
		{0, "a&b", "q=a%26b&type=0"},
	}
	for _, tt := range tests {
		if got := entityListParams(tt.typeID, tt.query); got != tt.want {
			t.Errorf("entityListParams(%d, %q) = %q, want %q", tt.typeID, tt.query, got, tt.want)
		}
	}
}
