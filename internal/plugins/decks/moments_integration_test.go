package decks

// Moments on the real database: the unique key, the claim inside the draw
// transaction, and the races between two Directors and between a Director and
// a player. Skips without CHRONICLE_TEST_DB_DSN.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// syncCapture records what the service tells people, safe across goroutines.
type syncCapture struct {
	mu      sync.Mutex
	told    []Audience
	rings   []BellEntry
	moments int
}

func (c *syncCapture) Changed(_ string, to Audience) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.told = append(c.told, to)
}

func (c *syncCapture) Ring(_ context.Context, _ string, e BellEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rings = append(c.rings, e)
	return nil
}

func (c *syncCapture) MomentChanged(string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.moments++
}

func (c *syncCapture) momentCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.moments
}

type momentRig struct {
	t     *testing.T
	fx    *dbFixture
	svc   Service
	f     *fakeFacts
	cap   *syncCapture
	dir   Viewer
	tam   Viewer
	moms  MomentRepository
	pokes atomic.Int64
}

func newMomentRig(t *testing.T) *momentRig {
	t.Helper()
	fx := newDBFixture(t)
	r := &momentRig{t: t, fx: fx, f: newFakeFacts(), cap: &syncCapture{}, moms: NewMomentRepository(fx.db)}
	r.f.opts.Heroes = []OptionHero{{ID: fx.hero, Name: "Tam"}, {ID: fx.hero2, Name: "Ilse"}}
	heroes := fakeHeroes{pages: map[string]Hero{
		fx.hero:  {ID: fx.hero, Name: "Tam", OwnerUserID: fx.tam},
		fx.hero2: {ID: fx.hero2, Name: "Ilse"},
	}}
	members := fakeMembers{list: []Member{
		{UserID: fx.owner, Role: 3, Director: true, JoinedAt: longAgo},
		{UserID: fx.tam, Role: 1, JoinedAt: longAgo},
	}}
	r.svc = NewService(fx.repo, heroes, members, WithNotifier(r.cap), WithBell(r.cap), WithFacts(r.f), WithMoments(r.moms),
		WithMomentNotifier(r.cap), WithRandom(func(int) int { return 0 }), WithPoke(func(string) { r.pokes.Add(1) }))
	r.dir = Viewer{UserID: fx.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	r.tam = Viewer{UserID: fx.tam, MemberRole: 1, VisibilityRole: 1}
	return r
}

func (r *momentRig) must(err error) {
	r.t.Helper()
	if err != nil {
		r.t.Fatalf("unexpected error: %v", err)
	}
}

// deck makes a deck as the Director.
func (r *momentRig) deck(name, kind, holder string, settings *SettingsInput) string {
	r.t.Helper()
	v, err := r.svc.Create(bg, r.fx.campaign, r.dir, CreateDeckInput{Name: name, Kind: kind, HolderEntityID: holder, Settings: settings})
	r.must(err)
	return v.(DirectorDeck).ID
}

func (r *momentRig) card(deck, name string, copies int, when WhenList) string {
	r.t.Helper()
	v, err := r.svc.CreateCard(bg, r.fx.campaign, deck, r.dir, CreateCardInput{Name: name, Copies: ptr(copies), When: when})
	r.must(err)
	for _, c := range v.(DirectorDeck).Cards {
		if c.Name == name {
			return c.ID
		}
	}
	r.t.Fatalf("card %q not stored", name)
	return ""
}

// cursed is a test on whether Tam's page has the tag Cursed.
func (r *momentRig) cursed(has bool) WhenList {
	return WhenList{{Test: TestTag, Who: r.fx.hero, Tag: it(7), Has: bl(has)}}
}

func (r *momentRig) setCursed(on bool) {
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	if on {
		r.f.tags[r.fx.hero] = []TagFact{{ID: 7, Name: "Cursed"}}
	} else {
		delete(r.f.tags, r.fx.hero)
	}
}

func (r *momentRig) recheck() {
	r.t.Helper()
	r.must(r.svc.Recheck(bg, r.fx.campaign))
}

func (r *momentRig) open() []MomentView {
	r.t.Helper()
	list, err := r.moms.ListOpen(bg, r.fx.campaign)
	r.must(err)
	out := make([]MomentView, 0, len(list))
	for _, m := range list {
		out = append(out, MomentView{ID: m.ID, DeckID: m.DeckID, State: m.State, FitCount: m.FitCount, Quiet: m.Quiet,
			Card: &DirectorCard{ID: m.CardID}, InvitedUserID: m.InvitedUserID})
	}
	return out
}

func (r *momentRig) closedState(id string) string {
	r.t.Helper()
	m, ok, err := r.moms.Get(bg, r.fx.campaign, id)
	r.must(err)
	if !ok {
		r.t.Fatalf("no moment %s", id)
	}
	return m.State
}

func (r *momentRig) pile(deck string) int {
	r.t.Helper()
	counts, err := r.moms.PileCounts(bg, r.fx.campaign, deck)
	r.must(err)
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

func TestIntegration_AMomentRisesOnTheEdgeAndNotWhileItKeepsHolding(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	card := r.card(deck, "Doom", 1, r.cursed(true))

	r.recheck()
	if len(r.open()) != 0 {
		t.Fatal("a moment came before the test held")
	}
	r.setCursed(true)
	before := r.cap.momentCount()
	r.recheck()
	open := r.open()
	if len(open) != 1 || open[0].Card.ID != card || open[0].State != MomentWaiting || open[0].FitCount != 1 {
		t.Fatalf("open moments = %+v", open)
	}
	if r.cap.momentCount() != before+1 {
		t.Errorf("Directors were told %d times", r.cap.momentCount()-before)
	}
	// Looking again while nothing changed raises nothing and tells no one.
	told := r.cap.momentCount()
	r.recheck()
	r.recheck()
	if len(r.open()) != 1 || r.cap.momentCount() != told {
		t.Errorf("a recheck with nothing new changed things: %d open, told %d", len(r.open()), r.cap.momentCount()-told)
	}

	// Not now: the card still fits, so nothing comes back until it stops and starts.
	_, err := r.svc.DismissMoment(bg, r.fx.campaign, open[0].ID, r.dir)
	r.must(err)
	r.recheck()
	if len(r.open()) != 0 {
		t.Fatal("a dismissed moment came straight back")
	}
	r.setCursed(false)
	r.recheck()
	r.setCursed(true)
	r.recheck()
	again := r.open()
	if len(again) != 1 {
		t.Fatalf("after the edge, open = %+v", again)
	}
	if !again[0].Quiet {
		t.Error("a moment within five minutes of the last one should be quiet")
	}
}

func TestIntegration_AMomentLapsesWhenNothingFitsAndNeverDeals(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "Doom", 1, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	open := r.open()
	if len(open) != 1 {
		t.Fatalf("open = %+v", open)
	}
	r.setCursed(false)
	r.recheck()
	if len(r.open()) != 0 || r.closedState(open[0].ID) != MomentLapsed {
		t.Errorf("the moment did not lapse: %q", r.closedState(open[0].ID))
	}
	if r.pile(deck) != 1 {
		t.Error("a moment moved a card")
	}
}

func TestIntegration_AMomentIsPickedAgainWhileAnotherCardFits(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	a := r.card(deck, "A", 1, r.cursed(true))
	b := r.card(deck, "B", 1, WhenList{{Test: TestSay}})
	r.setCursed(true)
	_, err := r.svc.Say(bg, r.fx.campaign, deck, b, r.dir, true)
	r.must(err)
	r.recheck()
	open := r.open()
	if len(open) != 1 || open[0].FitCount != 2 {
		t.Fatalf("open = %+v", open)
	}
	first := open[0].Card.ID
	if first != a && first != b {
		t.Fatalf("picked %s", first)
	}
	// The picked card stops fitting; the other is picked.
	if first == a {
		r.setCursed(false)
	} else {
		_, err = r.svc.Say(bg, r.fx.campaign, deck, b, r.dir, false)
		r.must(err)
	}
	r.recheck()
	open = r.open()
	if len(open) != 1 || open[0].Card.ID == first {
		t.Fatalf("after the first stopped fitting, open = %+v", open)
	}
}

func TestIntegration_OnlyOneMomentIsOpenPerDeckEvenUnderRacingRechecks(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "A", 1, r.cursed(true))
	r.card(deck, "B", 1, r.cursed(true))
	r.setCursed(true)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.svc.Recheck(bg, r.fx.campaign)
		}()
	}
	wg.Wait()
	if n := len(r.open()); n != 1 {
		t.Fatalf("%d moments are open for one deck", n)
	}
	var open int
	if err := r.fx.db.QueryRow(`SELECT COUNT(*) FROM deck_moments WHERE deck_id = ? AND open_deck_id IS NOT NULL`, deck).Scan(&open); err != nil || open != 1 {
		t.Fatalf("open rows = %d, %v", open, err)
	}
}

func TestIntegration_ADirectorDealsAMomentOnce(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	card := r.card(deck, "Doom", 3, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]

	// Two Directors answer at once: one deal happens, the other is told it was
	// answered, and exactly one copy leaves the pile.
	var ok, gone atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.svc.DealMoment(bg, r.fx.campaign, m.ID, r.dir)
			switch code(err) {
			case 0:
				ok.Add(1)
			case http.StatusConflict:
				if !strings.Contains(err.Error(), "already been answered") {
					t.Errorf("conflict says %v", err)
				}
				gone.Add(1)
			default:
				t.Errorf("deal: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || gone.Load() != 1 {
		t.Fatalf("deals ok %d, answered %d", ok.Load(), gone.Load())
	}
	if r.pile(deck) != 2 {
		t.Errorf("pile = %d, want 2", r.pile(deck))
	}
	if r.closedState(m.ID) != MomentDealt {
		t.Errorf("state = %q", r.closedState(m.ID))
	}
	// Deals and the rest of the answers on a closed moment are 409s.
	for name, call := range map[string]func() error{
		"deal":    func() error { _, err := r.svc.DealMoment(bg, r.fx.campaign, m.ID, r.dir); return err },
		"another": func() error { _, err := r.svc.AnotherMoment(bg, r.fx.campaign, m.ID, r.dir); return err },
		"invite":  func() error { _, err := r.svc.InviteMoment(bg, r.fx.campaign, m.ID, r.dir); return err },
		"dismiss": func() error { _, err := r.svc.DismissMoment(bg, r.fx.campaign, m.ID, r.dir); return err },
	} {
		if err := call(); code(err) != http.StatusConflict {
			t.Errorf("%s on a closed moment: %v", name, err)
		}
	}
	_ = card
}

func TestIntegration_ADealRacingADismissLeavesOneAnswer(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "Doom", 2, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]
	var wins atomic.Int64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := r.svc.DealMoment(bg, r.fx.campaign, m.ID, r.dir); err == nil {
			wins.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := r.svc.DismissMoment(bg, r.fx.campaign, m.ID, r.dir); err == nil {
			wins.Add(1)
		}
	}()
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d answers won", wins.Load())
	}
	switch r.closedState(m.ID) {
	case MomentDealt:
		if r.pile(deck) != 1 {
			t.Errorf("dealt, but pile = %d", r.pile(deck))
		}
	case MomentDismissed:
		if r.pile(deck) != 2 {
			t.Errorf("dismissed, but pile = %d", r.pile(deck))
		}
	default:
		t.Errorf("state = %q", r.closedState(m.ID))
	}
}

func TestIntegration_DrawAnotherPicksAmongTheOtherCardsThatFit(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	a := r.card(deck, "A", 1, r.cursed(true))
	b := r.card(deck, "B", 1, r.cursed(true))
	r.card(deck, "C", 1, r.cursed(false))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]
	got, err := r.svc.AnotherMoment(bg, r.fx.campaign, m.ID, r.dir)
	r.must(err)
	if got.Card == nil || got.Card.ID == m.Card.ID || (got.Card.ID != a && got.Card.ID != b) {
		t.Fatalf("another picked %+v after %s", got.Card, m.Card.ID)
	}
	// With only one card fitting there is no other.
	r.setCursed(false)
	r.recheck()
	for _, o := range r.open() {
		if _, err := r.svc.AnotherMoment(bg, r.fx.campaign, o.ID, r.dir); err == nil {
			t.Error("another succeeded with no other card fitting")
		}
	}
}

func TestIntegration_InvitingLetsThePlayerDrawForFreeAndClosesTheMoment(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Tam's omens", KindHero, r.fx.hero, &SettingsInput{Draws: ptr("directors")})
	card := r.card(deck, "Doom", 2, r.cursed(true))
	r.card(deck, "Fine", 2, nil)
	// Tam is chosen for the deck with one draw, which an invited draw must not spend.
	_, err := r.svc.SetPlayers(bg, r.fx.campaign, deck, r.dir, SetPlayersInput{Version: 1, Players: []PlayerInput{{UserID: r.fx.tam, DrawsLeft: ptr(1)}}})
	r.must(err)
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]

	// Before an invitation Tam cannot draw (the Director draws on this deck).
	if _, err := r.svc.Draw(bg, r.fx.campaign, deck, r.tam, DrawInput{HeroID: r.fx.hero}); code(err) != http.StatusForbidden {
		t.Fatalf("an uninvited draw: %v", err)
	}
	if _, err := r.svc.InviteMoment(bg, r.fx.campaign, m.ID, r.tam); code(err) != http.StatusNotFound {
		t.Fatalf("a player invited: %v", err)
	}
	v, err := r.svc.InviteMoment(bg, r.fx.campaign, m.ID, r.dir)
	r.must(err)
	if v.State != MomentInvited || v.InvitedUserID != r.fx.tam {
		t.Fatalf("moment = %+v", v)
	}
	r.cap.mu.Lock()
	rang := len(r.cap.rings) == 1 && r.cap.rings[0].Kind == BellInvited && len(r.cap.rings[0].Users) == 1 && r.cap.rings[0].Users[0] == r.fx.tam
	rangText := fmt.Sprintf("%+v", r.cap.rings)
	r.cap.mu.Unlock()
	if !rang {
		t.Errorf("bell = %s", rangText)
	}
	if strings.Contains(rangText, "Cursed") || strings.Contains(rangText, "Doom") {
		t.Errorf("the invitation names the card or the test: %s", rangText)
	}
	// The invited draw picks among the cards that fit, so it can only be Doom
	// (the one with a test) and not "Fine".
	res, err := r.svc.Draw(bg, r.fx.campaign, deck, r.tam, DrawInput{HeroID: r.fx.hero})
	r.must(err)
	got, ok, err := r.fx.repo.GetCopy(bg, r.fx.campaign, deck, res.CopyID)
	if err != nil || !ok || got.CardID != card || got.Zone != ZoneHand || got.HolderEntityID != r.fx.hero {
		t.Fatalf("the invited draw gave %+v, %v", got, err)
	}
	if r.closedState(m.ID) != MomentDrawn {
		t.Errorf("state = %q", r.closedState(m.ID))
	}
	d, _, _ := r.fx.repo.GetDeck(bg, r.fx.campaign, deck)
	if len(d.Players) != 1 || d.Players[0].DrawsLeft == nil || *d.Players[0].DrawsLeft != 1 {
		t.Errorf("the invited draw spent a draw: %+v", d.Players)
	}
	// The invitation is used up.
	if _, err := r.svc.Draw(bg, r.fx.campaign, deck, r.tam, DrawInput{HeroID: r.fx.hero}); code(err) != http.StatusForbidden {
		t.Errorf("a second draw: %v", err)
	}
}

func TestIntegration_AnInvitationIsTakenBackByDismissingAndLapsesWithTheFacts(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Tam's omens", KindHero, r.fx.hero, nil)
	r.card(deck, "Doom", 1, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]
	_, err := r.svc.InviteMoment(bg, r.fx.campaign, m.ID, r.dir)
	r.must(err)
	r.cap.mu.Lock()
	r.cap.told = nil
	r.cap.mu.Unlock()
	r.setCursed(false)
	r.recheck()
	if r.closedState(m.ID) != MomentLapsed {
		t.Fatalf("state = %q", r.closedState(m.ID))
	}
	r.cap.mu.Lock()
	toldTam := false
	for _, a := range r.cap.told {
		toldTam = toldTam || (len(a.Users) == 1 && a.Users[0] == r.fx.tam)
	}
	r.cap.mu.Unlock()
	if !toldTam {
		t.Error("the invited player was not told the invitation ended")
	}
	if _, err := r.svc.Draw(bg, r.fx.campaign, deck, r.tam, DrawInput{HeroID: r.fx.hero}); code(err) == 0 {
		t.Error("a lapsed invitation still let the player draw")
	}
}

func TestIntegration_SayingSoRaisesAMomentAndClearsWhenACopyLeaves(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	card := r.card(deck, "Say", 2, WhenList{{Test: TestSay}})
	r.recheck()
	if len(r.open()) != 0 {
		t.Fatal("an unsaid card raised a moment")
	}
	_, err := r.svc.Say(bg, r.fx.campaign, deck, card, r.dir, true)
	r.must(err)
	r.recheck()
	m := r.open()
	if len(m) != 1 {
		t.Fatalf("open = %+v", m)
	}
	if _, err := r.svc.DealMoment(bg, r.fx.campaign, m[0].ID, r.dir); err != nil {
		t.Fatal(err)
	}
	cards, err := r.fx.repo.ListCards(bg, r.fx.campaign, deck)
	r.must(err)
	if cards[0].SaidAt != nil {
		t.Error("said_at survived a copy leaving the pile")
	}
	r.recheck()
	if len(r.open()) != 0 {
		t.Error("the second copy raised a moment without being told so again")
	}
	if _, err := r.svc.Say(bg, r.fx.campaign, deck, "00000000-0000-0000-0000-0000000000ee", r.dir, true); code(err) != http.StatusNotFound {
		t.Errorf("saying a card that is not there: %v", err)
	}
}

func TestIntegration_OnlyDirectorsSeeMoments(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "Doom", 2, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	if _, err := r.svc.Moments(bg, r.fx.campaign, r.tam); code(err) != http.StatusNotFound {
		t.Errorf("a player read the moments: %v", err)
	}
	if _, err := r.svc.Moments(bg, r.fx.campaign, Viewer{}); code(err) != http.StatusNotFound {
		t.Errorf("a visitor read the moments: %v", err)
	}
	list, err := r.svc.Moments(bg, r.fx.campaign, r.dir)
	r.must(err)
	if len(list) != 1 || list[0].Card == nil || len(list[0].Answers) == 0 || list[0].Answers[0] != AnswerDeal {
		t.Fatalf("the Director reads %+v", list)
	}
}

func TestIntegration_ADirectorsReadLooksAgainAtMostOnceEveryFewSeconds(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "Doom", 1, r.cursed(true))
	_, err := r.svc.Moments(bg, r.fx.campaign, r.dir)
	r.must(err)
	first := r.f.reads
	for i := 0; i < 5; i++ {
		_, err = r.svc.Moments(bg, r.fx.campaign, r.dir)
		r.must(err)
	}
	if r.f.reads != first {
		t.Errorf("facts were read %d more times by repeated reads", r.f.reads-first)
	}
	// A change no hook saw is caught by the first read after the window.
	r.setCursed(true)
	s := r.svc.(*service)
	s.lookedMu.Lock()
	s.looked[r.fx.campaign] = time.Now().Add(-time.Minute)
	s.lookedMu.Unlock()
	list, err := r.svc.Moments(bg, r.fx.campaign, r.dir)
	r.must(err)
	if len(list) != 1 {
		t.Errorf("a read after the window did not look again: %+v", list)
	}
}

func TestIntegration_RecheckIsOneIndexedQueryWhenNothingIsWatched(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	r.card(deck, "Plain", 1, nil)
	before := r.f.reads
	r.recheck()
	if r.f.reads != before {
		t.Error("a campaign with no test read facts")
	}
	ids, err := r.moms.WatchedDecks(bg, r.fx.campaign)
	r.must(err)
	if len(ids) != 0 {
		t.Errorf("watched = %v", ids)
	}
	r.card(deck, "Doom", 1, r.cursed(true))
	ids, err = r.moms.WatchedDecks(bg, r.fx.campaign)
	r.must(err)
	if len(ids) != 1 || ids[0] != deck {
		t.Errorf("watched = %v", ids)
	}
	// A quiet deck is not watched.
	cur, _, err := r.fx.repo.GetDeck(bg, r.fx.campaign, deck)
	r.must(err)
	_, err = r.svc.Update(bg, r.fx.campaign, deck, r.dir, UpdateDeckInput{Version: patchOf(cur.Version), Settings: patchOf(SettingsInput{Notice: ptr("quiet")})})
	if err != nil {
		t.Fatal(err)
	}
	ids, err = r.moms.WatchedDecks(bg, r.fx.campaign)
	r.must(err)
	if len(ids) != 0 {
		t.Errorf("a quiet deck is watched: %v", ids)
	}
}

// A draw restricted to cards that fit takes only those, says so when none is
// allowed, and reshuffles only a pile that is really empty: a pile of copies
// that do not fit is not empty.
func TestIntegration_ARestrictedDrawNeverReshufflesAPileThatHoldsCopies(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	fits := fx.card(t, d, "Fits", 1)
	unfit := fx.card(t, d, "Unfit", 1)
	plan := drawPlan(fx, d, Place{Zone: ZoneShown})
	plan.Restrict, plan.Allowed = true, map[string]bool{fits.ID: true}
	got, err := fx.draw1(plan)
	if err != nil || got.CardID != fits.ID {
		t.Fatalf("draw = %+v, %v", got, err)
	}
	// Put the drawn copy in the used pile; only the unfit card is left in the pile.
	fx.exec(t, `UPDATE deck_copies SET zone='used', holder_entity_id=NULL, face_up=1 WHERE id = ?`, got.ID)
	plan.Reshuffle = true
	if _, err := fx.draw1(plan); err != errNoneFit {
		t.Fatalf("nothing allowed in a pile that holds copies: %v", err)
	}
	if z := fx.zones(t, d); z.Used != 1 || z.Pile != 1 {
		t.Errorf("zones = %+v: the used copy must stay used", z)
	}
	// A really empty pile still reshuffles.
	fx.exec(t, `UPDATE deck_copies SET zone='gone' WHERE card_id = ?`, unfit.ID)
	again, err := fx.draw1(plan)
	if err != nil || again.CardID != fits.ID {
		t.Fatalf("reshuffled draw = %+v, %v", again, err)
	}
	// An unrestricted plan sees the unfit card too.
	fx.exec(t, `UPDATE deck_copies SET zone='pile' WHERE card_id = ?`, unfit.ID)
	open := drawPlan(fx, d, Place{Zone: ZoneShown})
	if c, err := fx.draw1(open); err != nil || c.CardID != unfit.ID {
		t.Fatalf("unrestricted draw = %+v, %v", c, err)
	}
}

// A moment claimed inside the draw closes with it, and a draw whose claim was
// lost moves nothing.
func TestIntegration_AClaimedMomentClosesWithTheDrawAndALostClaimMovesNothing(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	card := r.card(deck, "Doom", 2, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	m := r.open()[0]
	claim := func(id string) DrawPlan {
		return DrawPlan{CampaignID: r.fx.campaign, DeckID: deck, To: Place{Zone: ZonePlay, FaceUp: true}, CardID: card, Pick: firstPick,
			Moment: &MomentClaim{ID: id, From: []string{MomentWaiting}, To: MomentDealt, By: r.fx.owner},
			Log:    LogLine{Action: ActDeal, ActorUserID: r.fx.owner}}
	}
	if _, err := r.fx.repo.Draw(bg, claim("00000000-0000-0000-0000-0000000000aa")); err != errMomentGone {
		t.Fatalf("a claim on no moment: %v", err)
	}
	if r.pile(deck) != 2 {
		t.Fatalf("a lost claim moved a copy: pile = %d", r.pile(deck))
	}
	if _, err := r.fx.repo.Draw(bg, claim(m.ID)); err != nil {
		t.Fatal(err)
	}
	if r.closedState(m.ID) != MomentDealt || r.pile(deck) != 1 {
		t.Errorf("state %q, pile %d", r.closedState(m.ID), r.pile(deck))
	}
	if _, err := r.fx.repo.Draw(bg, claim(m.ID)); err != errMomentGone {
		t.Errorf("a second claim: %v", err)
	}
}

// A look that read the cards before another look marked them must not raise a
// moment of its own: the store, not the stale read, says what really started.
func TestIntegration_OnlyTheLookThatMarksACardStartsIt(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	card := r.card(deck, "Doom", 1, r.cursed(true))
	n, err := r.moms.SetFits(bg, r.fx.campaign, deck, []string{card}, nil)
	r.must(err)
	if n != 1 {
		t.Fatalf("the first look started %d cards", n)
	}
	n, err = r.moms.SetFits(bg, r.fx.campaign, deck, []string{card}, nil)
	r.must(err)
	if n != 0 {
		t.Errorf("a second look started %d cards again", n)
	}
}

// A look that judged a moment's old card must not write that card's hero and
// reading onto the moment once it has been picked again.
func TestIntegration_ARefreshForACardTheMomentLeftChangesNothing(t *testing.T) {
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", nil)
	a := r.card(deck, "Doom", 1, r.cursed(true))
	b := r.card(deck, "Gloom", 1, r.cursed(true))
	r.setCursed(true)
	r.recheck()
	open := r.open()
	if len(open) != 1 {
		t.Fatalf("open = %+v", open)
	}
	held, other := a, b
	if open[0].Card.ID == b {
		held, other = b, a
	}
	moved, err := r.moms.Refresh(bg, r.fx.campaign, open[0].ID, other, Moment{FitCount: 9, Reading: Reading{Text: "stale"}})
	r.must(err)
	if moved {
		t.Fatal("a refresh for another card was written")
	}
	if now := r.open(); len(now) != 1 || now[0].Card.ID != held || now[0].FitCount == 9 {
		t.Errorf("the moment changed: %+v", now)
	}
	moved, err = r.moms.Refresh(bg, r.fx.campaign, open[0].ID, held, Moment{FitCount: 2, Reading: Reading{Text: strings.Repeat("x", 900)}})
	r.must(err)
	if !moved {
		t.Error("a refresh for the held card was not written")
	}
}

// momentWorld plays the same deck over the real store with and without a
// moment that opens, is told to wait, and lapses, and returns everything a
// player can read and every message sent to someone who is not a Director.
func momentWorld(t *testing.T, stirs bool) map[string]string {
	t.Helper()
	r := newMomentRig(t)
	deck := r.deck("Omens", KindEvent, "", &SettingsInput{Knows: ptr("kinds"), Draws: ptr("players")})
	_, err := r.svc.SetPlayers(bg, r.fx.campaign, deck, r.dir, SetPlayersInput{Version: 1, Players: []PlayerInput{{UserID: r.fx.tam, DrawsLeft: ptr(3)}}})
	r.must(err)
	r.card(deck, "Plain", 2, nil)
	r.card(deck, "Doom", 1, r.cursed(true))
	r.recheck()
	if stirs {
		r.setCursed(true)
		r.recheck()
		open := r.open()
		if len(open) != 1 {
			t.Fatalf("no moment opened: %+v", open)
		}
		_, err := r.svc.AnotherMoment(bg, r.fx.campaign, open[0].ID, r.dir)
		if err == nil {
			t.Fatal("drawing another with one card that fits should be refused")
		}
		_, err = r.svc.DismissMoment(bg, r.fx.campaign, open[0].ID, r.dir)
		r.must(err)
		r.setCursed(false)
		r.recheck()
		r.setCursed(true)
		r.recheck()
		r.setCursed(false)
		r.recheck()
		if len(r.open()) != 0 {
			t.Fatal("the second moment did not lapse")
		}
	}
	out := map[string]string{}
	put := func(label string, v any, err error) {
		if err != nil {
			out[label] = "error " + err.Error()
			return
		}
		raw, _ := json.Marshal(v)
		s := string(raw)
		for id, word := range map[string]string{deck: "DECK", r.fx.campaign: "CAMPAIGN", r.fx.hero: "HERO", r.fx.hero2: "HERO2", r.fx.tam: "TAM", r.fx.owner: "OWNER"} {
			s = strings.ReplaceAll(s, id, word)
		}
		out[label] = stripIDs(s)
	}
	l, err := r.svc.List(bg, r.fx.campaign, r.tam)
	put("list", l, err)
	d, err := r.svc.Get(bg, r.fx.campaign, deck, r.tam)
	put("deck", d, err)
	tb, err := r.svc.Table(bg, r.fx.campaign, r.tam)
	put("table", tb, err)
	h, err := r.svc.Hand(bg, r.fx.campaign, r.tam)
	put("hand", h, err)
	res, err := r.svc.Draw(bg, r.fx.campaign, deck, r.tam, DrawInput{HeroID: r.fx.hero})
	if err == nil {
		res.CopyID, res.CopyIDs = "", nil
	}
	put("draw", res, err)
	r.cap.mu.Lock()
	defer r.cap.mu.Unlock()
	put("told", r.cap.told, nil)
	put("rings", r.cap.rings, nil)
	return out
}

// A moment that opens, is refused another card, is told to wait, comes again
// and lapses changes nothing a player reads and sends them nothing.
func TestIntegration_TwoWorldsAMomentComingAndGoingIsInvisibleToPlayers(t *testing.T) {
	with, without := momentWorld(t, true), momentWorld(t, false)
	if len(with) == 0 || len(with) != len(without) {
		t.Fatalf("worlds returned %d and %d replies", len(with), len(without))
	}
	for k := range with {
		if with[k] != without[k] {
			t.Errorf("%s differs:\n with a moment: %s\n without:       %s", k, with[k], without[k])
		}
	}
}
