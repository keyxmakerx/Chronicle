package syncapi

// Replace swaps a campaign's whole scene list and its report row; Get tells
// a world that never reported from one with no scenes. Skips when no server
// is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestFoundryScenesIntegration

import (
	"context"
	"testing"
	"time"
)

func TestFoundryScenesIntegration_Replace(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid := "u-fs-0000000000000000000000000000001", "c-fs-0000000000000000000000000000001"
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, "fs@example.test", "Ren", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Scenes", "scenes", uid); err != nil {
		t.Fatal(err)
	}
	repo := NewFoundrySceneRepository(db)
	if got, err := repo.Get(ctx, cid); err != nil || got != nil {
		t.Fatalf("never reported: %+v %v", got, err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	first := FoundrySceneReport{ReportedAt: now.Add(-time.Hour), Scenes: []FoundryScene{
		{SceneID: "s2", Name: "Sunken Crypt", Folder: "Act 2"},
		{SceneID: "s1", Name: "Dragon's Lair", Active: true, InNavigation: true},
	}}
	if err := repo.Replace(ctx, cid, first); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	// Foundry's order is kept, not sorted by name.
	if len(got.Scenes) != 2 || got.Scenes[0].Name != "Sunken Crypt" || got.Scenes[0].Folder != "Act 2" ||
		got.Scenes[1].Name != "Dragon's Lair" || !got.Scenes[1].Active || !got.Scenes[1].InNavigation || got.Truncated {
		t.Fatalf("first report: %+v", got)
	}
	if err := repo.Replace(ctx, cid, FoundrySceneReport{ReportedAt: now, Truncated: true, Scenes: []FoundryScene{}}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Scenes) != 0 || !got.Truncated || !got.ReportedAt.Equal(now) {
		t.Fatalf("empty report: %+v", got)
	}
}
