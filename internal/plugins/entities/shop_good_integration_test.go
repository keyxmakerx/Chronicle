package entities

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// A shop good is a real page kept out of every list and search, for every
// viewer, until the Director makes it a full page. Each read below runs for
// the owner and a player, since the mark must hide it from both: it is not a
// visibility.
func TestShopGoods_KeptOutOfLists_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, player, camp := testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{userID, player} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, u, "good-"+u+"@example.test", "Good", "x")
	}
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Goods", "goods-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, userID, player)
	}()
	res, err := db.ExecContext(ctx, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'item', 'Item', 'Items')`, camp)
	if err != nil {
		t.Fatal(err)
	}
	typeID, _ := res.LastInsertId()

	page := func(name string, shopGood, private bool) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, shop_good, is_private, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typeID, name, "g-"+id[:8], shopGood, private, userID)
		return id
	}
	page("Longsword", false, false)
	crowbar := page("Crowbar", true, false)
	page("Waterskin", true, true)

	repo := NewEntityRepository(db)
	roles := []struct {
		name string
		role int
		user string
	}{{"owner", int(permissions.RoleOwner), userID}, {"player", int(permissions.RolePlayer), player}}

	names := func(es []Entity) map[string]bool {
		m := map[string]bool{}
		for _, e := range es {
			m[e.Name] = true
		}
		return m
	}
	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			list, total, err := repo.ListByCampaign(ctx, camp, nil, r.role, r.user, ListOptions{Page: 1, PerPage: 50})
			if err != nil {
				t.Fatal(err)
			}
			if n := names(list); !n["Longsword"] || n["Crowbar"] || n["Waterskin"] || total != 1 {
				t.Errorf("ListByCampaign: %v total %d", n, total)
			}
			found, ftotal, err := repo.Search(ctx, camp, "Crowbar", nil, r.role, r.user, ListOptions{Page: 1, PerPage: 50})
			if err != nil || len(found) != 0 || ftotal != 0 {
				t.Errorf("Search leaked a shop good: %v %d %v", names(found), ftotal, err)
			}
			byContent, _, err := repo.Search(ctx, camp, "Longsword", nil, r.role, r.user, ListOptions{Page: 1, PerPage: 50})
			if err != nil || len(byContent) != 1 {
				t.Errorf("Search for an ordinary page: %v %v", names(byContent), err)
			}
			counts, err := repo.CountByType(ctx, camp, r.role, r.user)
			if err != nil || counts[int(typeID)] != 1 {
				t.Errorf("CountByType = %v %v, want 1", counts, err)
			}
			recent, err := repo.ListRecent(ctx, camp, r.role, r.user, 20)
			if err != nil || len(recent) != 1 || recent[0].Name != "Longsword" {
				t.Errorf("ListRecent: %v %v", names(recent), err)
			}
			ln, err := repo.ListNames(ctx, camp, r.role, r.user)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range ln {
				if e.Name == "Crowbar" || e.Name == "Waterskin" {
					t.Errorf("Auto-link names include shop good %q", e.Name)
				}
			}
		})
	}

	t.Run("pages since", func(t *testing.T) {
		got, err := NewPagesSinceReader(db).Since(ctx, camp, time.Now().UTC().Add(-time.Hour), int(permissions.RoleOwner), userID, 5)
		if err != nil || got.Count != 1 {
			t.Errorf("Since = %+v %v, want one page", got, err)
		}
	})

	t.Run("modes", func(t *testing.T) {
		only, _, err := repo.ListByCampaign(ctx, camp, nil, int(permissions.RoleOwner), userID, ListOptions{Page: 1, PerPage: 50, ShopGoods: ShopGoodsOnly})
		if err != nil || len(only) != 2 || names(only)["Longsword"] {
			t.Errorf("ShopGoodsOnly: %v %v", names(only), err)
		}
		all, _, err := repo.ListByCampaign(ctx, camp, nil, int(permissions.RoleOwner), userID, ListOptions{Page: 1, PerPage: 50, ShopGoods: ShopGoodsInclude})
		if err != nil || len(all) != 3 {
			t.Errorf("ShopGoodsInclude (export): %v %v", names(all), err)
		}
	})

	t.Run("a player can still open a shop good by id", func(t *testing.T) {
		e, err := repo.FindByID(ctx, crowbar)
		if err != nil || !e.ShopGood {
			t.Fatalf("FindByID: %+v %v", e, err)
		}
		view, err := repo.FilterViewableEntityIDs(ctx, camp, []string{crowbar}, int(permissions.RolePlayer), player)
		if err != nil || !view[crowbar] {
			t.Errorf("a player must still see the page the shop sells: %v %v", view, err)
		}
	})

	t.Run("same name finds one page, only among shop goods", func(t *testing.T) {
		got, err := repo.FindShopGoodByName(ctx, camp, "CROWBAR")
		if err != nil || got == nil || got.ID != crowbar {
			t.Fatalf("FindShopGoodByName = %+v %v", got, err)
		}
		if none, _ := repo.FindShopGoodByName(ctx, camp, "Longsword"); none != nil {
			t.Errorf("an ordinary page must never be taken over as a good")
		}
	})

	t.Run("make it a full page", func(t *testing.T) {
		if err := repo.SetShopGood(ctx, camp, crowbar, false); err != nil {
			t.Fatal(err)
		}
		list, _, _ := repo.ListByCampaign(ctx, camp, nil, int(permissions.RolePlayer), player, ListOptions{Page: 1, PerPage: 50})
		if !names(list)["Crowbar"] {
			t.Errorf("a promoted good should list: %v", names(list))
		}
		if err := repo.SetShopGood(ctx, testUUID(t), crowbar, true); err == nil {
			t.Errorf("another campaign must not change the mark")
		}
	})
}
