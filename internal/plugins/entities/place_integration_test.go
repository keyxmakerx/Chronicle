package entities

import (
	"context"
	"database/sql"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// placesDB is a scratch campaign with the real repositories and services, so
// the SQL that decides visibility, the Trash and the cycle walk runs for real.
type placesDB struct {
	db         *sql.DB
	ctx        context.Context
	campaignID string
	otherCamp  string
	svc        EntityService
	places     PlaceService
	safety     PageSafetyService
	typeID     int
	owner      string
}

func newPlacesDB(t *testing.T) *placesDB {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()

	owner := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "own-"+owner+"@example.test", "Owner", "x")
	mk := func(name string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			id, name, "pl-"+id[:8], owner)
		return id
	}
	campaignID, otherCamp := mk("Places"), mk("Elsewhere")

	typeRepo := NewEntityTypeRepository(db)
	et := &EntityType{
		CampaignID: campaignID, Slug: "pages", Name: "Page", NamePlural: "Pages",
		Icon: "fa-file", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
		SortOrder: 1, Enabled: true,
	}
	mustCreate(t, typeRepo, ctx, et)
	et2 := &EntityType{
		CampaignID: otherCamp, Slug: "pages", Name: "Page", NamePlural: "Pages",
		Icon: "fa-file", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
		SortOrder: 1, Enabled: true,
	}
	mustCreate(t, typeRepo, ctx, et2)

	entityRepo := NewEntityRepository(db)
	safetyRepo := NewPageSafetyRepository(db)
	svc := NewEntityService(entityRepo, typeRepo, NewEntityPermissionRepository(db))
	svc.SetPageSafety(safetyRepo)
	places := NewPlaceService(entityRepo, NewPlaceRepository(db))
	svc.(*entityService).SetPlaceGuard(places)

	return &placesDB{
		db: db, ctx: WithActor(ctx, owner), campaignID: campaignID, otherCamp: otherCamp,
		svc: svc, places: places, safety: NewPageSafetyService(safetyRepo, entityRepo, svc, nil),
		typeID: et.ID, owner: owner,
	}
}

func (p *placesDB) page(t *testing.T, name, parent string, private bool) *Entity {
	t.Helper()
	e, err := p.svc.Create(p.ctx, p.campaignID, p.owner, CreateEntityInput{
		Name: name, EntityTypeID: p.typeID, ParentID: parent, IsPrivate: private,
	})
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	return e
}

func names(links []PlaceLink) []string {
	var out []string
	for _, l := range links {
		out = append(out, l.EntityName+"@"+l.ParentName)
	}
	return out
}

// TestPlaces_Integration_TreeVisibility: an extra listing never widens who can
// see a page. A private page listed under a public parent is absent for a
// player, a public page listed under a private parent is absent for a player,
// and the owner sees both.
func TestPlaces_Integration_TreeVisibility(t *testing.T) {
	p := newPlacesDB(t)
	player := testUUID(t)

	city := p.page(t, "City", "", false)
	vault := p.page(t, "Vault", "", true)       // private parent
	secret := p.page(t, "Secret NPC", "", true) // private page
	cook := p.page(t, "Cook", "", false)        // public page

	for _, l := range []struct{ e, par string }{
		{secret.ID, city.ID}, // private page under a public parent
		{cook.ID, vault.ID},  // public page under a private parent
		{cook.ID, city.ID},   // public page under a public parent
	} {
		if err := p.places.AddPlace(p.ctx, p.campaignID, l.e, l.par, p.owner); err != nil {
			t.Fatalf("AddPlace: %v", err)
		}
	}

	t.Run("player sees only the public page under the public parent", func(t *testing.T) {
		under, err := p.places.PlacesUnder(p.ctx, p.campaignID, []string{city.ID, vault.ID}, permissions.RolePlayer, player)
		if err != nil {
			t.Fatal(err)
		}
		got := names(under)
		if len(got) != 1 || got[0] != "Cook@City" {
			t.Fatalf("player sees %v, want only Cook@City", got)
		}
	})
	t.Run("a player who cannot see the page never sees its extra line", func(t *testing.T) {
		of, err := p.places.PlacesOf(p.ctx, p.campaignID, secret.ID, permissions.RolePlayer, player)
		if err != nil || len(of) != 0 {
			t.Fatalf("PlacesOf(secret) = %v, %v; want nothing", names(of), err)
		}
	})
	t.Run("a player who can see the page but not the extra parent does not see that branch", func(t *testing.T) {
		of, err := p.places.PlacesOf(p.ctx, p.campaignID, cook.ID, permissions.RolePlayer, player)
		if err != nil {
			t.Fatal(err)
		}
		got := names(of)
		if len(got) != 1 || got[0] != "Cook@City" {
			t.Fatalf("player sees %v for Cook, want only Cook@City", got)
		}
	})
	t.Run("the owner sees every listing", func(t *testing.T) {
		under, err := p.places.PlacesUnder(p.ctx, p.campaignID, []string{city.ID, vault.ID}, permissions.RoleOwner, p.owner)
		if err != nil || len(under) != 3 {
			t.Fatalf("owner sees %v (%v), want 3", names(under), err)
		}
	})
}

// TestPlaces_Integration_TrashHidesAndRestoreBringsBack: a page in the Trash,
// or a trashed parent, hides the listing; Restore brings it back with no
// bookkeeping of its own.
func TestPlaces_Integration_TrashHidesAndRestoreBringsBack(t *testing.T) {
	p := newPlacesDB(t)
	city := p.page(t, "City", "", false)
	guild := p.page(t, "Guild", "", false)
	cook := p.page(t, "Cook", "", false)
	if err := p.places.AddPlace(p.ctx, p.campaignID, cook.ID, city.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	if err := p.places.AddPlace(p.ctx, p.campaignID, cook.ID, guild.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		under, err := p.places.PlacesUnder(p.ctx, p.campaignID, []string{city.ID, guild.ID}, permissions.RoleOwner, p.owner)
		if err != nil {
			t.Fatal(err)
		}
		return len(under)
	}
	if count() != 2 {
		t.Fatalf("setup: want 2 listings, got %d", count())
	}

	// Trash the page: both listings hide.
	if err := p.svc.Delete(p.ctx, cook.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 0 {
		t.Fatalf("a trashed page keeps %d listings visible", n)
	}
	if _, err := p.safety.RestoreFromTrash(p.ctx, p.campaignID, cook.ID); err != nil {
		t.Fatalf("restore page: %v", err)
	}
	if n := count(); n != 2 {
		t.Fatalf("after Restore want 2 listings, got %d", n)
	}

	// Trash one parent: only its branch goes.
	if err := p.svc.Delete(p.ctx, guild.ID); err != nil {
		t.Fatal(err)
	}
	under, _ := p.places.PlacesUnder(p.ctx, p.campaignID, []string{city.ID, guild.ID}, permissions.RoleOwner, p.owner)
	if got := names(under); len(got) != 1 || got[0] != "Cook@City" {
		t.Fatalf("with Guild trashed want Cook@City only, got %v", got)
	}
	// The page's own line also leaves out the trashed parent.
	of, _ := p.places.PlacesOf(p.ctx, p.campaignID, cook.ID, permissions.RoleOwner, p.owner)
	if len(of) != 1 {
		t.Fatalf("PlacesOf with a trashed parent = %v", names(of))
	}
	// A trashed parent can't take a new listing.
	other := p.page(t, "Other", "", false)
	if err := p.places.AddPlace(p.ctx, p.campaignID, other.ID, guild.ID, p.owner); err == nil {
		t.Fatal("listing a page under a trashed parent should be refused")
	}
	if _, err := p.safety.RestoreFromTrash(p.ctx, p.campaignID, guild.ID); err != nil {
		t.Fatalf("restore parent: %v", err)
	}
	if n := count(); n != 2 {
		t.Fatalf("after restoring the parent want 2 listings, got %d", n)
	}
}

// TestPlaces_Integration_CyclesAndCampaigns runs the refusal rules against the
// real recursive walk, including a loop through an extra listing.
func TestPlaces_Integration_CyclesAndCampaigns(t *testing.T) {
	p := newPlacesDB(t)
	root := p.page(t, "Root", "", false)
	mid := p.page(t, "Mid", root.ID, false)
	leaf := p.page(t, "Leaf", mid.ID, false)
	side := p.page(t, "Side", "", false)

	foreign, err := p.svc.Create(p.ctx, p.otherCamp, p.owner, CreateEntityInput{Name: "Foreign", EntityTypeID: p.foreignType(t)})
	if err != nil {
		t.Fatal(err)
	}

	add := func(e, par string) error { return p.places.AddPlace(p.ctx, p.campaignID, e, par, p.owner) }
	refused := []struct {
		name   string
		e, par string
	}{
		{"itself", leaf.ID, leaf.ID},
		{"its real parent", leaf.ID, mid.ID},
		{"a descendant", root.ID, leaf.ID},
		{"a foreign parent", leaf.ID, foreign.ID},
		{"a foreign page", foreign.ID, root.ID},
	}
	for _, tc := range refused {
		if err := add(tc.e, tc.par); err == nil {
			t.Errorf("listing under %s should be refused", tc.name)
		}
	}

	// Side is listed under Leaf; Root under Side would now loop (Root -> Mid
	// -> Leaf -> Side), a cycle only the listing makes.
	if err := add(side.ID, leaf.ID); err != nil {
		t.Fatalf("Side under Leaf: %v", err)
	}
	if err := add(root.ID, side.ID); err == nil {
		t.Error("a loop through an extra listing should be refused")
	}
	// The same rule guards a real move.
	if err := p.svc.ReorderEntity(p.ctx, p.campaignID, root.ID, &side.ID, nil, 0); err == nil {
		t.Error("a real move that loops through a listing should be refused")
	}
	if _, err := p.svc.Update(p.ctx, root.ID, UpdateEntityInput{ParentID: patch.Of(side.ID)}); err == nil {
		t.Error("a metadata parent change that loops through a listing should be refused")
	}

	// A cross-campaign row inserted behind the service's back never shows.
	mustExec(t, p.db, `INSERT INTO entity_places (entity_id, parent_entity_id, campaign_id) VALUES (?, ?, ?)`,
		foreign.ID, root.ID, p.campaignID)
	under, _ := p.places.PlacesUnder(p.ctx, p.campaignID, []string{root.ID}, permissions.RoleOwner, p.owner)
	if len(under) != 0 {
		t.Errorf("a cross-campaign row leaked into the tree: %v", names(under))
	}
}

func (p *placesDB) foreignType(t *testing.T) int {
	t.Helper()
	var id int
	if err := p.db.QueryRow(`SELECT id FROM entity_types WHERE campaign_id = ?`, p.otherCamp).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPlaces_Integration_RealMoveAndPurge: moving a page for real under a
// parent it was also listed under drops that now-redundant listing, removing a
// listing leaves the page alone, and purging a page removes its listings.
func TestPlaces_Integration_RealMoveAndPurge(t *testing.T) {
	p := newPlacesDB(t)
	a := p.page(t, "A", "", false)
	b := p.page(t, "B", "", false)
	if err := p.places.AddPlace(p.ctx, p.campaignID, b.ID, a.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	// Adding twice is a no-op, not a duplicate row.
	if err := p.places.AddPlace(p.ctx, p.campaignID, b.ID, a.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM entity_places`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}

	if err := p.svc.ReorderEntity(p.ctx, p.campaignID, b.ID, &a.ID, nil, 0); err != nil {
		t.Fatalf("real move: %v", err)
	}
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM entity_places`).Scan(&n)
	if n != 0 {
		t.Fatalf("the listing under B's new real parent should be gone, rows = %d", n)
	}

	c := p.page(t, "C", "", false)
	if err := p.places.AddPlace(p.ctx, p.campaignID, c.ID, a.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	if err := p.places.RemovePlace(p.ctx, p.campaignID, c.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := p.svc.GetByID(p.ctx, c.ID); err != nil || got == nil {
		t.Fatalf("removing a listing must leave the page: %v", err)
	}

	// Purge: hard-deleting a page takes its listings with it (cascade).
	if err := p.places.AddPlace(p.ctx, p.campaignID, c.ID, a.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.db, `DELETE FROM entities WHERE id = ?`, c.ID)
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM entity_places`).Scan(&n)
	if n != 0 {
		t.Fatalf("purging a page should remove its listings, rows = %d", n)
	}
}

// TestPlaces_Integration_ExportAndOwnRealParent covers the export read (trashed
// ends included) and the repository's guard against drawing a page twice under
// its own real parent.
func TestPlaces_Integration_ExportAndOwnRealParent(t *testing.T) {
	p := newPlacesDB(t)
	a := p.page(t, "A", "", false)
	b := p.page(t, "B", "", false)
	if err := p.places.AddPlace(p.ctx, p.campaignID, b.ID, a.ID, p.owner); err != nil {
		t.Fatal(err)
	}
	if err := p.svc.Delete(p.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	all, err := p.places.ExportPlaces(p.ctx, p.campaignID)
	if err != nil || len(all) != 1 {
		t.Fatalf("export should keep a listing whose parent is in the Trash: %v, %v", all, err)
	}

	// A second writer makes A the real parent of B without the service: the
	// stale listing is not drawn a second time under A.
	if _, err := p.safety.RestoreFromTrash(p.ctx, p.campaignID, a.ID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.db, `UPDATE entities SET parent_id = ? WHERE id = ?`, a.ID, b.ID)
	under, _ := p.places.PlacesUnder(p.ctx, p.campaignID, []string{a.ID}, permissions.RoleOwner, p.owner)
	if len(under) != 0 {
		t.Fatalf("a page must not be drawn twice under its own real parent: %v", names(under))
	}
}
