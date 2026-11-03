package connector

// Row-level tests for the watcher's tables: the switches, the per-key app
// list, alert upserts and their campaign scoping can only be shown against
// the database. Skips without CHRONICLE_TEST_DB_DSN, like the others here.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

func TestRepoIntegration_Watch(t *testing.T) {
	db := newScratchDB(t)
	repo := NewRepository(db)
	w := NewWatchRepository(db)
	ctx := context.Background()
	camp, user := seedCampaign(t, db)
	other, otherUser := seedCampaign(t, db)
	now := time.Now().UTC().Truncate(time.Second)

	// Server switches default to on, players allowed, and round-trip.
	if a, err := w.ServerAccess(ctx); err != nil || a.AIOff || a.PlayersBlocked {
		t.Fatalf("default server access = %+v, %v", a, err)
	}
	if err := w.SetServerAccess(ctx, ServerAccess{AIOff: true, PlayersBlocked: true}, user, now); err != nil {
		t.Fatal(err)
	}
	if a, _ := w.ServerAccess(ctx); !a.AIOff || !a.PlayersBlocked {
		t.Fatalf("saved server access = %+v", a)
	}

	// Campaign cut-off is per campaign and clears.
	if err := w.SetCutOff(ctx, camp, &now, now); err != nil {
		t.Fatal(err)
	}
	if a, _ := w.CampaignAccess(ctx, camp); a.CutOffAt == nil {
		t.Fatal("cut-off not saved")
	}
	if a, _ := w.CampaignAccess(ctx, other); a.CutOffAt != nil {
		t.Fatal("cut-off leaked to another campaign")
	}
	if err := w.SetCutOff(ctx, camp, nil, now); err != nil {
		t.Fatal(err)
	}
	if a, _ := w.CampaignAccess(ctx, camp); a.CutOffAt != nil {
		t.Fatal("cut-off not cleared")
	}

	mk := func(campaign, u string, n int) int64 {
		h := fmt.Sprintf("%064d", n)
		id, err := repo.Replace(ctx, &Connection{CampaignID: campaign, UserID: u, KeyHash: h, KeyHint: h[60:],
			Privacy: aiexport.PrivacyModePermitted, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	key, theirs := mk(camp, user, 1), mk(other, otherUser, 2)

	// The first app is quiet, the same app again is quiet, a second app warns.
	for i, tc := range []struct {
		app  string
		want bool
	}{{"Claude", false}, {"claude", false}, {"Other, Inc", true}, {"Other  Inc", false}} {
		got, err := w.NoteApp(ctx, key, tc.app)
		if err != nil || got != tc.want {
			t.Fatalf("NoteApp #%d %q = %v, %v; want %v", i, tc.app, got, err, tc.want)
		}
	}
	// A list at the column's limit still warns but stores nothing more.
	if _, err := db.Exec(`UPDATE ai_connections SET seen_apps = ? WHERE id = ?`, strings.Repeat("x", 250), theirs); err != nil {
		t.Fatal(err)
	}
	if got, err := w.NoteApp(ctx, theirs, "a long app name"); err != nil || !got {
		t.Fatalf("full list NoteApp = %v, %v", got, err)
	}

	// Alerts upsert per key and kind, and per address when there is no key.
	raise := func(a Alert) {
		a.CreatedAt, a.UpdatedAt = now, now
		if err := w.RaiseAlert(ctx, &a); err != nil {
			t.Fatal(err)
		}
	}
	raise(Alert{CampaignID: camp, ConnectionID: key, Kind: AlertLimit, Hits: 1})
	raise(Alert{CampaignID: camp, ConnectionID: key, Kind: AlertLimit, Hits: 2})
	raise(Alert{CampaignID: camp, ConnectionID: key, Kind: AlertNewApp, Hits: 1})
	raise(Alert{CampaignID: other, ConnectionID: theirs, Kind: AlertLimit, Hits: 1})
	raise(Alert{Kind: AlertWrongKeys, Address: "203.0.113.9", Hits: 20})
	raise(Alert{Kind: AlertWrongKeys, Address: "203.0.113.9", Hits: 5})
	raise(Alert{Kind: AlertWrongKeys, Address: "198.51.100.1", Hits: 20})

	mine, err := w.OpenAlerts(ctx, camp, 10)
	if err != nil || len(mine) != 2 {
		t.Fatalf("campaign alerts = %d, %v; want 2", len(mine), err)
	}
	all, _ := w.OpenAlerts(ctx, "", 10)
	if len(all) != 5 {
		t.Fatalf("all open alerts = %d; want 5", len(all))
	}
	var limitHits, floodHits int
	var limitID int64
	for _, a := range all {
		switch {
		case a.Kind == AlertLimit && a.ConnectionID == key:
			limitHits, limitID = a.Hits, a.ID
		case a.Kind == AlertWrongKeys && a.Address == "203.0.113.9":
			floodHits = a.Hits
			if a.CampaignID != "" || a.ConnectionID != 0 {
				t.Fatalf("flood alert carried a campaign or key: %+v", a)
			}
		}
	}
	if limitHits != 3 || floodHits != 25 {
		t.Fatalf("upserted hits limit=%d flood=%d; want 3 and 25", limitHits, floodHits)
	}

	// An owner can't settle another campaign's alert; the right one can, once.
	if a, err := w.SettleAlert(ctx, other, limitID, SettledMine, now); err != nil || a != nil {
		t.Fatalf("cross-campaign settle = %+v, %v", a, err)
	}
	if a, err := w.SettleAlert(ctx, camp, limitID, SettledMine, now); err != nil || a == nil || a.SettledHow != SettledMine {
		t.Fatalf("settle = %+v, %v", a, err)
	}
	if a, _ := w.SettleAlert(ctx, camp, limitID, SettledMine, now); a != nil {
		t.Fatal("settled twice")
	}
	// A settled alert no longer absorbs repeats: the next one is new.
	raise(Alert{CampaignID: camp, ConnectionID: key, Kind: AlertLimit, Hits: 1})
	if mine, _ = w.OpenAlerts(ctx, camp, 10); len(mine) != 2 {
		t.Fatalf("after settle, campaign alerts = %d; want 2", len(mine))
	}
	if recent, _ := w.RecentAlerts(ctx, now.Add(-time.Minute), 20); len(recent) != 6 {
		t.Fatalf("recent alerts = %d; want 6 including the settled one", len(recent))
	}

	// Live keys name the campaign and person and count today's calls; a
	// revoked key drops out.
	for i := 0; i < 3; i++ {
		if err := repo.AddCall(ctx, &Call{ConnectionID: key, CampaignID: camp, At: now, Tool: "read_page", OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.AddCall(ctx, &Call{ConnectionID: key, CampaignID: camp, At: now.Add(-48 * time.Hour), Tool: "read_page", OK: true}); err != nil {
		t.Fatal(err)
	}
	day := now.Add(-24 * time.Hour)
	keys, err := w.LiveKeys(ctx, now, day)
	if err != nil || len(keys) != 2 {
		t.Fatalf("live keys = %d, %v; want 2", len(keys), err)
	}
	for _, k := range keys {
		if k.KeyHash != "" {
			t.Fatal("LiveKeys returned a key hash")
		}
		if k.ID == key && (k.CampaignName != "Table" || k.PersonName != "Ana" || k.CallsToday != 3 || k.CallsTotal != 4) {
			t.Fatalf("key row = %+v", k)
		}
	}
	if n, err := w.CallsSince(ctx, day); err != nil || n != 3 {
		t.Fatalf("calls since = %d, %v; want 3", n, err)
	}
	if err := repo.RevokeOne(ctx, other, theirs, now); err != nil {
		t.Fatal(err)
	}
	if keys, _ = w.LiveKeys(ctx, now, day); len(keys) != 1 || keys[0].ID != key {
		t.Fatalf("after revoke, live keys = %+v", keys)
	}
	if c, err := w.ConnectionByID(ctx, theirs); err != nil || c == nil || c.RevokedAt == nil {
		t.Fatalf("ConnectionByID revoked = %+v, %v", c, err)
	}
	if c, _ := w.ConnectionByID(ctx, 999999); c != nil {
		t.Fatal("ConnectionByID found a missing key")
	}
}
