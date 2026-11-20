package systems

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func ashbornProps(emberText string, emberCost float64, flare bool) map[string]any {
	traits := []any{
		map[string]any{"name": "Cinder Step", "cost": 1.0, "description": "Leave embers behind you."},
		map[string]any{"name": "Ember Sight", "cost": emberCost, "description": emberText},
	}
	if flare {
		traits = append(traits, map[string]any{"name": "Flare", "cost": 2.0, "description": "You flare."})
	}
	return map[string]any{"ancestry_points": 3.0, "purchased_traits": traits}
}

const emberTen = "You always know where living creatures are within 10 squares."

// tellPeople: Sable Varr (a hero) holds Ember Sight and Cinder Step; Old
// Marrow is no hero; Gimli is of another people.
func tellPeople() fakePeople {
	return fakePeople{people: []fakePerson{
		{EntryPerson: EntryPerson{ID: "sable", Name: "Sable Varr", Hero: true, Value: "Ashborn", Traits: []string{"Cinder Step", "Ember Sight"}}, field: "ancestry", owner: "pl"},
		{EntryPerson: EntryPerson{ID: "marrow", Name: "Old Marrow", Value: "Ashborn", Traits: []string{"Flare"}}, field: "ancestry"},
		{EntryPerson: EntryPerson{ID: "gimli", Name: "Gimli", Hero: true, Value: "Dwarf", Traits: []string{"Ember Sight"}}, field: "ancestry", owner: "pl"},
	}}
}

func TestTellQuestion(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	svc.people = tellPeople()
	ctx := context.Background()
	ash := entryByName(t, repo, "Ashborn")
	base := func() *SystemEntry {
		e := ash
		e.Properties = ashbornProps(emberTen, 2, true)
		return &e
	}
	with := func(f func(e *SystemEntry)) *SystemEntry { e := base(); f(e); return e }
	tests := []struct {
		name    string
		actor   EntryActor
		before  *SystemEntry
		after   *SystemEntry
		holders string
		line    string
	}{
		{"a number in a held trait's text", dirActor, base(),
			with(func(e *SystemEntry) { e.Properties = ashbornProps(strings.Replace(emberTen, "10", "5", 1), 2, true) }),
			"Sable Varr holds this trait.", "Ember Sight: 5 squares, was 10"},
		{"a held trait reworded", dirActor, base(),
			with(func(e *SystemEntry) { e.Properties = ashbornProps("You see warmth through walls.", 2, true) }),
			"Sable Varr holds this trait.", "Ember Sight reworded"},
		{"a held trait's cost", dirActor, base(),
			with(func(e *SystemEntry) { e.Properties = ashbornProps(emberTen, 1, true) }),
			"Sable Varr holds this trait.", "Ember Sight now costs 1 point, was 2"},
		{"a held trait removed", dirActor, base(),
			with(func(e *SystemEntry) {
				e.Properties = map[string]any{"purchased_traits": []any{map[string]any{"name": "Cinder Step", "cost": 1.0, "description": "Leave embers behind you."}}}
			}),
			"Sable Varr holds this trait.", "Ember Sight removed"},
		{"a trait only a non-hero holds", dirActor, base(),
			with(func(e *SystemEntry) { e.Properties = ashbornProps(emberTen, 2, false) }), "", ""},
		{"a trait added", dirActor, with(func(e *SystemEntry) { e.Properties = ashbornProps(emberTen, 2, false) }), base(), "", ""},
		{"only the story changed", dirActor, base(), with(func(e *SystemEntry) { e.Description = "<p>New story.</p>" }), "", ""},
		{"the entry is Directors only", dirActor, with(func(e *SystemEntry) { e.Visibility = EntryVisibilityDirectors }),
			with(func(e *SystemEntry) {
				e.Visibility = EntryVisibilityDirectors
				e.Properties = ashbornProps("Changed.", 2, true)
			}), "", ""},
		{"players did not read it before", dirActor, with(func(e *SystemEntry) { e.Visibility = EntryVisibilityDirectors }),
			with(func(e *SystemEntry) { e.Properties = ashbornProps("Changed.", 2, true) }), "", ""},
		{"a player's own draft", dirActor, with(func(e *SystemEntry) { e.Status = EntryStatusSent }),
			with(func(e *SystemEntry) { e.Status = EntryStatusSent; e.Properties = ashbornProps("Changed.", 2, true) }), "", ""},
		{"not a Director", playerActor, base(), with(func(e *SystemEntry) { e.Properties = ashbornProps("Changed.", 2, true) }), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := svc.tellQuestion(ctx, "c1", tt.actor, 3, tt.before, tt.after)
			if err != nil {
				t.Fatal(err)
			}
			if tt.holders == "" {
				if q != nil {
					t.Fatalf("asked %+v, want nothing", q)
				}
				return
			}
			if q == nil || q.Holders != tt.holders || q.Line != tt.line {
				t.Fatalf("question = %+v\nwant holders %q line %q", q, tt.holders, tt.line)
			}
		})
	}
}

// End to end: the save stands and comes back with the question; a hero who
// stored an old name still counts.
func TestUpdateTelling(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	svc.people = tellPeople()
	ctx := context.Background()
	ash := entryByName(t, repo, "Ashborn")
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Properties: patch.Of(ashbornProps(emberTen, 2, true))}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("Emberkin")}); err != nil {
		t.Fatal(err)
	}
	e, q, err := svc.UpdateTelling(ctx, "c1", dirActor, 3, ash.ID,
		UpdateSystemEntryInput{Properties: patch.Of(ashbornProps(strings.Replace(emberTen, "10", "5", 1), 2, true))})
	if err != nil || e == nil {
		t.Fatalf("save: %v", err)
	}
	if q == nil || q.Line != "Ember Sight: 5 squares, was 10" || strings.Join(q.Heroes, ",") != "Sable Varr" {
		t.Fatalf("question = %+v", q)
	}
	// Saving the same again asks nothing: no trait changed.
	if _, q, _ := svc.UpdateTelling(ctx, "c1", dirActor, 3, ash.ID, UpdateSystemEntryInput{Summary: patch.Of("Fire folk.")}); q != nil {
		t.Fatalf("asked on a summary change: %+v", q)
	}
}

func TestNumberChange(t *testing.T) {
	tests := []struct {
		was, now, want string
		ok             bool
	}{
		{"within 10 squares.", "within 5 squares.", "5 squares, was 10", true},
		{"deal 2 damage", "deal 3 damage", "3 damage, was 2", true},
		{"a +1 bonus", "a +2 bonus", "2 bonus, was 1", true},
		{"range 10", "range 12", "12, was 10", true},
		{"within 10 squares and 2 turns", "within 5 squares and 1 turns", "", false},
		{"within 10 squares", "within 10 feet", "", false},
		{"same 10", "same 10", "", false},
	}
	for _, tt := range tests {
		got, ok := numberChange(tt.was, tt.now)
		if got != tt.want || ok != tt.ok {
			t.Errorf("numberChange(%q, %q) = %q %v, want %q %v", tt.was, tt.now, got, ok, tt.want, tt.ok)
		}
	}
}

func TestToldRecently(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	ash := entryByName(t, repo, "Ashborn")
	if _, err := svc.AddNote(ctx, "c1", dirActor, ash.ID, "Ember Sight: 5 squares, was 10"); err != nil {
		t.Fatal(err)
	}
	l, err := svc.Catalog(ctx, "c1", "ancestries", playerActor)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range l.Entries {
		if e.ToldRecently != (e.Name == "Ashborn") {
			t.Fatalf("%s toldRecently = %v", e.Name, e.ToldRecently)
		}
	}
	// An old note no longer marks it.
	repo.notes[0].CreatedAt = repo.notes[0].CreatedAt.Add(-ToldWindow - 1)
	l, _ = svc.Catalog(ctx, "c1", "ancestries", playerActor)
	for _, e := range l.Entries {
		if e.ToldRecently {
			t.Fatalf("%s still marked after two weeks", e.Name)
		}
	}
}
