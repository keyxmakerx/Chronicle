package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

type fakeAddonSwitch struct {
	on  map[string]bool
	err error
}

func (f fakeAddonSwitch) IsEnabledForCampaign(_ context.Context, _, slug string) (bool, error) {
	return f.on[slug], f.err
}

func TestAIFeatureGate(t *testing.T) {
	cases := []struct {
		name string
		sw   fakeAddonSwitch
		want map[string]string
	}{
		{"all on", fakeAddonSwitch{on: map[string]bool{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, "maps": true, entity_notes.AddonSlug: true, crafting.AddonSlug: true, armory.AddonSlug: true, transformations.AddonSlug: true, decks.AddonSlug: true, cutscenes.AddonSlug: true}}, map[string]string{}},
		{"new campaign", fakeAddonSwitch{on: map[string]bool{}}, map[string]string{
			records.KindCalendar: "Calendar", "event": "Calendar", "weather": "Calendar", "party-place": "Calendar", "roll-night": "Calendar", "moon": "Calendar",
			"game-night": "Game nights", "map": "Maps", "hex-map": "Maps", "pin": "Maps", records.KindPlayerNote: "Player Notes",
			"recipe": "Crafting", "material": "Crafting", "drops": "Crafting", "bench": "Crafting",
			"shop-room": "Armory", "storefront": "Armory", "item-look": "Armory", "layer": "Transformations",
			"deck": "Card decks", "card": "Card decks", cutscenes.KindName: "Cut scenes",
		}},
		{"calendar off, game nights on", fakeAddonSwitch{on: map[string]bool{sessions.SessionsAddonSlug: true, "maps": true, entity_notes.AddonSlug: true, crafting.AddonSlug: true, armory.AddonSlug: true, transformations.AddonSlug: true, decks.AddonSlug: true, cutscenes.AddonSlug: true}}, map[string]string{
			records.KindCalendar: "Calendar", "event": "Calendar", "weather": "Calendar", "party-place": "Calendar", "roll-night": "Calendar", "moon": "Calendar",
			"game-night": "Calendar",
		}},
		{"card decks off, everything else on", fakeAddonSwitch{on: map[string]bool{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, "maps": true, entity_notes.AddonSlug: true, crafting.AddonSlug: true, armory.AddonSlug: true, transformations.AddonSlug: true, cutscenes.AddonSlug: true}}, map[string]string{
			"deck": "Card decks", "card": "Card decks",
		}},
		{"cut scenes off, everything else on", fakeAddonSwitch{on: map[string]bool{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, "maps": true, entity_notes.AddonSlug: true, crafting.AddonSlug: true, armory.AddonSlug: true, transformations.AddonSlug: true, decks.AddonSlug: true}}, map[string]string{
			cutscenes.KindName: "Cut scenes",
		}},
		{"lookup fails", fakeAddonSwitch{err: errors.New("db down")}, map[string]string{}},
	}
	for _, c := range cases {
		got := aiFeatureGate(c.sw)(context.Background(), "camp")
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q", c.name, k, got[k], v)
			}
		}
	}
}

// Every kind the card decks' AI part answers for sits behind the Card decks
// switch and refuses a key that is not a Director's, read from the part as
// the app wires it. A kind added to decks and forgotten in either place fails
// here, by name. The decks service is a stub with no methods, so a kind that
// read anything before refusing would crash the test instead.
func TestEveryDecksAIKindIsGatedAndDirectorOnly(t *testing.T) {
	everythingOn := map[string]bool{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, "maps": true,
		entity_notes.AddonSlug: true, crafting.AddonSlug: true, armory.AddonSlug: true, transformations.AddonSlug: true, decks.AddonSlug: true,
		cutscenes.AddonSlug: true}
	decksOff := map[string]bool{}
	for slug, on := range everythingOn {
		decksOff[slug] = on && slug != decks.AddonSlug
	}

	wired := func(on map[string]bool) []aiplug.Kind {
		d := newAIDeps(aiDeps{Health: database.NewPluginHealthRegistry(), Decks: stubDecks{}, Addons: fakeAddonSwitch{on: on}})
		if d.DecksAI == nil {
			t.Fatal("the decks AI part was not wired")
		}
		return d.DecksAI.Kinds()
	}
	kinds := wired(everythingOn)
	if len(kinds) == 0 {
		t.Fatal("the decks AI part has no kinds")
	}

	gate := aiFeatureGate(fakeAddonSwitch{on: decksOff})(context.Background(), "camp")
	if len(gate) != len(kinds) {
		t.Errorf("with only Card decks off the gate names %v, want exactly the %d decks kinds", gate, len(kinds))
	}

	director := aiplug.Actor{UserID: "u-dir", Role: 3}
	notDirectors := map[string]aiplug.Actor{
		"a player's key":                      {UserID: "u-tam", Role: 1, ReadsOwnNotes: true},
		"a scribe's key":                      {UserID: "u-scribe", Role: 2, ReadsOwnNotes: true},
		"the owner's what-players-see key":    {UserID: "u-dir", Role: 1},
		"a key with no place in the campaign": {UserID: "u-none"},
	}
	actions := []string{aiplug.ActionCreate, aiplug.ActionUpdate, aiplug.ActionDelete}

	for i, k := range kinds {
		name := k.Name()
		if gate[name] != "Card decks" {
			t.Errorf("kind %q is not behind the Card decks switch in aiFeatureGate (got %q)", name, gate[name])
		}
		for _, action := range actions {
			rec := aiplug.Record{Kind: name, Action: action, Name: "Omens", Fields: map[string]any{"kind": name, "deck": "Omens"}}
			for who, actor := range notDirectors {
				if p := k.Plan(context.Background(), "camp", actor, rec); p.Error == "" {
					t.Errorf("kind %q planned a %s for %s", name, action, who)
				}
				if err := k.Apply(context.Background(), "camp", actor, rec); err == nil {
					t.Errorf("kind %q applied a %s for %s", name, action, who)
				}
			}
			// The kind's own switch, which holds even where the gate above is
			// not asked: off refuses a Director too.
			off := wired(decksOff)[i]
			if p := off.Plan(context.Background(), "camp", director, rec); p.Error == "" {
				t.Errorf("kind %q planned a %s with Card decks switched off", name, action)
			}
			if err := off.Apply(context.Background(), "camp", director, rec); err == nil {
				t.Errorf("kind %q applied a %s with Card decks switched off", name, action)
			}
		}
	}
}
