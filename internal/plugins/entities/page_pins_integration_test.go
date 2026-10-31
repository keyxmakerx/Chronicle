package entities

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/pinschema"
)

// openPinsDB is a scratch schema with the core migrations and the page_pins
// migration applied. Strict: a migration that does not apply fails here.
func openPinsDB(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(pinschema.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_pins_",
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: pinschema.PluginSlug, FS: sub}},
	})
}

// TestPagePins_Integration runs the pins repository and service over real
// tables: pin order, members' own pins read from their stars, the visibility
// filter that keeps a hidden page from a player, the Trash and deletion.
func TestPagePins_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openPinsDB(t)
	ctx := context.Background()

	owner, player, camp := testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{owner, player} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "pin-"+u+"@example.test", "Pins Test", "x")
	}
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Pins", "pin-"+camp[:8], owner)

	types := NewEntityTypeRepository(db)
	typ := &EntityType{CampaignID: camp, Slug: "place", Name: "Place", NamePlural: "Places", Icon: "fa-map", Color: "#112233", Enabled: true}
	mustCreate(t, types, ctx, typ)
	page := func(name string, private bool) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, search_text, created_by, is_private, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typ.ID, name, "p-"+id[:8], name, owner, private)
		return id
	}
	rules, port, secret, bell := page("House Rules", false), page("Port Rell", false), page("The Duke's Ledger", true), page("The Drowned Bell", false)

	repo := NewPagePinRepository(db)
	svc := NewPagePinService(repo, NewEntityRepository(db), true)
	dm := PinViewer{CampaignID: camp, UserID: owner, Role: int(permissions.RoleOwner), Director: true}
	tyne := PinViewer{CampaignID: camp, UserID: player, Role: int(permissions.RolePlayer)}

	for _, id := range []string{rules, port, secret} {
		if err := svc.SetShared(ctx, dm, id, true); err != nil {
			t.Fatalf("SetShared: %v", err)
		}
		// Pins made in the same millisecond keep their order by the time.
		mustExec(t, db, `UPDATE page_pins SET created_at = created_at - INTERVAL 1 MINUTE`)
	}
	if err := svc.SetShared(ctx, dm, rules, true); err != nil {
		t.Fatalf("pinning again: %v", err)
	}
	// Tyne starred two pages: one the Director also pinned, and one of their own.
	mustExec(t, db, `INSERT INTO entity_favorites (user_id, entity_id, campaign_id, created_at) VALUES (?, ?, ?, NOW() - INTERVAL 1 HOUR), (?, ?, ?, NOW())`,
		player, port, camp, player, bell, camp)

	names := func(ps []PinnedPage) string {
		s := make([]string, len(ps))
		for i, p := range ps {
			s[i] = p.Name
		}
		return strings.Join(s, ", ")
	}

	t.Run("the migration can run again without error", func(t *testing.T) {
		up, err := fs.ReadFile(pinschema.MigrationsFS, "migrations/001_page_pins.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(up), ";\n") {
			if strings.TrimSpace(stripSQLComments(stmt)) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("re-running the up migration: %v", err)
			}
		}
	})

	t.Run("the Director sees every pin, in the order pinned", func(t *testing.T) {
		got, err := svc.ForViewer(ctx, dm)
		if err != nil {
			t.Fatal(err)
		}
		if want := "House Rules, Port Rell, The Duke's Ledger"; names(got.Shared) != want {
			t.Errorf("shared = %q, want %q", names(got.Shared), want)
		}
		if got.Shared[0].TypeName != "Place" || got.Shared[0].TypeIcon != "fa-map" {
			t.Errorf("card lost its category: %+v", got.Shared[0])
		}
	})

	t.Run("a player never sees a hidden page, and their own pins come after", func(t *testing.T) {
		got, err := svc.ForViewer(ctx, tyne)
		if err != nil {
			t.Fatal(err)
		}
		if want := "House Rules, Port Rell"; names(got.Shared) != want {
			t.Errorf("shared = %q, want %q", names(got.Shared), want)
		}
		if want := "The Drowned Bell"; names(got.Own) != want {
			t.Errorf("own = %q, want %q (Port Rell is already above)", names(got.Own), want)
		}
	})

	t.Run("a page in the Trash drops off and comes back when restored", func(t *testing.T) {
		mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, rules)
		got, _ := svc.ForViewer(ctx, tyne)
		if strings.Contains(names(got.Shared), "House Rules") {
			t.Errorf("a trashed page still shows: %q", names(got.Shared))
		}
		if n, _ := repo.CountShared(ctx, camp); n != 2 {
			t.Errorf("count = %d, want 2 while one page is in the Trash", n)
		}
		mustExec(t, db, `UPDATE entities SET deleted_at = NULL WHERE id = ?`, rules)
		got, _ = svc.ForViewer(ctx, tyne)
		if !strings.Contains(names(got.Shared), "House Rules") {
			t.Errorf("the restored page didn't come back: %q", names(got.Shared))
		}
	})

	t.Run("unpinning and removing your own pin", func(t *testing.T) {
		if err := svc.SetShared(ctx, dm, port, false); err != nil {
			t.Fatal(err)
		}
		if ok, _ := repo.IsShared(ctx, camp, port); ok {
			t.Error("Port Rell is still pinned for everyone")
		}
		got, _ := svc.ForViewer(ctx, tyne)
		if want := "The Drowned Bell, Port Rell"; names(got.Own) != want {
			t.Errorf("own = %q, want %q once the Director unpinned it", names(got.Own), want)
		}
		if err := svc.RemoveOwn(ctx, tyne, bell); err != nil {
			t.Fatal(err)
		}
		got, _ = svc.ForViewer(ctx, tyne)
		if want := "Port Rell"; names(got.Own) != want {
			t.Errorf("own = %q, want %q", names(got.Own), want)
		}
	})

	t.Run("deleting a page removes its pin", func(t *testing.T) {
		mustExec(t, db, `DELETE FROM entities WHERE id = ?`, secret)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM page_pins WHERE entity_id = ?`, secret).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%d pins left for a deleted page", n)
		}
	})
}
