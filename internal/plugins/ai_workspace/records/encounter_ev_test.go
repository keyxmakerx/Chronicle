package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestEncounterEV(t *testing.T) {
	creature := entities.EntityType{ID: 5, Name: "Creature", Fields: []entities.FieldDefinition{{Key: "level"}, {Key: "ev"}, {Key: "organization"}}}
	plain := entities.EntityType{ID: 6, Name: "Creature", Fields: []entities.FieldDefinition{{Key: "challenge_rating"}}}
	mk := func(id, name string, fields map[string]any) entities.Entity {
		p := page(id, name, "Creature", false)
		p.EntityTypeID, p.FieldsData = 5, fields
		return p
	}
	pages := []entities.Entity{
		mk("chief", "Goblin War Chief", map[string]any{"ev": float64(12), "organization": "leader"}),
		mk("sold", "Hobgoblin Soldier", map[string]any{"ev": "4", "organization": "platoon"}),
		mk("run", "Goblin Runner", map[string]any{"ev": float64(3), "organization": "Minion"}),
		mk("ogre", "Ogre", map[string]any{"organization": "elite"}),
	}
	fight := EncounterRef{Name: "Ramparts", PageName: "Harrowmere",
		Creatures: []EncounterCreature{
			{EntityID: "chief", Name: "Goblin War Chief", Count: 1},
			{EntityID: "sold", Name: "Hobgoblin Soldier", Count: 2},
			{EntityID: "run", Name: "Goblin Runner", Count: 6},
			{EntityID: "ogre", Name: "Ogre", Count: 1},
			{Name: "Bone Wyrm", Count: 1},
			{EntityID: "gone", Name: "Lost Page", Count: 1},
		},
		Versions: []EncounterVersion{{Name: "Walls held", Creatures: []EncounterCreature{{EntityID: "sold", Name: "Hobgoblin Soldier", Count: 3}}}}}
	tests := []struct {
		name    string
		types   []entities.EntityType
		want    []string
		notWant []string
	}{
		{name: "adds up only stored EV and names the rest", types: []entities.EntityType{creature},
			want: []string{
				"EV lines add up the EV written on each creature page",
				"not the party's budget",
				"EV, added up from the creature pages: 20 (1 × Goblin War Chief at EV 12, 2 × Hobgoblin Soldier at EV 4).",
				`Not counted, no page: "Bone Wyrm".`,
				`Not counted, no EV on the page: "Ogre".`,
				`Not counted, the page could not be read: "Lost Page".`,
				"Not counted, minions, whose page EV is for a set of them rather than each one",
				"6 Goblin Runner (EV 3 on its page)",
				`EV of "Walls held", added up from the creature pages: 12 (3 × Hobgoblin Soldier at EV 4).`,
			}},
		{name: "a system with no EV field gets no EV lines", types: []entities.EntityType{plain},
			notWant: []string{"EV"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lookups{
				Encounters: &fakeEncounters{byPage: map[string][]EncounterRef{"p1": {fight}}},
				Pages:      &fakePages{all: pages, types: tc.types},
			}
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup("encounters", nil)})[0]
			if ans.Error != "" {
				t.Fatalf("unexpected error %q", ans.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("missing %q in:\n%s", w, ans.Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(ans.Text, w) {
					t.Errorf("unexpected %q in:\n%s", w, ans.Text)
				}
			}
		})
	}
}

func TestEvOf(t *testing.T) {
	tests := []struct {
		in   any
		want float64
		ok   bool
	}{
		{float64(8), 8, true},
		{" 6 ", 6, true},
		{"", 0, false},
		{"lots", 0, false},
		{float64(0), 0, false},
		{nil, 0, false},
		{[]any{1}, 0, false},
	}
	for _, tc := range tests {
		got, ok := evOf(map[string]any{"ev": tc.in})
		if got != tc.want || ok != tc.ok {
			t.Errorf("evOf(%v) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
