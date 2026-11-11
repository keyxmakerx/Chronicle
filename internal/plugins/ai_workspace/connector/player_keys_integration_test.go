package connector

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// Each person's key, calls and requests are theirs: a player's new key
// leaves the owner's live, the owner sees none of the player's requests or
// calls, each has their own waiting cap, and the owner's list of players'
// keys holds only players' keys.
func TestRepoIntegration_PerPerson(t *testing.T) {
	db := newScratchDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	camp, owner := seedCampaign(t, db)
	player := newUUID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, player, player+"@example.test", "Bea", "x"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(u string, n int, forPlayer bool) int64 {
		hash := fmt.Sprintf("%064d", n)
		id, err := repo.Replace(ctx, &Connection{CampaignID: camp, UserID: u, KeyHash: hash, KeyHint: hash[60:],
			Privacy: aiexport.PrivacyModeSafe, CanAsk: true, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime), ForPlayer: forPlayer})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ownerKey := mk(owner, 1, false)
	playerKey := mk(player, 2, true)

	if c, _ := repo.Current(ctx, camp, owner); c == nil || c.ID != ownerKey || c.ForPlayer {
		t.Fatalf("the player's key replaced the owner's: %+v", c)
	}
	if c, _ := repo.Current(ctx, camp, player); c == nil || c.ID != playerKey || !c.ForPlayer {
		t.Fatalf("player's key = %+v", c)
	}
	if c, _ := repo.FindByHash(ctx, fmt.Sprintf("%064d", 2)); c == nil || !c.ForPlayer {
		t.Fatalf("for_player not read back: %+v", c)
	}

	for i, k := range []int64{ownerKey, playerKey} {
		if err := repo.AddCall(ctx, &Call{ConnectionID: k, CampaignID: camp, At: now, Tool: "read_page", Summary: fmt.Sprintf("call %d", i), OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	if calls, _ := repo.RecentCalls(ctx, camp, owner, 10); len(calls) != 1 || calls[0].ConnectionID != ownerKey {
		t.Errorf("owner's calls = %+v", calls)
	}
	if err := repo.PruneCalls(ctx, camp, owner, 1); err != nil {
		t.Fatal(err)
	}
	if calls, _ := repo.RecentCalls(ctx, camp, player, 10); len(calls) != 1 {
		t.Errorf("pruning the owner's calls touched the player's: %+v", calls)
	}

	ask := func(conn int64, s string) (int64, error) {
		return repo.AddRequest(ctx, &Request{CampaignID: camp, ConnectionID: conn, Summary: s, Counts: "1 to change", Blocks: 1, Body: "x " + s, CreatedAt: now}, 1)
	}
	pq, err := ask(playerKey, "player's")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ask(ownerKey, "owner's"); err != nil {
		t.Fatalf("the player's waiting request used up the owner's cap: %v", err)
	}
	if _, err := ask(playerKey, "again"); !errors.Is(err, ErrTooManyWaiting) {
		t.Errorf("player's own cap: %v", err)
	}
	if list, _ := repo.Requests(ctx, camp, owner, RequestWaiting, 10); len(list) != 1 || list[0].Summary != "owner's" {
		t.Errorf("owner's waiting = %+v", list)
	}
	if q, _ := repo.Request(ctx, camp, owner, pq); q != nil {
		t.Error("the owner opened the player's request")
	}
	if ok, _ := repo.MoveRequest(ctx, camp, owner, pq, RequestWaiting, RequestTurnedDown, "x", now); ok {
		t.Error("the owner answered the player's request")
	}
	if ok, err := repo.MoveRequest(ctx, camp, player, pq, RequestWaiting, RequestAnswered, "1 applied", now); err != nil || !ok {
		t.Fatalf("the player's answer: %v %v", ok, err)
	}

	keys, err := repo.PlayerKeys(ctx, camp, now, now.Add(-time.Hour))
	if err != nil || len(keys) != 1 || keys[0].ID != playerKey || keys[0].PersonName != "Bea" || keys[0].KeyHash != "" || keys[0].CallsToday != 1 {
		t.Fatalf("player keys = %+v, %v", keys, err)
	}

	// Replacing the player's key revokes only theirs.
	newer := mk(player, 3, true)
	if c, _ := repo.Current(ctx, camp, owner); c == nil || c.ID != ownerKey {
		t.Fatal("a player's new key revoked the owner's")
	}
	if c, _ := repo.Current(ctx, camp, player); c == nil || c.ID != newer {
		t.Fatalf("player's current = %+v", c)
	}
}
