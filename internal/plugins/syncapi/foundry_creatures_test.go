package syncapi

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestFoundryCreaturesFragment(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	tests := []struct {
		name    string
		view    FoundryCreaturesView
		want    []string
		notWant []string
	}{
		{name: "not reported", view: FoundryCreaturesView{CampaignID: "c1"}, want: []string{"hasn't sent its monster list yet"}},
		{name: "look-alikes first, with buttons", view: FoundryCreaturesView{CampaignID: "c1", Matches: FoundryMatches{ReportedAt: &at, Total: 3, Matches: []CreatureMatch{
			{EntityID: "p1", EntityName: "Stalker", State: MatchMatched, Use: MatchUseFoundry, Monster: &FoundryMonster{Name: "Stalker", Pack: "Monsters"}},
			{EntityID: "p2", EntityName: "Wyrm", State: MatchNone},
			{EntityID: "p3", EntityName: "Goblin Snipers", State: MatchSuggested, Monster: &FoundryMonster{Name: "Goblin Sniper", Pack: "Monsters"}},
		}}}, want: []string{
			"Matched &middot; Foundry&#39;s numbers", "Foundry&#39;s Stalker, in the Monsters compendium", "Yours only &middot; sync makes its actor",
			"Looks like Foundry's Goblin Sniper", `hx-post="/campaigns/c1/foundry/creatures/p3"`, ">Match</button>", ">Not the same</button>",
			"Your AI sees each match",
		}, notWant: []string{"Showing the first"}},
		{name: "a refused change is said", view: FoundryCreaturesView{CampaignID: "c1", Error: "Foundry did not report a monster by that name there", Matches: FoundryMatches{ReportedAt: &at}},
			want: []string{`role="alert"`, "did not report", "no creature pages yet"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := FoundryCreaturesFragment(tc.view).Render(context.Background(), &b); err != nil {
				t.Fatal(err)
			}
			html := b.String()
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in:\n%s", w, html)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q", w)
				}
			}
			if tc.name == "look-alikes first, with buttons" && strings.Index(html, "Goblin Snipers") > strings.Index(html, ">Stalker<") {
				t.Error("the look-alike waiting on the owner comes first")
			}
		})
	}
}
