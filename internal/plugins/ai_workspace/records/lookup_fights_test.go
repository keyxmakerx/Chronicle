package records

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFightLookups(t *testing.T) {
	at := time.Now().Add(-3 * time.Hour)
	monsters := fakeMonsters{FoundryMonsterReport{ReportedAt: &at, Monsters: []FoundryMonster{
		{Name: "Stalker", Pack: "Monsters", Level: "3", Role: "Harrier", Organization: "Horde", EV: "5"},
		{Name: "Goblin Sniper", Pack: "Monsters", Level: "1", Role: "Artillery", Organization: "Horde", EV: "3"},
		{Name: "Captain Vex", Level: "4", Role: "Brute", Organization: "Leader"},
	}, Matches: []CreatureMatch{
		{PageName: "Stalker", MonsterName: "Stalker", MonsterPack: "Monsters", FoundryNumbers: true},
		{PageName: "Vex the Captain", MonsterName: "Captain Vex", Suggested: true},
	}}}
	road := EncounterRef{Name: "Road fight", PageName: "Lastwell",
		Creatures: []EncounterCreature{{Name: "Stalker", Count: 3, Group: "A", Start: "seen"}},
		Versions: []EncounterVersion{{Name: "Six", Test: "Road to Lastwell", Result: "Failure",
			Creatures: []EncounterCreature{{Name: "Stalker", Count: 6, Group: "A", Start: "seen"}, {Name: "Bone Wyrm", Count: 1, Group: "B", Start: "later"}}}},
		Open: []string{"Road to Lastwell"}}
	gate := EncounterRef{Name: "Gate", PageName: "Harrowmere", Playing: "Night",
		Creatures: []EncounterCreature{{EntityID: "gob", Name: "Goblin War Chief", Count: 1, Group: "A", Start: "hidden"}},
		Versions:  []EncounterVersion{{Name: "Night", Creatures: []EncounterCreature{{EntityID: "gob", Name: "Goblin War Chief", Count: 2, Group: "A", Start: "hidden"}}}}}
	l := &Lookups{
		Encounters: &fakeEncounters{byPage: map[string][]EncounterRef{"p1": {road}, "p2": {gate}}},
		Tests:      fakeTests{{Name: "Road to Lastwell", PageName: "Lastwell", Results: []TestResult{{Label: "Success"}, {Label: "Failure"}}, Recorded: "Failure"}},
		Monsters:   monsters,
	}
	tests := []struct {
		name    string
		rec     Record
		actor   Actor
		want    []string
		notWant []string
		wantErr string
	}{
		{name: "encounters as blocks", rec: lookup("encounters", nil), actor: owner, want: []string{
			"## Encounters (2)", "kind: encounter", `name: "Six"`, `test: "Road to Lastwell"`, `"6 Stalker, group A, starts seen"`,
			"Plays now: the base creatures, until Road to Lastwell has a result.", `Foundry can't place: "Bone Wyrm"`, `Plays now: "Night"`}},
		{name: "one page's encounters", rec: lookup("encounters", map[string]any{"page": "harrowmere"}), actor: owner,
			want: []string{"## Encounters (1)", `name: "Gate"`}, notWant: []string{"Road fight"}},
		{name: "encounters are the DM's", rec: lookup("encounters", nil), actor: player, wantErr: "Safe privacy"},
		{name: "tests", rec: lookup("tests", nil), actor: owner, want: []string{"- Road to Lastwell (on Lastwell): results Success, Failure; recorded: Failure"}},
		{name: "tests are the DM's", rec: lookup("tests", nil), actor: player, wantErr: "Safe privacy"},
		{name: "all monsters", rec: lookup("foundry-monsters", nil), actor: owner, want: []string{
			"## Foundry's monsters (3)", "3 hours ago", "- Stalker · level 3 · Horde · Harrier · EV 5 (in the Monsters compendium); it is your page Stalker", "- Captain Vex · level 4 · Leader · Brute (an actor in the world)\n",
				"## Creature pages matched to these (2)", "never write it a second stat block",
				"- Stalker is Foundry's Stalker (in the Monsters compendium) and plays with Foundry's numbers: keep the page to lore",
				"- Vex the Captain looks like Foundry's Captain Vex (an actor in the world); the DM hasn't said yet"}},
		{name: "by part of a name", rec: lookup("foundry-monsters", map[string]any{"name": "stalk"}), actor: owner,
			want: []string{"(1)", "Stalker"}, notWant: []string{"Goblin"}},
		{name: "by level and role", rec: lookup("foundry-monsters", map[string]any{"level": "1", "role": "horde"}), actor: owner,
			want: []string{"(1)", "Goblin Sniper"}, notWant: []string{"Stalker"}},
		{name: "monsters are the DM's", rec: lookup("foundry-monsters", nil), actor: player, wantErr: "Safe privacy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, tc.actor, []Record{tc.rec})[0]
			if tc.wantErr != "" {
				if !strings.Contains(ans.Error, tc.wantErr) {
					t.Fatalf("want error %q, got %+v", tc.wantErr, ans)
				}
				return
			}
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

func TestFoundryMonstersLookup_NoReport(t *testing.T) {
	l := &Lookups{Monsters: fakeMonsters{}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("foundry-monsters", nil)})[0]
	if ans.Error != "" || !strings.Contains(ans.Text, "hasn't reported any yet") {
		t.Fatalf("got %+v", ans)
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("encounters", nil)})[0]; !strings.Contains(got.Error, "can't be looked up") {
		t.Fatalf("no service: %+v", got)
	}
}

func TestFoundryMonsterReport_Find(t *testing.T) {
	r := FoundryMonsterReport{Monsters: []FoundryMonster{{Name: "Stalker", Pack: "Monsters"}, {Name: "stalker"}}}
	if m := r.Find("STALKER"); m == nil || m.Pack != "" {
		t.Fatalf("a world actor wins over a compendium entry, got %+v", m)
	}
	if r.Find("Wyrm") != nil {
		t.Fatal("no match must be nil")
	}
}
