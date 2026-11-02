package dmscreen

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestView_Sections(t *testing.T) {
	tests := []struct {
		name     string
		view     View
		wantIDs  string
		wantSums map[string]string
		badge    int
	}{
		{
			name:     "a bare campaign gets the sections that always have something to say",
			view:     View{CampaignID: "c1"},
			wantIDs:  "world party reveal",
			wantSums: map[string]string{"world": "Game state", "party": "No player characters yet", "reveal": "Nothing hidden right now"},
		},
		{
			name: "every source adds its section, in rail order",
			view: View{CampaignID: "c1", Fight: templ.Raw(""), Homebrew: templ.Raw(""),
				Conditions: []ConditionView{{Name: "Bleeding"}, {Name: "Slowed"}},
				Notes:      &NotesView{Label: "Tonight"},
				Adventures: &AdventuresView{Items: []AdventureItem{{ID: "i1"}}}},
			wantIDs:  "world fight party rules reveal notes adventures homebrew",
			wantSums: map[string]string{"rules": "2 conditions", "notes": "Tonight", "adventures": "1 card"},
		},
		{
			name: "waiting requests lead the world summary and badge it",
			view: View{CampaignID: "c1", World: &WorldView{DateLabel: "3 Frost", TimeLabel: "dusk"},
				Requests: &RequestsView{Items: []RequestView{{}, {}}, More: 3}},
			wantIDs:  "world party reveal",
			wantSums: map[string]string{"world": "5 requests waiting · 3 Frost · dusk"},
			badge:    5,
		},
		{
			name:     "with no request list the armory's pending count badges it",
			view:     View{CampaignID: "c1", Downtime: &DowntimeView{Pending: 1}},
			wantIDs:  "world party reveal",
			wantSums: map[string]string{"world": "1 request waiting · Game state"},
			badge:    1,
		},
		{
			name:     "more hidden than listed says so",
			view:     View{CampaignID: "c1", Hidden: []HiddenView{{ID: "e1"}}, HiddenMore: true},
			wantIDs:  "world party reveal",
			wantSums: map[string]string{"reveal": "More than 1 character hidden"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secs := tt.view.Sections()
			ids := make([]string, len(secs))
			byID := map[string]SectionView{}
			for i, s := range secs {
				ids[i] = s.ID
				byID[s.ID] = s
				if s.Title == "" || s.Icon == "" || s.Summary == "" {
					t.Errorf("section %q is missing a title, icon or summary: %+v", s.ID, s)
				}
			}
			if got := strings.Join(ids, " "); got != tt.wantIDs {
				t.Errorf("sections = %q, want %q", got, tt.wantIDs)
			}
			for id, want := range tt.wantSums {
				if got := byID[id].Summary; got != want {
					t.Errorf("%s summary = %q, want %q", id, got, want)
				}
			}
			if got := byID["world"].Badge; got != tt.badge {
				t.Errorf("world badge = %d, want %d", got, tt.badge)
			}
		})
	}
}
