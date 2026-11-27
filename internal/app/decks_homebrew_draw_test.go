package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// drawDecks is a decks service holding decks by campaign, which records the
// draws and copy moves a deck.draw makes.
type drawDecks struct {
	decks.Service
	held    map[string]decks.DirectorDeck // "campaign/deck"
	pile    []string                      // the copy ids a draw hands out, in order
	drawErr error
	dealErr map[int]error // the nth deal (from 0) fails with this
	draws   []decks.DrawInput
	moves   []string // "verb copy hero"
	viewers []decks.Viewer
}

func (f *drawDecks) Get(_ context.Context, campaignID, deckID string, v decks.Viewer) (decks.DeckView, error) {
	if d, ok := f.held[campaignID+"/"+deckID]; ok && v.Director {
		return d, nil
	}
	return nil, apperror.NewNotFound("deck not found")
}

func (f *drawDecks) Draw(_ context.Context, _, _ string, v decks.Viewer, in decks.DrawInput) (*decks.ActionResult, error) {
	f.draws = append(f.draws, in)
	f.viewers = append(f.viewers, v)
	if f.drawErr != nil {
		return nil, f.drawErr
	}
	n := in.Count
	if n > len(f.pile) {
		n = len(f.pile)
	}
	return &decks.ActionResult{CopyIDs: append([]string{}, f.pile[:n]...)}, nil
}

func (f *drawDecks) Act(_ context.Context, _, _, copyID, verb string, _ decks.Viewer, in decks.ActInput) (*decks.ActionResult, error) {
	if verb == deckVerbDeal {
		deals := 0
		for _, m := range f.moves {
			if strings.HasPrefix(m, deckVerbDeal+" ") {
				deals++
			}
		}
		if err := f.dealErr[deals]; err != nil {
			f.moves = append(f.moves, "refused "+copyID)
			return nil, err
		}
	}
	f.moves = append(f.moves, strings.TrimSpace(verb+" "+copyID+" "+in.HeroID))
	return &decks.ActionResult{}, nil
}

func (f *drawDecks) Roster(context.Context, string, decks.Viewer) (decks.Roster, error) {
	return decks.Roster{Heroes: []decks.Hero{{ID: "h-tam", Name: "Tam"}}}, nil
}

func newDrawDecks() *drawDecks {
	return &drawDecks{
		held: map[string]decks.DirectorDeck{"camp/d-omens": {ID: "d-omens", Name: "Omens"}, "other/d-boons": {ID: "d-boons", Name: "Boons"}},
		pile: []string{"c1", "c2", "c3"},
	}
}

func drawStory(svc decks.Service, on bool) *decksStory {
	st := newDecksStory(func(context.Context, string) bool { return on })
	st.bindDecks(svc)
	return st
}

func TestDeckPartCheck(t *testing.T) {
	tests := []struct {
		name string
		on   bool
		data string
		want string // stored data; "" means refused
		says string // part of the refusal
	}{
		{"a deck of this campaign, its name refreshed", true, `{"deckId":" d-omens ","name":"Old omens"}`, `{"deckId":"d-omens","name":"Omens"}`, ""},
		{"no name yet", true, `{"deckId":"d-omens"}`, `{"deckId":"d-omens","name":"Omens"}`, ""},
		{"another campaign's deck", true, `{"deckId":"d-boons","name":"Boons"}`, "", "The deck Boons is not one of this campaign's decks"},
		{"a missing deck", true, `{"deckId":"d-gone"}`, "", "That deck is not one of this campaign's decks"},
		{"no deck picked", true, `{}`, "", "pick one of the campaign's decks"},
		{"an id far too long", true, `{"deckId":"` + strings.Repeat("x", maxDeckIDChars+1) + `"}`, "", "is not one of the campaign's decks"},
		{"an unknown field", true, `{"deckId":"d-omens","copy":true}`, "", ""},
		{"card decks switched off", false, `{"deckId":"d-omens"}`, "", "switched off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := drawStory(newDrawDecks(), tt.on)
			got, err := st.checkDeckPart(context.Background(), "camp", json.RawMessage(tt.data))
			if tt.want == "" {
				if hbCode(err) != 422 || !strings.Contains(err.Error(), tt.says) {
					t.Fatalf("err = %v, want a 422 saying %q", err, tt.says)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

// A deck part places nothing a player sees and the deck's own rules decide who
// sees the deck, so the part is the Director's whatever the recipe says.
func TestDeckPartIsAlwaysTheDirectors(t *testing.T) {
	reg := homebrew.NewRegistry()
	if err := registerDecksHomebrew(reg, drawStory(newDrawDecks(), true)); err != nil {
		t.Fatal(err)
	}
	for _, p := range reg.Catalog().Parts {
		if p.Name == deckPartKind && p.ForceSee != homebrew.SeeDirector {
			t.Errorf("deck part forces %q", p.ForceSee)
		}
	}
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
	for _, see := range []string{homebrew.SeeParty, homebrew.SeeName, homebrew.SeeDirector, ""} {
		r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{{Key: "omens", Kind: deckPartKind, Label: "Omens", See: see, Data: json.RawMessage(`{"deckId":"d-omens"}`)}}})
		if err != nil {
			t.Fatalf("see %q: %v", see, err)
		}
		if got := r.Doc.Parts[0].See; got != homebrew.SeeDirector {
			t.Errorf("see %q was stored as %q", see, got)
		}
	}
}

func TestDrawArgsCheck(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string // "" means refused
	}{
		{"no count is one card", `{}`, `{"count":1}`},
		{"one", `{"count":1}`, `{"count":1}`},
		{"the most a draw takes", `{"count":5}`, `{"count":5}`},
		{"none", `{"count":0}`, ""},
		{"fewer than none", `{"count":-1}`, ""},
		{"more than a draw takes", `{"count":6}`, ""},
		{"not a whole number", `{"count":1.5}`, ""},
		{"text", `{"count":"2"}`, ""},
		// A deck keeps one pile, so there is no stack to name.
		{"a stack is not a field", `{"count":1,"stack":"discard"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkDrawArgs(context.Background(), "camp", json.RawMessage(tt.args))
			if tt.want == "" {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestDeckDrawRun(t *testing.T) {
	omens := &homebrew.Part{Key: "omens", Kind: deckPartKind, Label: "Omens", See: homebrew.SeeDirector, Data: json.RawMessage(`{"deckId":"d-omens","name":"Omens"}`)}
	gone := &homebrew.Part{Key: "gone", Kind: deckPartKind, Label: "Old", See: homebrew.SeeDirector, Data: json.RawMessage(`{"deckId":"d-boons","name":"Boons"}`)}
	director := homebrew.Viewer{UserID: "u-dm", Director: true, VisibilityRole: permissions.RoleOwner}
	tests := []struct {
		name    string
		part    *homebrew.Part
		count   int
		scope   string
		by      homebrew.Viewer
		setup   func(f *drawDecks)
		line    string   // the result line, when it runs
		refused string   // part of the refusal, when it does not
		code    int      // the refusal's code
		draws   int      // how many draws the decks service saw
		moves   []string // the copy moves, in order
	}{
		{name: "deals to the hero whose play set it off", part: omens, count: 2, scope: "h-tam", by: director,
			line: "Dealt 2 cards from Omens to Tam's hand.", draws: 1, moves: []string{"deal c1 h-tam", "deal c2 h-tam"}},
		{name: "one card", part: omens, count: 1, scope: "h-tam", by: director,
			line: "Dealt 1 card from Omens to Tam's hand.", draws: 1, moves: []string{"deal c1 h-tam"}},
		{name: "a pile with fewer cards deals what it holds", part: omens, count: 5, scope: "h-tam", by: director,
			line: "Dealt 3 cards from Omens to Tam's hand; the pile held only 3.", draws: 1, moves: []string{"deal c1 h-tam", "deal c2 h-tam", "deal c3 h-tam"}},
		{name: "a hero the roster does not name", part: omens, count: 1, scope: "h-new", by: director,
			line: "Dealt 1 card from Omens to a hero's hand.", draws: 1, moves: []string{"deal c1 h-new"}},
		{name: "no hero to draw for", part: omens, count: 1, scope: "", by: director,
			refused: "There is no hero to draw for", code: 422},
		{name: "an empty deck gives the deck's own message", part: omens, count: 1, scope: "h-tam", by: director,
			setup:   func(f *drawDecks) { f.drawErr = apperror.NewConflict("Nothing left in this deck.") },
			refused: "Nothing left in this deck.", code: 409, draws: 1},
		{name: "a deck that is gone", part: gone, count: 1, scope: "h-tam", by: director,
			refused: "The deck Boons is no longer one of this campaign's decks; choose it again in the Workshop.", code: 422},
		{name: "a refused first deal puts every drawn card back", part: omens, count: 2, scope: "h-tam", by: director,
			setup: func(f *drawDecks) {
				f.dealErr = map[int]error{0: apperror.NewValidation("that hero page is not in this campaign")}
			},
			refused: "that hero page is not in this campaign", code: 422, draws: 1,
			moves: []string{"refused c1", "put-back c1", "put-back c2"}},
		{name: "a deal lost halfway says what was dealt and puts the rest back", part: omens, count: 3, scope: "h-tam", by: director,
			setup:   func(f *drawDecks) { f.dealErr = map[int]error{1: apperror.NewConflict("That card has moved.")} },
			refused: "Dealt 1 card from Omens to Tam's hand, then stopped: That card has moved.", code: 409, draws: 1,
			moves: []string{"deal c1 h-tam", "refused c2", "put-back c2", "put-back c3"}},
		{name: "never as a player", part: omens, count: 1, scope: "h-tam", by: homebrew.Viewer{UserID: "u-tam"},
			refused: "Only a Director", code: 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDrawDecks()
			if tt.setup != nil {
				tt.setup(f)
			}
			st := drawStory(f, true)
			args, _ := json.Marshal(map[string]int{"count": tt.count})
			line, err := st.draw(context.Background(), "camp", homebrew.Act{Part: tt.part, Args: args, Scope: tt.scope, By: tt.by})
			if tt.refused != "" {
				if hbCode(err) != tt.code || !strings.Contains(err.Error(), tt.refused) {
					t.Fatalf("err = %v (code %d), want %d saying %q", err, hbCode(err), tt.code, tt.refused)
				}
			} else if err != nil || line != tt.line {
				t.Fatalf("got %q, %v; want %q", line, err, tt.line)
			}
			if len(f.draws) != tt.draws {
				t.Fatalf("draws = %+v, want %d", f.draws, tt.draws)
			}
			for i, d := range f.draws {
				// The draw is a Director's to the shown spot, never a player's
				// into a hand, so every guard of a Director's draw applies.
				if d.HeroID != "" || d.Count != tt.count || !f.viewers[i].Director || f.viewers[i].UserID != tt.by.UserID {
					t.Errorf("draw %d = %+v as %+v", i, d, f.viewers[i])
				}
			}
			if strings.Join(f.moves, "|") != strings.Join(tt.moves, "|") {
				t.Errorf("moves = %q, want %q", f.moves, tt.moves)
			}
		})
	}
}

// Dealing a card to someone is bigger than a number moving, so a rule that
// draws is held for a Director however it is timed.
func TestDeckDrawWaits(t *testing.T) {
	reg := homebrew.NewRegistry()
	if err := registerDecksHomebrew(reg, drawStory(newDrawDecks(), true)); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range reg.Catalog().Actions {
		if a.Name != deckDraw {
			continue
		}
		found = true
		if !a.Waits || a.ClientRun || len(a.PartKinds) != 1 || a.PartKinds[0] != deckPartKind || a.Says != "draws {count} from {part}" {
			t.Errorf("deck.draw = %+v", a)
		}
		if len(a.Fields) != 1 || a.Fields[0].Name != "count" || a.Fields[0].Type != homebrew.FieldWhole || a.Fields[0].Optional {
			t.Errorf("deck.draw fields = %+v", a.Fields)
		}
	}
	if !found {
		t.Fatal("deck.draw is not registered")
	}
}
