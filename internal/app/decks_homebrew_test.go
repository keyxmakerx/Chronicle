package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func TestCardEventChecks(t *testing.T) {
	long := strings.Repeat("x", decks.MaxNameChars+1)
	tests := []struct {
		name string
		args string
		want string // normalised args; "" means refused
	}{
		{"nothing set means any card", `{}`, `{}`},
		{"names are trimmed and single-spaced", `{"deck":"  Omens  ","card":" Ash   on the wind "}`, `{"deck":"Omens","card":"Ash on the wind"}`},
		{"blank is any", `{"deck":"   "}`, `{}`},
		{"deck only", `{"deck":"Omens"}`, `{"deck":"Omens"}`},
		{"unknown field", `{"deck":"Omens","hero":"Tam"}`, ""},
		{"wrong type", `{"deck":5}`, ""},
		{"too long", `{"card":"` + long + `"}`, ""},
		{"control character", `{"card":"a\u0007b"}`, ""},
	}
	for _, tt := range tests {
		for _, event := range []string{cardPlayedEvent, cardDrawnEvent} {
			t.Run(event+" "+tt.name, func(t *testing.T) {
				got, err := checkCardNames(context.Background(), "camp", json.RawMessage(tt.args))
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
}

func TestCardEventMatch(t *testing.T) {
	payload := json.RawMessage(`{"deckId":"d1","deck":"Omens","cardId":"c1","card":"Second wind"}`)
	tests := []struct {
		name string
		args string
		want bool
	}{
		{"neither", `{}`, true},
		{"deck", `{"deck":"Omens"}`, true},
		{"card", `{"card":"Second wind"}`, true},
		{"both", `{"deck":"Omens","card":"Second wind"}`, true},
		{"deck in other case", `{"deck":"oMENS"}`, true},
		{"card in other case", `{"card":"SECOND WIND"}`, true},
		{"both in other case", `{"deck":"omens","card":"second wind"}`, true},
		{"wrong deck", `{"deck":"Boons"}`, false},
		{"wrong card", `{"card":"Ash"}`, false},
		{"right deck, wrong card", `{"deck":"Omens","card":"Ash"}`, false},
		{"wrong deck, right card", `{"deck":"Boons","card":"Second wind"}`, false},
		{"a part of a name is not the name", `{"card":"Second"}`, false},
		{"absent args", ``, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cardMatches(json.RawMessage(tt.args), payload)
			if err != nil || got != tt.want {
				t.Errorf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	if _, err := cardMatches(json.RawMessage(`{}`), json.RawMessage(`nope`)); err == nil {
		t.Error("a payload that is not JSON should be an error")
	}
}

func TestDecksHomebrewRegistration(t *testing.T) {
	reg := homebrew.NewRegistry()
	st := newDecksStory(nil)
	if err := registerDecksHomebrew(reg, st); err != nil {
		t.Fatal(err)
	}
	if err := registerDecksHomebrew(reg, st); err == nil {
		t.Error("registering twice succeeded")
	}
	c := reg.Catalog()
	events, actions := map[string]homebrew.CatalogEntry{}, map[string]homebrew.CatalogEntry{}
	for _, e := range c.Events {
		events[e.Name] = e
	}
	for _, a := range c.Actions {
		actions[a.Name] = a
	}
	for _, name := range []string{cardPlayedEvent, cardDrawnEvent, cardPlayAsked} {
		e, ok := events[name]
		if !ok || len(e.Fields) != 2 || e.Fields[0].Name != "deck" || e.Fields[1].Name != "card" || len(e.PartKinds) != 0 {
			t.Errorf("event %s = %+v", name, e)
		}
	}
	for _, name := range []string{deckAddCopies, deckTakeOut, deckShuffleBack, deckDraw} {
		if _, ok := actions[name]; !ok {
			t.Errorf("action %s is not registered", name)
		}
	}
	// A rule ending in any deck action saves, timed for now, and is held.
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
	for _, a := range []struct{ name, args string }{
		{deckAddCopies, `{"deck":"Omens","card":"Ash","copies":2}`},
		{deckTakeOut, `{"deck":"Omens","card":"Ash"}`},
		{deckShuffleBack, `{"deck":"Omens"}`},
	} {
		r, err := saveDoc(t, svc, homebrew.Doc{Rules: []homebrew.Rule{{When: homebrew.When{Event: cardPlayedEvent}, Timing: homebrew.TimingNow,
			Then: []homebrew.Do{{Action: a.name, Args: json.RawMessage(a.args)}}}}})
		if err != nil || r.Doc.Rules[0].Then[0].Action != a.name {
			t.Errorf("%s: %v", a.name, err)
		}
	}
	for name := range actions {
		if strings.HasPrefix(name, "deck.") && name != deckAddCopies && name != deckTakeOut && name != deckShuffleBack && name != deckDraw {
			t.Errorf("an unexpected deck action %s", name)
		}
	}
}

func TestDeckActionChecks(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check homebrew.CheckFunc
		args  string
		want  string
	}{
		{"add copies", checkAddCopies, `{"deck":" Omens ","card":"Ash","copies":2}`, `{"deck":"Omens","card":"Ash","copies":2}`},
		{"add copies, no deck", checkAddCopies, `{"card":"Ash","copies":2}`, ""},
		{"add copies, no card", checkAddCopies, `{"deck":"Omens","copies":2}`, ""},
		{"add copies, none", checkAddCopies, `{"deck":"Omens","card":"Ash","copies":0}`, ""},
		{"add copies, absent", checkAddCopies, `{"deck":"Omens","card":"Ash"}`, ""},
		{"add copies, the limit", checkAddCopies, fmt.Sprintf(`{"deck":"Omens","card":"Ash","copies":%d}`, decks.MaxCopiesPerCard), `{"deck":"Omens","card":"Ash","copies":20}`},
		{"add copies, over the limit", checkAddCopies, fmt.Sprintf(`{"deck":"Omens","card":"Ash","copies":%d}`, decks.MaxCopiesPerCard+1), ""},
		{"take out", checkDeckAndCard, `{"deck":"Omens","card":"Ash"}`, `{"deck":"Omens","card":"Ash"}`},
		{"take out, no card", checkDeckAndCard, `{"deck":"Omens"}`, ""},
		{"shuffle back", checkDeckOnly, `{"deck":"Omens"}`, `{"deck":"Omens"}`},
		{"shuffle back, no deck", checkDeckOnly, `{}`, ""},
		{"shuffle back, a card is not a field", checkDeckOnly, `{"deck":"Omens","card":"Ash"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.check(ctx, "camp", json.RawMessage(tt.args))
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

// storyRecorder records what a story firing hands the Workshop.
type storyRecorder struct {
	homebrew.Service
	fired []homebrew.Fired
	by    []homebrew.Viewer
	err   error
	boom  bool
}

func (r *storyRecorder) Fire(_ context.Context, _ string, v homebrew.Viewer, f homebrew.Fired) error {
	if r.boom {
		panic("the rule blew up")
	}
	r.fired = append(r.fired, f)
	r.by = append(r.by, v)
	return r.err
}

func inlineStory(on bool, hb homebrew.Service) *decksStory {
	st := newDecksStory(func(context.Context, string) bool { return on })
	st.spawn = func(f func()) { f() }
	if hb != nil {
		st.bindHomebrew(hb)
	}
	return st
}

func TestCardEventsFireWithTheHeroAsScopeAndADirectorsStanding(t *testing.T) {
	rec := &storyRecorder{}
	st := inlineStory(true, rec)
	e := decks.CardEvent{DeckID: "d1", DeckName: "Omens", CardID: "c1", CardName: "Second wind", HeroID: "h-tam", By: decks.Viewer{UserID: "u-tam", MemberRole: 1}}
	st.CardPlayed(context.Background(), "camp", e)
	e.HeroID = ""
	st.CardDrawn(context.Background(), "camp", e)
	if len(rec.fired) != 2 {
		t.Fatalf("fired %+v", rec.fired)
	}
	played, drawn := rec.fired[0], rec.fired[1]
	const payload = `{"deckId":"d1","deck":"Omens","cardId":"c1","card":"Second wind"}`
	if played.Event != "card.played" || played.Scope != "h-tam" || string(played.Payload) != payload || played.RecipeID != "" || played.Part != "" {
		t.Errorf("played = %+v", played)
	}
	if drawn.Event != "card.drawn" || drawn.Scope != "" || string(drawn.Payload) != payload {
		t.Errorf("drawn = %+v", drawn)
	}
	// A player's move starts the Director's rule with a Director's standing,
	// and the player stays the one named as having set it off.
	want := homebrew.Viewer{UserID: "u-tam", Director: true, VisibilityRole: permissions.RoleOwner}
	if rec.by[0] != want {
		t.Errorf("by = %+v, want %+v", rec.by[0], want)
	}
}

func TestCardEventsAreQuietWhenTheyShouldBe(t *testing.T) {
	e := decks.CardEvent{DeckName: "Omens", CardName: "Ash"}
	t.Run("add-on off", func(t *testing.T) {
		rec := &storyRecorder{}
		st := inlineStory(false, rec)
		st.CardPlayed(context.Background(), "camp", e)
		st.CardDrawn(context.Background(), "camp", e)
		if len(rec.fired) != 0 {
			t.Errorf("fired %+v", rec.fired)
		}
	})
	t.Run("homebrew not bound", func(t *testing.T) {
		inlineStory(true, nil).CardPlayed(context.Background(), "camp", e)
	})
	t.Run("no campaign", func(t *testing.T) {
		rec := &storyRecorder{}
		inlineStory(true, rec).CardPlayed(context.Background(), "", e)
		if len(rec.fired) != 0 {
			t.Errorf("fired %+v", rec.fired)
		}
	})
	t.Run("a failing rule is only logged", func(t *testing.T) {
		rec := &storyRecorder{err: fmt.Errorf("the rule store is down")}
		inlineStory(true, rec).CardPlayed(context.Background(), "camp", e)
		if len(rec.fired) != 1 {
			t.Errorf("fired %+v", rec.fired)
		}
	})
	t.Run("a panicking rule is caught", func(t *testing.T) {
		inlineStory(true, &storyRecorder{boom: true}).CardDrawn(context.Background(), "camp", e)
	})
}

// Off the move's own goroutine by default, so a slow rule cannot hold a move.
func TestCardEventsRunOffTheMovesGoroutineByDefault(t *testing.T) {
	rec := &storyRecorder{}
	st := newDecksStory(func(context.Context, string) bool { return true })
	st.bindHomebrew(rec)
	done := make(chan struct{})
	st.spawn = func(f func()) {
		go func() { f(); close(done) }()
	}
	st.CardPlayed(context.Background(), "camp", decks.CardEvent{DeckName: "Omens", CardName: "Ash"})
	<-done
	if len(rec.fired) != 1 {
		t.Errorf("fired %+v", rec.fired)
	}
}
