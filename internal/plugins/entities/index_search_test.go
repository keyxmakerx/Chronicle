package entities

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The All Pages search box must filter #entity-list in place: a plain form
// submit to the search endpoint rendered its bare fragment as a whole page.
func TestEntityListContent_SearchFiltersInPlace(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe}
	one := []Entity{{ID: "e1", CampaignID: "c1", Name: "Old Harbor", TypeName: "Location", UpdatedAt: time.Now()}}
	tests := []struct {
		name     string
		query    string
		entities []Entity
		total    int
		want     []string
		notWant  []string
	}{
		{
			name:     "a list has the find field wired to swap in place",
			entities: one,
			total:    1,
			want: []string{
				`action="/campaigns/c1/entities"`,
				`hx-get="/campaigns/c1/entities"`,
				`hx-target="#entity-list"`,
				`id="entity-list-search"`,
				`hx-include="#entity-list-sort"`,
			},
			notWant: []string{`/entities/search`},
		},
		{
			name:    "query echoed and the empty result explains",
			query:   "harbor",
			want:    []string{`value="harbor"`, `0 pages match`, `Nothing matches that search`},
			notWant: []string{`Create Your First Page`, `None has been made yet`},
		},
		{
			name:    "no pages and no search is the bare opening, with no tools",
			want:    []string{`data-app-opening="empty"`},
			notWant: []string{`id="entity-list-search"`, `Create Your First Page`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultListOptions()
			opts.Query = tt.query
			var b strings.Builder
			if err := EntityListContent(cc, tt.entities, nil, map[int]int{}, tt.total, opts, 0, "", "tok").Render(context.Background(), &b); err != nil {
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
