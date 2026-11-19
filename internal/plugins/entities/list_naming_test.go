package entities

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var (
	tabTitleRe = regexp.MustCompile(`<title>([^<]*)</title>`)
	headingRe  = regexp.MustCompile(`<h1[^>]*>([^<]*)</h1>`)
)

// The list's browser tab is named as its heading is, the campaign after it.
// An address that names no category this viewer can see is All pages in both,
// never an empty heading, so a hidden category reads like one never made.
func TestEntityIndexPage_TabAndHeadingNameTheList(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Sunken Crown"}, MemberRole: campaigns.RolePlayer}
	types := []EntityType{{ID: 1, Slug: "places", Name: "Place", NamePlural: "Places", Enabled: true}}
	tests := []struct {
		name string
		slug string
		want string
	}{
		{"all pages", "", "All pages"},
		{"a category the viewer can see", "places", "Places"},
		{"an address that names no such category", "vaults", "All pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := EntityIndexPage(cc, nil, types, map[int]int{}, 0, DefaultListOptions(), 0, tt.slug, "tok").Render(context.Background(), &b); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := b.String()
			title := tabTitleRe.FindStringSubmatch(out)
			if title == nil || !strings.HasPrefix(title[1], tt.want+" - The Sunken Crown | ") {
				t.Errorf("tab title = %q, want it to start %q", title, tt.want+" - The Sunken Crown | ")
			}
			heading := headingRe.FindStringSubmatch(out)
			if heading == nil || strings.TrimSpace(heading[1]) != tt.want {
				t.Errorf("heading = %q, want %q", heading, tt.want)
			}
			if title != nil && strings.Contains(title[1], "Entities") {
				t.Errorf(`the tab still says "Entities": %q`, title[1])
			}
		})
	}
}

// The search page's count is worded by its script (resultsLabel), which says
// "1 result"; the template must not put a fixed "results" after a number.
func TestSearchPage_CountLineIsWordedByItsScript(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Sunken Crown"}}
	var b strings.Builder
	if err := SearchPage(cc, nil).Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `x-text="resultsLabel()"`) {
		t.Error("the count line does not ask the script for its words")
	}
	if regexp.MustCompile(`x-text="total"></span>\s*results`).MatchString(out) {
		t.Error(`the count line still writes a fixed "results" after the number`)
	}
}
