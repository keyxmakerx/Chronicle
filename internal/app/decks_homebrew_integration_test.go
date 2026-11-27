package app

// decks_homebrew_integration_test.go runs the card events and deck actions
// over the real decks, homebrew, trackers and entities services on a scratch
// database. Skipped under -short and without a database.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// deckCapture records what the decks service would have sent to browsers and
// to the bell.
type deckCapture struct {
	mu    sync.Mutex
	told  []decks.Audience
	rings []decks.BellEntry
}

func (c *deckCapture) Changed(_ string, to decks.Audience) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.told = append(c.told, to)
}

func (c *deckCapture) Ring(_ context.Context, _ string, e decks.BellEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rings = append(c.rings, e)
	return nil
}

func (c *deckCapture) snapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, _ := json.Marshal([]any{c.told, c.rings})
	return string(raw)
}

type storyWorld struct {
	t          *testing.T
	db         *sql.DB
	camp       string
	owner      string
	tamUser    string
	ilseUser   string
	tamHero    string
	orrinHero  string
	decks      decks.Service
	heroes     decks.Heroes
	members    decks.Members
	moments    decks.MomentRepository
	story      *decksStory
	homebrew   homebrew.Service
	trackers   trackers.Service
	rolls      rolls.Service
	capture    *deckCapture
	asks       *askCapture
	on         bool
	dir        decks.Viewer
	tam, ilse  decks.Viewer
	hbDirector homebrew.Viewer
}

var storyCtx = context.Background()

func newStoryWorld(t *testing.T) *storyWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("DB integration test")
	}
	plugin := func(slug string, fsys fs.FS) dbtest.PluginMigrations {
		sub, err := fs.Sub(fsys, database.PluginMigrationsSubdir)
		if err != nil {
			t.Fatal(err)
		}
		return dbtest.PluginMigrations{Slug: slug, FS: sub}
	}
	db := dbtest.Open(t, dbtest.Options{Prefix: "chronicle_deckstory_", Plugins: []dbtest.PluginMigrations{
		plugin(decks.PluginSlug, decks.MigrationsFS), plugin(trackers.PluginSlug, trackers.MigrationsFS),
		plugin(homebrew.PluginSlug, homebrew.MigrationsFS), plugin(rolls.PluginSlug, rolls.MigrationsFS)}})
	w := &storyWorld{t: t, db: db, on: true, capture: &deckCapture{}, asks: &askCapture{}}
	exec := func(q string, args ...any) { t.Helper(); mustGalleryExec(t, db, q, args...) }
	w.camp, w.owner, w.tamUser, w.ilseUser = galleryTestUUID(t), galleryTestUUID(t), galleryTestUUID(t), galleryTestUUID(t)
	for _, u := range []string{w.owner, w.tamUser, w.ilseUser} {
		exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, 'U', 'x')`, u, u+"@example.test")
	}
	exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, 'Story', ?, ?)`, w.camp, "c-"+w.camp[:8], w.owner)
	exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, fields) VALUES (?, 'hero', 'Hero', 'Heroes', ?)`, w.camp, heroFields)
	exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player'), (?, ?, 'player')`,
		w.camp, w.owner, w.camp, w.tamUser, w.camp, w.ilseUser)
	page := func(name, user string) string {
		id := galleryTestUUID(t)
		exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, owner_user_id, playable, fields_data, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'hero'), ?, ?, false, 'default', ?, ?, true, '{}', NOW(), NOW())`,
			id, w.camp, w.camp, name, "p-"+id[:8], w.owner, user)
		return id
	}
	w.tamHero, w.orrinHero = page("Tam", w.tamUser), page("Orrin", w.ilseUser)

	ents := entities.NewEntityService(entities.NewEntityRepository(db), entities.NewEntityTypeRepository(db), entities.NewEntityPermissionRepository(db))
	campSvc := campaigns.NewCampaignService(campaigns.NewCampaignRepository(db), trashUserFinder{}, nil, nil, "http://localhost")
	w.trackers = trackers.NewService(trackers.NewRepository(db), &trackersEntityAdapter{svc: ents, cards: entities.NewPageCards(db)}, &trackersMemberAdapter{svc: campSvc}, nil)

	w.story = newDecksStory(func(context.Context, string) bool { return w.on })
	w.story.spawn = func(f func()) { f() }
	w.moments = decks.NewMomentRepository(db)
	w.heroes, w.members = &decksHeroAdapter{svc: ents}, &decksMemberAdapter{svc: campSvc}
	w.decks = decks.NewService(decks.NewRepository(db), w.heroes, w.members,
		decks.WithNotifier(w.capture), decks.WithBell(w.capture), decks.WithMoments(w.moments),
		decks.WithRandom(func(int) int { return 0 }), decks.WithStory(w.story), decks.WithAsks(w.story))
	w.story.bindDecks(w.decks)

	reg := homebrew.NewRegistry()
	if err := registerTrackerParts(reg, w.trackers); err != nil {
		t.Fatal(err)
	}
	if err := registerDecksHomebrew(reg, w.story); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterAction(homebrew.AskDirector()); err != nil {
		t.Fatal(err)
	}
	w.rolls = rolls.NewService(rolls.NewRepository(db), nil, nil, &rollsCharacterAdapter{entities: ents})
	if err := reg.RegisterAction(rollFirstAction(w.rolls, nil)); err != nil {
		t.Fatal(err)
	}
	w.homebrew = homebrew.NewService(homebrew.NewRepository(db), reg, nil, homebrew.NewRunStore(db),
		homebrew.WithAskPeople(&homebrewAskPeople{svc: campSvc}), homebrew.WithBell(w.asks), homebrew.WithLive(w.asks),
		homebrew.WithPlayers(&homebrewScopePlayers{entities: ents}))
	w.story.bindHomebrew(w.homebrew)

	w.dir = decks.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	w.tam = decks.Viewer{UserID: w.tamUser, MemberRole: 1, VisibilityRole: 1}
	w.ilse = decks.Viewer{UserID: w.ilseUser, MemberRole: 1, VisibilityRole: 1}
	w.hbDirector = homebrew.Viewer{UserID: w.owner, Director: true, VisibilityRole: permissions.RoleOwner}
	return w
}

func (w *storyWorld) must(err error) {
	w.t.Helper()
	if err != nil {
		w.t.Fatalf("unexpected error: %v", err)
	}
}

func (w *storyWorld) deck(name, kind, holder string, s *decks.SettingsInput) string {
	w.t.Helper()
	v, err := w.decks.Create(storyCtx, w.camp, w.dir, decks.CreateDeckInput{Name: name, Kind: kind, HolderEntityID: holder, Settings: s})
	w.must(err)
	return v.(decks.DirectorDeck).ID
}

func (w *storyWorld) choose(deck string, players ...decks.PlayerInput) {
	w.t.Helper()
	v, err := w.decks.Get(storyCtx, w.camp, deck, w.dir)
	w.must(err)
	_, err = w.decks.SetPlayers(storyCtx, w.camp, deck, w.dir, decks.SetPlayersInput{Version: v.(decks.DirectorDeck).Version, Players: players})
	w.must(err)
}

func (w *storyWorld) card(deck, name string, copies int, in decks.CreateCardInput) string {
	w.t.Helper()
	in.Name, in.Copies = name, &copies
	v, err := w.decks.CreateCard(storyCtx, w.camp, deck, w.dir, in)
	w.must(err)
	for _, c := range v.(decks.DirectorDeck).Cards {
		if c.Name == name {
			return c.ID
		}
	}
	w.t.Fatalf("card %q not stored", name)
	return ""
}

func (w *storyWorld) view(deck string) decks.DirectorDeck {
	w.t.Helper()
	v, err := w.decks.Get(storyCtx, w.camp, deck, w.dir)
	w.must(err)
	return v.(decks.DirectorDeck)
}

// copyOf finds the id of one copy of a card in a zone.
func (w *storyWorld) copyOf(deck, cardID, zone string) string {
	w.t.Helper()
	for _, c := range w.view(deck).Copies {
		if c.CardID == cardID && c.Zone == zone {
			return c.ID
		}
	}
	w.t.Fatalf("no copy of the card in %s", zone)
	return ""
}

func (w *storyWorld) deal(deck, cardID, heroID string) string {
	w.t.Helper()
	res, err := w.decks.DealCard(storyCtx, w.camp, deck, w.dir, decks.DealCardInput{CardID: cardID, To: "hand", HeroID: heroID})
	w.must(err)
	return res.CopyID
}

// recipe saves a recipe with a Doom tracker part and these rules.
func (w *storyWorld) recipe(rules ...homebrew.Rule) *homebrew.Recipe {
	w.t.Helper()
	r, err := w.homebrew.Create(storyCtx, w.camp, w.hbDirector, homebrew.RecipeInput{Name: "Omens", Doc: homebrew.Doc{
		Parts: []homebrew.Part{{Key: "doom", Kind: "tracker", Label: "Doom", See: homebrew.SeeParty, Data: json.RawMessage(`{"shape":"count"}`)}},
		Rules: rules}})
	w.must(err)
	return r
}

func rule(event, args, timing string, then ...homebrew.Do) homebrew.Rule {
	return homebrew.Rule{When: homebrew.When{Event: event, Args: json.RawMessage(args)}, Timing: timing, Then: then}
}

func doomBy(n int) homebrew.Do {
	return homebrew.Do{Action: "step", Part: "doom", Args: json.RawMessage(fmt.Sprintf(`{"by":%d}`, n))}
}

func doAct(action, args string) homebrew.Do {
	return homebrew.Do{Action: action, Args: json.RawMessage(args)}
}

func (w *storyWorld) drawer() *homebrew.Drawer {
	w.t.Helper()
	d, err := w.homebrew.Drawer(storyCtx, w.camp, w.hbDirector)
	w.must(err)
	return d
}

// doomOn is the Doom tracker's value on a hero's page, -1 when it has none.
func (w *storyWorld) doomOn(page string) int {
	w.t.Helper()
	p, err := w.trackers.Page(storyCtx, w.camp, page, trackers.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true})
	w.must(err)
	for _, tv := range p.Trackers {
		if tv.Name == "Doom" && tv.ID != "" {
			return tv.Value
		}
	}
	return -1
}

func TestCardPlayedRunsANumberRuleAtOnceOnTheHeroWhoPlayedIt(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	wind := w.card(deck, "Second wind", 1, decks.CreateCardInput{})
	w.card(deck, "Other", 1, decks.CreateCardInput{})
	w.recipe(
		rule(cardPlayedEvent, `{"card":"second WIND"}`, homebrew.TimingNow, doomBy(6)),
		rule(cardPlayedEvent, `{"deck":"Elsewhere"}`, homebrew.TimingNow, doomBy(100)))
	copyID := w.deal(deck, wind, w.tamHero)

	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	d := w.drawer()
	if len(d.Waiting) != 0 || len(d.Recent) != 1 {
		t.Fatalf("a number rule runs at once: waiting %d, recent %d", len(d.Waiting), len(d.Recent))
	}
	r := d.Recent[0]
	if r.Status != homebrew.RunDone || strings.Join(r.Lines, "|") != "Doom is now 6." {
		t.Errorf("run = %+v", r)
	}
	if w.doomOn(w.tamHero) != 6 || w.doomOn(w.orrinHero) != -1 {
		t.Errorf("doom on Tam %d, on Orrin %d: the rule must land on the hero who played", w.doomOn(w.tamHero), w.doomOn(w.orrinHero))
	}
	// Played once, fired once: the same copy cannot be played again.
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err == nil {
		t.Fatal("a played card was played twice")
	}
	if got := len(w.drawer().Recent); got != 1 {
		t.Errorf("a refused play fired a rule: %d runs", got)
	}
	// Another card in the same deck is not Second wind.
	otherCopy := w.deal(deck, w.cardID(deck, "Other"), w.tamHero)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, otherCopy, "play", w.dir, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := len(w.drawer().Recent); got != 1 {
		t.Errorf("a different card's play started the rule: %d runs", got)
	}
}

func (w *storyWorld) cardID(deck, name string) string {
	w.t.Helper()
	for _, c := range w.view(deck).Cards {
		if c.Name == name {
			return c.ID
		}
	}
	w.t.Fatalf("no card %q", name)
	return ""
}

func ptrTo[T any](v T) *T { return &v }

func TestAskFirstFiresWhenADirectorLetsThemNotWhenTheyAsk(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	pact := w.card(deck, "Pact", 1, decks.CreateCardInput{AskFirst: true})
	w.recipe(rule(cardPlayedEvent, `{}`, homebrew.TimingNow, doomBy(1)))
	copyID := w.deal(deck, pact, w.tamHero)

	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	// The ask is the drawer's ask card, and card.played has not fired.
	if d := w.drawer(); len(d.Recent) != 0 || len(d.Waiting) != 1 || d.Waiting[0].Ask == nil || w.doomOn(w.tamHero) != -1 {
		t.Fatalf("the ask fired a rule: %+v", d)
	}
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "approve", w.dir, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	// Let them answered the ask (done) and the play set off the played rule.
	d := w.drawer()
	if len(d.Waiting) != 0 || len(d.Recent) != 2 || w.doomOn(w.tamHero) != 1 {
		t.Fatalf("Let them fired %+v, doom %d", d, w.doomOn(w.tamHero))
	}
	for _, r := range d.Recent {
		if r.Status != homebrew.RunDone {
			t.Errorf("run %+v", r)
		}
	}
}

// Each kind of draw fires once per card. A held rule is a count that does not
// change by itself: every firing is exactly one run waiting in the drawer.
func TestEveryKindOfDrawFiresOncePerCard(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Draws: ptrTo("players"), Arrives: ptrTo("up")})
	w.choose(deck, decks.PlayerInput{UserID: w.tamUser})
	ash := w.card(deck, "Ash", 6, decks.CreateCardInput{})
	say := w.card(deck, "Said so", 2, decks.CreateCardInput{When: decks.WhenList{{Test: decks.TestSay}}})
	w.recipe(rule(cardDrawnEvent, `{"deck":"omens"}`, homebrew.TimingNow, doAct(deckShuffleBack, `{"deck":"Omens"}`)))

	// Counted at the Workshop's door: a rule that waits is held once however
	// often the same card arrives, so the drawer is not a count of events.
	var fired []homebrew.Fired
	w.story.bindHomebrew(countedFires{Service: w.homebrew, fired: &fired})
	total := 0
	step := func(what string, want int) {
		t.Helper()
		total += want
		if len(fired) != total {
			t.Fatalf("%s: %d events fired, want %d", what, len(fired), total)
		}
		for _, f := range fired {
			if f.Event != cardDrawnEvent || !f.Once {
				t.Fatalf("%s: fired %+v, want a once-held %s", what, f, cardDrawnEvent)
			}
		}
	}
	drew, err := w.decks.Draw(storyCtx, w.camp, deck, w.dir, decks.DrawInput{Count: 3})
	w.must(err)
	step("a Director's draw of three is only a look", 0)
	_, err = w.decks.Act(storyCtx, w.camp, deck, drew.CopyIDs[0], "deal", w.dir, decks.ActInput{To: "hand", HeroID: w.tamHero})
	w.must(err)
	step("a card looked at, then dealt, fires once on the deal", 1)
	w.deal(deck, ash, w.orrinHero)
	step("a deal by name", 1)
	_, err = w.decks.Act(storyCtx, w.camp, deck, drew.CopyIDs[1], "another", w.dir, decks.ActInput{})
	w.must(err)
	step("another is only a look", 0)
	_, err = w.decks.Draw(storyCtx, w.camp, deck, w.tam, decks.DrawInput{HeroID: w.tamHero})
	w.must(err)
	step("a player's own draw", 1)

	// A moment: the Director deals it, or invites the hero's player to draw.
	_, err = w.decks.Say(storyCtx, w.camp, deck, say, w.dir, true)
	w.must(err)
	w.must(w.decks.Recheck(storyCtx, w.camp))
	open, err := w.decks.Moments(storyCtx, w.camp, w.dir)
	w.must(err)
	if len(open) != 1 {
		t.Fatalf("moments = %+v", open)
	}
	if _, err := w.decks.InviteMoment(storyCtx, w.camp, open[0].ID, w.dir); err != nil {
		t.Fatal(err)
	}
	step("inviting alone fires nothing", 0)
	if _, err := w.decks.Draw(storyCtx, w.camp, deck, w.tam, decks.DrawInput{HeroID: w.tamHero}); err != nil {
		t.Fatal(err)
	}
	step("an invited draw", 1)
	// The card stopped fitting when its copy left; a look says so, so saying it
	// again starts a new moment.
	w.must(w.decks.Recheck(storyCtx, w.camp))
	_, err = w.decks.Say(storyCtx, w.camp, deck, say, w.dir, true)
	w.must(err)
	w.must(w.decks.Recheck(storyCtx, w.camp))
	open, err = w.decks.Moments(storyCtx, w.camp, w.dir)
	w.must(err)
	if len(open) != 1 {
		t.Fatalf("moments = %+v", open)
	}
	if _, err := w.decks.DealMoment(storyCtx, w.camp, open[0].ID, w.dir); err != nil {
		t.Fatal(err)
	}
	step("a deal from a moment", 1)

	// A refused draw fires nothing.
	other := w.deck("Empty", decks.KindEvent, "", nil)
	if _, err := w.decks.Draw(storyCtx, w.camp, other, w.dir, decks.DrawInput{}); err == nil {
		t.Fatal("drew from an empty deck")
	}
	step("a draw that failed", 0)
}

// countedFires records each firing on its way into the Workshop.
type countedFires struct {
	homebrew.Service
	fired *[]homebrew.Fired
}

func (c countedFires) Fire(ctx context.Context, campaignID string, v homebrew.Viewer, f homebrew.Fired) error {
	*c.fired = append(*c.fired, f)
	return c.Service.Fire(ctx, campaignID, v, f)
}

// A player may draw and play the same card as often as the deck lets them. A
// rule that waits for the Director is asked once for it, not once per play, so
// a player cannot bury the Directors' drawer; another hero's play is its own.
func TestAPlayerPlayingACardAgainAndAgainHoldsARuleOnce(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindParty, "", &decks.SettingsInput{Arrives: ptrTo("up"), Fate: ptrTo("back")})
	ash := w.card(deck, "Ash", 1, decks.CreateCardInput{})
	w.recipe(rule(cardPlayedEvent, `{"card":"ash"}`, homebrew.TimingNow, doAct(deckShuffleBack, `{"deck":"Omens"}`)))
	play := func(hero string, by decks.Viewer) {
		t.Helper()
		copyID := w.deal(deck, ash, hero)
		_, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", by, decks.ActInput{})
		w.must(err)
	}
	for i := 0; i < 5; i++ {
		play(w.tamHero, w.tam)
	}
	if got := len(w.drawer().Waiting); got != 1 {
		t.Fatalf("five plays of one card by one hero: %d runs waiting, want 1", got)
	}
	play(w.orrinHero, w.ilse)
	d := w.drawer()
	if len(d.Waiting) != 2 {
		t.Fatalf("another hero's play is its own: %d runs waiting, want 2", len(d.Waiting))
	}
	// Once the Director has answered, the next play asks again.
	for _, r := range d.Waiting {
		if _, err := w.homebrew.RunHeld(storyCtx, w.camp, r.ID, w.hbDirector); err != nil {
			t.Fatal(err)
		}
	}
	play(w.tamHero, w.tam)
	if got := len(w.drawer().Waiting); got != 1 {
		t.Errorf("after the Director ran it: %d runs waiting, want 1", got)
	}
}

func TestDeckActionsWaitForADirectorThenRun(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, nil)
	ash := w.card(deck, "Ash on the wind", 4, decks.CreateCardInput{})
	w.card(deck, "Dust", 2, decks.CreateCardInput{})
	w.deal(deck, ash, w.orrinHero)
	w.recipe(
		rule(cardPlayedEvent, `{"card":"fire"}`, homebrew.TimingNow, doAct(deckAddCopies, `{"deck":"omens","card":"ash on the wind","copies":2}`)),
		rule(cardPlayedEvent, `{"card":"ice"}`, homebrew.TimingNow, doAct(deckTakeOut, `{"deck":"Omens","card":"Ash on the wind"}`)),
		rule(cardPlayedEvent, `{"card":"all"}`, homebrew.TimingNow, doAct(deckShuffleBack, `{"deck":"Omens"}`)),
		rule(cardPlayedEvent, `{"card":"last"}`, homebrew.TimingNow, doAct(deckTakeOut, `{"deck":"Omens","card":"Dust"}`)),
		rule(cardPlayedEvent, `{"card":"nodeck"}`, homebrew.TimingNow, doAct(deckShuffleBack, `{"deck":"Nope"}`)),
		rule(cardPlayedEvent, `{"card":"nocard"}`, homebrew.TimingNow, doAct(deckTakeOut, `{"deck":"Omens","card":"Nothing"}`)))
	fire := func(card string) {
		t.Helper()
		w.story.CardPlayed(storyCtx, w.camp, decks.CardEvent{DeckID: deck, DeckName: "Omens", CardName: card, By: decks.Viewer{UserID: w.tamUser}})
	}
	copies := func(name string) (total, pile int) {
		for _, c := range w.view(deck).Copies {
			if c.CardID == w.cardID(deck, name) {
				total++
				if c.Zone == decks.ZonePile {
					pile++
				}
			}
		}
		return
	}
	run := func() *homebrew.RunView {
		t.Helper()
		d := w.drawer()
		if len(d.Waiting) != 1 {
			t.Fatalf("waiting = %d, want exactly the one held rule", len(d.Waiting))
		}
		v, err := w.homebrew.RunHeld(storyCtx, w.camp, d.Waiting[0].ID, w.hbDirector)
		w.must(err)
		return v
	}

	fire("fire")
	if total, _ := copies("Ash on the wind"); total != 4 {
		t.Fatalf("the rule changed the deck before a Director pressed Run: %d copies", total)
	}
	if d := w.drawer(); len(d.Waiting) != 1 || d.Waiting[0].Waits != "Part of it is your call, so it waits for you." {
		t.Fatalf("drawer = %+v", d)
	}
	v := run()
	if v.Status != homebrew.RunDone || strings.Join(v.Lines, "|") != "Shuffled 2 copies of Ash on the wind into Omens; it now has 6." {
		t.Fatalf("add-copies: %+v", v)
	}
	if total, pile := copies("Ash on the wind"); total != 6 || pile != 5 {
		t.Errorf("after add-copies: %d copies, %d in the pile", total, pile)
	}

	fire("ice")
	v = run()
	want := "Took 5 copies of Ash on the wind out of Omens. 1 is still in Orrin's hand."
	if v.Status != homebrew.RunDone || strings.Join(v.Lines, "|") != want {
		t.Fatalf("take-out: %+v, want %q", v, want)
	}
	if total, pile := copies("Ash on the wind"); total != 1 || pile != 0 {
		t.Errorf("after take-out: %d copies, %d in the pile; the copy in the hand must stay", total, pile)
	}
	if got := w.copyOf(deck, w.cardID(deck, "Ash on the wind"), decks.ZoneHand); got == "" {
		t.Error("the copy in Orrin's hand is gone")
	}
	fire("ice")
	if v = run(); strings.Join(v.Lines, "|") != "No copy of Ash on the wind is in the pile of Omens, so nothing changed. 1 is still in Orrin's hand." {
		t.Errorf("take-out with nothing in the pile: %+v", v)
	}

	fire("last")
	if v = run(); strings.Join(v.Lines, "|") != "Took 2 copies of Dust out of Omens. It had no other copies, so the card is gone from the deck." {
		t.Errorf("take-out of every copy: %+v", v)
	}
	for _, c := range w.view(deck).Cards {
		if c.Name == "Dust" {
			t.Error("Dust is still in the deck")
		}
	}

	// Shuffling back moves only the used pile: a copy in a hand stays put.
	fire("all")
	if v = run(); strings.Join(v.Lines, "|") != "Omens has no used cards, so nothing changed." {
		t.Errorf("shuffle back with nothing used: %+v", v)
	}
	usedCopy := w.copyOf(deck, w.cardID(deck, "Ash on the wind"), decks.ZoneHand)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, usedCopy, "play", w.dir, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	// A second copy goes to Orrin's hand while the first lies in the used pile.
	_, err := w.decks.UpdateCard(storyCtx, w.camp, deck, w.cardID(deck, "Ash on the wind"), w.dir, decks.UpdateCardInput{Copies: patch.Of(2)})
	w.must(err)
	w.deal(deck, w.cardID(deck, "Ash on the wind"), w.orrinHero)
	fire("all")
	if v = run(); v.Status != homebrew.RunDone || strings.Join(v.Lines, "|") != "Shuffled 1 used card back into Omens." {
		t.Errorf("shuffle back: %+v", v)
	}
	if total, pile := copies("Ash on the wind"); total != 2 || pile != 1 {
		t.Errorf("after shuffle back: %d copies, %d in the pile", total, pile)
	}
	if got := w.copyOf(deck, w.cardID(deck, "Ash on the wind"), decks.ZoneHand); got == "" {
		t.Error("the copy in the hand was taken")
	}

	for _, tc := range []struct{ card, line string }{
		{"nodeck", "Shuffle the used pile back into a deck did not run: There is no deck called Nope."},
		{"nocard", "Take a card out of a deck for good did not run: There is no card called Nothing in Omens."},
	} {
		fire(tc.card)
		v = run()
		if v.Status != homebrew.RunFailed || strings.Join(v.Lines, "|") != tc.line {
			t.Errorf("%s: %+v, want %q", tc.card, v, tc.line)
		}
	}
}

// A deck action does what a Director may do and never more: not as a player,
// and not for a campaign that switched card decks off.
func TestDeckActionsNeverHaveMoreStandingThanTheDirectorWhoRanThem(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindEvent, "", nil)
	w.card(deck, "Ash", 2, decks.CreateCardInput{})
	act := func(by homebrew.Viewer) error {
		_, err := w.story.shuffleBack(storyCtx, w.camp, homebrew.Act{By: by, Args: json.RawMessage(`{"deck":"Omens"}`)})
		return err
	}
	if err := act(homebrew.Viewer{UserID: w.tamUser}); hbCode(err) != 403 {
		t.Errorf("a player ran a deck action: %v", err)
	}
	w.on = false
	if err := act(w.hbDirector); hbCode(err) != 422 || !strings.Contains(err.Error(), "switched off") {
		t.Errorf("with the add-on off: %v", err)
	}
	w.on = true
	if err := act(w.hbDirector); err != nil {
		t.Errorf("a Director could not: %v", err)
	}
}

func TestAddOnOffFiresNothingAndAFailingOrPanickingRuleNeverFailsAMove(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	ash := w.card(deck, "Ash", 4, decks.CreateCardInput{})
	w.recipe(rule(cardDrawnEvent, `{}`, homebrew.TimingNow, doomBy(1)))

	w.on = false
	w.deal(deck, ash, w.tamHero)
	if d := w.drawer(); len(d.Recent)+len(d.Waiting) != 0 {
		t.Fatalf("a campaign with card decks off fired a rule: %+v", d)
	}
	w.on = true
	w.deal(deck, ash, w.tamHero)
	if d := w.drawer(); len(d.Recent) != 1 {
		t.Fatalf("with the add-on on, runs = %+v", d)
	}

	// Rules that fail or panic are the rule's trouble, not the move's.
	for name, hb := range map[string]homebrew.Service{"fails": failingFire{}, "panics": panickingFire{}} {
		bad := newDecksStory(func(context.Context, string) bool { return true })
		bad.spawn = func(f func()) { f() }
		bad.bindHomebrew(hb)
		svc := decks.NewService(decks.NewRepository(w.db), w.heroes, w.members,
			decks.WithRandom(func(int) int { return 0 }), decks.WithStory(bad))
		if _, err := svc.DealCard(storyCtx, w.camp, deck, w.dir, decks.DealCardInput{CardID: ash, To: "hand", HeroID: w.tamHero}); err != nil {
			t.Errorf("%s: a draw failed because its rule did: %v", name, err)
		}
	}
	if got := len(w.view(deck).Copies); got != 4 {
		t.Fatalf("copies = %d", got)
	}
	if hand := w.view(deck).Counts.Hand; hand != 4 {
		t.Errorf("hand = %d, want all four deals stored", hand)
	}
}

type failingFire struct{ homebrew.Service }

func (failingFire) Fire(context.Context, string, homebrew.Viewer, homebrew.Fired) error {
	return fmt.Errorf("the rule store is down")
}

type panickingFire struct{ homebrew.Service }

func (panickingFire) Fire(context.Context, string, homebrew.Viewer, homebrew.Fired) error {
	panic("the rule blew up")
}

// ids are random per deck, card and copy, so two draws are compared after
// each is scrubbed down to what it names.
var uuidText = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
var wallClock = regexp.MustCompile(`\d{4}-\d\d-\d\dT[0-9:.]+Z`)

// A rule that listens to a deck changes nothing any player is told, rung for
// or answered. Two identical decks take the same face-down draw; only the
// second has a rule listening (an at-once rule and a held one), and every
// reply, message and bell entry is compared.
func TestAPlayersFaceDownDrawIsTheSameWithARuleListeningOrNot(t *testing.T) {
	w := newStoryWorld(t)
	quiet := w.deck("Quiet omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Draws: ptrTo("players"), Arrives: ptrTo("down"), OthersSee: ptrTo("back")})
	heard := w.deck("Heard omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Draws: ptrTo("players"), Arrives: ptrTo("down"), OthersSee: ptrTo("back")})
	for _, d := range []string{quiet, heard} {
		w.choose(d, decks.PlayerInput{UserID: w.tamUser}, decks.PlayerInput{UserID: w.ilseUser})
		w.card(d, "The dragon stirs", 3, decks.CreateCardInput{Words: "SECRETWORDS"})
	}
	w.recipe(
		rule(cardDrawnEvent, `{"deck":"Heard omens","card":"The dragon stirs"}`, homebrew.TimingNow, doomBy(1)),
		rule(cardDrawnEvent, `{"deck":"heard omens"}`, homebrew.TimingNow, doAct(deckShuffleBack, `{"deck":"Heard omens"}`)))

	play := func(deck, name string) []string {
		t.Helper()
		w.capture.mu.Lock()
		w.capture.told, w.capture.rings = nil, nil
		w.capture.mu.Unlock()
		res, err := w.decks.Draw(storyCtx, w.camp, deck, w.tam, decks.DrawInput{HeroID: w.tamHero})
		w.must(err)
		out := []string{}
		raw, _ := json.Marshal(res)
		out = append(out, string(raw))
		for _, v := range []decks.Viewer{w.tam, w.ilse} {
			got, err := w.decks.Get(storyCtx, w.camp, deck, v)
			w.must(err)
			raw, _ := json.Marshal(got)
			out = append(out, string(raw))
		}
		out = append(out, w.capture.snapshot())
		for i := range out {
			out[i] = wallClock.ReplaceAllString(out[i], "TIME")
			out[i] = strings.ReplaceAll(out[i], name, "DECKNAME")
			out[i] = uuidText.ReplaceAllString(out[i], "ID")
		}
		return out
	}
	a, b := play(quiet, "Quiet omens"), play(heard, "Heard omens")
	if d := w.drawer(); len(d.Recent) != 1 || len(d.Waiting) != 1 {
		t.Fatalf("the listening deck's rules did not run: %+v", d)
	}
	if len(a) != len(b) {
		t.Fatalf("%d and %d replies", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("part %d differs with a rule listening:\n without: %s\n with:    %s", i, a[i], b[i])
		}
	}
	for _, part := range b {
		if strings.Contains(part, "SECRETWORDS") || strings.Contains(part, "dragon") {
			t.Errorf("a face-down draw named its card to a player: %.200s", part)
		}
	}
}
