package decks

// The Card decks section's picks and the dealt-card "seen" marks on the real
// database: the foreign keys, the window and the filters. Skips without
// CHRONICLE_TEST_DB_DSN.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntegration_ScreenPicks(t *testing.T) {
	fx := newDBFixture(t)
	repo := NewScreenRepository(fx.db)
	a, b, c := fx.deck(t, "A"), fx.deck(t, "B"), fx.deck(t, "C")

	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Picks(fx.ctx, fx.campaign, fx.owner)
	if err != nil || strings.Join(got, ",") != strings.Join([]string{c.ID, a.ID, b.ID}, ",") {
		t.Fatalf("picks %v %v: order is the Director's", got, err)
	}
	// A save replaces; another Director's picks are their own.
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.tam, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Picks(fx.ctx, fx.campaign, fx.owner); len(got) != 1 || got[0] != b.ID {
		t.Errorf("replace: %v", got)
	}
	// A deck of nobody's campaign stores nothing.
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, []string{b.ID, uuid.NewString(), "x"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Picks(fx.ctx, fx.campaign, fx.owner); len(got) != 1 {
		t.Errorf("an unknown deck was stored: %v", got)
	}
	other := uuid.NewString()
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, other, "Two", "two", fx.owner)
	foreign := Deck{ID: uuid.NewString(), CampaignID: other, Name: "F", Kind: KindEvent, Settings: defaultSettings(KindEvent), Look: defaultLook()}
	if err := fx.repo.CreateDeck(fx.ctx, foreign); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, []string{foreign.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Picks(fx.ctx, fx.campaign, fx.owner); len(got) != 0 {
		t.Errorf("a foreign deck was stored: %v", got)
	}
	// A pick goes with its deck.
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if err := fx.repo.DeleteDeck(fx.ctx, fx.campaign, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Picks(fx.ctx, fx.campaign, fx.owner); len(got) != 1 || got[0] != b.ID {
		t.Errorf("after the deck went: %v", got)
	}
	if err := repo.SetPicks(fx.ctx, fx.campaign, fx.owner, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Picks(fx.ctx, fx.campaign, fx.owner); len(got) != 0 {
		t.Errorf("cleared: %v", got)
	}
}

func TestIntegration_ScreenLastMoves(t *testing.T) {
	fx := newDBFixture(t)
	repo := NewScreenRepository(fx.db)
	a, b := fx.deck(t, "A"), fx.deck(t, "B")
	fx.card(t, a, "Card", 2)
	fx.card(t, b, "Card", 2)
	fx.deck(t, "Quiet")
	for _, d := range []Deck{a, b, a} {
		if _, err := fx.draw1(drawPlan(fx, d, Place{Zone: ZoneShown})); err != nil {
			t.Fatal(err)
		}
	}
	moves, err := repo.LastMoves(fx.ctx, fx.campaign)
	if err != nil || len(moves) != 2 || moves[a.ID] <= moves[b.ID] || moves[b.ID] == 0 {
		t.Errorf("last moves %v %v: A moved last", moves, err)
	}
}

func TestIntegration_FreshCopies(t *testing.T) {
	fx := newDBFixture(t)
	repo := NewScreenRepository(fx.db)
	d := fx.deck(t, "Omens")
	fx.card(t, d, "Card", 12)
	hand := func(hero string, by string, faceUp bool) Copy {
		t.Helper()
		p := drawPlan(fx, d, Place{Zone: ZoneHand, HolderEntityID: hero, FaceUp: faceUp})
		p.DealtBy = by
		cp, err := fx.draw1(p)
		if err != nil {
			t.Fatal(err)
		}
		return cp
	}
	ids := func(rows []FreshRow) string {
		var out []string
		for _, r := range rows {
			out = append(out, r.CopyID)
		}
		return strings.Join(out, ",")
	}
	mine := hand(fx.hero, fx.owner, false)
	ownDraw := hand(fx.hero, fx.tam, false)
	faceUp := hand(fx.hero, fx.owner, true)
	forOther := hand(fx.hero2, fx.owner, false)
	_, _ = ownDraw, faceUp
	window := 10 * time.Minute

	rows, err := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero}, window, 3)
	if err != nil || ids(rows) != mine.ID {
		t.Fatalf("fresh %v %v: only the face-down copy someone else dealt to Tam's hero", ids(rows), err)
	}
	if rows[0].DeckID != d.ID {
		t.Errorf("deck %q", rows[0].DeckID)
	}
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, nil, window, 3); len(rows) != 0 {
		t.Errorf("no heroes: %v", ids(rows))
	}
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero2}, window, 3); ids(rows) != forOther.ID {
		t.Errorf("another hero's copy is only found by naming that hero: %v", ids(rows))
	}
	// Another campaign's id never matches.
	if rows, _ := repo.Fresh(fx.ctx, uuid.NewString(), fx.tam, []string{fx.hero}, window, 3); len(rows) != 0 {
		t.Errorf("a foreign campaign: %v", ids(rows))
	}
	// The window is the database's clock.
	fx.exec(t, `UPDATE deck_copies SET dealt_at = NOW() - INTERVAL 11 MINUTE WHERE id = ?`, mine.ID)
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero}, window, 3); len(rows) != 0 {
		t.Errorf("past the window: %v", ids(rows))
	}
	fx.exec(t, `UPDATE deck_copies SET dealt_at = NOW() - INTERVAL 9 MINUTE WHERE id = ?`, mine.ID)
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero}, window, 3); ids(rows) != mine.ID {
		t.Errorf("inside the window: %v", ids(rows))
	}
	// Seen is once, per person, and idempotent.
	for i := 0; i < 2; i++ {
		if err := repo.MarkSeen(fx.ctx, fx.campaign, fx.tam, mine.ID); err != nil {
			t.Fatal(err)
		}
	}
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero}, window, 3); len(rows) != 0 {
		t.Errorf("after seen: %v", ids(rows))
	}
	// The one who dealt a copy is never popped for it: only Tam's own draw is
	// "someone else's" to the owner.
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.owner, []string{fx.hero}, window, 3); ids(rows) != ownDraw.ID {
		t.Errorf("the dealer: %v", ids(rows))
	}
	// Newest deal first, and the limit.
	for i := 0; i < 3; i++ {
		hand(fx.hero, fx.owner, false)
	}
	fx.exec(t, `UPDATE deck_copies SET dealt_at = NOW() - INTERVAL 1 MINUTE WHERE holder_entity_id = ? AND face_up = 0 AND dealt_by = ?`, fx.hero, fx.owner)
	newest := hand(fx.hero, fx.owner, false)
	if rows, _ := repo.Fresh(fx.ctx, fx.campaign, fx.tam, []string{fx.hero}, window, 3); len(rows) != 3 || rows[0].CopyID != newest.ID {
		t.Errorf("limit and order: %v", ids(rows))
	}
	// Old marks are pruned by the next write and never bring a copy back early.
	fx.exec(t, `UPDATE deck_fresh_seen SET seen_at = NOW() - INTERVAL 2 DAY`)
	if err := repo.MarkSeen(fx.ctx, fx.campaign, fx.tam, newest.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := fx.db.QueryRow(`SELECT COUNT(*) FROM deck_fresh_seen`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d marks kept (%v), want 1", n, err)
	}
}
