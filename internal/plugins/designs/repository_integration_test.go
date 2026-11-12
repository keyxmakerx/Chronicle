package designs

// Real-MariaDB tests for the repository: what the SQL does is the point. A
// link answers only by its token and only while on, named owners are what
// "shared with me" is made from, search finds owners and nothing else and
// carries no campaign ids, stopping a share clears everything it should, and
// deleting a design or a campaign takes its share state along.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/designs/ -run TestDB

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

func scratch(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_dsg_",
		RequireDSN:       true,
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: PluginSlug, FS: sub}},
	})
}

func seedUser(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, id, id+"@example.test", name, "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedCampaign makes a campaign owned by owner on a game system.
func seedCampaign(t *testing.T, db *sql.DB, owner, name, system string) string {
	t.Helper()
	id := uuid.NewString()
	settings, _ := json.Marshal(map[string]string{"system_id": system})
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by, settings) VALUES (?,?,?,?,?)`, id, name, id, owner, string(settings)); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?,?, 'owner')`, id, owner); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	return id
}

func TestDBLibraryAndShares(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	repo := NewRepository(db)
	alice, bob := seedUser(t, db, "Alice Quill"), seedUser(t, db, "Bob Marsh")
	a1 := seedCampaign(t, db, alice, "Ashgate", "drawsteel")
	b1 := seedCampaign(t, db, bob, "Bramble", "drawsteel")

	d := &Design{ID: uuid.NewString(), CampaignID: a1, Kind: KindRoomTheme, SystemID: "drawsteel", Name: "Ward", Note: "warm", MakerUserID: alice, MakerName: "Alice Quill", Data: json.RawMessage(`{"part":"oak"}`), Origin: OriginMade}
	if err := repo.Insert(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, d.ID)
	if err != nil || got == nil || got.Name != "Ward" || string(got.Data) != `{"part":"oak"}` || got.MakerUserID != alice || got.Shared {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if none, _ := repo.Get(ctx, uuid.NewString()); none != nil {
		t.Error("a missing id returned a design")
	}
	if l, _ := repo.List(ctx, b1, ""); len(l) != 0 {
		t.Errorf("another campaign's list = %+v", l)
	}
	if n, _ := repo.Count(ctx, a1); n != 1 {
		t.Errorf("count = %d", n)
	}

	t.Run("a link answers by its token, only while on", func(t *testing.T) {
		if d, _, _ := repo.FindByToken(ctx, "nope"); d != nil {
			t.Error("an unknown token found a design")
		}
		if d, _, _ := repo.FindByToken(ctx, ""); d != nil {
			t.Error("an empty token found a design")
		}
		if err := repo.SetLink(ctx, d.ID, "tok-1", true); err != nil {
			t.Fatal(err)
		}
		if f, on, err := repo.FindByToken(ctx, "tok-1"); err != nil || f == nil || !on || f.ID != d.ID || !f.Shared {
			t.Fatalf("on = %+v %v %v", f, on, err)
		}
		if err := repo.SetLink(ctx, d.ID, "", false); err != nil {
			t.Fatal(err)
		}
		if f, _, _ := repo.FindByToken(ctx, "tok-1"); f != nil {
			t.Error("a link switched off still resolved")
		}
		if err := repo.SetLink(ctx, d.ID, "tok-1", true); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("named owners are what Shared with me lists", func(t *testing.T) {
		if err := repo.AddOwner(ctx, d.ID, bob); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddOwner(ctx, d.ID, bob); err != nil {
			t.Fatalf("adding twice should be harmless: %v", err)
		}
		if ok, _ := repo.IsNamedOwner(ctx, d.ID, bob); !ok {
			t.Error("bob is not named")
		}
		if ok, _ := repo.IsNamedOwner(ctx, d.ID, alice); ok {
			t.Error("alice is named on her own design")
		}
		if l, _ := repo.SharedWith(ctx, bob, "drawsteel", KindRoomTheme, b1); len(l) != 1 || l[0].ID != d.ID || l[0].FromCampaign != "Ashgate" {
			t.Errorf("shared with bob = %+v", l)
		}
		if l, _ := repo.SharedWith(ctx, bob, "dnd5e", "", b1); len(l) != 0 {
			t.Errorf("another system listed %+v", l)
		}
		if l, _ := repo.SharedWith(ctx, bob, "drawsteel", "", a1); len(l) != 0 {
			t.Errorf("a design is not listed for its own campaign: %+v", l)
		}
		if _, _, owners, _ := repo.Share(ctx, d.ID); len(owners) != 1 || owners[0].Name != "Bob Marsh" {
			t.Errorf("owners = %+v", owners)
		}
	})

	t.Run("search finds owners by display name and gives no campaign ids", func(t *testing.T) {
		hits, err := repo.SearchOwners(ctx, "marsh", alice, 10)
		if err != nil || len(hits) != 1 || hits[0].DisplayName != "Bob Marsh" || len(hits[0].Campaigns) != 1 || hits[0].Campaigns[0].Name != "Bramble" || hits[0].Campaigns[0].SystemID != "drawsteel" {
			t.Fatalf("hits = %+v, %v", hits, err)
		}
		raw, _ := json.Marshal(hits)
		if string(raw) != `[{"userId":"`+bob+`","displayName":"Bob Marsh","campaigns":[{"name":"Bramble","systemId":"drawsteel"}]}]` {
			t.Errorf("a hit carries more than a name and its campaigns: %s", raw)
		}
		if h, _ := repo.SearchOwners(ctx, "quill", alice, 10); len(h) != 0 {
			t.Errorf("the searcher found themselves: %+v", h)
		}
		if h, _ := repo.SearchOwners(ctx, "%", alice, 10); len(h) != 0 {
			t.Errorf("a wildcard matched everyone: %+v", h)
		}
		player := seedUser(t, db, "Marsh Player")
		if _, err := db.Exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?,?, 'player')`, a1, player); err != nil {
			t.Fatal(err)
		}
		if h, _ := repo.SearchOwners(ctx, "Marsh Player", alice, 10); len(h) != 0 {
			t.Errorf("a player is not an owner: %+v", h)
		}
	})

	t.Run("stopping a share clears the link and the named owners together", func(t *testing.T) {
		if err := repo.StopSharing(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
		tok, on, owners, _ := repo.Share(ctx, d.ID)
		if tok != "" || on || len(owners) != 0 {
			t.Errorf("share = %q %v %+v", tok, on, owners)
		}
		if g, _ := repo.Get(ctx, d.ID); g.Shared {
			t.Error("still reads as shared")
		}
	})

	t.Run("deleting a design takes its share state; a campaign takes its designs", func(t *testing.T) {
		_ = repo.SetLink(ctx, d.ID, "tok-2", true)
		_ = repo.AddOwner(ctx, d.ID, bob)
		if err := repo.Delete(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = db.QueryRow(`SELECT (SELECT COUNT(*) FROM design_shares) + (SELECT COUNT(*) FROM design_share_owners)`).Scan(&n)
		if n != 0 {
			t.Errorf("%d share rows left", n)
		}
		e := &Design{ID: uuid.NewString(), CampaignID: b1, Kind: KindRoomTheme, SystemID: "drawsteel", Name: "x", Data: json.RawMessage(`{}`), Origin: OriginCode}
		if err := repo.Insert(ctx, e); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, b1); err != nil {
			t.Fatal(err)
		}
		if g, _ := repo.Get(ctx, e.ID); g != nil {
			t.Error("a deleted campaign left its design")
		}
	})
}
