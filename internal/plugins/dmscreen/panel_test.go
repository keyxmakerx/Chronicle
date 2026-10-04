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
			name:    "scribe gets no confirm and nothing that posts",
			view:    View{CampaignID: "c1", Downtime: &DowntimeView{CanToggle: false}},
			notWant: []string{"data-dms-confirm", "armory/downtime"},
		},
		{
			name: "owner's switch only asks; the confirm's yes button posts",
			view: View{CampaignID: "c1", Downtime: &DowntimeView{Open: false, CanToggle: true, Pending: 2}},
			want: []string{"data-dms-ask", "data-dms-confirm", "Start downtime? 2 waiting requests go through now and shops open.",
				`hx-post="/campaigns/c1/armory/downtime"`, `hx-vals="{&#34;open&#34;:&#34;true&#34;}"`, "data-dms-cancel"},
		},
		{
			name: "heroes start folded with the bar and resource chip on the line",
			view: View{CampaignID: "c1", PartyFilled: true, Party: []HeroView{{
				Name: "Aria", PlayerName: "Sam", Subtitle: "Tactician", Conditions: []string{"Bleeding", "Slowed"}, Meters: []MeterView{
					{Label: "Stamina", Current: "34", Max: "42", HasMax: true, Percent: 81},
					{Label: "Recoveries", Current: "6", Max: "8", HasMax: true, Percent: 75},
					{Label: "Focus", Current: "3"},
				}}}},
			want: []string{`aria-expanded="false"`, `data-dms-hrow`, `<span class="dms-chip">Focus 3</span>`, "34/42", "Played by Sam", "Recoveries", "data-dms-party", "Tactician", `title="Bleeding, Slowed"`, "<span>Slowed</span>"},
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

func TestHeroView_Folded(t *testing.T) {
	stam := MeterView{Label: "Stamina", HasMax: true}
	rec := MeterView{Label: "Recoveries", HasMax: true}
	res := MeterView{Label: "Focus"}
	ac := MeterView{Label: "AC"}
	tests := []struct {
		name      string
		meters    []MeterView
		wantBar   string
		wantChips int
		wantRest  int
	}{
		{"draw steel: stamina bar, resource chip, recoveries fold away", []MeterView{stam, rec, res}, "Stamina", 1, 1},
		{"5e: hit points bar, armour class chip", []MeterView{stam, ac}, "Stamina", 1, 0},
		{"chips only: no bar", []MeterView{res, ac}, "", 2, 0},
		{"no meters", nil, "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar, chips, rest := HeroView{Meters: tt.meters}.Folded()
			got := ""
			if bar != nil {
				got = bar.Label
			}
			if got != tt.wantBar || len(chips) != tt.wantChips || len(rest) != tt.wantRest {
				t.Fatalf("bar=%q chips=%d rest=%d, want %q %d %d", got, len(chips), len(rest), tt.wantBar, tt.wantChips, tt.wantRest)
			}
		})
	}
}

func TestDowntimeConfirm(t *testing.T) {
	tests := []struct {
		name     string
		d        DowntimeView
		wantText string
		wantYes  string
	}{
		{"open ends it", DowntimeView{Open: true}, "End downtime? Moves will need your OK again and shops close.", "End downtime"},
		{"nothing waiting", DowntimeView{}, "Start downtime? Moves will happen at once and shops open.", "Start downtime"},
		{"one waiting", DowntimeView{Pending: 1}, "Start downtime? 1 waiting request goes through now and shops open.", "Start downtime"},
		{"several waiting", DowntimeView{Pending: 3}, "Start downtime? 3 waiting requests go through now and shops open.", "Start downtime"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, yes := tc.d.Confirm()
			if text != tc.wantText || yes != tc.wantYes {
				t.Fatalf("got %q / %q", text, yes)
			}
		})
	}
}
