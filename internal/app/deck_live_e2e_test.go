package app

// deck_live_e2e_test.go proves what a player's socket is sent when decks move.
// It runs the real decks service over a scratch database, turns each
// notification into the same hub message the app sends, and asks the hub's own
// delivery gate which connections would receive it. Skipped under -short and
// without CHRONICLE_TEST_DB_DSN.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// socketBus records every message the publisher sends and, per connection,
// which of them the hub's gate lets through.
type socketBus struct {
	clients map[string]*ws.Client
	seen    map[string][]string
	all     int
}

func (b *socketBus) Publish(m *ws.Message) {
	b.all++
	raw, err := m.Encode()
	if err != nil {
		panic(err)
	}
	for name, c := range b.clients {
		if m.DeliversTo(c) {
			b.seen[name] = append(b.seen[name], string(raw))
		}
	}
}

// liveRun plays one visible deal, plus the hidden Director acts when asked,
// and returns what each connection was sent.
func liveRun(t *testing.T, hidden bool) map[string][]string {
	t.Helper()
	w := newCardWorld(t)
	bus := &socketBus{seen: map[string][]string{}, clients: map[string]*ws.Client{
		"director": {UserID: w.owner, Role: 3},
		"tam":      {UserID: w.tam, Role: 1},
		"ilse":     {UserID: w.ilse, Role: 1},
		"scribe":   {UserID: w.scribe, Role: 2},
	}}
	campSvc := campaigns.NewCampaignService(campaigns.NewCampaignRepository(w.db), trashUserFinder{}, nil, nil, "http://localhost")
	w.decks = decks.NewService(decks.NewRepository(w.db),
		cardHeroes{owner: map[string]string{w.tam: w.heroTam, w.ilse: w.heroIlse}},
		&decksMemberAdapter{svc: campSvc},
		decks.WithPictures(&decksPictureAdapter{svc: w.mediaSvc}),
		decks.WithNotifier(&decksLivePublisher{bus: bus}))
	ctx := context.Background()

	// Omens is a deck Tam and Ilse both see and count, so anything a Director
	// does to its pile out of their sight would show up here as a message.
	down, count := "down", "count"
	omens := w.deck("Omens", &decks.SettingsInput{Arrives: &down, Knows: &count})
	w.choose(omens, w.tam, w.ilse)
	seen := w.card(omens, "The Drowned Bell", "")
	var fillers []string
	for _, n := range []string{"One", "Two", "Three", "Four", "Five"} {
		fillers = append(fillers, w.card(omens, "Filler "+n, ""))
	}
	bus.all = 0
	bus.seen = map[string][]string{}

	// On the deck the players see: look at five and put them all back, look at
	// one, swap it for another and put that back, name a next and a last card,
	// and edit a card nobody holds.
	hiddenOnSeenDeck := func() {
		res, err := w.decks.Draw(ctx, w.camp, omens, w.dir_, decks.DrawInput{Count: 5})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range res.CopyIDs {
			w.act(omens, id, "put-back", w.dir_)
		}
		res, err = w.decks.Draw(ctx, w.camp, omens, w.dir_, decks.DrawInput{Count: 1})
		if err != nil {
			t.Fatal(err)
		}
		again, err := w.decks.Act(ctx, w.camp, omens, res.CopyIDs[0], "another", w.dir_, decks.ActInput{})
		if err != nil {
			t.Fatal(err)
		}
		w.act(omens, again.CopyID, "put-back", w.dir_)
		d, err := w.decks.Get(ctx, w.camp, omens, w.dir_)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.decks.Update(ctx, w.camp, omens, w.dir_, decks.UpdateDeckInput{
			Version: patch.Of(d.(decks.DirectorDeck).Version), NextCardID: patch.Of(fillers[0]), LastCardID: patch.Of(fillers[1])}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.decks.UpdateCard(ctx, w.camp, omens, fillers[2], w.dir_, decks.UpdateCardInput{Words: patch.Of("edited words")}); err != nil {
			t.Fatal(err)
		}
	}

	hiddenActs := func() {
		hiddenOnSeenDeck()
		vault := w.deck("Vault", nil)
		var ids []string
		for _, n := range []string{"A", "B", "C", "D", "E", "F"} {
			ids = append(ids, w.card(vault, "Secret "+n, ""))
		}
		res, err := w.decks.Draw(ctx, w.camp, vault, w.dir_, decks.DrawInput{Count: 5})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range res.CopyIDs[:4] {
			w.act(vault, id, "put-back", w.dir_)
		}
		d, err := w.decks.Get(ctx, w.camp, vault, w.dir_)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.decks.Update(ctx, w.camp, vault, w.dir_, decks.UpdateDeckInput{
			Version: patch.Of(d.(decks.DirectorDeck).Version), NextCardID: patch.Of(ids[1]), LastCardID: patch.Of(ids[0])}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.decks.UpdateCard(ctx, w.camp, vault, ids[2], w.dir_, decks.UpdateCardInput{Words: patch.Of("edited words")}); err != nil {
			t.Fatal(err)
		}
		npc := w.page("Nobody's hero", "")
		d, err = w.decks.Get(ctx, w.camp, vault, w.dir_)
		if err != nil {
			t.Fatal(err)
		}
		inPile := ""
		for _, c := range d.(decks.DirectorDeck).Copies {
			if c.Zone == decks.ZonePile {
				inPile = c.CardID
				break
			}
		}
		if _, err := w.decks.DealCard(ctx, w.camp, vault, w.dir_, decks.DealCardInput{CardID: inPile, To: "hand", HeroID: npc}); err != nil {
			t.Fatal(err)
		}
	}
	if hidden {
		hiddenActs()
	}
	w.deal(omens, seen, w.heroTam)
	// The two worlds are two campaigns; only their ids may differ.
	out := map[string][]string{}
	for who, msgs := range bus.seen {
		for _, raw := range msgs {
			out[who] = append(out[who], strings.ReplaceAll(raw, w.camp, "CAMPAIGN"))
		}
	}
	return out
}

// A player's socket is sent the same messages, in the same order, whether or
// not the Director also did things the player may not know about.
func TestLiveMessagesAreTheSameInTwoWorlds(t *testing.T) {
	plain := liveRun(t, false)
	busy := liveRun(t, true)
	for _, who := range []string{"tam", "ilse", "scribe"} {
		if !reflect.DeepEqual(plain[who], busy[who]) {
			t.Fatalf("what %s was sent differs:\nplain %v\nbusy  %v", who, plain[who], busy[who])
		}
	}
	if len(busy["director"]) <= len(plain["director"]) {
		t.Errorf("the Director was not told of the hidden acts: plain %d, busy %d", len(plain["director"]), len(busy["director"]))
	}
	// The deal is one message: to its holder, to the other player who counts
	// the pile, and to the Director. A Scribe the deck is not for hears nothing.
	if len(plain["tam"]) != 1 || len(plain["ilse"]) != 1 || len(plain["director"]) != 1 {
		t.Fatalf("the deal told tam %d, ilse %d, the Director %d times, want once each", len(plain["tam"]), len(plain["ilse"]), len(plain["director"]))
	}
	if len(plain["scribe"]) != 0 {
		t.Errorf("someone who may not see the deck was told: %v", plain["scribe"])
	}
	for who, msgs := range plain {
		for _, raw := range msgs {
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				t.Fatal(err)
			}
			if m["payload"] != nil || m["resourceId"] != nil || strings.Contains(raw, "Drowned") {
				t.Errorf("%s was sent more than \"your decks view changed\": %s", who, raw)
			}
		}
	}
}
