package dmscreen

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func render(t *testing.T, v *View) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Panel(v, "tok").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPanel(t *testing.T) {
	tests := []struct {
		name    string
		view    View
		want    []string
		notWant []string
	}{
		{
			name: "scribe sees downtime but can't switch it",
			view: View{CampaignID: "c1", Downtime: &DowntimeView{Open: false, CanToggle: false, Pending: 1}},
			want: []string{"Only the campaign owner can switch this.", "1 request waiting on you", "/campaigns/c1/armory/stashes"},
		},
		{
			name:    "no conditions hides the rules tab",
			view:    View{CampaignID: "c1"},
			want:    []string{`data-dms-tab="reveal"`, "Nothing hidden right now."},
			notWant: []string{`data-dms-tab="rules"`},
		},
		{
			name: "conditions show the rules tab first",
			view: View{CampaignID: "c1", Conditions: []ConditionView{{Name: "Bleeding", Text: "Lose stamina."}}},
			want: []string{`data-dms-tab="rules"`, `data-dms-cond="Bleeding"`, "Lose stamina."},
		},
		{
			name: "party without system meters says why",
			view: View{CampaignID: "c1", SystemName: "Draw Steel", Party: []HeroView{{Name: "Aria", PlayerName: "Sam"}}},
			want: []string{"Aria", "Sam", "Draw Steel doesn't fill in hero numbers yet."},
		},
		{
			name: "hidden rows post to the campaign's reveal route",
			view: View{CampaignID: "c1", Hidden: []HiddenView{{ID: "e9", Name: "Vosk"}}},
			want: []string{`/campaigns/c1/dm-screen/reveal/e9`, "Vosk"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := render(t, &tt.view)
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q", w)
				}
			}
		})
	}
}
