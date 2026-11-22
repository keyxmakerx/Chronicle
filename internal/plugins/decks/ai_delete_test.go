package decks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// deckWithCardsHeld makes "Omens" with four cards, one copy dealt to a hand
// and one on the table.
func deckWithCardsHeld(r *aiRig) string {
	id := r.fx.deck("Omens", KindEvent, nil)
	a := r.fx.card(id, CreateCardInput{Name: "Frostbite", Copies: ptr(2)})
	b := r.fx.card(id, CreateCardInput{Name: "Doom"})
	r.fx.card(id, CreateCardInput{Name: "Hush"})
	r.fx.place(r.fx.copiesOf(a)[0], ZoneHand, "h-tam", true)
	r.fx.place(r.fx.copiesOf(b)[0], ZonePlay, "", true)
	return id
}

func (r *aiRig) deckNames() []string {
	r.t.Helper()
	list, err := r.fx.svc.List(bg, camp, r.fx.dir)
	r.fx.must(err)
	var out []string
	for _, d := range list.(DirectorList).Decks {
		out = append(out, d.Name)
	}
	return out
}

func TestAIDeckDeletePlan(t *testing.T) {
	const gone = `deletes the deck "Omens" for good: 4 cards, 2 of them in hands or on the table`
	cases := []struct {
		name    string
		rec     aiplug.Record
		seed    func(r *aiRig)
		summary string
		err     string
	}{
		{name: "says what goes, with the counts", seed: func(r *aiRig) { deckWithCardsHeld(r) },
			rec: aiRec("deck", "delete", "Omens", nil), summary: gone},
		{name: "a reason is an engine key and is allowed", seed: func(r *aiRig) { deckWithCardsHeld(r) },
			rec: aiRec("deck", "delete", "Omens", map[string]any{"reason": "retired"}), summary: gone},
		{name: "a name that is no deck", rec: aiRec("deck", "delete", "Omens", nil), err: `no deck called "Omens"`},
		{name: "two decks of one name", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil); r.fx.deck("omens", KindParty, nil) },
			rec: aiRec("deck", "delete", "Omens", nil), err: `more than one deck is called "Omens"`},
		{name: "another field is refused by name", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec: aiRec("deck", "delete", "Omens", map[string]any{"rename_to": "Portents"}),
			err: "a delete takes a deck's name and nothing else; remove `rename_to` from the block"},
		{name: "a move key is refused too", seed: func(r *aiRig) { r.fx.deck("Omens", KindEvent, nil) },
			rec: aiRec("deck", "delete", "Omens", map[string]any{"shuffle": true}),
			err: "a delete takes a deck's name and nothing else; remove `shuffle` from the block"},
		{name: "a delete needs a name", rec: aiRec("deck", "delete", "", nil), err: "a deck needs a name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAIRig(t)
			if tc.seed != nil {
				tc.seed(r)
			}
			before := len(r.deckNames())
			p := r.deck.Plan(bg, camp, aiDirector, tc.rec)
			if len(r.deckNames()) != before {
				t.Fatal("planning removed a deck")
			}
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) || p.Summary != "" {
					t.Errorf("plan = %+v, want error containing %q", p, tc.err)
				}
				return
			}
			want := []string{"Deleting this deck removes its cards and anything players hold from it.", "This cannot be undone."}
			if p.Error != "" || p.Summary != tc.summary || fmt.Sprint(p.Warnings) != fmt.Sprint(want) {
				t.Errorf("plan = %+v, want summary %q and warnings %q", p, tc.summary, want)
			}
		})
	}
}

func TestAIDeckDeleteApply(t *testing.T) {
	r := newAIRig(t)
	deckWithCardsHeld(r)
	r.fx.deck("Keep", KindParty, nil)
	rec := aiRec("deck", "delete", "Omens", nil)
	r.applyDeck(rec)
	if got := r.deckNames(); fmt.Sprint(got) != "[Keep]" {
		t.Errorf("decks after the delete = %v, want only Keep", got)
	}
	// The deck was removed between the plan and the yes: refused, not applied.
	err := r.deck.Apply(bg, camp, aiDirector, rec)
	if code(err) != 400 || !strings.Contains(err.Error(), `no deck called "Omens"`) {
		t.Errorf("apply of a deck already deleted = %v, want a 400 saying there is no such deck", err)
	}
	if got := r.deckNames(); fmt.Sprint(got) != "[Keep]" {
		t.Errorf("decks = %v", got)
	}
}

// Every key that is not a Director's, and the add-on off, gets one refusal
// whether or not the deck exists, so a delete cannot be used to find a name.
func TestAIDeckDeleteIsRefusedTheSameForAnyName(t *testing.T) {
	keys := []struct {
		name  string
		actor aiplug.Actor
		off   bool
		want  string
	}{
		{"a player's key", aiPlayer, false, "only the Director changes card decks"},
		{"a scribe's key", aiplug.Actor{UserID: "u-scribe", Role: 2, ReadsOwnNotes: true}, false, "only the Director changes card decks"},
		{"the owner's what-players-see key", aiSafe, false, "only the Director changes card decks"},
		{"the add-on off", aiDirector, true, "Card decks are switched off in this campaign"},
	}
	for _, k := range keys {
		t.Run(k.name, func(t *testing.T) {
			r := newAIRig(t)
			deckWithCardsHeld(r)
			r.on = !k.off
			a := aiRec("deck", "delete", "Omens", nil)
			b := aiRec("deck", "delete", "No such deck", nil)
			pa, pb := r.deck.Plan(bg, camp, k.actor, a), r.deck.Plan(bg, camp, k.actor, b)
			if pa.Error != k.want || fmt.Sprintf("%+v", pa) != fmt.Sprintf("%+v", pb) {
				t.Errorf("plans differ or are not the refusal: %+v vs %+v, want %q", pa, pb, k.want)
			}
			ea, eb := r.deck.Apply(bg, camp, k.actor, a), r.deck.Apply(bg, camp, k.actor, b)
			if ea == nil || eb == nil || ea.Error() != eb.Error() || !strings.Contains(ea.Error(), k.want) {
				t.Errorf("applies differ or are not the refusal: %v vs %v, want %q", ea, eb, k.want)
			}
			if got := r.deckNames(); fmt.Sprint(got) != "[Omens]" {
				t.Errorf("a refused delete left decks %v", got)
			}
		})
	}
}

// Stage never writes, and a card row after a delete row of the same paste is
// refused when it is applied, because Apply plans again against what is left.
func TestAIDeckDeleteThenACardOnIt(t *testing.T) {
	r := newAIRig(t)
	deckWithCardsHeld(r)
	del := aiRec("deck", "delete", "Omens", nil)
	ctx := r.deck.(aiplug.Staged).Stage(bg, camp, aiDirector, del)
	if len(stagedDecks(ctx)) != 0 || len(r.deckNames()) != 1 {
		t.Fatal("staging a delete staged or removed something")
	}
	r.applyDeck(del)
	err := r.card.Apply(ctx, camp, aiDirector, aiRec("card", "create", "Late", map[string]any{"deck": "Omens"}))
	if code(err) != 400 || !strings.Contains(err.Error(), `no deck called "Omens"`) {
		t.Errorf("card after a delete = %v, want the no-deck refusal", err)
	}
}
