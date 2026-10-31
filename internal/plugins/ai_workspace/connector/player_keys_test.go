package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// playerService is a watched service whose camp-1 lets players use AI.
func playerService(t *testing.T) (*Service, *memRepo, *memWatch) {
	t.Helper()
	svc, repo, _, _ := newTestService()
	w := newMemWatch(repo)
	svc.SetWatch(w)
	if err := svc.SetPlayersAllowed(context.Background(), "camp-1", "owner-1", true); err != nil {
		t.Fatal(err)
	}
	return svc, repo, w
}

// A player's key needs both switches on, and the server's admin's and the
// owner's switches are read on every call, not just when the key is made.
func TestPlayerKey_NeedsBothSwitches(t *testing.T) {
	ctx := context.Background()

	t.Run("no switches wired refuses players", func(t *testing.T) {
		svc, _, _, _ := newTestService()
		if _, _, err := svc.Connect(ctx, "camp-1", "player-1", aiexport.PrivacyModeSafe, true); err == nil {
			t.Fatal("a player made a key with no switches to allow it")
		}
	})
	t.Run("owner hasn't allowed players", func(t *testing.T) {
		svc, repo, _, _ := newTestService()
		svc.SetWatch(newMemWatch(repo))
		if _, _, err := svc.Connect(ctx, "camp-1", "player-1", aiexport.PrivacyModeSafe, true); err == nil {
			t.Fatal("a player made a key before the owner allowed it")
		}
	})
	t.Run("not in the campaign", func(t *testing.T) {
		svc, _, _ := playerService(t)
		if _, _, err := svc.Connect(ctx, "camp-1", "stranger", aiexport.PrivacyModeSafe, true); err == nil {
			t.Fatal("a stranger made a key")
		}
	})

	svc, _, w := playerService(t)
	raw, c, err := svc.Connect(ctx, "camp-1", "player-1", aiexport.PrivacyModePermitted, true)
	if err != nil {
		t.Fatal(err)
	}
	if !c.ForPlayer || c.Privacy != aiexport.PrivacyModeSafe {
		t.Fatalf("player key = %+v, want ForPlayer and Safe whatever was asked", c)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); err != nil {
		t.Fatalf("allowed player key refused: %v", err)
	}

	if err := svc.SetPlayersAllowed(ctx, "camp-1", "owner-1", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrPlayersOff) {
		t.Fatalf("after the owner turned players off: %v, want ErrPlayersOff", err)
	}
	if err := svc.SetPlayersAllowed(ctx, "camp-1", "owner-1", true); err != nil {
		t.Fatal(err)
	}
	w.server.PlayersBlocked = true
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrPlayersBlocked) {
		t.Fatalf("after the admin blocked players: %v, want ErrPlayersBlocked", err)
	}
	if err := svc.SetPlayersAllowed(ctx, "camp-1", "owner-1", true); err == nil {
		t.Error("the owner turned players on while the admin blocks them")
	}
	w.server.PlayersBlocked = false

	// Leaving the campaign ends the key's use at once.
	testMembers["camp-1"] = []string{"player-2"}
	defer func() { testMembers["camp-1"] = []string{"player-1", "player-2"} }()
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrPlayerGone) {
		t.Fatalf("after the player left: %v, want ErrPlayerGone", err)
	}
}

// A player's key reads as that player, whatever privacy its row holds, so
// a stored "permitted" can never widen it to the owner's view.
func TestPlayerKey_ReadsAsThePlayer(t *testing.T) {
	c := &Connection{UserID: "player-1", ForPlayer: true, Privacy: aiexport.PrivacyModePermitted}
	a := actorFor(c)
	if a.UserID != "player-1" || a.Role != int(campaigns.RolePlayer) || a.CanAuthorDmOnly() {
		t.Fatalf("actor = %+v, want player-1 as a player", a)
	}
	own := actorFor(&Connection{UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted})
	if own.Role != int(campaigns.RoleOwner) {
		t.Fatalf("owner's permitted key reads as role %d", own.Role)
	}
}

// Each person has their own key: a player's key leaves the owner's alone,
// and each one's requests wait only for them.
func TestPlayerKey_OnePerPersonAndOwnRequests(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := playerService(t)
	_, ownerKey, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModePermitted, true)
	if err != nil {
		t.Fatal(err)
	}
	_, playerKey, err := svc.Connect(ctx, "camp-1", "player-1", aiexport.PrivacyModeSafe, true)
	if err != nil {
		t.Fatal(err)
	}
	if cur, _ := svc.Current(ctx, "camp-1", "owner-1"); cur == nil || cur.ID != ownerKey.ID {
		t.Fatalf("the player's key replaced the owner's: %+v", cur)
	}
	if cur, _ := svc.Current(ctx, "camp-1", "player-2"); cur != nil {
		t.Fatalf("player-2 has player-1's key: %+v", cur)
	}

	q, err := svc.Ask(ctx, playerKey, "My hero's class", "---\naction: update\nname: Tyne\nfields:\n  class: Fury\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := svc.Waiting(ctx, "camp-1", "owner-1"); len(w) != 0 {
		t.Errorf("the owner sees the player's request: %+v", w)
	}
	if w, _ := svc.Waiting(ctx, "camp-1", "player-1"); len(w) != 1 || w[0].ID != q.ID {
		t.Errorf("player's waiting = %+v", w)
	}
	if _, err := svc.WaitingRequest(ctx, "camp-1", "owner-1", q.ID); err == nil {
		t.Error("the owner opened the player's request")
	}
	if err := svc.TurnDown(ctx, "camp-1", "player-2", q.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ := svc.Waiting(ctx, "camp-1", "player-1"); len(w) != 1 {
		t.Error("another player turned down player-1's request")
	}
}

// The owner can stop and restart one player's key; nobody else can, and the
// owner's own key is not a player's.
func TestStopPlayerKey(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := playerService(t)
	raw, pk, err := svc.Connect(ctx, "camp-1", "player-1", aiexport.PrivacyModeSafe, false)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopPlayerKey(ctx, "camp-1", "player-2", pk.ID, true); err == nil {
		t.Error("a player stopped another player's key")
	}
	if _, err := svc.StopPlayerKey(ctx, "camp-1", "owner-1", ok.ID, true); err == nil {
		t.Error("the owner's own key was treated as a player's")
	}
	if _, err := svc.StopPlayerKey(ctx, "camp-2", "owner-2", pk.ID, true); err == nil {
		t.Error("another campaign's owner stopped this key")
	}
	if _, err := svc.StopPlayerKey(ctx, "camp-1", "owner-1", pk.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyPaused) {
		t.Fatalf("stopped key: %v, want ErrKeyPaused", err)
	}
	keys, err := svc.PlayerKeys(ctx, "camp-1", "owner-1")
	if err != nil || len(keys) != 1 || keys[0].PausedAt == nil {
		t.Fatalf("player keys = %+v, %v", keys, err)
	}
	if _, err := svc.PlayerKeys(ctx, "camp-1", "player-1"); err == nil {
		t.Error("a player listed the campaign's player keys")
	}
	if _, err := svc.StopPlayerKey(ctx, "camp-1", "owner-1", pk.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); err != nil {
		t.Fatalf("restarted key: %v", err)
	}
}

// Only the owner can let players in.
func TestSetPlayersAllowed_OwnerOnly(t *testing.T) {
	svc, repo, _, _ := newTestService()
	svc.SetWatch(newMemWatch(repo))
	if err := svc.SetPlayersAllowed(context.Background(), "camp-1", "player-1", true); err == nil {
		t.Fatal("a player turned on players' AI")
	}
}

// A player's key lists none of the owner's tools, can't call one by name,
// and is told its changes wait for the player and reach only their sheet.
func TestPlayerKey_Tools(t *testing.T) {
	c := &Connection{ID: 9, CampaignID: "camp-1", UserID: "player-1", ForPlayer: true, CanAsk: true, Privacy: aiexport.PrivacyModeSafe}
	sess := Session{Conn: c, CampaignName: "Ashfall", Actor: actorFor(c)}
	for _, tl := range toolsFor(c) {
		if tl.Owner {
			t.Errorf("player key lists %s", tl.Name)
		}
	}
	for _, name := range []string{"players", "waiting_for_review", "encounters", "rolling_tables", "foundry_monsters"} {
		if findTool(c, name) != nil {
			t.Errorf("player key can call %s", name)
		}
		var r rpcReply
		if err := json.Unmarshal((&Server{}).Handle(context.Background(), sess, callBody(name, nil)).Body, &r); err != nil || r.Error == nil {
			t.Errorf("%s answered a player's key: %+v %v", name, r, err)
		}
	}
	if d := findTool(c, toolAsk).describe(true); !strings.Contains(d, "your own character") {
		t.Errorf("ask_for_changes for a player says %q", d)
	}
	if got := instructions(sess); !strings.Contains(got, "what you, a player in it, can see") || strings.Contains(got, "secrets") {
		t.Errorf("instructions = %q", got)
	}
}
