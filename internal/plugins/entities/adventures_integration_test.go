package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The Adventures category is closed to anyone below Scribe in every read the
// repository serves, whatever a page's own visibility or grants say. Each
// subtest runs the same read for a player and for the DM team against a real
// schema, because the guard is one SQL predicate and only the database proves it.
func TestAdventuresVisibility_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "adv-"+userID+"@example.test", "Adv Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Adv", "adv-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	types := NewEntityTypeRepository(db)
	svc := NewEntityService(NewEntityRepository(db), types, NewEntityPermissionRepository(db)).(*entityService)
	adv, made, err := svc.EnsureAdventures(ctx, camp)
	if err != nil || !made {
		t.Fatalf("EnsureAdventures: made=%v err=%v", made, err)
	}
	npc := &EntityType{CampaignID: camp, Slug: "npc", Name: "NPC", NamePlural: "NPCs", Icon: "fa-user", Color: "#112233", Enabled: true}
	mustCreate(t, types, ctx, npc)

	page := func(typ int, name string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, search_text, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typ, name, "p-"+id[:8], name, userID)
		return id
	}
	barkeep := page(npc.ID, "Barkeep Mirelle")
	crypt := page(adv.ID, "Mirelle's Sunken Crypt")
	custom := page(adv.ID, "Mirelle's Secret Hook")
	// A grant that would let a player in anywhere else must not widen past the category.
	mustExec(t, db, `UPDATE entities SET visibility = 'custom' WHERE id = ?`, custom)
	mustExec(t, db, `INSERT INTO entity_permissions (entity_id, subject_type, subject_id, permission) VALUES (?, 'role', '1', 'view')`, custom)
	// A child of an ordinary page, itself an Adventures page.
	child := page(adv.ID, "Mirelle's Cellar")
	mustExec(t, db, `UPDATE entities SET parent_id = ? WHERE id = ?`, barkeep, child)

	repo := NewEntityRepository(db)
	ids := func(es []Entity) map[string]bool {
		m := map[string]bool{}
		for _, e := range es {
			m[e.ID] = true
		}
		return m
	}
	hidden := []string{crypt, custom, child}

	roles := []struct {
		name string
		role int
		sees bool
	}{
		{"anonymous", permissions.RoleNone, false},
		{"player", permissions.RolePlayer, false},
		{"scribe", permissions.RoleScribe, true},
		{"owner", permissions.RoleOwner, true},
	}
	for _, r := range roles {
		r := r
		check := func(t *testing.T, what string, got map[string]bool) {
			t.Helper()
			if !got[barkeep] && what != "children" && what != "viewable" && what != "ancestors" {
				t.Errorf("%s: the ordinary page must stay listed", what)
			}
			for _, id := range hidden {
				if got[id] && !r.sees {
					t.Errorf("%s: %s leaked an Adventures page %s to role %d", what, r.name, id, r.role)
				}
			}
			if r.sees && what != "ancestors" && what != "children" && !got[crypt] {
				t.Errorf("%s: %s should see the Adventures page", what, r.name)
			}
		}

		t.Run(r.name+"/list", func(t *testing.T) {
			es, total, err := repo.ListByCampaign(ctx, camp, nil, r.role, "u-"+r.name, DefaultListOptions())
			if err != nil {
				t.Fatal(err)
			}
			check(t, "list", ids(es))
			if !r.sees && total != 1 {
				t.Errorf("total = %d, a player must be counted only the ordinary page", total)
			}
		})
		t.Run(r.name+"/list filtered to the category", func(t *testing.T) {
			es, _, err := repo.ListByCampaign(ctx, camp, []int{adv.ID}, r.role, "u-"+r.name, DefaultListOptions())
			if err != nil {
				t.Fatal(err)
			}
			if !r.sees && len(es) != 0 {
				t.Errorf("a player filtering by the category got %d pages", len(es))
			}
			if r.sees && len(es) != 3 {
				t.Errorf("the DM team should see all 3, got %d", len(es))
			}
		})
		t.Run(r.name+"/search", func(t *testing.T) {
			es, _, err := repo.Search(ctx, camp, "Mirelle", nil, r.role, "u-"+r.name, DefaultListOptions())
			if err != nil {
				t.Fatal(err)
			}
			check(t, "search", ids(es))
		})
		t.Run(r.name+"/counts", func(t *testing.T) {
			counts, err := repo.CountByType(ctx, camp, r.role, "u-"+r.name)
			if err != nil {
				t.Fatal(err)
			}
			if got := counts[adv.ID]; r.sees && got != 3 || !r.sees && got != 0 {
				t.Errorf("Adventures count = %d for %s", got, r.name)
			}
		})
		t.Run(r.name+"/recent", func(t *testing.T) {
			es, err := repo.ListRecent(ctx, camp, r.role, "u-"+r.name, 20)
			if err != nil {
				t.Fatal(err)
			}
			check(t, "recent", ids(es))
		})
		t.Run(r.name+"/children", func(t *testing.T) {
			es, err := repo.FindChildren(ctx, barkeep, r.role, "u-"+r.name)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(es)[child]; got != r.sees {
				t.Errorf("child listed = %v, want %v", got, r.sees)
			}
		})
		t.Run(r.name+"/viewable ids", func(t *testing.T) {
			got, err := repo.FilterViewableEntityIDs(ctx, camp, append([]string{barkeep}, hidden...), r.role, "u-"+r.name)
			if err != nil {
				t.Fatal(err)
			}
			check(t, "viewable", got)
		})
		t.Run(r.name+"/mention names", func(t *testing.T) {
			names, err := repo.ListNames(ctx, camp, r.role, "u-"+r.name)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, n := range names {
				got[n.ID] = true
			}
			check(t, "names", got)
		})
	}

	// The service's single-page gate, over the real schema.
	for _, r := range roles {
		perm, err := svc.CheckEntityAccess(ctx, custom, r.role, "u-"+r.name)
		if err != nil {
			t.Fatal(err)
		}
		if perm.CanView != r.sees {
			t.Errorf("CheckEntityAccess(custom Adventures page) for %s = %v, want %v", r.name, perm.CanView, r.sees)
		}
	}

	// A page owned by someone, then moved into Adventures, is no longer theirs
	// to list: owner lists feed player-facing bands.
	mustExec(t, db, `UPDATE entities SET owner_user_id = ? WHERE id IN (?, ?)`, userID, barkeep, crypt)
	owned, err := repo.ListByOwner(ctx, camp, userID)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(owned); !got[barkeep] || got[crypt] {
		t.Errorf("ListByOwner = %v, want the ordinary page and not the Adventures one", got)
	}
}

// The seeded and the added category are the same shape, and pressing the
// button twice changes nothing.
func TestEnsureAdventures_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, seeded, existing := testUUID(t), testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "ens-"+userID+"@example.test", "Ensure Test", "x")
	for _, c := range []string{seeded, existing} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Ens", "ens-"+c[:8], userID)
	}
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, seeded, existing)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	types := NewEntityTypeRepository(db)
	svc := NewEntityService(NewEntityRepository(db), types, NewEntityPermissionRepository(db)).(*entityService)

	t.Run("a new campaign is seeded with it, and the button then does nothing", func(t *testing.T) {
		if err := types.SeedDefaults(ctx, seeded); err != nil {
			t.Fatal(err)
		}
		before, _ := types.ListByPresetCategory(ctx, seeded, PresetCategoryAdventures)
		if len(before) != 1 || !before[0].DMOnly() || before[0].Slug != SlugAdventures || len(before[0].Layout.Rows) != 2 {
			t.Fatalf("seeded = %+v", before)
		}
		et, made, err := svc.EnsureAdventures(ctx, seeded)
		if err != nil || made || et.ID != before[0].ID {
			t.Fatalf("button on a seeded campaign: made=%v err=%v", made, err)
		}
	})

	t.Run("an existing campaign gets it once", func(t *testing.T) {
		// A hand-made category already holds the slug.
		mustCreate(t, types, ctx, &EntityType{CampaignID: existing, Slug: SlugAdventures, Name: "Adventures", NamePlural: "Adventures", Icon: "fa-x", Color: "#112233", Enabled: true})
		first, made, err := svc.EnsureAdventures(ctx, existing)
		if err != nil || !made || !first.DMOnly() || first.Slug == SlugAdventures {
			t.Fatalf("first press: %+v made=%v err=%v", first, made, err)
		}
		second, made, err := svc.EnsureAdventures(ctx, existing)
		if err != nil || made || second.ID != first.ID {
			t.Fatalf("second press: %+v made=%v err=%v", second, made, err)
		}
		all, _ := types.ListByPresetCategory(ctx, existing, PresetCategoryAdventures)
		if len(all) != 1 {
			t.Errorf("want one Adventures category, got %d", len(all))
		}
	})
}
