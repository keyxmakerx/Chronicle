package records

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeScenes struct{ rep *FoundryScenes }

func (f fakeScenes) FoundryScenes(context.Context, string) (*FoundryScenes, error) { return f.rep, nil }

func TestLookups_FoundryScenes(t *testing.T) {
	rep := &FoundryScenes{ReportedAt: time.Date(2026, 10, 25, 9, 30, 0, 0, time.UTC), Scenes: []FoundrySceneRef{
		{Name: "Dragon's Lair", Folder: "Act 2"},
		{Name: "Harrowmere Docks", Active: true},
		{Name: "Sunken Crypt", Folder: "Act 2 / Caves"},
	}}
	cases := []struct {
		name     string
		l        *Lookups
		a        Actor
		fields   map[string]any
		want     []string
		lacks    []string
		wantErr  string
		wantChip string
	}{
		{name: "every scene", l: &Lookups{Scenes: fakeScenes{rep}}, a: owner,
			want:     []string{"## Foundry scenes (3)", "2026-10-25 09:30 UTC", "- Dragon's Lair (in Act 2)", "- Harrowmere Docks (the scene players are on)"},
			wantChip: "Foundry scenes · 3"},
		{name: "a name narrows by name or folder", l: &Lookups{Scenes: fakeScenes{rep}}, a: owner, fields: map[string]any{"name": "act 2"},
			want: []string{"matching \"act 2\" (2 of 3)", "- Sunken Crypt"}, lacks: []string{"Harrowmere"}},
		{name: "no match says no map yet", l: &Lookups{Scenes: fakeScenes{rep}}, a: owner, fields: map[string]any{"name": "tavern"},
			want: []string{"(0 of 3)", "Foundry has no map of that yet"}},
		{name: "truncated report says so", l: &Lookups{Scenes: fakeScenes{&FoundryScenes{Truncated: true, ReportedAt: rep.ReportedAt}}}, a: owner,
			want: []string{"The Foundry world has no scenes.", "some are missing here"}},
		{name: "never reported", l: &Lookups{Scenes: fakeScenes{nil}}, a: owner,
			want: []string{"Foundry hasn't sent its scenes yet"}},
		{name: "a player is refused", l: &Lookups{Scenes: fakeScenes{rep}}, a: player, wantErr: "choose Permitted"},
		{name: "unwired", l: &Lookups{}, a: owner, wantErr: "can't be looked up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans := tc.l.Answer(context.Background(), camp, tc.a, []Record{lookup("scenes", tc.fields)})[0]
			if tc.wantErr != "" {
				if !strings.Contains(ans.Error, tc.wantErr) || ans.Text != "" {
					t.Fatalf("got %+v", ans)
				}
				return
			}
			if ans.Error != "" {
				t.Fatal(ans.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans.Text)
				}
			}
			for _, w := range tc.lacks {
				if strings.Contains(ans.Text, w) {
					t.Errorf("answer has %q:\n%s", w, ans.Text)
				}
			}
			if tc.wantChip != "" && ans.Chip != tc.wantChip {
				t.Errorf("chip %q", ans.Chip)
			}
		})
	}
	if !strings.Contains(LookupDoc(), "`what: scenes`") {
		t.Error("LookupDoc lacks the scenes lookup")
	}
}
