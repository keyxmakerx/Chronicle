package decks

// What the `decks` lookup shows each kind of key. A key that reads as a player
// must get nothing that player's own screens would not show them, so these
// run the real service over the real store, with a hidden test that holds and
// a moment that opens, and compare what each reader is told. Skips without
// CHRONICLE_TEST_DB_DSN.

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

const sightIlse = "00000000-0000-0000-0000-000000000003"

// sightWorld is one campaign: an event deck only the Director sees, with a
// card whose own test is hidden and a card lying on the table, and a hero deck
// with a face-up and a face-down card in each of two players' hands.
type sightWorld struct {
	t     *testing.T
	fx    *dbFixture
	svc   Service
	ai    *AI
	facts *fakeFacts
	cap   *syncCapture
	omens string
	boons string
	tam   Viewer
	ilse  Viewer
	dir   Viewer
}

// sightSwitches are what differs between worlds: whether the Director's own
// test holds, whether the moment it raises is left open, and which of two
// cards is the one Tam holds face down.
type sightSwitches struct {
	holds, moment, otherDown bool
}

func (w *sightWorld) must(err error) {
	w.t.Helper()
	if err != nil {
		w.t.Fatalf("unexpected error: %v", err)
	}
}

func (w *sightWorld) cardID(deck, name string) string {
	w.t.Helper()
	v, err := w.svc.Get(bg, w.fx.campaign, deck, w.dir)
	w.must(err)
	for _, c := range v.(DirectorDeck).Cards {
		if c.Name == name {
			return c.ID
		}
	}
	w.t.Fatalf("no card %q", name)
	return ""
}

// give deals a named card of Boons to a hero and, for a card that should lie
// face up, has its holder turn it over.
func (w *sightWorld) give(card, hero string, holder Viewer, faceUp bool) {
	w.t.Helper()
	res, err := w.svc.DealCard(bg, w.fx.campaign, w.boons, w.dir, DealCardInput{CardID: w.cardID(w.boons, card), To: "hand", HeroID: hero})
	w.must(err)
	if faceUp {
		_, err = w.svc.Act(bg, w.fx.campaign, w.boons, res.CopyID, verbTurn, holder, ActInput{})
		w.must(err)
	}
}

func newSightWorld(t *testing.T, sw sightSwitches) *sightWorld {
	t.Helper()
	fx := newDBFixture(t)
	fx.exec(t, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, sightIlse, "u3@example.test", "U", "x")
	w := &sightWorld{t: t, fx: fx, facts: newFakeFacts(), cap: &syncCapture{}}
	w.facts.opts.Heroes = []OptionHero{{ID: fx.hero, Name: "Tam"}, {ID: fx.hero2, Name: "Ilse"}}
	heroes := fakeHeroes{pages: map[string]Hero{
		fx.hero:  {ID: fx.hero, Name: "Tam", OwnerUserID: fx.tam},
		fx.hero2: {ID: fx.hero2, Name: "Ilse", OwnerUserID: sightIlse},
	}}
	members := fakeMembers{list: []Member{
		{UserID: fx.owner, Role: 3, Director: true, JoinedAt: longAgo},
		{UserID: fx.tam, Role: 1, Name: "Tam's player", JoinedAt: longAgo},
		{UserID: sightIlse, Role: 1, Name: "Ilse's player", JoinedAt: longAgo},
	}}
	w.svc = NewService(fx.repo, heroes, members, WithNotifier(w.cap), WithBell(w.cap), WithFacts(w.facts), WithMoments(NewMomentRepository(fx.db)),
		WithMomentNotifier(w.cap), WithRandom(func(int) int { return 0 }), WithPoke(func(string) {}))
	w.ai = NewAI(w.svc, aiPages{"tam": {fx.hero, "Tam"}}, func(context.Context, string) bool { return true })
	w.dir = Viewer{UserID: fx.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	w.tam = Viewer{UserID: fx.tam, MemberRole: 1, VisibilityRole: 1}
	w.ilse = Viewer{UserID: sightIlse, MemberRole: 1, VisibilityRole: 1}

	// The event deck: nobody but a Director may see it. Doom waits for a tag
	// the Director chose not to print on the card.
	v, err := w.svc.Create(bg, fx.campaign, w.dir, CreateDeckInput{Name: "Omens", Kind: KindEvent})
	w.must(err)
	w.omens = v.(DirectorDeck).ID
	for _, in := range []CreateCardInput{
		{Name: "Lantern", Words: "LANTERNWORDS", Copies: ptr(1)},
		{Name: "Doom", Words: "DOOMWORDS", Copies: ptr(1), When: WhenList{{Test: TestTag, Who: fx.hero, Tag: it(8), Has: bl(true)}}},
	} {
		_, err = w.svc.CreateCard(bg, fx.campaign, w.omens, w.dir, in)
		w.must(err)
	}
	_, err = w.svc.DealCard(bg, fx.campaign, w.omens, w.dir, DealCardInput{CardID: w.cardID(w.omens, "Lantern"), To: "table"})
	w.must(err)

	// The hero deck: four cards of two kinds, two to each player.
	v, err = w.svc.Create(bg, fx.campaign, w.dir, CreateDeckInput{Name: "Boons", Kind: KindHero,
		Look: &LookInput{Kinds: []KindDef{{Key: "blessing", Name: "Blessing", Colour: "teal"}, {Key: "hexed", Name: "HexedKind", Colour: "rose"}}}})
	w.must(err)
	w.boons = v.(DirectorDeck).ID
	for _, in := range []CreateCardInput{
		{Name: "Aegis", Words: "AEGISWORDS", KindKey: "blessing", Copies: ptr(1)},
		{Name: "Emberheart", Words: "EMBERWORDS", KindKey: "blessing", Copies: ptr(1)},
		{Name: "Vowbreaker", Words: "VOWWORDS", KindKey: "hexed", Copies: ptr(1)},
		{Name: "Veilcaller", Words: "VEILWORDS", KindKey: "hexed", Copies: ptr(1)},
		{Name: "Galeforce", Words: "GALEWORDS", KindKey: "hexed", Copies: ptr(1)},
	} {
		_, err = w.svc.CreateCard(bg, fx.campaign, w.boons, w.dir, in)
		w.must(err)
	}
	w.give("Aegis", fx.hero, w.tam, true)
	down := "Vowbreaker"
	if sw.otherDown {
		down = "Veilcaller"
	}
	w.give(down, fx.hero, w.tam, false)
	w.give("Emberheart", fx.hero2, w.ilse, true)
	w.give("Galeforce", fx.hero2, w.ilse, false)

	// The Director's own test holds or does not, and the moment it raises is
	// left waiting or set aside.
	if sw.holds {
		w.facts.mu.Lock()
		w.facts.tags[fx.hero] = []TagFact{{ID: 8, Name: "Secretly-a-spy"}}
		w.facts.mu.Unlock()
	}
	w.must(w.svc.Recheck(bg, fx.campaign))
	open, err := w.svc.Moments(bg, fx.campaign, w.dir)
	w.must(err)
	if sw.holds != (len(open) == 1) {
		t.Fatalf("holds=%v but %d moments are open", sw.holds, len(open))
	}
	if sw.holds && !sw.moment {
		_, err = w.svc.DismissMoment(bg, fx.campaign, open[0].ID, w.dir)
		w.must(err)
	}
	return w
}

func (w *sightWorld) actor(who Viewer, readsOwn bool, role int) aiplug.Actor {
	return aiplug.Actor{UserID: who.UserID, Role: role, ReadsOwnNotes: readsOwn}
}

// ask is the chip and the answer for one lookup as one actor, joined so a
// difference in either shows.
func (w *sightWorld) ask(a aiplug.Actor, deck string) string {
	w.t.Helper()
	fields := map[string]any{"what": "decks"}
	if deck != "" {
		fields["deck"] = deck
	}
	chip, text, err := w.ai.Lookup().Answer(bg, w.fx.campaign, a, aiplug.Record{Kind: "lookup", Fields: fields})
	if err != nil {
		return "error: " + err.Error()
	}
	if !strings.HasPrefix(text, "## ") {
		w.t.Fatalf("the answer does not start with its own heading: %q", text)
	}
	return chip + "\n" + text
}

// answers are what every reader of the world is told.
func (w *sightWorld) answers() map[string]string {
	owner := w.actor(w.dir, false, 3)
	tam := w.actor(w.tam, true, 1)
	ilse := w.actor(w.ilse, true, 1)
	safe := w.actor(w.dir, false, 1)
	return map[string]string{
		"tam":         w.ask(tam, ""),
		"tam/boons":   w.ask(tam, "Boons"),
		"tam/omens":   w.ask(tam, "Omens"),
		"tam/nowhere": w.ask(tam, "Omens-"),
		"ilse":        w.ask(ilse, ""),
		"ilse/boons":  w.ask(ilse, "Boons"),
		"ilse/omens":  w.ask(ilse, "Omens"),
		"safe":        w.ask(safe, ""),
		"safe/omens":  w.ask(safe, "Omens"),
		"safe/boons":  w.ask(safe, "Boons"),
		"owner":       w.ask(owner, ""),
		"owner/omens": w.ask(owner, "Omens"),
		"owner/boons": w.ask(owner, "Boons"),
	}
}

// Everything a player's own key must not carry, whatever the world.
var sightHidden = []string{
	// the other player's cards
	"Emberheart", "EMBERWORDS", "Galeforce", "GALEWORDS", "Ilse",
	// a face-down card's name, words and kind, whichever card it is
	"Vowbreaker", "VOWWORDS", "Veilcaller", "VEILWORDS", "HexedKind",
	// the event deck, its cards, the Director's own test and the moment
	"Omens", "Doom", "DOOMWORDS", "Cursed", "Lantern", "moment", "Moment", "fits", "waiting", "invite",
}

func TestIntegration_AIDecksALookupShowsEachKeyOnlyWhatItsScreensShow(t *testing.T) {
	w := newSightWorld(t, sightSwitches{holds: true, moment: true})
	got := w.answers()

	t.Run("the Director's answer names all of it", func(t *testing.T) {
		list, omens, boons := got["owner"], got["owner/omens"], got["owner/boons"]
		for _, want := range []string{"Omens (an event deck)", "Boons (a hero deck)", "A card's moment is waiting on you", "1 in the pile", "1 on the table"} {
			if !strings.Contains(list, want) {
				t.Errorf("the list lacks %q:\n%s", want, list)
			}
		}
		for _, want := range []string{"Doom", "DOOMWORDS", "Lantern", "Secretly-a-spy (only you)", "fits now", "### The open moment", "Doom fits and the moment is waiting",
			"On the table (1): Lantern", "Comes up next: none", "only Directors draw"} {
			if !strings.Contains(omens, want) {
				t.Errorf("the Omens answer lacks %q:\n%s", want, omens)
			}
		}
		for _, want := range []string{"In Tam's hand (2): Aegis (face up), Vowbreaker (face down)", "In Ilse's hand (2): Emberheart (face up), Galeforce (face down)",
			"kind Blessing", "kind HexedKind", "VOWWORDS", "In the pile (1): Veilcaller"} {
			if !strings.Contains(boons, want) {
				t.Errorf("the Boons answer lacks %q:\n%s", want, boons)
			}
		}
	})

	t.Run("a Director's answer says a test that does not hold", func(t *testing.T) {
		no := newSightWorld(t, sightSwitches{})
		omens := no.answers()["owner/omens"]
		if !strings.Contains(omens, "does not fit now") || strings.Contains(omens, "The open moment") {
			t.Errorf("answer:\n%s", omens)
		}
	})

	t.Run("each player's own key holds their face-up card and no one else's", func(t *testing.T) {
		tam := got["tam"]
		for _, want := range []string{"## Your cards", "Tam holds:", "- Aegis (Blessing): AEGISWORDS", "- a face-down card from Boons"} {
			if !strings.Contains(tam, want) {
				t.Errorf("Tam's answer lacks %q:\n%s", want, tam)
			}
		}
		if n := strings.Count(tam, "face-down"); n != 1 {
			t.Errorf("Tam holds one face-down card and the answer has %d lines of them:\n%s", n, tam)
		}
		ilse := got["ilse"]
		for _, want := range []string{"Ilse holds:", "- Emberheart (Blessing): EMBERWORDS", "- a face-down card from Boons"} {
			if !strings.Contains(ilse, want) {
				t.Errorf("Ilse's answer lacks %q:\n%s", want, ilse)
			}
		}
		for _, label := range []string{"tam", "tam/boons", "ilse", "ilse/boons"} {
			own := "Tam"
			if strings.HasPrefix(label, "ilse") {
				own = "Ilse"
			}
			for _, bad := range append(append([]string{}, sightHidden...), secretMarkers...) {
				if bad == own || (own == "Ilse" && (bad == "Emberheart" || bad == "EMBERWORDS")) {
					continue
				}
				if own == "Ilse" && (bad == "Ilse") {
					continue
				}
				// Tam's own cards are the other player's to Ilse and the reverse.
				if own == "Tam" && bad == "Ilse" || own == "Ilse" && (bad == "Aegis" || bad == "AEGISWORDS") {
					if !strings.Contains(got[label], bad) {
						continue
					}
				}
				if strings.Contains(got[label], bad) {
					t.Errorf("%s names %q:\n%s", label, bad, got[label])
				}
			}
		}
		if strings.Contains(got["ilse"], "Aegis") || strings.Contains(got["tam"], "Tam's player") {
			t.Errorf("a player's answer holds another's:\n%s\n%s", got["ilse"], got["tam"])
		}
	})

	t.Run("a deck a player cannot see answers as one that does not exist", func(t *testing.T) {
		asked := strings.ReplaceAll(got["tam/omens"], "Omens", "Omens-")
		if asked != got["tam/nowhere"] {
			t.Errorf("a hidden deck and a missing one differ:\n%s\n%s", got["tam/omens"], got["tam/nowhere"])
		}
		if !strings.Contains(got["tam/boons"], "- Aegis (Blessing)") || strings.Contains(got["tam/boons"], "Tam holds:\n- Aegis (Blessing): AEGISWORDS\n- a face-down card from Boons\nNothing") {
			t.Errorf("narrowing to Boons:\n%s", got["tam/boons"])
		}
	})

	t.Run("the what-players-see key holds only the cards lying face up on the table", func(t *testing.T) {
		safe := got["safe"]
		for _, want := range []string{"## Cards on the table", "- Lantern (Omens): LANTERNWORDS"} {
			if !strings.Contains(safe, want) {
				t.Errorf("the table answer lacks %q:\n%s", want, safe)
			}
		}
		for _, bad := range []string{"Aegis", "AEGISWORDS", "Emberheart", "Galeforce", "Vowbreaker", "Veilcaller", "HexedKind", "Doom", "DOOMWORDS", "Boons", "moment", "Moment", "Cursed", "pile", "Director"} {
			if strings.Contains(safe, bad) {
				t.Errorf("the table answer names %q:\n%s", bad, safe)
			}
		}
		for _, bad := range secretMarkers {
			if strings.Contains(safe, bad) {
				t.Errorf("the table answer names %q", bad)
			}
		}
		// Narrowed to a deck with nothing on the table it says so, as it does
		// for a deck that is not there.
		if !strings.Contains(got["safe/boons"], "No card of a deck called \"Boons\" is lying on the table.") {
			t.Errorf("answer:\n%s", got["safe/boons"])
		}
	})
}

// The same campaign four ways: nothing Director-only moves what a player or
// the what-players-see key is told, down to the byte.
func TestIntegration_AIDecksTwoWorldsAPlayersAnswerIsTheSameBytes(t *testing.T) {
	base := newSightWorld(t, sightSwitches{}).answers()
	worlds := map[string]sightSwitches{
		"the hidden test holds and the moment waits":      {holds: true, moment: true},
		"the hidden test holds and the moment was set by": {holds: true},
		"the other card is the one face down":             {otherDown: true},
		"the other card is face down, test and moment":    {holds: true, moment: true, otherDown: true},
	}
	for name, sw := range worlds {
		t.Run(name, func(t *testing.T) {
			got := newSightWorld(t, sw).answers()
			for label, text := range got {
				// The Director's own answer is supposed to differ.
				if strings.HasPrefix(label, "owner") {
					continue
				}
				if sortedLines(text) != sortedLines(base[label]) {
					t.Errorf("%s differs:\n here:  %q\n other: %q", label, text, base[label])
				}
			}
		})
	}
	for label, text := range base {
		if !strings.HasPrefix(label, "owner") && strings.Contains(text, "error") {
			t.Errorf("%s: %s", label, text)
		}
	}
}

// A player is told one thing of a moment: that they are invited to draw. No
// other player, and not the what-players-see key, is told it.
func TestIntegration_AIDecksOnlyTheInvitedPlayerIsToldOfAnInvitation(t *testing.T) {
	r := newMomentRig(t)
	ai := NewAI(r.svc, aiPages{}, func(context.Context, string) bool { return true })
	deck := r.deck("Tam's omens", KindHero, r.fx.hero, nil)
	r.card(deck, "Doom", 1, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]
	ask := func(a aiplug.Actor) string {
		_, text, err := ai.Lookup().Answer(bg, r.fx.campaign, a, aiplug.Record{Fields: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	tam := aiplug.Actor{UserID: r.fx.tam, Role: 1, ReadsOwnNotes: true}
	other := aiplug.Actor{UserID: "00000000-0000-0000-0000-0000000000ff", Role: 1, ReadsOwnNotes: true}
	safe := aiplug.Actor{UserID: r.fx.owner, Role: 1}
	before := ask(tam)
	_, err := r.svc.InviteMoment(bg, r.fx.campaign, m.ID, r.dir)
	r.must(err)
	after := ask(tam)
	if !strings.Contains(after, "Your Director invites you to draw from Tam's omens, into Tam's hand.") || strings.Contains(before, "invites") {
		t.Errorf("before:\n%s\nafter:\n%s", before, after)
	}
	for _, bad := range []string{"Doom", "Cursed", "moment", "fits"} {
		if strings.Contains(after, bad) {
			t.Errorf("the invitation names %q:\n%s", bad, after)
		}
	}
	for _, a := range []aiplug.Actor{other, safe} {
		if text := ask(a); strings.Contains(text, "invites") || strings.Contains(text, "Tam's omens") {
			t.Errorf("%+v is told of the invitation:\n%s", a, text)
		}
	}
}

// sortedLines makes two answers comparable regardless of the order of cards
// dealt within the same second: a copy's tie-break is its random id, which is
// no part of what a player is told, so only the lines themselves must match.
func sortedLines(s string) string {
	lines := strings.Split(s, "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// The what-players-see key reads Table as a viewer who owns no hero: Table
// refuses an empty id, and the Director's own id may own a hero page that
// players may not open, so a fixed id that matches no user is the safe one.
func TestIntegration_AIDecksTableViewerOwnsNoHero(t *testing.T) {
	w := newSightWorld(t, sightSwitches{holds: true, moment: true})
	if _, err := w.svc.Table(bg, w.fx.campaign, Viewer{MemberRole: 1, VisibilityRole: 1}); err == nil {
		t.Error("Table answered a viewer with no id")
	}
	if tableViewerID == w.fx.owner || tableViewerID == w.fx.tam || tableViewerID == sightIlse {
		t.Fatalf("the table viewer id is a real user's")
	}
	_, safe, err := w.ai.Lookup().Answer(bg, w.fx.campaign, w.actor(w.dir, false, 1), aiplug.Record{Kind: "lookup", Fields: map[string]any{"what": "decks"}})
	w.must(err)
	if strings.Contains(safe, "Aegis") || strings.Contains(safe, "Emberheart") {
		t.Errorf("the what-players-see answer holds a hero's card:\n%s", safe)
	}
}
