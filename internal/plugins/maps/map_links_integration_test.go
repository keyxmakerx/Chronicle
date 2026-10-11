// map_links_integration_test.go proves on real MariaDB that the tree of linked
// maps is built from what each viewer may see: the SQL visibility predicate,
// per-player rules, and the fog all apply to links exactly as to pins. It also
// pins the column itself: round-trip, deleting a map turns linking pins back
// into plain pins, and a link that names another campaign's map reads as none.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/maps/... -run Links_Integration
package maps

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

type linkFixture struct {
	campaign, other                                   string
	world, isle, port, vault, barrow, cellar, foreign string
}

func TestLinks_TreeFiltersPerViewer_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewMapRepository(db)
	svc := NewMapService(repo)

	owner := newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, owner+"@example.test", "Links Owner", "x")
	f := linkFixture{campaign: newMapsDBID(t), other: newMapsDBID(t)}
	for _, c := range []string{f.campaign, f.other} {
		mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Links", c, owner)
	}
	mk := func(campaign, name string) string {
		m, err := svc.CreateMap(ctx, CreateMapInput{CampaignID: campaign, Name: name})
		if err != nil {
			t.Fatalf("CreateMap %s: %v", name, err)
		}
		return m.ID
	}
	f.world, f.isle, f.port = mk(f.campaign, "World"), mk(f.campaign, "Isle"), mk(f.campaign, "Port")
	f.vault, f.barrow, f.cellar = mk(f.campaign, "Vault"), mk(f.campaign, "Barrow"), mk(f.campaign, "Cellar")
	f.foreign = mk(f.other, "Elsewhere")

	_, _, lx, ly := fogPositions()
	dx, dy, _, _ := fogPositions()
	pin := func(on, name, to, vis string, rules *string, x, y float64) *Marker {
		m, err := svc.CreateMarker(ctx, CreateMarkerInput{
			MapID: on, Name: name, X: x, Y: y, LinkedMapID: &to,
			Visibility: vis, VisibilityRules: rules, CreatedBy: owner,
		})
		if err != nil {
			t.Fatalf("CreateMarker %s: %v", name, err)
		}
		return m
	}
	pin(f.world, "To the isle", f.isle, "everyone", nil, lx, ly)
	pin(f.isle, "To the port", f.port, "everyone", nil, lx, ly)
	pin(f.port, "Back to the world", f.world, "everyone", nil, lx, ly)
	pin(f.world, "To the vault", f.vault, "dm_only", nil, lx, ly)
	pin(f.world, "To the barrow", f.barrow, "everyone", strPtrMK(`{"allowed_users":["u-mira"]}`), lx, ly)
	pin(f.world, "To the cellar", f.cellar, "everyone", nil, dx, dy)

	// The fog covers the same hexes on every map in this fixture; only the
	// cellar pin sits in unexplored land. Children follow the pins' own order
	// (by name). Where a viewer's links form a pure loop (World, Isle, Port),
	// the loop starts at the map with the most links out, then the campaign's
	// order, so the players' tree starts at Isle.
	svc.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})

	names := map[string]string{f.world: "World", f.isle: "Isle", f.port: "Port", f.vault: "Vault", f.barrow: "Barrow", f.cellar: "Cellar"}
	shape := func(nodes []LinkNode) string {
		var walk func([]LinkNode) string
		walk = func(ns []LinkNode) string {
			out := ""
			for i, n := range ns {
				if i > 0 {
					out += ","
				}
				out += names[n.ID]
				if len(n.Children) > 0 {
					out += "(" + walk(n.Children) + ")"
				}
			}
			return out
		}
		return walk(nodes)
	}

	player := int(permissions.RolePlayer)
	cases := []struct {
		name   string
		role   int
		userID string
		want   string
	}{
		{"owner sees every link, the cycle placed once", int(permissions.RoleOwner), owner, "World(Barrow,Cellar,Isle(Port),Vault)"},
		{"a named player follows the rules link, not the DM-only or fogged one", player, "u-mira", "World(Barrow,Isle(Port))"},
		{"another player sees neither", player, "u-kael", "Isle(Port(World))"},
		{"an anonymous viewer sees the public chain only", int(permissions.RoleNone), "", "Isle(Port(World))"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := svc.LinkTree(ctx, f.campaign, tc.role, tc.userID)
			if err != nil {
				t.Fatalf("LinkTree: %v", err)
			}
			if got := shape(tree.Roots); got != tc.want {
				t.Errorf("tree = %s, want %s", got, tc.want)
			}
			if tree.Truncated {
				t.Error("this small tree must not be truncated")
			}
		})
	}

	// The marker list itself: a player's World pins carry only the links the
	// tree showed them, with the target's name.
	ms, err := svc.ListMarkers(ctx, f.campaign, f.world, player, "u-mira")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.LinkedMapID == nil {
			continue
		}
		switch *m.LinkedMapID {
		case f.isle, f.barrow:
			if m.LinkedMapName == "" {
				t.Errorf("pin %q lost its map name", m.Name)
			}
		default:
			t.Errorf("player was sent a link to %s on pin %q", names[*m.LinkedMapID], m.Name)
		}
	}

	// A link may never name another campaign's map, through either write.
	if _, err := svc.CreateMarker(ctx, CreateMarkerInput{MapID: f.world, Name: "x", LinkedMapID: &f.foreign}); err == nil {
		t.Error("a link to another campaign's map was accepted on create")
	}
	plain, err := svc.CreateMarker(ctx, CreateMarkerInput{MapID: f.world, Name: "Plain", X: lx, Y: ly, CreatedBy: owner})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateMarker(ctx, plain.ID, UpdateMarkerInput{LinkedMapID: patch.Of(f.foreign)}, true); err == nil {
		t.Error("a link to another campaign's map was accepted on update")
	}

	// A row written around the service (a hand edit, an old import) that names
	// another campaign's map reads back as no link and no name.
	mustExecMaps(t, db, `UPDATE map_markers SET linked_map_id = ? WHERE id = ?`, f.foreign, plain.ID)
	got, err := repo.GetMarker(ctx, plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LinkedMapID != nil || got.LinkedMapName != "" {
		t.Errorf("a cross-campaign link read back as %v %q, want none", got.LinkedMapID, got.LinkedMapName)
	}

	// Deleting a map leaves the pins that opened it as plain pins.
	if err := svc.DeleteMap(ctx, f.barrow, nil); err != nil {
		t.Fatal(err)
	}
	ownerPins, err := svc.ListMarkers(ctx, f.campaign, f.world, int(permissions.RoleOwner), owner)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range ownerPins {
		if m.Name == "To the barrow" {
			found = true
			if m.LinkedMapID != nil {
				t.Errorf("the pin still opens the deleted map %s", *m.LinkedMapID)
			}
		}
	}
	if !found {
		t.Error("deleting the target map removed the pin that opened it")
	}
}
