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
	// Every case reads the report as one day old unless it sets its own clock.
	fresh := func() time.Time { return rep.ReportedAt.Add(26 * time.Hour) }
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
		{name: "every scene", l: &Lookups{Scenes: fakeScenes{rep}, Now: fresh}, a: owner,
			want:     []string{"## Foundry scenes (3)", "2026-10-25 09:30 UTC (1 day ago)", "- Dragon's Lair (in Act 2)", "- Harrowmere Docks (the scene players are on)"},
			wantChip: "Foundry scenes · 3"},
		{name: "a name narrows by name or folder", l: &Lookups{Scenes: fakeScenes{rep}, Now: fresh}, a: owner, fields: map[string]any{"name": "act 2"},
			want: []string{"matching \"act 2\" (2 of 3)", "- Sunken Crypt"}, lacks: []string{"Harrowmere"}},
		{name: "no match says no map yet", l: &Lookups{Scenes: fakeScenes{rep}, Now: fresh}, a: owner, fields: map[string]any{"name": "tavern"},
			want: []string{"(0 of 3)", "Foundry has no map of that yet"}},
		{name: "truncated report says so", l: &Lookups{Scenes: fakeScenes{&FoundryScenes{Truncated: true, ReportedAt: rep.ReportedAt}}}, a: owner,
			want: []string{"The Foundry world has no scenes.", "some are missing here"}},
		{name: "never reported", l: &Lookups{Scenes: fakeScenes{nil}}, a: owner,
			want: []string{"Foundry hasn't sent its scenes yet"}},
		{name: "a player is refused", l: &Lookups{Scenes: fakeScenes{rep}, Now: fresh}, a: player, wantErr: "choose Permitted"},
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
	if !strings.Contains((&Lookups{}).LookupDoc(), "`what: scenes`") {
		t.Error("LookupDoc lacks the scenes lookup")
	}
}

// A report is dated in days, and one older than three days comes with a plain
// warning that Foundry may have changed since, which says what the stamp means.
func TestLookups_FoundryScenesStaleness(t *testing.T) {
	reported := time.Date(2026, 10, 25, 9, 30, 0, 0, time.UTC)
	rep := &FoundryScenes{ReportedAt: reported, Scenes: []FoundrySceneRef{{Name: "Harrowmere Docks"}}}
	cases := []struct {
		name  string
		age   time.Duration
		want  []string
		lacks []string
	}{
		{name: "minutes old", age: 5 * time.Minute, want: []string{"(less than a day ago)"}, lacks: []string{"Warning"}},
		{name: "from the future", age: -time.Hour, want: []string{"(less than a day ago)"}, lacks: []string{"Warning"}},
		{name: "two days", age: 49 * time.Hour, want: []string{"(2 days ago)"}, lacks: []string{"Warning"}},
		{name: "exactly three days is not yet stale", age: 72 * time.Hour, want: []string{"(3 days ago)"}, lacks: []string{"Warning"}},
		{name: "just past three days", age: 72*time.Hour + time.Minute, want: []string{
			"(3 days ago)",
			"Warning: Foundry hasn't connected or synced in 3 days, so this list may be out of date.",
			"updates each time the GM connects and whenever scenes change; it does not mean Foundry is connected now",
		}},
		{name: "weeks", age: 40 * 24 * time.Hour, want: []string{"(40 days ago)", "hasn't connected or synced in 40 days"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lookups{Scenes: fakeScenes{rep}, Now: func() time.Time { return reported.Add(tc.age) }}
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup("scenes", nil)})[0]
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
		})
	}
}
