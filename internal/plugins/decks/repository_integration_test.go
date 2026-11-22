package decks

// The deck SQL is hand-written, so the fake the service tests use proves
// nothing about it; this runs it against a migrated scratch schema. Skips
// when CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/decks/ -run Integration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/database"
)

type dbFixture struct {
	db                   *sql.DB
	repo                 Repository
	owner, tam, campaign string
	hero, hero2          string
	ctx                  context.Context
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the decks repository tests")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("invalid CHRONICLE_TEST_DB_DSN: %v", err)
	}
	cfg.ParseTime = true
	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB reachable: %v", err)
	}
	name := fmt.Sprintf("chronicle_decks_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { db.Close() })

	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: PluginSlug, MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("decks migrations did not apply: %v", res.Error)
		}
	}

	fx := &dbFixture{db: db, repo: NewRepository(db), ctx: context.Background(),
		owner:    "00000000-0000-0000-0000-000000000001",
		tam:      "00000000-0000-0000-0000-000000000002",
		campaign: "00000000-0000-0000-0000-000000000010",
		hero:     "00000000-0000-0000-0000-000000000011",
		hero2:    "00000000-0000-0000-0000-000000000012",
	}
	for i, u := range []string{fx.owner, fx.tam} {
		fx.exec(t, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, u, fmt.Sprintf("u%d@example.test", i), "U", "x")
	}
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.campaign, "One", "one", fx.owner)
	fx.exec(t, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, fx.campaign, "hero", "Hero", "Heroes")
	for _, p := range []string{fx.hero, fx.hero2} {
		fx.exec(t, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'hero'), ?, ?, false, 'default', ?, NOW(), NOW())`,
			p, fx.campaign, fx.campaign, "Hero "+p[len(p)-1:], "h"+p[len(p)-1:], fx.owner)
	}
	return fx
}

func (fx *dbFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func (fx *dbFixture) deck(t *testing.T, name string) Deck {
	t.Helper()
	d := Deck{ID: uuid.NewString(), CampaignID: fx.campaign, Name: name, Kind: KindEvent,
		Settings: defaultSettings(KindEvent), Look: defaultLook()}
	if err := fx.repo.CreateDeck(fx.ctx, d); err != nil {
		t.Fatalf("create deck: %v", err)
	}
	return d
}

func (fx *dbFixture) card(t *testing.T, d Deck, name string, copies int) Card {
	t.Helper()
	c := Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: fx.campaign, Name: name, Copies: copies}
	if err := fx.repo.CreateCard(fx.ctx, c); err != nil {
		t.Fatalf("create card: %v", err)
	}
	return c
}

func (fx *dbFixture) zones(t *testing.T, d Deck) ZoneCounts {
	t.Helper()
	counts, err := fx.repo.ZoneCounts(fx.ctx, fx.campaign)
	if err != nil {
		t.Fatal(err)
	}
	return counts[d.ID]
}

func firstPick(int) int { return 0 }

// draw1 is a draw of one copy, for the tests written before a draw could take
// several.
func (fx *dbFixture) draw1(p DrawPlan) (Copy, error) {
	got, err := fx.repo.Draw(fx.ctx, p)
	if err != nil {
		return Copy{}, err
	}
	return got[0], nil
}

func drawPlan(fx *dbFixture, d Deck, to Place) DrawPlan {
	return DrawPlan{CampaignID: fx.campaign, DeckID: d.ID, To: to, Pick: firstPick,
		Log: LogLine{Action: ActDraw, ActorUserID: fx.owner}}
}

func TestIntegration_MigrationIsIdempotentAndReversible(t *testing.T) {
	fx := newDBFixture(t)
	up, err := MigrationsFS.ReadFile("migrations/001_decks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := MigrationsFS.ReadFile("migrations/001_decks.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	// The migration runner applies a multi-statement file; do the same twice.
	db2, err := sql.Open("mysql", mustDSN(t)+"&multiStatements=true")
	if err != nil {
		t.Skipf("no multi-statement connection: %v", err)
	}
	defer db2.Close()
	var name string
	if err := fx.db.QueryRow(`SELECT DATABASE()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if _, err := db2.Exec("USE `" + name + "`"); err != nil {
		t.Skipf("cannot select schema: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db2.Exec(string(up)); err != nil {
			t.Fatalf("up, run %d: %v", i+1, err)
		}
	}
	if _, err := db2.Exec(string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	var n int
	if err := fx.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name LIKE 'deck%'`, name).Scan(&n); err != nil || n != 0 {
		t.Errorf("after down %d deck tables remain (%v)", n, err)
	}
	if _, err := db2.Exec(string(up)); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}

func mustDSN(t *testing.T) string {
	cfg, err := mysql.ParseDSN(os.Getenv("CHRONICLE_TEST_DB_DSN"))
	if err != nil {
		t.Skip(err)
	}
	cfg.ParseTime = true
	cfg.DBName = ""
	return cfg.FormatDSN()
}

func TestIntegration_DecksCardsAndCopies(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	if err := fx.repo.CreateDeck(fx.ctx, Deck{ID: uuid.NewString(), CampaignID: fx.campaign, Name: "Omens", Kind: KindEvent,
		Settings: defaultSettings(KindEvent), Look: defaultLook()}); err != errDuplicate {
		t.Errorf("duplicate deck name: %v", err)
	}
	got, ok, err := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
	if err != nil || !ok || got.Version != 1 || got.Settings != d.Settings || got.Look.Border != "double" {
		t.Fatalf("get deck = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := fx.repo.GetDeck(fx.ctx, "00000000-0000-0000-0000-0000000000ff", d.ID); ok {
		t.Error("a deck read through another campaign")
	}

	a := fx.card(t, d, "A", 3)
	if err := fx.repo.CreateCard(fx.ctx, Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: fx.campaign, Name: "A", Copies: 1}); err != errDuplicate {
		t.Errorf("duplicate card name: %v", err)
	}
	if z := fx.zones(t, d); z.Pile != 3 {
		t.Fatalf("zones = %+v, want 3 in the pile", z)
	}

	// Resize: up adds to the pile; down removes pile first, then used, then gone.
	a.Copies = 5
	if res, err := fx.repo.UpdateCard(fx.ctx, a); err != nil || res.Added != 2 {
		t.Fatalf("raise = %+v %v", res, err)
	}
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
	fx.exec(t, `UPDATE deck_copies SET zone='used' WHERE id = ?`, copies[0].ID)
	fx.exec(t, `UPDATE deck_copies SET zone='hand', holder_entity_id = ? WHERE id = ?`, fx.hero, copies[1].ID)
	fx.exec(t, `UPDATE deck_copies SET zone='play', holder_entity_id = ? WHERE id = ?`, fx.hero, copies[2].ID)
	a.Copies = 3 // pile 2, used 1, held 2: removes 2 from the pile
	if _, err := fx.repo.UpdateCard(fx.ctx, a); err != nil {
		t.Fatal(err)
	}
	if z := fx.zones(t, d); z.Pile != 0 || z.Used != 1 || z.Hand != 1 || z.Play != 1 {
		t.Errorf("zones = %+v", z)
	}
	a.Copies = 1 // would take a held copy out
	if _, err := fx.repo.UpdateCard(fx.ctx, a); err == nil {
		t.Error("lowering below the held copies should be refused")
	} else if h, ok := err.(*heldError); !ok || h.N != 2 {
		t.Errorf("err = %v, want 2 held", err)
	}
	if z := fx.zones(t, d); z.Used != 1 || z.Hand != 1 || z.Play != 1 {
		t.Errorf("a refused resize changed the copies: %+v", z)
	}
	stored, _, _ := fx.repo.GetCard(fx.ctx, fx.campaign, d.ID, a.ID)
	if stored.Copies != 3 {
		t.Errorf("a refused resize stored copies = %d", stored.Copies)
	}

	// Deleting the card removes its copies wherever they are.
	if err := fx.repo.DeleteCard(fx.ctx, fx.campaign, d.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if z := fx.zones(t, d); z != (ZoneCounts{}) {
		t.Errorf("copies left after deleting the card: %+v", z)
	}
}

func TestIntegration_DeckVersionPlayersAndCascade(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	d.Name = "Omens 2"
	if ok, err := fx.repo.UpdateDeck(fx.ctx, d, 1, PickChange{}); err != nil || !ok {
		t.Fatalf("update = %v %v", ok, err)
	}
	if ok, _ := fx.repo.UpdateDeck(fx.ctx, d, 1, PickChange{}); ok {
		t.Error("a stale version saved")
	}
	three := 3
	now := time.Now().UTC().Truncate(time.Second)
	if ok, err := fx.repo.SetPlayers(fx.ctx, fx.campaign, d.ID, []Player{{UserID: fx.tam, DrawsLeft: &three, GrantedAt: now}}, 2); err != nil || !ok {
		t.Fatalf("set players = %v %v", ok, err)
	}
	if ok, _ := fx.repo.SetPlayers(fx.ctx, fx.campaign, d.ID, nil, 2); ok {
		t.Error("set players with a stale version")
	}
	got, _, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
	if got.Version != 3 || len(got.Players) != 1 || got.Players[0].DrawsLeft == nil || *got.Players[0].DrawsLeft != 3 {
		t.Fatalf("deck = %+v", got)
	}
	if list, _ := fx.repo.ListDecks(fx.ctx, fx.campaign); len(list) != 1 || len(list[0].Players) != 1 {
		t.Errorf("list = %+v", list)
	}
	// A hero page's purge clears a hero deck's holder and takes held copies with it.
	hd := Deck{ID: uuid.NewString(), CampaignID: fx.campaign, Name: "Hero", Kind: KindHero, HolderEntityID: fx.hero,
		Settings: defaultSettings(KindHero), Look: defaultLook()}
	if err := fx.repo.CreateDeck(fx.ctx, hd); err != nil {
		t.Fatal(err)
	}
	c := fx.card(t, hd, "A", 2)
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, hd.ID)
	fx.exec(t, `UPDATE deck_copies SET zone='hand', holder_entity_id = ? WHERE id = ?`, fx.hero, copies[0].ID)
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.hero)
	if after, ok, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, hd.ID); !ok || after.HolderEntityID != "" {
		t.Errorf("hero deck after purge = %+v (found %v)", after, ok)
	}
	if z := fx.zones(t, hd); z.Pile != 1 || z.Hand != 0 {
		t.Errorf("zones after purge = %+v", z)
	}
	// A copies change adds the missing copy back to the pile.
	c.Copies = 2
	if res, err := fx.repo.UpdateCard(fx.ctx, c); err != nil || res.Added != 1 {
		t.Errorf("repair = %+v %v", res, err)
	}
	// Deleting the deck removes everything under it.
	if err := fx.repo.DeleteDeck(fx.ctx, fx.campaign, d.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_players WHERE deck_id = ?`, d.ID).Scan(&n)
	if n != 0 {
		t.Errorf("%d player rows remain", n)
	}
}

func TestIntegration_DrawZonesShownAndReshuffle(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	fx.card(t, d, "A", 1)
	fx.card(t, d, "A2", 1)
	shown := Place{Zone: ZoneShown}
	first, err := fx.draw1(drawPlan(fx, d, shown))
	if err != nil || first.Zone != ZoneShown {
		t.Fatalf("draw = %+v %v", first, err)
	}
	// Draw another returns the shown copy and never draws it again when another
	// exists. The returned copy has a new id, so "the same copy" is the same card here.
	plan := drawPlan(fx, d, shown)
	for i := 0; i < 4; i++ {
		plan.PutBackCopyID = first.ID
		got, err := fx.draw1(plan)
		if err != nil || got.CardID == first.CardID || got.ID == first.ID {
			t.Fatalf("another #%d = %+v %v (first %+v)", i, got, err, first)
		}
		first = got
	}
	if z := fx.zones(t, d); z.Shown != 1 || z.Pile != 1 {
		t.Errorf("zones = %+v", z)
	}
	// Dealing to a hand stamps who dealt it.
	hand := drawPlan(fx, d, Place{Zone: ZoneHand, HolderEntityID: fx.hero2})
	hand.DealtBy = fx.owner
	got, err := fx.draw1(hand)
	if err != nil || got.DealtBy != fx.owner || got.DealtAt == nil || got.HolderEntityID != fx.hero2 {
		t.Fatalf("hand draw = %+v %v", got, err)
	}
	if _, err := fx.draw1(hand); err != errEmptyPile {
		t.Errorf("empty pile: %v", err)
	}
	// Reshuffle brings the used pile back.
	fx.exec(t, `UPDATE deck_copies SET zone='used', holder_entity_id=NULL, face_up=1 WHERE id = ?`, got.ID)
	hand.Reshuffle = true
	again, err := fx.draw1(hand)
	if err != nil || again.CardID != got.CardID || again.ID == got.ID {
		t.Fatalf("reshuffled draw = %+v %v (was %+v)", again, err, got)
	}
	// A named card's draw only looks at that card.
	other := fx.card(t, d, "B", 1)
	named := drawPlan(fx, d, Place{Zone: ZonePlay, FaceUp: true})
	named.CardID = other.ID
	if c, err := fx.draw1(named); err != nil || c.CardID != other.ID {
		t.Fatalf("named draw = %+v %v", c, err)
	}
	if _, err := fx.draw1(named); err != errEmptyPile {
		t.Errorf("named draw of an empty card: %v", err)
	}
	// The table lists the face-up play copies with the face and no note.
	rows, err := fx.repo.ListTable(fx.ctx, fx.campaign)
	if err != nil || len(rows) != 1 || rows[0].Face.Name != "B" || rows[0].DeckName != "Omens" {
		t.Errorf("table = %+v %v", rows, err)
	}
}

// Two draws race for the last copy: exactly one wins, the other finds the
// pile empty, and nothing is left half done.
func TestIntegration_RaceForTheLastCopy(t *testing.T) {
	fx := newDBFixture(t)
	for round := 0; round < 5; round++ {
		d := fx.deck(t, fmt.Sprintf("Omens %d", round))
		fx.card(t, d, "A", 1)
		const racers = 8
		var wg sync.WaitGroup
		results := make([]error, racers)
		start := make(chan struct{})
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				plan := drawPlan(fx, d, Place{Zone: ZoneHand, HolderEntityID: fx.hero2})
				plan.DealtBy = fx.owner
				_, results[i] = fx.draw1(plan)
			}(i)
		}
		close(start)
		wg.Wait()
		wins, empties := 0, 0
		for _, err := range results {
			switch err {
			case nil:
				wins++
			case errEmptyPile:
				empties++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}
		if wins != 1 || empties != racers-1 {
			t.Fatalf("round %d: %d won and %d found it empty, want 1 and %d", round, wins, empties, racers-1)
		}
		if z := fx.zones(t, d); z.Hand != 1 || z.Pile != 0 {
			t.Errorf("zones = %+v", z)
		}
		lines, _ := fx.repo.ListLog(fx.ctx, fx.campaign, d.ID, 50)
		if len(lines) != 1 {
			t.Errorf("%d log lines for one draw", len(lines))
		}
	}
}

// Two players race for one remaining draw while the pile has plenty: exactly
// one spends it.
func TestIntegration_RaceForTheLastDraw(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	fx.card(t, d, "A", 10)
	one := 1
	if ok, err := fx.repo.SetPlayers(fx.ctx, fx.campaign, d.ID, []Player{{UserID: fx.tam, DrawsLeft: &one, GrantedAt: time.Now().UTC()}}, 1); err != nil || !ok {
		t.Fatal(ok, err)
	}
	const racers = 6
	var wg sync.WaitGroup
	results := make([]error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			plan := drawPlan(fx, d, Place{Zone: ZoneHand, HolderEntityID: fx.hero2})
			plan.SpendUserID, plan.DealtBy = fx.tam, fx.tam
			_, results[i] = fx.draw1(plan)
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch err {
		case nil:
			wins++
		case errNoDraws:
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d draws succeeded on one remaining draw", wins)
	}
	got, _, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
	if got.Players[0].DrawsLeft == nil || *got.Players[0].DrawsLeft != 0 {
		t.Errorf("draws left = %v, want 0", got.Players[0].DrawsLeft)
	}
	// A draw from an empty pile gives the spent draw back (the transaction rolls back).
	d2 := fx.deck(t, "Empty")
	fx.card(t, d2, "A", 1)
	fx.exec(t, `UPDATE deck_copies SET zone='gone' WHERE deck_id = ?`, d2.ID)
	two := 2
	if ok, err := fx.repo.SetPlayers(fx.ctx, fx.campaign, d2.ID, []Player{{UserID: fx.tam, DrawsLeft: &two, GrantedAt: time.Now().UTC()}}, 1); err != nil || !ok {
		t.Fatal(ok, err)
	}
	plan := drawPlan(fx, d2, Place{Zone: ZoneHand, HolderEntityID: fx.hero2})
	plan.SpendUserID = fx.tam
	if _, err := fx.draw1(plan); err != errEmptyPile {
		t.Fatalf("err = %v", err)
	}
	after, _, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, d2.ID)
	if *after.Players[0].DrawsLeft != 2 {
		t.Errorf("an empty draw spent one: %d left", *after.Players[0].DrawsLeft)
	}
}

// Two people move the same copy: exactly one claim lands.
func TestIntegration_MoveClaimsTheCopy(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	c := fx.card(t, d, "A", 1)
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
	id := copies[0].ID
	fx.exec(t, `UPDATE deck_copies SET zone='hand', holder_entity_id = ?, face_up = 1 WHERE id = ?`, fx.hero, id)
	const racers = 6
	var wg sync.WaitGroup
	results := make([]error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: id,
				From: Place{Zone: ZoneHand, HolderEntityID: fx.hero, FaceUp: true}, To: Place{Zone: ZoneUsed, FaceUp: true},
				Asked: AskedClear, Log: LogLine{Action: ActPlay, ActorUserID: fx.owner, HolderEntityID: fx.hero}})
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch err {
		case nil:
			wins++
		case errLostClaim:
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d moves landed, want 1", wins)
	}
	lines, _ := fx.repo.ListLog(fx.ctx, fx.campaign, d.ID, 10)
	if len(lines) != 1 || lines[0].CardID != c.ID || lines[0].CopyID != id || lines[0].HolderEntityID != fx.hero {
		t.Errorf("log = %+v", lines)
	}
	// A move from the wrong face or holder claims nothing.
	if err := fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: id,
		From: Place{Zone: ZoneUsed, FaceUp: false}, To: Place{Zone: ZonePile}}); err != errLostClaim {
		t.Errorf("wrong face: %v", err)
	}
	// Another campaign's id never matches.
	if err := fx.repo.Move(fx.ctx, Move{CampaignID: "00000000-0000-0000-0000-0000000000ff", DeckID: d.ID, CopyID: id,
		From: Place{Zone: ZoneUsed, FaceUp: true}, To: Place{Zone: ZonePile}}); err != errLostClaim {
		t.Errorf("other campaign: %v", err)
	}
	// Approve needs the asked mark.
	fx.exec(t, `UPDATE deck_copies SET zone='hand', holder_entity_id = ?, face_up = 0, asked_at = NULL WHERE id = ?`, fx.hero, id)
	ask := Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: id, From: Place{Zone: ZoneHand, HolderEntityID: fx.hero},
		To: Place{Zone: ZoneUsed, FaceUp: true}, ExpectAsked: true, Asked: AskedClear}
	if err := fx.repo.Move(fx.ctx, ask); err != errLostClaim {
		t.Errorf("approve with no ask: %v", err)
	}
	mark := ask
	mark.ExpectAsked, mark.To, mark.Asked = false, Place{Zone: ZoneHand, HolderEntityID: fx.hero}, AskedMark
	if err := fx.repo.Move(fx.ctx, mark); err != nil {
		t.Fatal(err)
	}
	if err := fx.repo.Move(fx.ctx, ask); err != nil {
		t.Errorf("approve after the ask: %v", err)
	}
}

func TestIntegration_ShuffleGatherAndLogTrim(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	fx.card(t, d, "A", 4)
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
	fx.exec(t, `UPDATE deck_copies SET zone='used' WHERE id IN (?, ?)`, copies[0].ID, copies[1].ID)
	fx.exec(t, `UPDATE deck_copies SET zone='hand', holder_entity_id = ?, face_up = 1, dealt_by = ? WHERE id = ?`, fx.hero, fx.owner, copies[2].ID)
	fx.exec(t, `UPDATE deck_copies SET zone='gone' WHERE id = ?`, copies[3].ID)
	log := LogLine{Action: ActShuffle, ActorUserID: fx.owner}
	if n, err := fx.repo.Shuffle(fx.ctx, fx.campaign, d.ID, log); err != nil || n != 2 {
		t.Fatalf("shuffle = %d %v", n, err)
	}
	if n, err := fx.repo.Gather(fx.ctx, fx.campaign, d.ID, LogLine{Action: ActGather, ActorUserID: fx.owner}); err != nil || n != 2 {
		t.Fatalf("gather = %d %v", n, err)
	}
	if z := fx.zones(t, d); z.Pile != 4 {
		t.Errorf("zones = %+v", z)
	}
	var stale int
	_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_copies WHERE deck_id = ? AND (dealt_by IS NOT NULL OR face_up = 1 OR holder_entity_id IS NOT NULL)`, d.ID).Scan(&stale)
	if stale != 0 {
		t.Errorf("gather left the old hand's state on %d copies", stale)
	}
	for _, c := range copies {
		if _, ok, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, d.ID, c.ID); ok && c.ID != copies[3].ID {
			// copies 0 to 2 came back to the pile; copy 3 was gone and came back too
		}
	}
	after, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
	old := map[string]bool{}
	for _, c := range copies {
		old[c.ID] = true
	}
	for _, c := range after {
		if old[c.ID] {
			t.Errorf("copy %s kept its id through a shuffle or gather", c.ID)
		}
	}
	if _, err := fx.repo.Shuffle(fx.ctx, fx.campaign, "00000000-0000-0000-0000-0000000000ff", log); err != errLostClaim {
		t.Errorf("shuffle of a deck outside the campaign: %v", err)
	}
	// The log keeps the newest 1000 lines.
	for i := 0; i < LogKept+10; i++ {
		if _, err := fx.repo.Shuffle(fx.ctx, fx.campaign, d.ID, log); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_log WHERE deck_id = ?`, d.ID).Scan(&n)
	if n != LogKept {
		t.Errorf("%d log lines, want %d", n, LogKept)
	}
}

// The service on the real repository: a hero deck from draw to play.
func TestIntegration_ServiceEndToEnd(t *testing.T) {
	fx := newDBFixture(t)
	heroes := fakeHeroes{pages: map[string]Hero{fx.hero2: {ID: fx.hero2, Name: "Two", OwnerUserID: fx.tam}}}
	members := fakeMembers{list: []Member{
		{UserID: fx.owner, Role: 3, Director: true, JoinedAt: longAgo},
		{UserID: fx.tam, Role: 1, JoinedAt: longAgo},
	}}
	svc := NewService(fx.repo, heroes, members)
	dir := Viewer{UserID: fx.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	tam := Viewer{UserID: fx.tam, MemberRole: 1, VisibilityRole: 1}
	created, err := svc.Create(bg, fx.campaign, dir, CreateDeckInput{Name: "Fate", Kind: KindHero, HolderEntityID: fx.hero2,
		Settings: &SettingsInput{Draws: ptr("players")}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.(DirectorDeck).ID
	if _, err := svc.CreateCard(bg, fx.campaign, id, dir, CreateCardInput{Name: "Boon", Words: "A gift.", Copies: ptr(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlayers(bg, fx.campaign, id, dir, SetPlayersInput{Version: 1, Players: []PlayerInput{{UserID: fx.tam, DrawsLeft: ptr(1)}}}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Draw(bg, fx.campaign, id, tam, DrawInput{HeroID: fx.hero2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Draw(bg, fx.campaign, id, tam, DrawInput{HeroID: fx.hero2}); code(err) != 403 {
		t.Errorf("second draw: code %d, want 403", code(err))
	}
	if _, err := svc.Act(bg, fx.campaign, id, res.CopyID, "turn", tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	played, err := svc.Act(bg, fx.campaign, id, res.CopyID, "play", tam, ActInput{})
	if err != nil {
		t.Fatal(err)
	}
	pd := played.Deck.(PlayerDeck)
	if pd.DrawsLeft == nil || *pd.DrawsLeft != 0 || pd.CanDraw {
		t.Errorf("player deck = %+v", pd)
	}
	dd, _ := svc.Get(bg, fx.campaign, id, dir)
	if z := dd.(DirectorDeck).Counts; z.Used != 1 || z.Pile != 1 {
		t.Errorf("counts = %+v", z)
	}
}

// A copy that returns to the pile by any path gets a fresh id, and the log
// still names the card.
func TestIntegration_ReturnedCopiesGetFreshIDs(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	fx.card(t, d, "Ember", 1)
	only := func() Copy {
		cs, err := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
		if err != nil || len(cs) != 1 {
			t.Fatalf("copies = %v %v", cs, err)
		}
		return cs[0]
	}
	table := Place{Zone: ZonePlay, FaceUp: true}
	pile := Place{Zone: ZonePile}
	toTable := func() Copy {
		plan := drawPlan(fx, d, table)
		if _, err := fx.draw1(plan); err != nil {
			t.Fatal(err)
		}
		return only()
	}

	seen := toTable()
	if err := fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: seen.ID, From: table, To: pile,
		Asked: AskedClear, Log: LogLine{Action: ActTakeBack, ActorUserID: fx.owner}}); err != nil {
		t.Fatal(err)
	}
	if now := only(); now.ID == seen.ID || now.Zone != ZonePile {
		t.Errorf("after a take back: %+v (was %s)", now, seen.ID)
	}
	if _, ok, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, d.ID, seen.ID); ok {
		t.Error("the old id still names a copy")
	}
	lines, _ := fx.repo.ListLog(fx.ctx, fx.campaign, d.ID, 5)
	if len(lines) != 2 || lines[0].CardName != "Ember" || lines[0].CopyID != seen.ID {
		t.Errorf("log = %+v", lines)
	}

	for name, back := range map[string]func(c Copy) error{
		"shuffle": func(c Copy) error {
			if err := fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: c.ID, From: table, To: Place{Zone: ZoneUsed, FaceUp: true}}); err != nil {
				return err
			}
			_, err := fx.repo.Shuffle(fx.ctx, fx.campaign, d.ID, LogLine{Action: ActShuffle, ActorUserID: fx.owner})
			return err
		},
		"gather": func(Copy) error {
			_, err := fx.repo.Gather(fx.ctx, fx.campaign, d.ID, LogLine{Action: ActGather, ActorUserID: fx.owner})
			return err
		},
		"put back shown": func(c Copy) error {
			if err := fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: c.ID, From: table, To: Place{Zone: ZoneShown}}); err != nil {
				return err
			}
			plan := drawPlan(fx, d, Place{Zone: ZoneShown})
			plan.PutBackCopyID = c.ID
			_, err := fx.draw1(plan)
			return err
		},
		"reshuffle on empty": func(c Copy) error {
			if err := fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: c.ID, From: table, To: Place{Zone: ZoneUsed, FaceUp: true}}); err != nil {
				return err
			}
			plan := drawPlan(fx, d, Place{Zone: ZoneHand, HolderEntityID: fx.hero2})
			plan.Reshuffle = true
			_, err := fx.draw1(plan)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := only()
			if c.Zone != ZonePile {
				fx.exec(t, `UPDATE deck_copies SET zone='pile', holder_entity_id=NULL, face_up=0, id=? WHERE id=?`, uuid.NewString(), c.ID)
				c = only()
			}
			c.Zone = ZonePile
			if _, err := fx.draw1(drawPlan(fx, d, table)); err != nil {
				t.Fatal(err)
			}
			c = only()
			if err := back(c); err != nil {
				t.Fatal(err)
			}
			if now := only(); now.ID == c.ID {
				t.Errorf("%s: the copy kept id %s", name, c.ID)
			}
		})
	}
}

func TestIntegration_NewColumnsRoundTrip(t *testing.T) {
	fx := newDBFixture(t)
	var schema string
	if err := fx.db.QueryRow(`SELECT DATABASE()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for _, col := range [][2]string{{"deck_cards", "rare"}, {"deck_copies", "is_open"}, {"decks", "next_card_id"}, {"decks", "last_card_id"}} {
		var n int
		if err := fx.db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?`,
			schema, col[0], col[1]).Scan(&n); err != nil || n != 1 {
			t.Errorf("column %s.%s missing (%v)", col[0], col[1], err)
		}
	}
	d := fx.deck(t, "Omens")
	c := Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: fx.campaign, Name: "Rare one", Rare: true, Copies: 2}
	if err := fx.repo.CreateCard(fx.ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := fx.repo.GetCard(fx.ctx, fx.campaign, d.ID, c.ID); !got.Rare {
		t.Error("rare did not round trip through create")
	}
	c.Rare = false
	if _, err := fx.repo.UpdateCard(fx.ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := fx.repo.GetCard(fx.ctx, fx.campaign, d.ID, c.ID); got.Rare {
		t.Error("rare did not round trip through update")
	}
	table := fx.draw1
	if _, err := table(drawPlan(fx, d, Place{Zone: ZonePlay, FaceUp: true})); err != nil {
		t.Fatal(err)
	}
	rows, _ := fx.repo.ListTable(fx.ctx, fx.campaign)
	if len(rows) != 1 || rows[0].Face.Rare {
		t.Errorf("table rows = %+v", rows)
	}

	// The open mark is claimed like the rest of the copy's state, and a move
	// that leaves the hand or turns the card down clears it.
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, d.ID)
	var pile Copy
	for _, cp := range copies {
		if cp.Zone == ZonePile {
			pile = cp
		}
	}
	hand := Place{Zone: ZoneHand, HolderEntityID: fx.hero, FaceUp: true}
	mv := func(from, to Place) error {
		return fx.repo.Move(fx.ctx, Move{CampaignID: fx.campaign, DeckID: d.ID, CopyID: pile.ID, From: from, To: to,
			Log: LogLine{Action: ActShow, ActorUserID: fx.owner}})
	}
	if err := mv(Place{Zone: ZonePile}, hand); err != nil {
		t.Fatal(err)
	}
	open := hand
	open.Open = true
	if err := mv(hand, open); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, d.ID, pile.ID); !got.Open {
		t.Error("the open mark was not stored")
	}
	if err := mv(hand, open); err != errLostClaim {
		t.Errorf("a move claiming the old closed state: %v", err)
	}
	down := Place{Zone: ZoneHand, HolderEntityID: fx.hero, FaceUp: false, Open: true}
	if err := mv(open, down); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, d.ID, pile.ID); got.Open || got.FaceUp {
		t.Errorf("a card turned down is still open: %+v", got)
	}
}

func TestIntegration_DrawSeveralWithNextAndLast(t *testing.T) {
	fx := newDBFixture(t)
	d := fx.deck(t, "Omens")
	a := fx.card(t, d, "A", 3)
	b := fx.card(t, d, "B", 1)
	c := fx.card(t, d, "C", 3)
	setPicks := func(next, last string) {
		d.Version = fx.currentVersion(t, d)
		if ok, err := fx.repo.UpdateDeck(fx.ctx, d, d.Version, PickChange{SetNext: true, SetLast: true, Next: next, Last: last}); err != nil || !ok {
			t.Fatalf("set picks: %v %v", ok, err)
		}
	}
	picks := func() (string, string) {
		got, _, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
		return got.NextCardID, got.LastCardID
	}
	cardsOf := func(cs []Copy) []string {
		var out []string
		for _, cp := range cs {
			out = append(out, cp.CardID)
		}
		return out
	}
	shown := Place{Zone: ZoneShown}

	// Five at once, and a sixth is refused with nothing taken.
	setPicks(c.ID, "")
	plan := drawPlan(fx, d, shown)
	plan.Count = 5
	got, err := fx.repo.Draw(fx.ctx, plan)
	if err != nil || len(got) != 5 {
		t.Fatalf("draw of five = %d copies, %v", len(got), err)
	}
	if got[0].CardID != c.ID {
		t.Errorf("the next card was not first: %v", cardsOf(got))
	}
	if n, _ := picks(); n != "" {
		t.Errorf("next stayed set: %q", n)
	}
	if _, err := fx.repo.Draw(fx.ctx, drawPlan(fx, d, shown)); err != errShownFull {
		t.Errorf("sixth shown card: %v, want errShownFull", err)
	}
	if z := fx.zones(t, d); z.Shown != 5 || z.Pile != 2 {
		t.Errorf("zones = %+v", z)
	}
	// A refused draw of two (one place left after one is put back) takes nothing.
	one := drawPlan(fx, d, shown)
	one.PutBackCopyID = got[1].ID
	if _, err := fx.repo.Draw(fx.ctx, one); err != nil {
		t.Fatal(err)
	}
	two := drawPlan(fx, d, shown)
	two.Count = 2
	if _, err := fx.repo.Draw(fx.ctx, two); err != errShownFull {
		t.Errorf("draw of two into one free place: %v", err)
	}
	if z := fx.zones(t, d); z.Shown != 5 {
		t.Errorf("zones after refusal = %+v", z)
	}

	// Put everything back, then: the last card is drawn only when nothing else is left.
	if _, err := fx.repo.Gather(fx.ctx, fx.campaign, d.ID, LogLine{Action: ActGather, ActorUserID: fx.owner}); err != nil {
		t.Fatal(err)
	}
	setPicks("", a.ID)
	plan = drawPlan(fx, d, shown)
	plan.Count = 4
	got, err = fx.repo.Draw(fx.ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range cardsOf(got) {
		if id == a.ID {
			t.Errorf("the last card was drawn with %d other copies in the pile: %v", 7-4, cardsOf(got))
		}
	}
	if _, l := picks(); l != a.ID {
		t.Error("last cleared before it was drawn")
	}
	// Only the last card's copies are left in the pile, so the fifth draw takes
	// one, and the field clears.
	got, err = fx.repo.Draw(fx.ctx, drawPlan(fx, d, shown))
	if err != nil || got[0].CardID != a.ID {
		t.Fatalf("the last card should be drawn when it is all that is left: %v %v", cardsOf(got), err)
	}
	if _, l := picks(); l != "" {
		t.Errorf("last stayed set after it was drawn: %q", l)
	}

	// Deleting the named card clears the field in the same transaction.
	setPicks(b.ID, b.ID)
	if err := fx.repo.DeleteCard(fx.ctx, fx.campaign, d.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if n, l := picks(); n != "" || l != "" {
		t.Errorf("deleting the card left next %q last %q", n, l)
	}
	// A save that names neither nudge never writes them back.
	setPicks(c.ID, c.ID)
	fresh, _, _ := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
	fx.exec(t, `UPDATE decks SET next_card_id = NULL WHERE id = ?`, d.ID)
	fresh.Name = "Omens renamed"
	if ok, err := fx.repo.UpdateDeck(fx.ctx, fresh, fresh.Version, PickChange{}); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if n, l := picks(); n != "" || l != c.ID {
		t.Errorf("a save with no picks wrote them: next %q last %q", n, l)
	}
}

func (fx *dbFixture) currentVersion(t *testing.T, d Deck) int {
	t.Helper()
	got, ok, err := fx.repo.GetDeck(fx.ctx, fx.campaign, d.ID)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	return got.Version
}

// keepWorld puts one card's copy in a hand of the source deck.
func (fx *dbFixture) keepSetup(t *testing.T) (src, dst Deck, card Card, cp Copy) {
	t.Helper()
	suffix := uuid.NewString()[:8]
	src, dst = fx.deck(t, "Src "+suffix), fx.deck(t, "Dst "+suffix)
	card = Card{ID: uuid.NewString(), DeckID: src.ID, CampaignID: fx.campaign, Name: "Ember", KindKey: "boon", Words: "EMBERWORDS",
		DirectorNote: "EMBERNOTE", Corner: "IV", Rare: true, Fate: FateGone, AskFirst: true, WhenOpen: true, Copies: 2}
	if err := fx.repo.CreateCard(fx.ctx, card); err != nil {
		t.Fatal(err)
	}
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, src.ID)
	cp = copies[0]
	fx.exec(t, `UPDATE deck_copies SET zone = 'hand', holder_entity_id = ?, face_up = 1, is_open = 1 WHERE id = ?`, fx.hero, cp.ID)
	return src, dst, card, cp
}

func (fx *dbFixture) keepPlan(src, dst Deck, cp Copy, kinds ...string) KeepPlan {
	k := map[string]bool{}
	for _, key := range kinds {
		k[key] = true
	}
	return KeepPlan{CampaignID: fx.campaign, DeckID: src.ID, CopyID: cp.ID,
		From:         Place{Zone: ZoneHand, HolderEntityID: fx.hero, FaceUp: true, Open: true},
		TargetDeckID: dst.ID, TargetKinds: k, Log: LogLine{Action: ActKeep, ActorUserID: fx.owner, HolderEntityID: fx.hero}}
}

func TestIntegration_KeepCopiesTheCardInOneTransaction(t *testing.T) {
	fx := newDBFixture(t)
	src, dst, _, cp := fx.keepSetup(t)
	if err := fx.repo.Keep(fx.ctx, fx.keepPlan(src, dst, cp, "boon")); err != nil {
		t.Fatal(err)
	}
	cards, _ := fx.repo.ListCards(fx.ctx, fx.campaign, dst.ID)
	if len(cards) != 1 {
		t.Fatalf("target has %d cards", len(cards))
	}
	got := cards[0]
	if got.Name != "Ember" || got.Words != "EMBERWORDS" || got.DirectorNote != "EMBERNOTE" || got.Corner != "IV" || !got.Rare ||
		got.Fate != FateGone || !got.AskFirst || got.KindKey != "boon" || got.Copies != 1 || got.WhenOpen || got.When != "" {
		t.Errorf("kept card = %+v", got)
	}
	if z := fx.zones(t, dst); z.Pile != 1 {
		t.Errorf("target zones = %+v", z)
	}
	gone, _, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, src.ID, cp.ID)
	if gone.Zone != ZoneGone || gone.Open || gone.HolderEntityID != "" {
		t.Errorf("source copy = %+v", gone)
	}
	if lines, _ := fx.repo.ListLog(fx.ctx, fx.campaign, src.ID, 5); len(lines) != 1 || lines[0].Action != ActKeep || lines[0].CardName != "Ember" {
		t.Errorf("log = %+v", lines)
	}
	// A second keep of the other copy merges into the card now in the target,
	// and a deck without that kind drops it.
	copies, _ := fx.repo.ListCopies(fx.ctx, fx.campaign, src.ID)
	var second Copy
	for _, c := range copies {
		if c.ID != cp.ID {
			second = c
		}
	}
	fx.exec(t, `UPDATE deck_copies SET zone = 'hand', holder_entity_id = ?, face_up = 1, is_open = 1 WHERE id = ?`, fx.hero, second.ID)
	if err := fx.repo.Keep(fx.ctx, fx.keepPlan(src, dst, second)); err != nil {
		t.Fatal(err)
	}
	cards, _ = fx.repo.ListCards(fx.ctx, fx.campaign, dst.ID)
	if len(cards) != 1 || cards[0].Copies != 2 || cards[0].KindKey != "boon" {
		t.Errorf("merged = %+v", cards)
	}
	if z := fx.zones(t, dst); z.Pile != 2 {
		t.Errorf("target zones after the merge = %+v", z)
	}
	// The first copy is put back in a hand so a third keep can go to a deck
	// that has no kind by that key.
	other := fx.deck(t, "NoKinds")
	fx.exec(t, `UPDATE deck_copies SET zone = 'hand', holder_entity_id = ?, face_up = 1, is_open = 1 WHERE id = ?`, fx.hero, cp.ID)
	if err := fx.repo.Keep(fx.ctx, fx.keepPlan(src, other, cp)); err != nil {
		t.Fatal(err)
	}
	if cards, _ := fx.repo.ListCards(fx.ctx, fx.campaign, other.ID); len(cards) != 1 || cards[0].KindKey != "" {
		t.Errorf("a kind the target lacks was kept: %+v", cards)
	}
}

// A refusal after the source copy was claimed rolls the claim back, so neither
// deck, nor the log, shows any of it.
func TestIntegration_KeepRefusedHalfwayChangesNothing(t *testing.T) {
	fx := newDBFixture(t)
	for _, tt := range []struct {
		name string
		fill func(t *testing.T, dst Deck)
		want error
	}{
		{"card limit", func(t *testing.T, dst Deck) {
			for i := 0; i < MaxCardsPerDeck; i++ {
				fx.exec(t, `INSERT INTO deck_cards (id, deck_id, campaign_id, name, words, director_note, copies) VALUES (?, ?, ?, ?, '', '', 0)`,
					uuid.NewString(), dst.ID, fx.campaign, fmt.Sprintf("Filler %03d", i))
			}
		}, errTargetFull},
		{"copy limit", func(t *testing.T, dst Deck) {
			for i := 0; i < 30; i++ {
				fx.exec(t, `INSERT INTO deck_cards (id, deck_id, campaign_id, name, words, director_note, copies) VALUES (?, ?, ?, ?, '', '', 20)`,
					uuid.NewString(), dst.ID, fx.campaign, fmt.Sprintf("Filler %03d", i))
			}
		}, errTargetCopies},
		{"per card limit", func(t *testing.T, dst Deck) {
			fx.exec(t, `INSERT INTO deck_cards (id, deck_id, campaign_id, name, words, director_note, copies) VALUES (?, ?, ?, 'Ember', '', '', 20)`,
				uuid.NewString(), dst.ID, fx.campaign)
		}, errCardCopies},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src, dst, _, cp := fx.keepSetup(t)
			tt.fill(t, dst)
			var cardsBefore, copiesBefore, logBefore int
			count := func() (int, int, int) {
				var c, k, l int
				_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_cards WHERE deck_id IN (?, ?)`, src.ID, dst.ID).Scan(&c)
				_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_copies WHERE deck_id IN (?, ?)`, src.ID, dst.ID).Scan(&k)
				_ = fx.db.QueryRow(`SELECT COUNT(*) FROM deck_log WHERE deck_id IN (?, ?)`, src.ID, dst.ID).Scan(&l)
				return c, k, l
			}
			cardsBefore, copiesBefore, logBefore = count()
			if err := fx.repo.Keep(fx.ctx, fx.keepPlan(src, dst, cp, "boon")); err != tt.want {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			c, k, l := count()
			if c != cardsBefore || k != copiesBefore || l != logBefore {
				t.Errorf("cards %d->%d copies %d->%d log %d->%d", cardsBefore, c, copiesBefore, k, logBefore, l)
			}
			back, _, _ := fx.repo.GetCopy(fx.ctx, fx.campaign, src.ID, cp.ID)
			if back.Zone != ZoneHand || back.HolderEntityID != fx.hero || !back.FaceUp || !back.Open {
				t.Errorf("the source copy was left changed: %+v", back)
			}
		})
	}
}

// Two Directors keep the same copy at once: exactly one lands, and the target
// holds exactly one new copy.
func TestIntegration_KeepRaceForOneCopy(t *testing.T) {
	fx := newDBFixture(t)
	src, dst, _, cp := fx.keepSetup(t)
	const racers = 6
	var wg sync.WaitGroup
	results := make([]error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = fx.repo.Keep(fx.ctx, fx.keepPlan(src, dst, cp, "boon"))
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch err {
		case nil:
			wins++
		case errLostClaim:
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d keeps landed, want 1", wins)
	}
	if z := fx.zones(t, dst); z.Pile != 1 {
		t.Errorf("target zones = %+v, want one pile copy", z)
	}
	if lines, _ := fx.repo.ListLog(fx.ctx, fx.campaign, src.ID, 10); len(lines) != 1 {
		t.Errorf("%d log lines for one keep", len(lines))
	}
}
