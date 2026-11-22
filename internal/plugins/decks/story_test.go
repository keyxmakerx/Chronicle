package decks

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// recStory records what the deck told the story, in order.
type recStory struct {
	drawn, played []CardEvent
	panics        bool
}

func (r *recStory) CardDrawn(_ context.Context, campaignID string, e CardEvent) {
	if campaignID != camp {
		panic("wrong campaign " + campaignID)
	}
	r.drawn = append(r.drawn, e)
	if r.panics {
		panic("the rule blew up")
	}
}

func (r *recStory) CardPlayed(_ context.Context, campaignID string, e CardEvent) {
	if campaignID != camp {
		panic("wrong campaign " + campaignID)
	}
	r.played = append(r.played, e)
	if r.panics {
		panic("the rule blew up")
	}
}

// clock matches the wall-clock times a log line and a deal are stamped with,
// which differ between two runs of the same script.
var clock = regexp.MustCompile(`\d{4}-\d\d-\d\dT[0-9:.]+Z`)

func eventLines(es []CardEvent) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.CardName+"@"+e.HeroID+" by "+e.By.UserID)
	}
	return out
}

// A draw or a deal tells once per card it put somewhere, and only a play that
// really happened is a play: an ask is not, a Director's yes is.
func TestStoryIsToldOncePerCardAfterTheMoveIsStored(t *testing.T) {
	st := &recStory{}
	fx := newFixtureWith(t, WithStory(st))
	deck := fx.deck("Omens", KindHero, &SettingsInput{Arrives: ptr("up"), Draws: ptr("players")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	ash := fx.card(deck, CreateCardInput{Name: "Ash", Copies: ptr(6)})
	boon := fx.card(deck, CreateCardInput{Name: "Boon", AskFirst: true})
	fx.card(deck, CreateCardInput{Name: "Wind", Copies: ptr(3)})

	check := func(step string, wantDrawn, wantPlayed []string) {
		t.Helper()
		if got := eventLines(st.drawn); strings.Join(got, "|") != strings.Join(wantDrawn, "|") {
			t.Errorf("%s: drawn = %v, want %v", step, got, wantDrawn)
		}
		if got := eventLines(st.played); strings.Join(got, "|") != strings.Join(wantPlayed, "|") {
			t.Errorf("%s: played = %v, want %v", step, got, wantPlayed)
		}
		st.drawn, st.played = nil, nil
	}

	res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3})
	fx.must(err)
	if len(res.CopyIDs) != 3 {
		t.Fatalf("a draw of three took %d", len(res.CopyIDs))
	}
	// A Director's draw to the shown spot is a look, not an arrival.
	check("a Director's draw to the shown spot", nil, nil)

	// A card looked at and then dealt fires exactly once, on the deal.
	_, err = fx.svc.Act(bg, camp, deck, res.CopyIDs[0], "deal", fx.dir, ActInput{To: "hand", HeroID: "h-tam"})
	fx.must(err)
	check("deal from shown", []string{"Ash@h-tam by u-dir"}, nil)

	// Deal by name, to the table.
	_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: ash, To: "table"})
	fx.must(err)
	check("deal by name to the table", []string{"Ash@ by u-dir"}, nil)

	// Another and a put back are the Director looking again.
	_, err = fx.svc.Act(bg, camp, deck, res.CopyIDs[1], "another", fx.dir, ActInput{})
	fx.must(err)
	check("another", nil, nil)
	_, err = fx.svc.Act(bg, camp, deck, res.CopyIDs[2], "put-back", fx.dir, ActInput{})
	fx.must(err)
	check("a put back", nil, nil)

	// A player's own draw.
	pr, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
	fx.must(err)
	if len(st.drawn) != 1 || st.drawn[0].HeroID != "h-tam" || st.drawn[0].By.UserID != "u-tam" || len(st.played) != 0 {
		t.Fatalf("a player's draw told %+v", st.drawn)
	}
	st.drawn = nil

	// Playing: Tam plays the card he drew (up, because the deck arrives up).
	if _, err := fx.svc.Act(bg, camp, deck, pr.CopyID, "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if len(st.played) != 1 || st.played[0].HeroID != "h-tam" || st.played[0].By.UserID != "u-tam" || len(st.drawn) != 0 {
		t.Fatalf("a player's play told played=%+v drawn=%+v", st.played, st.drawn)
	}
	st.played = nil

	// Ask first: the ask tells nothing, the Director's yes tells once, a no never.
	b, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: boon, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	st.drawn = nil
	if _, err := fx.svc.Act(bg, camp, deck, b.CopyID, "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	check("the ask", nil, nil)
	if _, err := fx.svc.Act(bg, camp, deck, b.CopyID, "turn-down", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
	check("a turn-down", nil, nil)
	if _, err := fx.svc.Act(bg, camp, deck, b.CopyID, "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Act(bg, camp, deck, b.CopyID, "approve", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
	check("a yes", nil, []string{"Boon@h-tam by u-dir"})

	// A move that fails tells nothing.
	if _, err := fx.svc.Act(bg, camp, deck, b.CopyID, "play", fx.tam, ActInput{}); err == nil {
		t.Fatal("a played card played again")
	}
	check("a refused play", nil, nil)
}

// Whether anyone is listening, and whether the listener panics, changes
// nothing a move answers and nothing any player is told or rung for. The same
// script runs in three worlds and every reply and every notice (the last entry) must match.
func TestTwoWorldsAStoryListenerChangesNoReplyAndNoAudience(t *testing.T) {
	// The deck's and cards' ids are random per world, so they are scrubbed from
	// what is compared, as are wall-clock times.
	run := func(t *testing.T, st Story) []string {
		var opts []Option
		if st != nil {
			opts = append(opts, WithStory(st))
		}
		fx := newFixtureWith(t, opts...)
		deck := fx.deck("Omens", KindHero, &SettingsInput{Knows: ptr("count"), Arrives: ptr("down"), OthersSee: ptr("back"), Draws: ptr("players")})
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-ilse"})
		fx.card(deck, CreateCardInput{Name: "Dragon", Words: "DRAGONWORDS", Copies: ptr(3)})
		ask := fx.card(deck, CreateCardInput{Name: "Pact", AskFirst: true, Copies: ptr(2)})
		fx.card(deck, CreateCardInput{Name: "Wind", Copies: ptr(3)})
		var replies []string
		keep := func(v any, err error) {
			t.Helper()
			fx.must(err)
			raw, _ := json.Marshal(v)
			replies = append(replies, string(raw))
		}
		view := func(user Viewer) {
			// A Scribe on no list is answered 404; that answer is compared too.
			d, err := fx.svc.Get(bg, camp, deck, user)
			if err != nil {
				replies = append(replies, err.Error())
				return
			}
			keep(d, err)
		}
		drawn, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3})
		keep(drawn, err)
		dealt, err := fx.svc.Act(bg, camp, deck, drawn.CopyIDs[0], "deal", fx.dir, ActInput{To: "hand", HeroID: "h-ilse"})
		keep(dealt, err)
		// A player's own draw of a card that lies face down.
		mine, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		keep(mine, err)
		turned, err := fx.svc.Act(bg, camp, deck, mine.CopyID, "turn", fx.tam, ActInput{})
		keep(turned, err)
		played, err := fx.svc.Act(bg, camp, deck, mine.CopyID, "play", fx.tam, ActInput{})
		keep(played, err)
		pact, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: ask, To: "hand", HeroID: "h-tam"})
		keep(pact, err)
		turned, err = fx.svc.Act(bg, camp, deck, pact.CopyID, "turn", fx.tam, ActInput{})
		keep(turned, err)
		asked, err := fx.svc.Act(bg, camp, deck, pact.CopyID, "play", fx.tam, ActInput{})
		keep(asked, err)
		approved, err := fx.svc.Act(bg, camp, deck, pact.CopyID, "approve", fx.dir, ActInput{})
		keep(approved, err)
		another, err := fx.svc.Act(bg, camp, deck, drawn.CopyIDs[1], "another", fx.dir, ActInput{})
		keep(another, err)
		for _, v := range []Viewer{fx.dir, fx.tam, fx.ilse, fx.scribe} {
			view(v)
		}
		notices, _ := json.Marshal([]any{fx.cap.told, fx.cap.rings})
		replies = append(replies, string(notices))
		for i := range replies {
			replies[i] = clock.ReplaceAllString(replies[i], "TIME")
			replies[i] = strings.ReplaceAll(replies[i], deck, "DECK")
			for _, c := range fx.repo.cards {
				replies[i] = strings.ReplaceAll(replies[i], c.ID, "CARD-"+c.Name)
			}
		}
		return replies
	}
	base := run(t, nil)
	for name, st := range map[string]Story{"a listener": &recStory{}, "a listener that panics": &recStory{panics: true}} {
		t.Run(name, func(t *testing.T) {
			got := run(t, st)
			if len(got) != len(base) {
				t.Fatalf("%d replies, want %d", len(got), len(base))
			}
			for i := range base {
				if got[i] != base[i] {
					at := 0
					for at < len(got[i]) && at < len(base[i]) && got[i][at] == base[i][at] {
						at++
					}
					lo := max(at-60, 0)
					t.Errorf("reply %d differs near byte %d:\n without: ...%s\n with:    ...%s", i, at, base[i][lo:min(at+60, len(base[i]))], got[i][lo:min(at+60, len(got[i]))])
				}
			}
		})
	}
}
