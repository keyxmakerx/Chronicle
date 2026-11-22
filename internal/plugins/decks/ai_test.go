package decks

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// aiPage is a page the AI part may find by name.
type aiPage struct{ id, name string }

type aiPages map[string]aiPage

func (p aiPages) FindPage(_ context.Context, _, name string) (string, string, bool, error) {
	pg, ok := p[strings.ToLower(name)]
	return pg.id, pg.name, ok, nil
}

type aiRig struct {
	t    *testing.T
	fx   *fixture
	ai   *AI
	deck aiplug.Kind
	card aiplug.Kind
	on   bool
}

var (
	aiDirector = aiplug.Actor{UserID: "u-dir", Role: 3}
	aiPlayer   = aiplug.Actor{UserID: "u-tam", Role: 1, ReadsOwnNotes: true}
	aiSafe     = aiplug.Actor{UserID: "u-dir", Role: 1}
)

func newAIRig(t *testing.T) *aiRig {
	t.Helper()
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	rig := &aiRig{t: t, fx: fx, on: true}
	pages := aiPages{"tam": {"h-tam", "Tam"}, "ilse": {"h-ilse", "Ilse"}}
	rig.ai = NewAI(fx.svc, pages, func(context.Context, string) bool { return rig.on })
	kinds := rig.ai.Kinds()
	rig.deck, rig.card = kinds[0], kinds[1]
	return rig
}

func aiRec(kind, action, name string, fields map[string]any) aiplug.Record {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["kind"] = kind
	return aiplug.Record{Kind: kind, Action: action, Name: name, Fields: fields}
}

// yaml is what a YAML list of maps arrives as.
type yaml = map[any]any

func (r *aiRig) director(name string) DirectorDeck {
	r.t.Helper()
	list, err := r.fx.svc.List(bg, camp, r.fx.dir)
	r.fx.must(err)
	for _, d := range list.(DirectorList).Decks {
		if d.Name == name {
			v, err := r.fx.svc.Get(bg, camp, d.ID, r.fx.dir)
			r.fx.must(err)
			return v.(DirectorDeck)
		}
	}
	r.t.Fatalf("no deck %q", name)
	return DirectorDeck{}
}

func (r *aiRig) cardOf(deck, name string) DirectorCard {
	r.t.Helper()
	for _, c := range r.director(deck).Cards {
		if c.Name == name {
			return c
		}
	}
	r.t.Fatalf("no card %q in %q", name, deck)
	return DirectorCard{}
}

func (r *aiRig) applyDeck(rec aiplug.Record) {
	r.t.Helper()
	if err := r.deck.Apply(bg, camp, aiDirector, rec); err != nil {
		r.t.Fatalf("apply deck: %v", err)
	}
}

func (r *aiRig) applyCard(rec aiplug.Record) {
	r.t.Helper()
	if err := r.card.Apply(bg, camp, aiDirector, rec); err != nil {
		r.t.Fatalf("apply card: %v", err)
	}
}

func TestAIDeckPlan(t *testing.T) {
	borders := "border must be one of: " + strings.Join(borderValues, ", ")
	cases := []struct {
		name    string
		off     bool
		actor   aiplug.Actor
		rec     aiplug.Record
		seed    func(r *aiRig)
		summary string
		err     string
	}{
		{name: "makes a deck from a style, a single look key overriding it",
			rec:     aiRec("deck", "create", "Omens", map[string]any{"for": "events", "style": "iron-omens", "border": "line"}),
			summary: `makes the event deck "Omens", in the Iron omens style; look: border line`},
		{name: "a hero deck for a page found by name",
			rec:     aiRec("deck", "create", "Boons", map[string]any{"for": "hero", "hero": "Tam", "knows": "faces", "others_see": "face", "discard": "yes"}),
			summary: `makes the hero deck "Boons" for Tam; rules: knows faces, others_see face, discard true`},
		{name: "a look value outside its list is refused in the service's words",
			rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "border": "velvet"}), err: borders},
		{name: "a style that is not one",
			rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "style": "gothic"}), err: `style: "gothic" is not one of ` + styleKeys()},
		{name: "the workshop's own camelCase key works", rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "othersSee": "nothing"}),
			summary: `makes the event deck "Omens"; rules: others_see nothing`},
		{name: "add-on off", off: true, rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events"}),
			err: "Card decks are switched off in this campaign"},
		{name: "a player is refused", actor: aiPlayer, rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events"}),
			err: "only the Director changes card decks"},
		{name: "the owner's what-players-see key is refused", actor: aiSafe, rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events"}),
			err: "only the Director changes card decks"},
		{name: "who may draw is refused by name", rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "draws": "players"}),
			err: "`draws` is not something the AI sets: " + whyDraws},
		{name: "the chosen players are refused by name", rec: aiRec("deck", "update", "Omens", map[string]any{"Players": "Tam"}),
			err: "`Players` is not something the AI sets: " + whyDraws},
		{name: "comes up next is refused by name", rec: aiRec("deck", "update", "Omens", map[string]any{"next_card": "Doom"}),
			err: "`next_card` is not something the AI sets: " + whyNext},
		{name: "moving a card is refused by name", rec: aiRec("deck", "update", "Omens", map[string]any{"shuffle": true}),
			err: "`shuffle` is not something the AI sets: " + whyMoves},
		{name: "an unknown key is refused by name", rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "colour": "red"}),
			err: "`colour` is not a key a deck block takes"},
		{name: "two spellings of one key", rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "others_see": "back", "OthersSee": "face"}),
			err: "are the same key; write one"},
		{name: "a deck needs for", rec: aiRec("deck", "create", "Omens", nil), err: "a deck needs `for`: hero, party or events"},
		{name: "a hero deck needs its hero", rec: aiRec("deck", "create", "Boons", map[string]any{"for": "hero"}),
			err: "a hero deck needs `hero`, the page name of the hero it is for"},
		{name: "a hero that is not a page", rec: aiRec("deck", "create", "Boons", map[string]any{"for": "hero", "hero": "Nobody"}),
			err: `no page called "Nobody" for the hero`},
		{name: "only a hero deck has a hero", rec: aiRec("deck", "create", "Pot", map[string]any{"for": "party", "hero": "Tam"}),
			err: "only a hero deck belongs to a hero page"},
		{name: "a name already taken", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events"}), err: `a deck called "Omens" already exists; use action: update`},
		{name: "kinds with a key a kind does not have", rec: aiRec("deck", "create", "Omens", map[string]any{"for": "events", "kinds": []any{yaml{"key": "bane", "name": "Bane", "hue": "red"}}}),
			err: `kinds: entry 1 has no key "hue"; a kind has key, name, colour and sign`},
		{name: "update says what it changes", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec:     aiRec("deck", "update", "Omens", map[string]any{"rename_to": "Portents", "fate": "gone", "back_words": "Beware"}),
			summary: `changes the deck "Omens": renames it to "Portents"; rules: fate gone; look: back_words Beware`},
		{name: "update of a deck that is not there", rec: aiRec("deck", "update", "Omens", map[string]any{"fate": "gone"}), err: `no deck called "Omens"`},
		{name: "update with nothing to change", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec: aiRec("deck", "update", "Omens", nil), err: "nothing to change: name a key to change"},
		{name: "update refuses a bad rule in the service's words", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec: aiRec("deck", "update", "Omens", map[string]any{"fate": "burn"}), err: "fate must be one of: " + strings.Join(fateValues, ", ")},
		{name: "a style on update warns what it replaces", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec:     aiRec("deck", "update", "Omens", map[string]any{"style": "Ember curses"}),
			summary: `changes the deck "Omens": applies the Ember curses style`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAIRig(t)
			r.on = !tc.off
			if tc.seed != nil {
				tc.seed(r)
			}
			actor := tc.actor
			if actor == (aiplug.Actor{}) {
				actor = aiDirector
			}
			p := r.deck.Plan(bg, camp, actor, tc.rec)
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) || p.Summary != "" {
					t.Errorf("plan = %+v, want error containing %q", p, tc.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tc.summary {
				t.Errorf("plan = %+v, want summary %q", p, tc.summary)
			}
		})
	}
}

func TestAIDeckApply(t *testing.T) {
	r := newAIRig(t)
	r.applyDeck(aiRec("deck", "create", "Omens", map[string]any{
		"for": "events", "style": "iron-omens", "border": "line", "knows": "kinds", "discard": true,
		"kinds": []any{yaml{"key": "bane", "name": "Bane", "colour": "rose"}},
	}))
	d := r.director("Omens")
	if d.Kind != KindEvent || d.Look.Border != "line" || d.Look.Turn != "slam" || d.Look.Back.Pattern != "iron" || d.Look.Finish != "glow" {
		t.Errorf("look = %+v", d.Look)
	}
	if d.Settings.Knows != "kinds" || !d.Settings.Discard || d.Settings.Draws != "directors" {
		t.Errorf("settings = %+v", d.Settings)
	}
	if len(d.Look.Kinds) != 1 || d.Look.Kinds[0].Key != "bane" || d.Look.Kinds[0].Sign == "" {
		t.Errorf("kinds = %+v", d.Look.Kinds)
	}
	if len(d.Players) != 0 {
		t.Errorf("the AI chose players: %+v", d.Players)
	}

	// A partial update changes only what it names: the look the rule-only
	// update never mentions stays, and the version moves once.
	r.applyDeck(aiRec("deck", "update", "Omens", map[string]any{"fate": "gone"}))
	u := r.director("Omens")
	if u.Settings.Fate != "gone" || u.Settings.Knows != "kinds" || u.Look.Border != "line" || len(u.Look.Kinds) != 1 || u.Version != d.Version+1 {
		t.Errorf("after a rule-only update: %+v", u)
	}
	r.applyDeck(aiRec("deck", "update", "Omens", map[string]any{"border": "brass", "rename_to": "Portents"}))
	u = r.director("Portents")
	if u.Look.Border != "brass" || u.Look.Turn != "slam" || u.Settings.Fate != "gone" {
		t.Errorf("after a look update: %+v", u)
	}

	// Apply re-plans: a deck the campaign no longer has is not written.
	if err := r.deck.Apply(bg, camp, aiDirector, aiRec("deck", "update", "Omens", map[string]any{"fate": "used"})); err == nil {
		t.Error("an update of a deck that is gone was applied")
	}
	if err := r.deck.Apply(bg, camp, aiPlayer, aiRec("deck", "delete", "Portents", nil)); err == nil || r.director("Portents").ID == "" {
		t.Error("a player's key deleted a deck")
	}
	if err := r.deck.Apply(bg, camp, aiPlayer, aiRec("deck", "create", "Mine", map[string]any{"for": "party"})); err == nil {
		t.Error("a player's key made a deck")
	}
}

func TestAIHeroDeckApply(t *testing.T) {
	r := newAIRig(t)
	r.applyDeck(aiRec("deck", "create", "Boons", map[string]any{"for": "hero", "hero": "Tam"}))
	if d := r.director("Boons"); d.Kind != KindHero || d.HolderEntityID != "h-tam" {
		t.Errorf("deck = %+v", d)
	}
	// Moving a hero deck to the party needs it taken off its hero, said so.
	if p := r.deck.Plan(bg, camp, aiDirector, aiRec("deck", "update", "Boons", map[string]any{"for": "party"})); !strings.Contains(p.Error, "hero: none") {
		t.Errorf("plan = %+v", p)
	}
	r.applyDeck(aiRec("deck", "update", "Boons", map[string]any{"for": "party", "hero": "none"}))
	if d := r.director("Boons"); d.Kind != KindParty || d.HolderEntityID != "" {
		t.Errorf("deck = %+v", d)
	}
}

func tests(ts ...any) map[string]any { return map[string]any{"when": ts} }

func TestAICardPlan(t *testing.T) {
	hp := func(extra yaml) yaml {
		m := yaml{"test": "number", "who": "Tam", "field": "hp", "op": "lte", "value": 5}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	seed := func(r *aiRig) {
		omens := r.fx.deck("Omens", KindEvent, nil)
		_, err := r.fx.svc.Create(bg, camp, r.fx.dir, CreateDeckInput{Name: "Wards", Kind: KindEvent,
			Look: &LookInput{Kinds: []KindDef{{Key: "ward", Name: "Ward", Colour: "teal"}}}})
		r.fx.must(err)
		r.fx.card(omens, CreateCardInput{Name: "Old"})
	}
	fields := func(m map[string]any) map[string]any {
		if m == nil {
			m = map[string]any{}
		}
		m["deck"] = "Omens"
		return m
	}
	cases := []struct {
		name    string
		off     bool
		actor   aiplug.Actor
		rec     aiplug.Record
		summary string
		err     string
	}{
		{name: "a printed and an own test, said as the editor says them",
			rec: aiRec("card", "create", "Frostbite", fields(map[string]any{"copies": 3,
				"when": []any{hp(yaml{"open": true}), yaml{"test": "say"}}})),
			summary: "adds Frostbite to Omens, 3 copies; comes up when Tam's Hit points is at or below 5 (on the card) and the Director says so (only you)"},
		{name: "a tracker and a tag by name, a page by name",
			rec: aiRec("card", "create", "Dread", fields(map[string]any{"when": []any{
				yaml{"test": "tracker", "who": "Tam", "kind": "Doom", "op": "gte", "value": 3},
				yaml{"test": "tag", "who": "any", "tag": "cursed", "has": true}}})),
			summary: "adds Dread to Omens, 1 copy; comes up when Tam's Doom is at or above 3 (only you) and any hero has the tag Cursed (only you)"},
		{name: "the extras are said", rec: aiRec("card", "create", "Aegis", map[string]any{"deck": "Wards", "card_kind": "Ward", "rare": true, "ask_first": true, "after_use": "gone", "note": "secret"}),
			summary: "adds Aegis to Wards, 1 copy; card kind ward; rare; ask first; after use: gone; a note for the Director"},
		{name: "a card kind the deck does not have", rec: aiRec("card", "create", "Aegis", map[string]any{"deck": "Wards", "card_kind": "Hex"}),
			err: `card_kind: "Hex" is not one of Wards's kinds; its kinds are Ward`},
		{name: "a deck with no kinds", rec: aiRec("card", "create", "Aegis", fields(map[string]any{"card_kind": "Hex"})),
			err: "it has no card kinds yet; add them with `kinds` on a deck block"},
		{name: "a test with an unknown key is refused by name",
			rec: aiRec("card", "create", "Frostbite", fields(tests(hp(yaml{"colour": "red"})))),
			err: `test 1 (number) has an unknown key "colour"`},
		{name: "a key of another kind of test is refused by name",
			rec: aiRec("card", "create", "Frostbite", fields(tests(yaml{"test": "say", "who": "Tam"}))),
			err: `test 1 (say) has an unknown key "who"`},
		{name: "a page name that is not a page",
			rec: aiRec("card", "create", "Frostbite", fields(tests(hp(yaml{"who": "Nobody"})))),
			err: `test 1: no page called "Nobody"`},
		{name: "a tag that does not exist",
			rec: aiRec("card", "create", "Frostbite", fields(tests(yaml{"test": "tag", "who": "Tam", "tag": "Missing", "has": true}))),
			err: `test 1: no tag called "Missing"`},
		{name: "a field the service does not know is refused in its words",
			rec: aiRec("card", "create", "Frostbite", fields(tests(hp(yaml{"field": "mana"})))),
			err: "test 1: that is not a number field of the page"},
		{name: "five tests",
			rec: aiRec("card", "create", "Frostbite", fields(tests(yaml{"test": "say"}, yaml{"test": "say"}, yaml{"test": "say"}, yaml{"test": "say"}, yaml{"test": "say"}))),
			err: "a card can have at most 4 tests"},
		{name: "four tests are fine",
			rec:     aiRec("card", "create", "Frostbite", fields(tests(yaml{"test": "say"}, yaml{"test": "say"}, yaml{"test": "say"}, yaml{"test": "say"}))),
			summary: "adds Frostbite to Omens, 1 copy; comes up when the Director says so (only you) and the Director says so (only you) and the Director says so (only you) and the Director says so (only you)"},
		{name: "a test that is not a map", rec: aiRec("card", "create", "Frostbite", fields(tests("say"))),
			err: "test 1: each test is a map"},
		{name: "a card on a deck that does not exist", rec: aiRec("card", "create", "Frostbite", map[string]any{"deck": "Nowhere"}),
			err: `no deck called "Nowhere"`},
		{name: "a card needs its deck", rec: aiRec("card", "create", "Frostbite", nil), err: "a card needs `deck`"},
		{name: "copies of zero is the service's own refusal", rec: aiRec("card", "create", "Frostbite", fields(map[string]any{"copies": 0})),
			err: "copies must be between 1 and 20"},
		{name: "copies that are not a number", rec: aiRec("card", "create", "Frostbite", fields(map[string]any{"copies": "many"})),
			err: "not a number"},
		{name: "a fate outside its list", rec: aiRec("card", "create", "Frostbite", fields(map[string]any{"after_use": "burn"})),
			err: "fate must be one of: " + strings.Join(fateValues, ", ")},
		{name: "add-on off", off: true, rec: aiRec("card", "create", "Frostbite", fields(nil)), err: "Card decks are switched off in this campaign"},
		{name: "a player is refused", actor: aiPlayer, rec: aiRec("card", "create", "Frostbite", fields(nil)), err: "only the Director changes card decks"},
		{name: "delete is refused", rec: aiRec("card", "delete", "Old", fields(nil)),
			err: "The AI does not remove a card. Delete it in the deck workshop."},
		{name: "a card that exists is not made twice", rec: aiRec("card", "create", "Old", fields(nil)),
			err: `a card called "Old" already exists in Omens; use action: update`},
		{name: "an update of a card that is not there", rec: aiRec("card", "update", "Missing", fields(map[string]any{"words": "x"})),
			err: `no card called "Missing" in Omens`},
		{name: "an update with nothing to change", rec: aiRec("card", "update", "Old", fields(nil)), err: "nothing to change: name a key to change"},
		{name: "an update says what it changes", rec: aiRec("card", "update", "Old", fields(map[string]any{"copies": 2, "rare": false, "when": []any{}})),
			summary: "changes Old in Omens: 2 copies; not rare; no tests, so it comes up whenever it is drawn"},
		{name: "moving a card is refused by name", rec: aiRec("card", "update", "Old", fields(map[string]any{"deal": "Tam"})),
			err: "`deal` is not something the AI sets: " + whyMoves},
		{name: "the deck's own kind is card_kind, not kind", rec: aiRec("card", "create", "X", fields(map[string]any{"colour": "x"})),
			err: "`colour` is not a key a card block takes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAIRig(t)
			seed(r)
			r.on = !tc.off
			actor := tc.actor
			if actor == (aiplug.Actor{}) {
				actor = aiDirector
			}
			p := r.card.Plan(bg, camp, actor, tc.rec)
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) || p.Summary != "" {
					t.Errorf("plan = %+v, want error containing %q", p, tc.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tc.summary {
				t.Errorf("plan = %+v, want summary %q", p, tc.summary)
			}
		})
	}
}

func TestAICardApply(t *testing.T) {
	r := newAIRig(t)
	r.applyDeck(aiRec("deck", "create", "Omens", map[string]any{"for": "events",
		"kinds": []any{yaml{"key": "bane", "name": "Bane", "colour": "rose"}}}))
	r.applyCard(aiRec("card", "create", "Frostbite", map[string]any{"deck": "Omens", "copies": 3, "card_kind": "Bane", "words": "Cold.",
		"note": "for later", "rare": true, "corner": "IV",
		"when": []any{
			yaml{"test": "number", "who": "Tam", "field": "hp", "op": "lte", "value": 5, "open": true},
			yaml{"test": "say"}}}))
	c := r.cardOf("Omens", "Frostbite")
	if c.Copies != 3 || c.KindKey != "bane" || c.Words != "Cold." || c.DirectorNote != "for later" || !c.Rare || c.Corner != "IV" {
		t.Errorf("card = %+v", c)
	}
	if len(c.When) != 2 || !c.When[0].Open || c.When[0].Who != "h-tam" || c.When[1].Open || !c.WhenOpen {
		t.Errorf("tests = %+v", c.When)
	}
	if c.When[0].Sentence != "Tam's Hit points is at or below 5" {
		t.Errorf("sentence = %q", c.When[0].Sentence)
	}

	// A words-only update leaves copies, kind, note and tests as they are.
	r.applyCard(aiRec("card", "update", "Frostbite", map[string]any{"deck": "Omens", "words": "Colder."}))
	u := r.cardOf("Omens", "Frostbite")
	if u.Words != "Colder." || u.Copies != 3 || u.KindKey != "bane" || u.DirectorNote != "for later" || len(u.When) != 2 || !u.Rare {
		t.Errorf("after a words-only update: %+v", u)
	}
	// A copies-only update leaves the words.
	r.applyCard(aiRec("card", "update", "Frostbite", map[string]any{"deck": "Omens", "copies": 5}))
	if u = r.cardOf("Omens", "Frostbite"); u.Copies != 5 || u.Words != "Colder." {
		t.Errorf("after a copies-only update: %+v", u)
	}
	// An empty value clears what can be cleared; an empty list clears the tests.
	r.applyCard(aiRec("card", "update", "Frostbite", map[string]any{"deck": "Omens", "card_kind": "", "note": nil, "when": []any{}, "rename_to": "Rime"}))
	u = r.cardOf("Omens", "Rime")
	if u.KindKey != "" || u.DirectorNote != "" || len(u.When) != 0 || u.Words != "Colder." || u.Copies != 5 {
		t.Errorf("after clearing: %+v", u)
	}
	// The service's own limit applies on Apply as on review.
	if err := r.card.Apply(bg, camp, aiDirector, aiRec("card", "update", "Rime", map[string]any{"deck": "Omens", "copies": 99})); err == nil {
		t.Error("99 copies were applied")
	}
	if err := r.card.Apply(bg, camp, aiDirector, aiRec("card", "delete", "Rime", map[string]any{"deck": "Omens"})); err == nil {
		t.Error("a card was deleted through the AI")
	}
	if len(r.director("Omens").Cards) != 1 {
		t.Error("a card went missing")
	}
	// Copies of zero is the service's, refused with its words, nothing written.
	err := r.card.Apply(bg, camp, aiDirector, aiRec("card", "update", "Rime", map[string]any{"deck": "Omens", "copies": 0}))
	if err == nil || !strings.Contains(err.Error(), "copies must be between 1 and 20") {
		t.Errorf("copies 0: %v", err)
	}
}

// A card row may name a deck an earlier row of the same paste makes: review
// plans it against what the deck row will make, and by commit the deck exists.
func TestAICardOnADeckMadeByAnEarlierRow(t *testing.T) {
	r := newAIRig(t)
	deckRow := aiRec("deck", "create", "Wards", map[string]any{"for": "events",
		"kinds": []any{yaml{"key": "ward", "name": "Ward", "colour": "teal"}}})
	cardRow := aiRec("card", "create", "Sigil", map[string]any{"deck": "wards", "card_kind": "Ward", "copies": 2})

	if p := r.card.Plan(bg, camp, aiDirector, cardRow); p.Error != `no deck called "wards"` {
		t.Fatalf("without the deck row: %+v", p)
	}
	if p := r.deck.Plan(bg, camp, aiDirector, deckRow); p.Error != "" {
		t.Fatalf("deck row: %+v", p)
	}
	ctx := r.deck.(aiplug.Staged).Stage(bg, camp, aiDirector, deckRow)
	p := r.card.Plan(ctx, camp, aiDirector, cardRow)
	if p.Error != "" || p.Summary != "adds Sigil to Wards, 2 copies; card kind ward" {
		t.Fatalf("with the deck row staged: %+v", p)
	}
	// A staged deck is only for the rows after it: the context handed out
	// earlier never learns it, and a row that failed stages nothing.
	if p := r.card.Plan(bg, camp, aiDirector, cardRow); p.Error == "" {
		t.Error("the earlier context learned the staged deck")
	}
	bad := aiRec("deck", "create", "Ghosts", map[string]any{"for": "events", "border": "velvet"})
	if ctx2 := r.deck.(aiplug.Staged).Stage(bg, camp, aiDirector, bad); len(stagedDecks(ctx2)) != 0 {
		t.Error("a deck row that failed was staged")
	}
	if ctx2 := r.deck.(aiplug.Staged).Stage(bg, camp, aiDirector, aiRec("deck", "update", "Wards", map[string]any{"fate": "gone"})); len(stagedDecks(ctx2)) != 0 {
		t.Error("an update was staged")
	}
	// A staged hero deck lets a later card say `holder`.
	hero := aiRec("deck", "create", "Boons", map[string]any{"for": "hero", "hero": "Tam"})
	hctx := r.deck.(aiplug.Staged).Stage(bg, camp, aiDirector, hero)
	hc := aiRec("card", "create", "Aegis", map[string]any{"deck": "Boons", "when": []any{yaml{"test": "number", "who": "holder", "field": "hp", "op": "lte", "value": 5}}})
	if p := r.card.Plan(hctx, camp, aiDirector, hc); p.Error != "" || !strings.Contains(p.Summary, "Tam's Hit points is at or below 5 (only you)") {
		t.Errorf("holder on a staged hero deck: %+v", p)
	}

	// Commit: rows apply in order, so the card finds the deck.
	r.applyDeck(deckRow)
	r.applyCard(cardRow)
	if c := r.cardOf("Wards", "Sigil"); c.Copies != 2 || c.KindKey != "ward" {
		t.Errorf("card = %+v", c)
	}
}

func TestAIKindsAreBriefedAndDocumented(t *testing.T) {
	r := newAIRig(t)
	for _, k := range r.ai.Kinds() {
		b, ok := k.(aiplug.Briefed)
		if !ok || strings.TrimSpace(b.Brief()) == "" || b.Brief() == k.Label() {
			t.Errorf("%s has no brief", k.Name())
		}
		doc := k.Doc()
		for _, line := range strings.Split(doc, "\n") {
			if strings.HasPrefix(line, "## ") {
				t.Errorf("%s's format has a section heading: %q", k.Name(), line)
			}
		}
		if n := strings.Count(doc, "```"); n != 2 {
			t.Errorf("%s's format has %d fences, want one example", k.Name(), n)
		}
		if out, err := k.Export(bg, camp, aiDirector); out != "" || err != nil {
			t.Errorf("%s exports %q, %v", k.Name(), out, err)
		}
		if _, ok := k.(aiplug.Undoable); ok {
			t.Errorf("%s is undoable; no removal ships before changes can be undone, and these do not", k.Name())
		}
	}
	// The Director is told what the AI does not do.
	for _, want := range []string{"`action: delete` removes the deck", "does not remove a single card", "who may draw", "comes up next", "move a card"} {
		if !strings.Contains(r.deck.Doc(), want) {
			t.Errorf("the deck's format does not say %q", want)
		}
	}
	// The format's lists are the workshop's own.
	for _, v := range append(append([]string{}, borderValues...), knowsValues...) {
		if !strings.Contains(r.deck.Doc(), v) {
			t.Errorf("the deck's format lacks the value %q", v)
		}
	}
}

// The lookup is a player's tool as well as the Director's, so it must not be
// marked owner-only, and it names the tool and the player's own description.
func TestAILookupInfo(t *testing.T) {
	info := NewAI(newFixture(t).svc, aiPages{}, func(context.Context, string) bool { return true }).Lookup().LookupInfo()
	if info.What != "decks" || info.Tool.Owner {
		t.Errorf("what=%q owner=%v", info.What, info.Tool.Owner)
	}
	if info.Tool.Name != "card_decks" || strings.TrimSpace(info.Tool.PlayerDesc) == "" {
		t.Errorf("tool=%q playerDesc=%q", info.Tool.Name, info.Tool.PlayerDesc)
	}
}

// A change that alters who can see a deck or what players are told says so on
// the review line; a change that does not stays quiet.
func TestAIDeckUpdateWarnsWhenSightChanges(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{"who it is for", map[string]any{"for": "party"}, "Who a deck is for decides which players can see it and its used cards."},
		{"what players know", map[string]any{"knows": "faces"}, "This changes what players are told about the pile or about each other's cards."},
		{"what others see", map[string]any{"others_see": "face"}, "This changes what players are told about the pile or about each other's cards."},
		{"a rule that changes no sight", map[string]any{"fate": "gone"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newAIRig(t)
			r.fx.deck("Omens", KindEvent, nil)
			p := r.deck.Plan(bg, camp, aiDirector, aiRec("deck", "update", "Omens", tt.fields))
			if p.Error != "" {
				t.Fatalf("plan = %+v", p)
			}
			got := strings.Join(p.Warnings, "|")
			if got != tt.want {
				t.Errorf("warnings = %q, want %q", got, tt.want)
			}
		})
	}
}

// A tracker name two trackers share is refused, as a tag's is, where it used
// to bind whichever came first.
func TestAICardTrackerNameSharedByTwoIsRefused(t *testing.T) {
	fx := newFixture(t)
	facts := newFakeFacts()
	facts.opts.Trackers = append(facts.opts.Trackers, TrackerKindOption{ID: "k-doom-2", Name: "doom"})
	newFactsFixture(t, fx, facts)
	ai := NewAI(fx.svc, aiPages{"tam": {"h-tam", "Tam"}}, func(context.Context, string) bool { return true })
	fx.deck("Omens", KindEvent, nil)
	p := ai.Kinds()[1].Plan(bg, camp, aiDirector, aiRec("card", "create", "Dread", map[string]any{"deck": "Omens",
		"when": []any{yaml{"test": "tracker", "who": "Tam", "kind": "Doom", "op": "gte", "value": 3}}}))
	if want := `test 1: more than one tracker is called "Doom"`; !strings.Contains(p.Error, want) || p.Summary != "" {
		t.Errorf("plan = %+v, want error containing %q", p, want)
	}
}

// A deck with more cards than one answer may list is read in parts, and each
// part says where the next begins, so no card is out of the AI's reach.
func TestAILookupReadsALongDeckInParts(t *testing.T) {
	r := newAIRig(t)
	deck := r.fx.deck("Omens", KindEvent, nil)
	for i := 1; i <= cardsPerAnswer+5; i++ {
		r.fx.card(deck, CreateCardInput{Name: fmt.Sprintf("Card %03d", i)})
	}
	last := fmt.Sprintf("Card %03d", cardsPerAnswer+5)
	tests := []struct {
		name string
		from any
		has  []string
		not  []string
		err  string
	}{
		{name: "the first part", has: []string{"- Card 001 ", fmt.Sprintf("Cards 1 to %d of %d. Ask again with `from: %d` for the next.", cardsPerAnswer, cardsPerAnswer+5, cardsPerAnswer+1)},
			not: []string{"- " + last + " "}},
		{name: "the rest", from: cardsPerAnswer + 1, has: []string{"- " + last + " ", fmt.Sprintf("Cards %d to %d of %d.", cardsPerAnswer+1, cardsPerAnswer+5, cardsPerAnswer+5)},
			not: []string{"- Card 001 ", "Ask again"}},
		{name: "past the end", from: 999, has: []string{fmt.Sprintf("This deck has %d cards, so there is none from 999 on.", cardsPerAnswer+5)}},
		{name: "not a count", from: -3, err: "`from` is the number of the first card to list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]any{"deck": "Omens"}
			if tt.from != nil {
				fields["from"] = tt.from
			}
			_, text, err := r.ai.Lookup().Answer(bg, camp, aiDirector, aiplug.Record{Fields: fields})
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
				return
			}
			r.fx.must(err)
			for _, want := range tt.has {
				if !strings.Contains(text, want) {
					t.Errorf("the answer lacks %q:\n%s", want, text)
				}
			}
			for _, no := range tt.not {
				if strings.Contains(text, no) {
					t.Errorf("the answer holds %q", no)
				}
			}
			// Where the copies are is said in full whichever part is asked for.
			if !strings.Contains(text, fmt.Sprintf("In the pile (%d)", cardsPerAnswer+5)) {
				t.Errorf("the copies line is missing:\n%s", text)
			}
		})
	}
}

// With card decks switched off, no key reads anything: the refusal comes
// before any deck is read.
func TestAILookupIsRefusedForEveryKeyWhenTheAddOnIsOff(t *testing.T) {
	r := newAIRig(t)
	deck := r.fx.deck("Omens", KindEvent, nil)
	r.fx.card(deck, CreateCardInput{Name: "Doom"})
	r.on = false
	scribe := aiplug.Actor{UserID: "u-scribe", Role: 2, ReadsOwnNotes: true}
	for name, a := range map[string]aiplug.Actor{"the Director": aiDirector, "a player's own key": aiPlayer, "a scribe's own key": scribe, "what players see": aiSafe} {
		_, text, err := r.ai.Lookup().Answer(bg, camp, a, aiplug.Record{Fields: map[string]any{}})
		if err == nil || !strings.Contains(err.Error(), "switched off") || text != "" {
			t.Errorf("%s: text %q, err %v", name, text, err)
		}
	}
}

// A scribe's own key reads as any member's: their own hand, and nothing of a
// deck only Directors see.
func TestAILookupAScribesKeyReadsOnlyTheirOwnHand(t *testing.T) {
	r := newAIRig(t)
	deck := r.fx.deck("Omens", KindEvent, nil)
	r.fx.card(deck, CreateCardInput{Name: "Doom", Words: "DOOMWORDS"})
	scribe := aiplug.Actor{UserID: "u-scribe", Role: 2, ReadsOwnNotes: true}
	for _, fields := range []map[string]any{{}, {"deck": "Omens"}, {"deck": "Omens", "from": 1}} {
		_, text, err := r.ai.Lookup().Answer(bg, camp, scribe, aiplug.Record{Fields: fields})
		r.fx.must(err)
		if strings.Contains(text, "Doom") || strings.Contains(text, "DOOMWORDS") || strings.Contains(text, "pile") {
			t.Errorf("a scribe's key was told of a Director's deck (%v):\n%s", fields, text)
		}
	}
}
