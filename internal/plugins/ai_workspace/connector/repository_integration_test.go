package connector

// Row-level tests for the connector's tables: revoking, the disabled-account
// filter, campaign scoping and pruning can only be shown against the
// database. Each test gets its own migrated scratch schema and skips (never
// fails) when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/ai_workspace/connector/ -run TestRepoIntegration

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

func newScratchDB(t *testing.T) *sql.DB {
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
	name := fmt.Sprintf("chronicle_aiconn_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	scratch.MultiStatements = true
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	root, _ := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	ups, err := fs.Glob(MigrationsFS, "migrations/*.up.sql")
	if err != nil || len(ups) == 0 {
		t.Fatalf("no migrations: %v", err)
	}
	sort.Strings(ups)
	for i := 0; i < 2; i++ { // idempotent DDL: a re-run must not fail
		for _, f := range ups {
			up, err := fs.ReadFile(MigrationsFS, f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(up)); err != nil {
				t.Fatalf("plugin migration %s run %d: %v", f, i+1, err)
			}
		}
	}
	return db
}

func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func seedCampaign(t *testing.T, db *sql.DB) (campaignID, userID string) {
	t.Helper()
	uid, cid := newUUID(t), newUUID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, uid+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Table", cid, uid); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return cid, uid
}

func TestRepoIntegration_ConnectionsAndCalls(t *testing.T) {
	db := newScratchDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	camp, user := seedCampaign(t, db)
	other, otherUser := seedCampaign(t, db)
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(campaign, u, hash string) int64 {
		id, err := repo.Replace(ctx, &Connection{CampaignID: campaign, UserID: u, KeyHash: hash, KeyHint: hash[60:],
			Privacy: aiexport.PrivacyModePermitted, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	h1, h2, h3 := fmt.Sprintf("%064d", 1), fmt.Sprintf("%064d", 2), fmt.Sprintf("%064d", 3)
	mk(camp, user, h1)
	theirs := mk(other, otherUser, h3)

	// A new key replaces the campaign's earlier one and leaves another
	// campaign's alone.
	second := mk(camp, user, h2)
	cur, err := repo.Current(ctx, camp)
	if err != nil || cur == nil || cur.ID != second || cur.Privacy != aiexport.PrivacyModePermitted || !cur.ExpiresAt.Equal(now.Add(KeyLifetime)) {
		t.Fatalf("current = %+v, %v", cur, err)
	}
	if old, _ := repo.FindByHash(ctx, h1); old == nil || old.RevokedAt == nil {
		t.Errorf("revoked key = %+v", old)
	}
	if c, _ := repo.FindByHash(ctx, h3); c == nil || c.RevokedAt != nil || c.ID != theirs {
		t.Errorf("another campaign's key was touched: %+v", c)
	}
	// A campaign that doesn't exist gets no key.
	if _, err := repo.Replace(ctx, &Connection{CampaignID: "no-such-campaign", UserID: user, KeyHash: fmt.Sprintf("%064d", 9), KeyHint: "0009",
		Privacy: aiexport.PrivacyModeSafe, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)}); err == nil {
		t.Error("a key was made for a campaign that doesn't exist")
	}
	if err := repo.RevokeAll(ctx, other, now); err != nil {
		t.Fatal(err)
	}
	if c, _ := repo.FindByHash(ctx, h3); c == nil || c.RevokedAt == nil {
		t.Errorf("Disconnect left the key live: %+v", c)
	}
	if cur, _ := repo.Current(ctx, other); cur != nil {
		t.Errorf("a disconnected campaign still has a connection: %+v", cur)
	}

	// A disabled account's keys are not found.
	if _, err := db.Exec(`UPDATE users SET is_disabled = TRUE WHERE id = ?`, user); err != nil {
		t.Fatal(err)
	}
	if c, _ := repo.FindByHash(ctx, h2); c != nil {
		t.Errorf("a disabled maker's key was found: %+v", c)
	}
	if _, err := db.Exec(`UPDATE users SET is_disabled = FALSE WHERE id = ?`, user); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkUsed(ctx, second, now, "Claude"); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkUsed(ctx, second, now.Add(time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if cur, _ := repo.Current(ctx, camp); cur.ClientName != "Claude" || cur.LastUsedAt == nil || !cur.LastUsedAt.Equal(now.Add(time.Minute)) {
		t.Errorf("after use = %+v", cur)
	}

	// Calls are listed newest first, per campaign, and pruned to the newest.
	for i := 0; i < 7; i++ {
		if err := repo.AddCall(ctx, &Call{ConnectionID: second, CampaignID: camp, At: now, Tool: "read_page", Summary: fmt.Sprintf("call %d", i), OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.AddCall(ctx, &Call{ConnectionID: theirs, CampaignID: other, At: now, Tool: "players", Summary: "theirs"}); err != nil {
		t.Fatal(err)
	}
	calls, err := repo.RecentCalls(ctx, camp, 3)
	if err != nil || len(calls) != 3 || calls[0].Summary != "call 6" || !calls[0].OK {
		t.Fatalf("recent = %+v, %v", calls, err)
	}
	if err := repo.PruneCalls(ctx, camp, 2); err != nil {
		t.Fatal(err)
	}
	if calls, _ := repo.RecentCalls(ctx, camp, 50); len(calls) != 2 || calls[1].Summary != "call 5" {
		t.Errorf("after pruning = %+v", calls)
	}
	if calls, _ := repo.RecentCalls(ctx, other, 50); len(calls) != 1 {
		t.Errorf("pruning reached another campaign: %+v", calls)
	}

	// Deleting the campaign takes its connections and calls with it.
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, camp); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ai_connection_calls WHERE campaign_id = ?`, camp).Scan(&n); err != nil || n != 0 {
		t.Errorf("calls left after delete: %d, %v", n, err)
	}
	if c, _ := repo.FindByHash(ctx, h2); c != nil {
		t.Errorf("connection left after delete: %+v", c)
	}
}

func TestRepoIntegration_Requests(t *testing.T) {
	db := newScratchDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	camp, user := seedCampaign(t, db)
	other, otherUser := seedCampaign(t, db)
	now := time.Now().UTC().Truncate(time.Second)

	conn, err := repo.Replace(ctx, &Connection{CampaignID: camp, UserID: user, KeyHash: fmt.Sprintf("%064d", 1), KeyHint: "0001",
		Privacy: aiexport.PrivacyModeSafe, CanAsk: true, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := repo.Current(ctx, camp); c == nil || !c.CanAsk {
		t.Fatalf("the Change setting wasn't kept: %+v", c)
	}
	theirConn, _ := repo.Replace(ctx, &Connection{CampaignID: other, UserID: otherUser, KeyHash: fmt.Sprintf("%064d", 2), KeyHint: "0002",
		Privacy: aiexport.PrivacyModeSafe, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)})
	if c, _ := repo.Current(ctx, other); c == nil || c.CanAsk {
		t.Fatalf("Only look read back as Ask: %+v", c)
	}

	add := func(campaign string, connID int64, summary string) (int64, error) {
		return repo.AddRequest(ctx, &Request{CampaignID: campaign, ConnectionID: connID, AppName: "Claude", Summary: summary,
			Counts: "1 to add", Blocks: 1, Body: "---\nname: " + summary + "\n---\nbody\n", CreatedAt: now}, 3)
	}
	var ids []int64
	for i := 0; i < 3; i++ {
		id, err := add(camp, conn, fmt.Sprintf("r%d", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := add(camp, conn, "one too many"); !errors.Is(err, ErrTooManyWaiting) {
		t.Errorf("the waiting cap: %v", err)
	}
	theirs, err := add(other, theirConn, "theirs")
	if err != nil {
		t.Fatalf("another campaign's cap is its own: %v", err)
	}

	list, err := repo.Requests(ctx, camp, RequestWaiting, 50)
	if err != nil || len(list) != 3 || list[0].ID != ids[2] || list[0].Body != "" || list[0].Summary != "r2" {
		t.Fatalf("waiting = %+v, %v", list, err)
	}
	if q, _ := repo.Request(ctx, camp, theirs); q != nil {
		t.Errorf("read another campaign's request: %+v", q)
	}
	q, err := repo.Request(ctx, camp, ids[0])
	if err != nil || q == nil || q.Body != "---\nname: r0\n---\nbody\n" || q.Status != RequestWaiting {
		t.Fatalf("request = %+v, %v", q, err)
	}

	if ok, err := repo.FinishRequest(ctx, other, ids[0], RequestAnswered, "x", now); err != nil || ok {
		t.Errorf("answered from another campaign: %v %v", ok, err)
	}
	if ok, err := repo.FinishRequest(ctx, camp, ids[0], RequestAnswered, "2 applied, 0 left out", now); err != nil || !ok {
		t.Fatalf("answer: %v %v", ok, err)
	}
	if ok, _ := repo.FinishRequest(ctx, camp, ids[0], RequestTurnedDown, "y", now); ok {
		t.Error("an answered request was answered again")
	}
	if q, _ := repo.Request(ctx, camp, ids[0]); q.Status != RequestAnswered || q.Outcome != "2 applied, 0 left out" || q.DecidedAt == nil {
		t.Errorf("after answering = %+v", q)
	}
	if _, err := add(camp, conn, "room again"); err != nil {
		t.Errorf("answering one should make room: %v", err)
	}
	if _, err := repo.FinishRequest(ctx, camp, ids[1], RequestTurnedDown, "nothing changed", now); err != nil {
		t.Fatal(err)
	}
	if err := repo.PruneRequests(ctx, camp, 1); err != nil {
		t.Fatal(err)
	}
	if q, _ := repo.Request(ctx, camp, ids[0]); q != nil {
		t.Errorf("pruning kept the older answered request")
	}
	if q, _ := repo.Request(ctx, camp, ids[1]); q == nil {
		t.Errorf("pruning took the newest answered request")
	}
	if list, _ := repo.Requests(ctx, camp, RequestWaiting, 50); len(list) != 2 {
		t.Errorf("pruning touched waiting requests: %+v", list)
	}

	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, camp); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ai_connection_requests WHERE campaign_id = ?`, camp).Scan(&n); err != nil || n != 0 {
		t.Errorf("requests left after delete: %d, %v", n, err)
	}
}
