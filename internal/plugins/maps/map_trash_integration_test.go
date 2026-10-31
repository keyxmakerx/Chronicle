// map_trash_integration_test.go drives a map through the campaign Trash
// against a real MariaDB: a trashed map and its pins drop out of every map
// read, a pin on another map keeps its link to it, Restore brings it all
// back, and the purge deletes it with its pins by foreign key.
//
// Skips when no database answers; see newMapsScratchDB.
package maps

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestMapTrash_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewMapRepository(db)
	svc := NewMapService(repo)

	userID, camp, other := newMapsDBID(t), newMapsDBID(t), newMapsDBID(t)
	cellar, town := newMapsDBID(t), newMapsDBID(t)
	pinInCellar, pinToCellar := newMapsDBID(t), newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Map Trash Test", "x")
	for _, c := range []string{camp, other} {
		mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Map Trash", c, userID)
	}
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?), (?, ?, ?)`,
		cellar, camp, "Cellar", town, camp, "Town")
	mustExecMaps(t, db, `INSERT INTO map_markers (id, map_id, name, visibility) VALUES (?, ?, 'Barrel', 'everyone')`, pinInCellar, cellar)
	mustExecMaps(t, db, `INSERT INTO map_markers (id, map_id, name, visibility, linked_map_id) VALUES (?, ?, 'Stairs', 'everyone', ?)`, pinToCellar, town, cellar)
	owner := int(permissions.RoleOwner)

	if err := repo.TrashMap(ctx, other, cellar, time.Now().UTC()); err == nil {
		t.Fatal("another campaign trashed the map")
	}
	if err := svc.DeleteMap(ctx, cellar, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetMap(ctx, cellar); err == nil {
		t.Error("a trashed map still opens")
	}
	if list, _ := repo.ListMaps(ctx, camp); len(list) != 1 || list[0].ID != town {
		t.Errorf("maps list = %v, want only Town", list)
	}
	if found, _ := repo.SearchMaps(ctx, camp, "Cell"); len(found) != 0 {
		t.Errorf("search found the trashed map: %v", found)
	}
	if _, err := svc.GetMarker(ctx, pinInCellar); err == nil {
		t.Error("a trashed map's pin still opens")
	}
	pins, err := repo.ListMarkers(ctx, town, owner, userID)
	if err != nil || len(pins) != 1 || pins[0].LinkedMapID == nil || *pins[0].LinkedMapID != cellar {
		t.Errorf("the Town pin lost its link to the trashed map: %v %v", pins, err)
	}
	trashed, err := svc.ListTrashedMaps(ctx, camp)
	if err != nil || len(trashed) != 1 || trashed[0].ID != cellar || trashed[0].Name != "Cellar" {
		t.Fatalf("Trash lists %v (%v), want Cellar", trashed, err)
	}
	if list, _ := svc.ListTrashedMaps(ctx, other); len(list) != 0 {
		t.Errorf("another campaign's Trash lists the map: %v", list)
	}

	if err := svc.RestoreMap(ctx, other, cellar); err == nil {
		t.Error("another campaign restored the map")
	}
	if err := svc.RestoreMap(ctx, camp, cellar); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetMarker(ctx, pinInCellar); err != nil {
		t.Errorf("restored map's pin is missing: %v", err)
	}

	if err := svc.DeleteMap(ctx, cellar, nil); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.PurgeTrashedMaps(ctx, time.Now().UTC().Add(-time.Hour)); err != nil || n != 0 {
		t.Errorf("purged %d (%v) before the cutoff passed", n, err)
	}
	if n, err := svc.PurgeTrashedMaps(ctx, time.Now().UTC().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("purged %d (%v), want 1", n, err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM map_markers WHERE id = ?`, pinInCellar).Scan(&left); err != nil || left != 0 {
		t.Errorf("purged map's pin is still there (%d, %v)", left, err)
	}
	pins, _ = repo.ListMarkers(ctx, town, owner, userID)
	if len(pins) != 1 || pins[0].LinkedMapID != nil {
		t.Errorf("after the purge the Town pin should be a plain pin: %v", pins)
	}
}
