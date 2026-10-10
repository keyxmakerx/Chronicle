package quests

// The repositories are hand-written SQL, so the fakes the service tests use
// prove nothing about it; this runs them against a migrated scratch schema.
// Skips when CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/quests/ -run Integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/database"
)

// questsFixture is one scratch schema with two campaigns, each owning pages,
// so cross-campaign and cross-page isolation can be asserted.
type questsFixture struct {
	db              *sql.DB
	campaign, other string
	page, page2     string // page2 and otherPage belong to a different page/campaign
	otherPage       string
	user            string
	typ, typ2       int // two categories of the first campaign
	otherTyp        int // a category of the other campaign
}

func newQuestsFixture(t *testing.T) *questsFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the quests repository tests")
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
	name := fmt.Sprintf("chronicle_quests_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("quests migrations did not apply: %v", res.Error)
		}
	}

	fx := &questsFixture{db: db,
		user:      "00000000-0000-0000-0000-000000000001",
		campaign:  "00000000-0000-0000-0000-000000000002",
		other:     "00000000-0000-0000-0000-000000000003",
		page:      "00000000-0000-0000-0000-000000000004",
		page2:     "00000000-0000-0000-0000-000000000005",
		otherPage: "00000000-0000-0000-0000-000000000006",
	}
	fx.exec(t, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, fx.user, "q@example.test", "Q", "x")
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.campaign, "One", "one", fx.user)
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.other, "Two", "two", fx.user)
	for _, c := range []string{fx.campaign, fx.other} {
		fx.exec(t, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, c, "place", "Place", "Places")
	}
	fx.typ = lookupType(t, db, fx.campaign)
	fx.otherTyp = lookupType(t, db, fx.other)
	fx.exec(t, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, fx.campaign, "faction", "Faction", "Factions")
	fx.typ2 = lookupType(t, db, fx.campaign, "faction")
	for _, p := range []struct{ id, campaign string }{{fx.page, fx.campaign}, {fx.page2, fx.campaign}, {fx.otherPage, fx.other}} {
		fx.exec(t, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'place'), ?, ?, false, 'default', ?, NOW(), NOW())`,
			p.id, p.campaign, p.campaign, "Page "+p.id[len(p.id)-1:], "p"+p.id[len(p.id)-1:], fx.user)
	}
	return fx
}

func lookupType(t *testing.T, db *sql.DB, campaign string, slug ...string) int {
	t.Helper()
	s := "place"
	if len(slug) > 0 {
		s = slug[0]
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM entity_types WHERE campaign_id = ? AND slug = ?`, campaign, s).Scan(&id); err != nil {
		t.Fatalf("entity type %s: %v", s, err)
	}
	return id
}

func (fx *questsFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == 404
}

func TestQuestRepositoryIntegration_SaveGetVersioning(t *testing.T) {
	fx := newQuestsFixture(t)
	ctx := context.Background()
	repo := NewQuestRepository(fx.db)

	if _, _, found, err := repo.Get(ctx, fx.campaign, fx.page); err != nil || found {
		t.Fatalf("empty page: found=%v err=%v", found, err)
	}

	// 0 means "no row yet"; a second first-save loses the race.
	if ok, err := repo.Save(ctx, fx.campaign, fx.page, []byte(`{"a":1}`), 0, fx.user); err != nil || !ok {
		t.Fatalf("first save: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Save(ctx, fx.campaign, fx.page, []byte(`{"a":2}`), 0, fx.user); err != nil || ok {
		t.Fatalf("duplicate first save must lose the race cleanly: ok=%v err=%v", ok, err)
	}

	steps := []struct {
		name     string
		expected int
		data     string
		userID   string
		wantOK   bool
		wantVer  int
		wantData string
	}{
		{"current version wins and bumps", 1, `{"a":2}`, fx.user, true, 2, `{"a":2}`},
		{"stale version fails and changes nothing", 1, `{"a":3}`, fx.user, false, 2, `{"a":2}`},
		{"empty user id stores NULL author", 2, `{"a":4}`, "", true, 3, `{"a":4}`},
		{"future version fails", 9, `{"a":5}`, fx.user, false, 3, `{"a":4}`},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := repo.Save(ctx, fx.campaign, fx.page, []byte(tc.data), tc.expected, tc.userID)
			if err != nil || ok != tc.wantOK {
				t.Fatalf("ok=%v err=%v, want ok=%v", ok, err, tc.wantOK)
			}
			data, ver, found, err := repo.Get(ctx, fx.campaign, fx.page)
			if err != nil || !found || ver != tc.wantVer || string(data) != tc.wantData {
				t.Fatalf("got %q v%d found=%v err=%v, want %q v%d", data, ver, found, err, tc.wantData, tc.wantVer)
			}
		})
	}

	var by sql.NullString
	if err := fx.db.QueryRow(`SELECT updated_by FROM quests WHERE entity_id = ?`, fx.page).Scan(&by); err != nil || by.Valid {
		t.Fatalf("updated_by should be NULL after an anonymous save: %v %v", by, err)
	}

	t.Run("another campaign cannot read or overwrite", func(t *testing.T) {
		if _, _, found, _ := repo.Get(ctx, fx.other, fx.page); found {
			t.Fatal("read across campaigns")
		}
		if ok, _ := repo.Save(ctx, fx.other, fx.page, []byte(`{"x":1}`), 3, fx.user); ok {
			t.Fatal("overwrote across campaigns")
		}
		if ok, _ := repo.Save(ctx, fx.other, fx.page, []byte(`{"x":1}`), 0, fx.user); ok {
			t.Fatal("inserted a duplicate sheet for another campaign")
		}
	})

	t.Run("a sheet for a missing page is rejected", func(t *testing.T) {
		// The entity foreign key is what stops a sheet for a deleted page.
		if _, err := repo.Save(ctx, fx.campaign, "e-missing", []byte(`{}`), 0, fx.user); err == nil {
			t.Fatal("a sheet for a nonexistent page must not insert")
		}
	})
}

func TestQuestRepositoryIntegration_DocumentRoundTripAndGetMany(t *testing.T) {
	fx := newQuestsFixture(t)
	ctx := context.Background()
	repo := NewQuestRepository(fx.db)

	amount := 12.5
	in := Quest{
		Notice:    Notice{Plate: "plate", Kicker: "k", Title: "Wolves “at” the gate \U0001F43A", Blurb: "b", Body: []string{"one", "two"}, PostedBy: "Mayor", Reward: "50 gp", Due: "Fri"},
		Status:    StatusActive,
		HandedOut: true,
		Steps:     []Step{{ID: "s1", Text: "Find it", Done: true, Shown: true}, {ID: "s2", Text: "Secret", Shown: false}},
		Rewards:   []Reward{{ID: "r1", Kind: "gold", Text: "coins", Amount: &amount}, {ID: "r2", Kind: "item", EntityID: fx.page2}},
		Foes:      []Foe{{ID: "f1", Text: "Wolf", Note: "x3", EntityID: fx.page2}},
		Links:     []Link{{ID: "l1", Kind: KindPage, RefID: fx.page2, Label: "Inn"}},
		MapID:     "map-1",
		Layout:    Layout{Notice: Rect{X: 1.5, Y: 2, W: 30, R: -3}, Map: Rect{Hidden: true}, Tag: Rect{X: 9}},
		Looks:     Looks{Board: LookParchment, Ledger: LookMidnight},
	}
	raw, _ := json.Marshal(in)
	if ok, err := repo.Save(ctx, fx.campaign, fx.page, raw, 0, fx.user); err != nil || !ok {
		t.Fatalf("save: %v %v", ok, err)
	}
	data, _, _, err := repo.Get(ctx, fx.campaign, fx.page)
	if err != nil {
		t.Fatal(err)
	}
	var out Quest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("document changed in storage:\n in: %+v\nout: %+v", in, out)
	}

	// A sheet larger than TEXT's 64 KB must survive (the column is LONGTEXT).
	pad := make([]byte, 200000)
	for i := range pad {
		pad[i] = 'a'
	}
	big := []byte(`{"pad":"` + string(pad) + `"}`)
	if ok, err := repo.Save(ctx, fx.campaign, fx.page2, big, 0, fx.user); err != nil || !ok {
		t.Fatalf("big save: %v %v", ok, err)
	}

	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{"none asked", nil, nil},
		{"one stored", []string{fx.page}, []string{fx.page}},
		{"missing ids are omitted", []string{fx.page, "e-nope"}, []string{fx.page}},
		{"several", []string{fx.page, fx.page2}, []string{fx.page, fx.page2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetMany(ctx, fx.campaign, tc.ids)
			if err != nil || len(got) != len(tc.want) {
				t.Fatalf("got %d docs err=%v, want %v", len(got), err, tc.want)
			}
			for _, id := range tc.want {
				if _, ok := got[id]; !ok {
					t.Fatalf("missing %s", id)
				}
			}
		})
	}
	if got, _ := repo.GetMany(ctx, fx.other, []string{fx.page}); len(got) != 0 {
		t.Fatal("GetMany read across campaigns")
	}
	if got, _ := repo.GetMany(ctx, fx.campaign, []string{fx.page2}); len(got[fx.page2]) != len(big) {
		t.Fatalf("large sheet was truncated to %d of %d bytes", len(got[fx.page2]), len(big))
	}

	// Deleting the page removes its sheet.
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page)
	if _, _, found, _ := repo.Get(ctx, fx.campaign, fx.page); found {
		t.Fatal("sheet outlived its page")
	}
}

func TestBoardRepositoryIntegration_LooksAndBoards(t *testing.T) {
	fx := newQuestsFixture(t)
	ctx := context.Background()
	repo := NewBoardRepository(fx.db)

	if l, err := repo.GetLooks(ctx, fx.campaign, PageHome(fx.page)); err != nil || l != (Looks{Board: LookLit, Ledger: LookLit}) {
		t.Fatalf("default looks: %+v %v", l, err)
	}
	for _, want := range []Looks{{LookPlain, LookParchment}, {LookMidnight, LookLit}} {
		if err := repo.SetLooks(ctx, fx.campaign, PageHome(fx.page), want); err != nil {
			t.Fatalf("set looks: %v", err)
		}
		if got, err := repo.GetLooks(ctx, fx.campaign, PageHome(fx.page)); err != nil || got != want {
			t.Fatalf("looks = %+v %v, want %+v (upsert)", got, err, want)
		}
	}
	if l, _ := repo.GetLooks(ctx, fx.campaign, PageHome(fx.page2)); l.Board != LookLit {
		t.Fatalf("looks leaked to another page: %+v", l)
	}

	mk := func(id, name, who string, order int) Board {
		return Board{ID: id, CampaignID: fx.campaign, Home: PageHome(fx.page), Name: name, Who: who, SortOrder: order}
	}
	boards := []Board{mk("b-1", "Bounties", WhoDM, 0), mk("b-2", "Notes", WhoAll, 1), mk("b-3", "Scribes", WhoScribe, 2)}
	for _, b := range boards {
		if err := repo.InsertBoard(ctx, b); err != nil {
			t.Fatalf("insert %s: %v", b.ID, err)
		}
	}
	if err := repo.InsertBoard(ctx, mk("b-bad", "x", "everyone", 0)); err == nil {
		t.Fatal("an unknown who value must be rejected by the ENUM")
	}
	if err := repo.InsertBoard(ctx, mk("b-1", "dup", WhoDM, 0)); err == nil {
		t.Fatal("duplicate board id must fail")
	}
	if err := repo.InsertBoard(ctx, Board{ID: "b-x", CampaignID: fx.campaign, Home: PageHome(fx.page), Name: "ok", Who: WhoDM}); err != nil {
		t.Fatalf("zero sort order board: %v", err)
	}
	if err := repo.DeleteBoard(ctx, fx.campaign, PageHome(fx.page), "b-x"); err != nil {
		t.Fatal(err)
	}

	ids := func(bs []Board) []string {
		var o []string
		for _, b := range bs {
			o = append(o, b.ID)
		}
		return o
	}
	assertOrder := func(want ...string) {
		t.Helper()
		got, err := repo.ListBoards(ctx, fx.campaign, PageHome(fx.page))
		if err != nil || !reflect.DeepEqual(ids(got), want) {
			t.Fatalf("order = %v err=%v, want %v", ids(got), err, want)
		}
	}
	assertOrder("b-1", "b-2", "b-3")

	t.Run("get is scoped to page and campaign", func(t *testing.T) {
		got, err := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-2")
		if err != nil || !reflect.DeepEqual(*got, boards[1]) {
			t.Fatalf("got %+v err=%v", got, err)
		}
		for name, args := range map[string]struct {
			campaign string
			home     Home
			id       string
		}{
			"other page":     {fx.campaign, PageHome(fx.page2), "b-2"},
			"other campaign": {fx.other, PageHome(fx.page), "b-2"},
			"unknown id":     {fx.campaign, PageHome(fx.page), "b-zzz"},
		} {
			if _, err := repo.GetBoard(ctx, args.campaign, args.home, args.id); !isNotFound(err) {
				t.Errorf("%s: want NotFound, got %v", name, err)
			}
		}
		if bs, _ := repo.ListBoards(ctx, fx.campaign, PageHome(fx.page2)); len(bs) != 0 {
			t.Error("boards listed for the wrong page")
		}
	})

	t.Run("rename and change who", func(t *testing.T) {
		b := boards[0]
		b.Name, b.Who = "Renamed", WhoScribe
		if err := repo.UpdateBoard(ctx, b); err != nil {
			t.Fatal(err)
		}
		got, _ := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-1")
		if got.Name != "Renamed" || got.Who != WhoScribe || got.SortOrder != 0 {
			t.Fatalf("got %+v", got)
		}
		// A wrong campaign must not touch the row.
		b.CampaignID, b.Name = fx.other, "Hijack"
		_ = repo.UpdateBoard(ctx, b)
		if got, _ := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-1"); got.Name != "Renamed" {
			t.Fatalf("update crossed campaigns: %+v", got)
		}
	})

	t.Run("reorder", func(t *testing.T) {
		if err := repo.SetOrder(ctx, fx.campaign, PageHome(fx.page), []string{"b-3", "b-1", "b-2"}); err != nil {
			t.Fatal(err)
		}
		assertOrder("b-3", "b-1", "b-2")
		// Ids from another page are ignored rather than reordered.
		if err := repo.SetOrder(ctx, fx.campaign, PageHome(fx.page2), []string{"b-2"}); err != nil {
			t.Fatal(err)
		}
		assertOrder("b-3", "b-1", "b-2")
		got, _ := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-2")
		if got.SortOrder != 2 {
			t.Fatalf("sort_order = %d, want 2", got.SortOrder)
		}
	})

	t.Run("delete is scoped and cascades to items", func(t *testing.T) {
		if err := repo.InsertItem(ctx, Item{ID: "i-1", BoardID: "b-1", CampaignID: fx.campaign, Kind: KindNote, Text: "hi"}); err != nil {
			t.Fatal(err)
		}
		_ = repo.DeleteBoard(ctx, fx.other, PageHome(fx.page), "b-1")
		_ = repo.DeleteBoard(ctx, fx.campaign, PageHome(fx.page2), "b-1")
		if _, err := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-1"); err != nil {
			t.Fatalf("board deleted through the wrong scope: %v", err)
		}
		if err := repo.DeleteBoard(ctx, fx.campaign, PageHome(fx.page), "b-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetBoard(ctx, fx.campaign, PageHome(fx.page), "b-1"); !isNotFound(err) {
			t.Fatalf("want NotFound after delete, got %v", err)
		}
		if n, _ := repo.CountItems(ctx, fx.campaign, "b-1"); n != 0 {
			t.Fatalf("%d items outlived their board", n)
		}
		assertOrder("b-3", "b-2")
	})
}

func TestBoardRepositoryIntegration_Items(t *testing.T) {
	fx := newQuestsFixture(t)
	ctx := context.Background()
	repo := NewBoardRepository(fx.db)
	for _, b := range []Board{
		{ID: "b-1", CampaignID: fx.campaign, Home: PageHome(fx.page), Name: "One", Who: WhoAll},
		{ID: "b-2", CampaignID: fx.campaign, Home: PageHome(fx.page), Name: "Two", Who: WhoAll},
	} {
		if err := repo.InsertBoard(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	items := []Item{
		{ID: "i-notice", BoardID: "b-1", CampaignID: fx.campaign, Kind: KindNotice, X: 10.5, Y: 20.25, W: 25, R: -4.5, ByDM: true, RefID: fx.page2},
		{ID: "i-note", BoardID: "b-1", CampaignID: fx.campaign, Kind: KindNote, X: 1, Y: 2, W: 20, OwnerUserID: fx.user, Text: "meet at “dawn” \U0001F319", Hidden: true},
		{ID: "i-string", BoardID: "b-1", CampaignID: fx.campaign, Kind: KindString, OwnerUserID: fx.user, FromID: "i-notice", ToID: "i-note"},
		{ID: "i-other", BoardID: "b-2", CampaignID: fx.campaign, Kind: KindMap, RefID: "map-9"},
	}
	for _, it := range items {
		if err := repo.InsertItem(ctx, it); err != nil {
			t.Fatalf("insert %s: %v", it.ID, err)
		}
	}

	t.Run("round trip keeps every column, NULL as empty", func(t *testing.T) {
		for _, want := range items {
			got, err := repo.GetItem(ctx, fx.campaign, want.BoardID, want.ID)
			if err != nil || !reflect.DeepEqual(*got, want) {
				t.Fatalf("%s: got %+v err=%v, want %+v", want.ID, got, err, want)
			}
		}
	})

	t.Run("by_dm is stored and not changed by update", func(t *testing.T) {
		it := items[0]
		it.ByDM, it.X = false, 99
		if err := repo.UpdateItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		got, _ := repo.GetItem(ctx, fx.campaign, "b-1", "i-notice")
		if !got.ByDM || got.X != 99 {
			t.Fatalf("by_dm must survive an edit and x must move: %+v", got)
		}
	})

	t.Run("update moves, sets and clears text, toggles hidden", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*Item)
		}{
			{"move and resize", func(i *Item) { i.X, i.Y, i.W, i.R = 5, 6, 30, 7.5 }},
			{"clear text", func(i *Item) { i.Text = "" }},
			{"set text", func(i *Item) { i.Text = "again" }},
			{"unhide", func(i *Item) { i.Hidden = false }},
		}
		cur, _ := repo.GetItem(ctx, fx.campaign, "b-1", "i-note")
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				tc.edit(cur)
				if err := repo.UpdateItem(ctx, *cur); err != nil {
					t.Fatal(err)
				}
				got, _ := repo.GetItem(ctx, fx.campaign, "b-1", "i-note")
				if !reflect.DeepEqual(got, cur) {
					t.Fatalf("got %+v, want %+v", got, cur)
				}
			})
		}
	})

	t.Run("lookups are scoped to board and campaign", func(t *testing.T) {
		if _, err := repo.GetItem(ctx, fx.campaign, "b-2", "i-note"); !isNotFound(err) {
			t.Errorf("wrong board: %v", err)
		}
		if _, err := repo.GetItem(ctx, fx.other, "b-1", "i-note"); !isNotFound(err) {
			t.Errorf("wrong campaign: %v", err)
		}
		moved := items[1]
		moved.BoardID, moved.Text = "b-2", "hijack"
		_ = repo.UpdateItem(ctx, moved)
		if got, _ := repo.GetItem(ctx, fx.campaign, "b-1", "i-note"); got.Text == "hijack" {
			t.Error("update crossed boards")
		}
	})

	t.Run("list and count", func(t *testing.T) {
		if got, err := repo.ListItems(ctx, fx.campaign, nil); err != nil || got != nil {
			t.Fatalf("no boards: %v %v", got, err)
		}
		all, err := repo.ListItems(ctx, fx.campaign, []string{"b-1", "b-2"})
		if err != nil || len(all) != 4 {
			t.Fatalf("got %d err=%v", len(all), err)
		}
		one, _ := repo.ListItems(ctx, fx.campaign, []string{"b-2"})
		if len(one) != 1 || one[0].ID != "i-other" {
			t.Fatalf("got %+v", one)
		}
		if got, _ := repo.ListItems(ctx, fx.other, []string{"b-1"}); len(got) != 0 {
			t.Fatal("listed across campaigns")
		}
		for board, want := range map[string]int{"b-1": 3, "b-2": 1, "b-none": 0} {
			if n, err := repo.CountItems(ctx, fx.campaign, board); err != nil || n != want {
				t.Errorf("count %s = %d err=%v, want %d", board, n, err, want)
			}
		}
		if n, _ := repo.CountItems(ctx, fx.other, "b-1"); n != 0 {
			t.Error("counted across campaigns")
		}
	})

	t.Run("delete only the named items on that board", func(t *testing.T) {
		if err := repo.DeleteItems(ctx, fx.campaign, "b-1", nil); err != nil {
			t.Fatalf("empty delete: %v", err)
		}
		// i-other lives on b-2, so naming it on b-1 must not remove it.
		if err := repo.DeleteItems(ctx, fx.campaign, "b-1", []string{"i-string", "i-other"}); err != nil {
			t.Fatal(err)
		}
		if n, _ := repo.CountItems(ctx, fx.campaign, "b-1"); n != 2 {
			t.Fatalf("b-1 has %d items, want 2", n)
		}
		if n, _ := repo.CountItems(ctx, fx.campaign, "b-2"); n != 1 {
			t.Fatalf("an item on another board was deleted")
		}
		_ = repo.DeleteItems(ctx, fx.other, "b-1", []string{"i-note"})
		if n, _ := repo.CountItems(ctx, fx.campaign, "b-1"); n != 2 {
			t.Fatal("delete crossed campaigns")
		}
	})

	t.Run("clearing player items keeps DM pieces", func(t *testing.T) {
		// The service clears a player's pins by listing then deleting the
		// player-owned, non-DM ids; the by_dm flag is what separates them.
		all, _ := repo.ListItems(ctx, fx.campaign, []string{"b-1"})
		var mine []string
		for _, it := range all {
			if it.OwnerUserID == fx.user && !it.ByDM {
				mine = append(mine, it.ID)
			}
		}
		if len(mine) != 1 || mine[0] != "i-note" {
			t.Fatalf("player items = %v", mine)
		}
		if err := repo.DeleteItems(ctx, fx.campaign, "b-1", mine); err != nil {
			t.Fatal(err)
		}
		left, _ := repo.ListItems(ctx, fx.campaign, []string{"b-1"})
		if len(left) != 1 || left[0].ID != "i-notice" || !left[0].ByDM {
			t.Fatalf("left = %+v", left)
		}
	})

	t.Run("an item needs an existing board", func(t *testing.T) {
		if err := repo.InsertItem(ctx, Item{ID: "i-orphan", BoardID: "b-none", CampaignID: fx.campaign, Kind: KindNote}); err == nil {
			t.Fatal("orphan item inserted")
		}
	})
}

func TestBoardRepositoryIntegration_TypeHomes(t *testing.T) {
	fx := newQuestsFixture(t)
	ctx := context.Background()
	repo := NewBoardRepository(fx.db)
	cat, cat2 := TypeHome(fx.typ), TypeHome(fx.typ2)

	t.Run("a home is exactly one of page or category", func(t *testing.T) {
		for name, h := range map[string]Home{"none": {}, "both": {EntityID: fx.page, TypeID: fx.typ}, "negative": {TypeID: -1}} {
			if _, err := repo.ListBoards(ctx, fx.campaign, h); err == nil {
				t.Errorf("%s: list accepted an invalid home", name)
			}
			if _, err := repo.GetLooks(ctx, fx.campaign, h); err == nil {
				t.Errorf("%s: looks accepted an invalid home", name)
			}
			if err := repo.InsertBoard(ctx, Board{ID: "b-bad", CampaignID: fx.campaign, Home: h, Name: "x", Who: WhoDM}); err == nil {
				t.Errorf("%s: insert accepted an invalid home", name)
			}
		}
	})

	t.Run("looks upsert per category and stay apart from pages", func(t *testing.T) {
		if l, err := repo.GetLooks(ctx, fx.campaign, cat); err != nil || l != (Looks{Board: LookLit, Ledger: LookLit}) {
			t.Fatalf("default looks: %+v %v", l, err)
		}
		for _, want := range []Looks{{LookPlain, LookParchment}, {LookMidnight, LookLit}} {
			if err := repo.SetLooks(ctx, fx.campaign, cat, want); err != nil {
				t.Fatal(err)
			}
			if got, err := repo.GetLooks(ctx, fx.campaign, cat); err != nil || got != want {
				t.Fatalf("looks = %+v %v, want %+v", got, err, want)
			}
		}
		if l, _ := repo.GetLooks(ctx, fx.campaign, cat2); l.Board != LookLit {
			t.Errorf("looks leaked to another category: %+v", l)
		}
		if l, _ := repo.GetLooks(ctx, fx.campaign, PageHome(fx.page)); l.Board != LookLit {
			t.Errorf("looks leaked to a page: %+v", l)
		}
		// The row belongs to the campaign it was written for.
		if l, _ := repo.GetLooks(ctx, fx.other, cat); l.Board != LookLit {
			t.Errorf("looks read across campaigns: %+v", l)
		}
	})

	mk := func(id string, h Home, order int) Board {
		return Board{ID: id, CampaignID: fx.campaign, Home: h, Name: id, Who: WhoAll, SortOrder: order}
	}
	for _, b := range []Board{mk("t-1", cat, 0), mk("t-2", cat, 1), mk("t-3", cat2, 0), mk("p-1", PageHome(fx.page), 0)} {
		if err := repo.InsertBoard(ctx, b); err != nil {
			t.Fatalf("insert %s: %v", b.ID, err)
		}
	}

	t.Run("homes list each place with boards once, in this campaign only", func(t *testing.T) {
		got, err := repo.ListHomes(ctx, fx.campaign)
		if err != nil {
			t.Fatal(err)
		}
		want := map[Home]bool{cat: true, cat2: true, PageHome(fx.page): true}
		if len(got) != len(want) {
			t.Fatalf("homes %+v, want %d", got, len(want))
		}
		for _, h := range got {
			if !want[h] {
				t.Errorf("unexpected home %+v", h)
			}
		}
		if other, err := repo.ListHomes(ctx, fx.other); err != nil || len(other) != 0 {
			t.Errorf("other campaign homes %+v err=%v", other, err)
		}
	})

	t.Run("round trip and listing are scoped to the home", func(t *testing.T) {
		got, err := repo.GetBoard(ctx, fx.campaign, cat, "t-2")
		if err != nil || !reflect.DeepEqual(*got, mk("t-2", cat, 1)) {
			t.Fatalf("got %+v err=%v", got, err)
		}
		if bs, _ := repo.ListBoards(ctx, fx.campaign, cat); len(bs) != 2 {
			t.Errorf("category lists %d boards, want 2", len(bs))
		}
		if bs, _ := repo.ListBoards(ctx, fx.campaign, PageHome(fx.page)); len(bs) != 1 || bs[0].ID != "p-1" {
			t.Errorf("page lists %+v, want only p-1", bs)
		}
		for name, c := range map[string]struct {
			campaign string
			home     Home
			id       string
		}{
			"other category":       {fx.campaign, cat2, "t-1"},
			"page asking for it":   {fx.campaign, PageHome(fx.page), "t-1"},
			"category asking page": {fx.campaign, cat, "p-1"},
			"other campaign":       {fx.other, cat, "t-1"},
		} {
			if _, err := repo.GetBoard(ctx, c.campaign, c.home, c.id); !isNotFound(err) {
				t.Errorf("%s: want NotFound, got %v", name, err)
			}
		}
	})

	t.Run("update, reorder and delete cannot cross homes", func(t *testing.T) {
		hijack := mk("t-1", cat2, 0)
		hijack.Name = "Hijack"
		_ = repo.UpdateBoard(ctx, hijack)
		_ = repo.DeleteBoard(ctx, fx.campaign, cat2, "t-1")
		if got, err := repo.GetBoard(ctx, fx.campaign, cat, "t-1"); err != nil || got.Name != "t-1" {
			t.Fatalf("board changed through the wrong home: %+v %v", got, err)
		}
		if err := repo.SetOrder(ctx, fx.campaign, cat, []string{"t-2", "t-1"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.SetOrder(ctx, fx.campaign, cat2, []string{"t-1"}); err != nil {
			t.Fatal(err)
		}
		bs, _ := repo.ListBoards(ctx, fx.campaign, cat)
		if len(bs) != 2 || bs[0].ID != "t-2" || bs[1].ID != "t-1" {
			t.Fatalf("order = %+v", bs)
		}
	})

	t.Run("deleting the category removes its boards, items and looks", func(t *testing.T) {
		if err := repo.InsertItem(ctx, Item{ID: "ti-1", BoardID: "t-3", CampaignID: fx.campaign, Kind: KindNote, Text: "hi"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.SetLooks(ctx, fx.campaign, cat2, Looks{LookPlain, LookPlain}); err != nil {
			t.Fatal(err)
		}
		fx.exec(t, `DELETE FROM entity_types WHERE id = ?`, fx.typ2)
		if bs, _ := repo.ListBoards(ctx, fx.campaign, cat2); len(bs) != 0 {
			t.Errorf("%d boards outlived their category", len(bs))
		}
		if n, _ := repo.CountItems(ctx, fx.campaign, "t-3"); n != 0 {
			t.Errorf("%d items outlived their category", n)
		}
		var n int
		if err := fx.db.QueryRow(`SELECT COUNT(*) FROM quest_board_type_looks WHERE entity_type_id = ?`, fx.typ2).Scan(&n); err != nil || n != 0 {
			t.Errorf("category looks outlived the category: %d %v", n, err)
		}
		if bs, _ := repo.ListBoards(ctx, fx.campaign, PageHome(fx.page)); len(bs) != 1 {
			t.Error("deleting a category touched a page's boards")
		}
	})
}
