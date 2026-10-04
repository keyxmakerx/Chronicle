package armory

// Row-level tests for the conditional SQL behind stash balances. A mock
// cannot show that a debit refuses to go below zero or that a duplicate name
// is a Conflict; only the database can. Each test gets its own migrated
// scratch schema and skips (never fails) when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/armory/ -run TestStashRepoIntegration

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/database"
)

func newStashScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (tools/start-test-db.sh) to run the row-level tests")
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
	name := fmt.Sprintf("chronicle_stash_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
	return db
}

func stashUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// seedStashCampaign inserts a user and a campaign and returns the campaign id.
func seedStashCampaign(t *testing.T, db *sql.DB) string {
	t.Helper()
	uid, cid := stashUUID(t), stashUUID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, uid+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Table", cid, uid); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return cid
}

// seedEntity inserts a minimal entity (and a type) and returns its id.
func seedStashEntity(t *testing.T, db *sql.DB, campaignID, name string) string {
	t.Helper()
	id := stashUUID(t)
	var creator string
	if err := db.QueryRow(`SELECT created_by FROM campaigns WHERE id = ?`, campaignID).Scan(&creator); err != nil {
		t.Fatalf("campaign creator: %v", err)
	}
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color) VALUES (?,?,?,?,?,?)`,
		campaignID, "t-"+id[:8], "T", "Ts", "fa-user", "#000000")
	if err != nil {
		t.Fatalf("seed type: %v", err)
	}
	tid, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,NOW(),NOW())`,
		id, campaignID, tid, name, "s-"+id[:8], creator); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	return id
}

func TestStashRepoIntegration_ItemDebitIsConditional(t *testing.T) {
	db := newStashScratchDB(t)
	repo := NewStashRepository(db)
	ctx := context.Background()
	camp := seedStashCampaign(t, db)
	item := seedStashEntity(t, db, camp, "Potion")
	st := &Stash{CampaignID: camp, Name: "Chest"}
	if err := repo.CreateStash(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreditItem(ctx, camp, st.ID, item, 3); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreditItem(ctx, camp, st.ID, item, 2); err != nil { // upsert adds
		t.Fatal(err)
	}
	qty := func() int {
		m, err := repo.ListItems(ctx, camp)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range m[st.ID] {
			return r.Quantity
		}
		return 0
	}
	if qty() != 5 {
		t.Fatalf("after credits: %d", qty())
	}
	tests := []struct {
		name   string
		camp   string
		take   int
		wantOK bool
		left   int
	}{
		{"more than held changes nothing", camp, 6, false, 5},
		{"another campaign changes nothing", "other-campaign", 1, false, 5},
		{"part", camp, 2, true, 3},
		{"exactly the rest removes the line", camp, 3, true, 0},
		{"empty line", camp, 1, false, 0},
	}
	for _, tc := range tests {
		ok, err := repo.DebitItem(ctx, tc.camp, st.ID, item, tc.take)
		if err != nil || ok != tc.wantOK || qty() != tc.left {
			t.Fatalf("%s: ok=%v err=%v left=%d", tc.name, ok, err, qty())
		}
	}
	// A credit into another campaign's stash id inserts nothing.
	if err := repo.CreditItem(ctx, "other-campaign", st.ID, item, 1); code(err) != http.StatusNotFound {
		t.Fatalf("cross-campaign credit: %v", err)
	}
}

func TestStashRepoIntegration_MoneyNeverGoesNegative(t *testing.T) {
	db := newStashScratchDB(t)
	repo := NewStashRepository(db)
	ctx := context.Background()
	camp := seedStashCampaign(t, db)
	st := &Stash{CampaignID: camp, Name: "Chest"}
	if err := repo.CreateStash(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreditMoney(ctx, camp, st.ID, 1050); err != nil {
		t.Fatal(err)
	}
	money := func() Cents {
		s, err := repo.GetStash(ctx, camp, st.ID)
		if err != nil {
			t.Fatal(err)
		}
		return s.Money
	}
	if ok, _ := repo.DebitMoney(ctx, camp, st.ID, 1051); ok || money() != 1050 {
		t.Fatalf("over-debit: money=%d", money())
	}
	if ok, _ := repo.DebitMoney(ctx, "other-campaign", st.ID, 1); ok || money() != 1050 {
		t.Fatal("cross-campaign debit")
	}
	if ok, err := repo.DebitMoney(ctx, camp, st.ID, 1050); !ok || err != nil || money() != 0 {
		t.Fatalf("exact debit: %v %v %d", ok, err, money())
	}

	// Concurrent debits of the last 10.00 can win only once.
	if err := repo.CreditMoney(ctx, camp, st.ID, 1000); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := repo.DebitMoney(ctx, camp, st.ID, 1000); ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 || money() != 0 {
		t.Fatalf("wins=%d money=%d", wins, money())
	}
}

func TestStashRepoIntegration_StashesViewersMovesDowntime(t *testing.T) {
	db := newStashScratchDB(t)
	repo := NewStashRepository(db)
	ctx := context.Background()
	camp := seedStashCampaign(t, db)
	other := seedStashCampaign(t, db)
	char := seedStashEntity(t, db, camp, "Thorin")

	st := &Stash{CampaignID: camp, Name: "Chest", Location: "Cellar"}
	if err := repo.CreateStash(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateStash(ctx, &Stash{CampaignID: camp, Name: "Chest"}); code(err) != http.StatusConflict {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := repo.CreateStash(ctx, &Stash{CampaignID: other, Name: "Chest"}); err != nil {
		t.Fatalf("same name in another campaign: %v", err)
	}
	if _, err := repo.GetStash(ctx, other, st.ID); code(err) != http.StatusNotFound {
		t.Fatalf("foreign get: %v", err)
	}

	if err := repo.SetViewers(ctx, camp, st.ID, []string{char}); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.ListViewers(ctx, camp); len(v[st.ID]) != 1 {
		t.Fatalf("viewers %v", v)
	}
	if err := repo.SetViewers(ctx, other, st.ID, nil); code(err) != http.StatusNotFound {
		t.Fatalf("foreign set viewers: %v", err)
	}
	if err := repo.SetViewers(ctx, camp, st.ID, nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.ListViewers(ctx, camp); len(v[st.ID]) != 0 {
		t.Fatalf("viewers not cleared %v", v)
	}

	// History: insert, settle once, a second settle loses.
	m := &Move{CampaignID: camp, Kind: MoveKindMoney, Amount: 1250, From: Endpoint{EndpointCharacter, char}, To: StashEndpoint(st.ID), Status: MovePending, RequestedBy: "u1"}
	if err := repo.InsertMove(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetMove(ctx, camp, m.ID)
	if err != nil || got.Amount != 1250 || got.Status != MovePending || got.To != StashEndpoint(st.ID) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := repo.GetMove(ctx, other, m.ID); code(err) != http.StatusNotFound {
		t.Fatalf("foreign get move: %v", err)
	}
	if p, _ := repo.ListPending(ctx, camp); len(p) != 1 {
		t.Fatalf("pending %v", p)
	}
	if ok, _ := repo.SettleMove(ctx, camp, m.ID, MoveApplied, "", "gm"); !ok {
		t.Fatal("first settle lost")
	}
	if ok, _ := repo.SettleMove(ctx, camp, m.ID, MoveDeclined, "", "gm"); ok {
		t.Fatal("second settle won")
	}
	ep := StashEndpoint(st.ID)
	if l, _ := repo.ListMoves(ctx, camp, MoveFilter{Endpoint: &ep}); len(l) != 1 || l[0].Status != MoveApplied || l[0].DecidedAt == nil {
		t.Fatalf("history %+v", l)
	}

	// Downtime: no row means closed.
	if d, _ := repo.GetDowntime(ctx, camp); d.IsOpen {
		t.Fatal("default should be closed")
	}
	if err := repo.SetDowntime(ctx, camp, true, "gm"); err != nil {
		t.Fatal(err)
	}
	if d, _ := repo.GetDowntime(ctx, camp); !d.IsOpen {
		t.Fatal("not open")
	}
	if err := repo.SetDowntime(ctx, camp, false, "gm"); err != nil {
		t.Fatal(err)
	}
	if d, _ := repo.GetDowntime(ctx, camp); d.IsOpen {
		t.Fatal("not closed")
	}

	// Deleting a stash takes its items and viewers with it; the history stays.
	if err := repo.DeleteStash(ctx, camp, st.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteStash(ctx, camp, st.ID); code(err) != http.StatusNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

// A sheet money edit is a signed self-move row. item_moves.amount must keep the
// sign (a lowered balance is a negative amount) and the two equal ends.
func TestStashRepoIntegration_MoneyEditRowKeepsSign(t *testing.T) {
	db := newStashScratchDB(t)
	repo := NewStashRepository(db)
	ctx := context.Background()
	camp := seedStashCampaign(t, db)
	char := seedStashEntity(t, db, camp, "Thorin")

	dir := &fakeDir{ents: map[string]*EntityRef{
		char: {ID: char, Name: "Thorin", IsCharacter: true, MoneyKey: "gp", MoneyLabel: "Wealth"},
	}}
	// The fake directory only knows the campaign named "camp".
	h := NewMoneyHistory(repo, campaignDir{dir, camp}, nil, nil)
	src := changesource.With(ctx, changesource.Source{Kind: changesource.KindFoundry, UserID: "gm"})

	for _, step := range []struct{ from, to float64 }{{20, 5}, {5, 8}} {
		if err := h.RecordFieldChange(src, camp, char, map[string]any{"gp": step.from}, map[string]any{"gp": step.to}); err != nil {
			t.Fatal(err)
		}
	}
	ep := Endpoint{Kind: EndpointCharacter, ID: char}
	rows, err := repo.ListMoves(ctx, camp, MoveFilter{Endpoint: &ep})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	// Newest first.
	if rows[0].Amount != 300 || rows[1].Amount != -1500 {
		t.Fatalf("amounts = %d, %d; want 300, -1500", rows[0].Amount, rows[1].Amount)
	}
	for _, r := range rows {
		if !r.IsMoneyEdit() || r.Status != MoveApplied || r.RequestedBy != "gm" {
			t.Fatalf("row = %+v", r)
		}
	}
	if rows[1].Reason != "Wealth changed 20 → 5 · in Foundry" {
		t.Fatalf("reason = %q", rows[1].Reason)
	}
}

// campaignDir answers a fake directory's lookups for a scratch campaign id.
type campaignDir struct {
	*fakeDir
	campaign string
}

func (d campaignDir) GetEntity(ctx context.Context, campaignID, id string) (*EntityRef, error) {
	if campaignID != d.campaign {
		return nil, nil
	}
	return d.fakeDir.GetEntity(ctx, "camp", id)
}
