package entities

import (
	"context"
	"reflect"
	"testing"
)

// Leaving a campaign frees the leaver's characters in that campaign only:
// each loses its owner and is marked playable (so it lands in "Free to
// pick"), nothing is deleted, and other people's and other campaigns'
// claims are untouched. A trashed character is freed too but not named.
func TestEntityRepository_ReleaseOwned_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tam, bob := testUUID(t), testUUID(t)
	camp, other := testUUID(t), testUUID(t)
	for _, u := range []string{tam, bob} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "release-int-"+u+"@example.test", "Release Int Test", "x")
	}
	for _, c := range []string{camp, other} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			c, "Release Int Test", "release-int-"+c[:8], tam)
	}
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, camp, other)
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, tam, bob)
	}()

	types := NewEntityTypeRepository(db)
	mkType := func(campaignID string) *EntityType {
		et := &EntityType{
			CampaignID: campaignID, Slug: "release-party", Name: "Party", NamePlural: "Party",
			Icon: "fa-user", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
		}
		mustCreate(t, types, ctx, et)
		return et
	}
	campType, otherType := mkType(camp), mkType(other)
	repo := NewEntityRepository(db)

	mk := func(campaignID string, typeID int, name, owner, deleted string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, owner_user_id, playable, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, NOW(), NOW())`, id, campaignID, typeID, name, id[:8], tam, nullable(owner))
		if deleted != "" {
			mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, id)
		}
		return id
	}
	brannoc := mk(camp, campType.ID, "Brannoc", tam, "")
	vex := mk(camp, campType.ID, "Vex", tam, "")
	gone := mk(camp, campType.ID, "Old Tam", tam, "trashed")
	sela := mk(camp, campType.ID, "Sela", bob, "")
	elsewhere := mk(other, otherType.ID, "Tam elsewhere", tam, "")

	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM entities WHERE campaign_id = ?`, camp).Scan(&before); err != nil {
		t.Fatal(err)
	}

	t.Run("names before", func(t *testing.T) {
		got, err := repo.(*entityRepository).ClaimedNames(ctx, tam, []string{camp, other})
		if err != nil {
			t.Fatalf("ClaimedNames: %v", err)
		}
		want := map[string][]string{camp: {"Brannoc", "Vex"}, other: {"Tam elsewhere"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClaimedNames = %v, want %v", got, want)
		}
	})

	names, err := repo.(*entityRepository).ReleaseOwned(ctx, camp, tam)
	if err != nil {
		t.Fatalf("ReleaseOwned: %v", err)
	}
	if want := []string{"Brannoc", "Vex"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}

	free := func(id string) bool {
		e, err := repo.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID %s: %v", id, err)
		}
		return e.OwnerUserID == nil && e.Playable
	}
	for _, id := range []string{brannoc, vex} {
		if !free(id) {
			t.Errorf("%s is not free to pick", id)
		}
	}
	var trashedOwner *string
	if err := db.QueryRow(`SELECT owner_user_id FROM entities WHERE id = ?`, gone).Scan(&trashedOwner); err != nil || trashedOwner != nil {
		t.Errorf("trashed character still claimed: %v err=%v", trashedOwner, err)
	}
	for id, want := range map[string]string{sela: bob, elsewhere: tam} {
		e, err := repo.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID %s: %v", id, err)
		}
		if e.OwnerUserID == nil || *e.OwnerUserID != want {
			t.Errorf("%s owner = %v, want %s", id, e.OwnerUserID, want)
		}
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM entities WHERE campaign_id = ?`, camp).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("pages in the campaign: %d before, %d after; nothing may be deleted", before, after)
	}

	again, err := repo.(*entityRepository).ReleaseOwned(ctx, camp, tam)
	if err != nil || len(again) != 0 {
		t.Errorf("second release = %v, %v; want nothing", again, err)
	}
}

// nullable turns an empty owner into SQL NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ReleaseOne frees one page the way ReleaseOwned frees all of a leaver's: the
// owner is cleared and the page is marked playable. It matches the owner and
// the campaign, so a page that changed hands, or one of another table, stays.
func TestEntityRepository_ReleaseOne_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tam, bob, camp, other := testUUID(t), testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{tam, bob} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "release-one-"+u+"@example.test", "Release One Test", "x")
	}
	for _, c := range []string{camp, other} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Release One", "release-one-"+c[:8], tam)
	}
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, camp, other)
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, tam, bob)
	}()
	et := &EntityType{CampaignID: camp, Slug: "release-one", Name: "Party", NamePlural: "Party", Icon: "fa-user", Color: "#111111",
		Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true}
	mustCreate(t, NewEntityTypeRepository(db), ctx, et)
	mk := func(name, owner string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, owner_user_id, playable, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, NOW(), NOW())`, id, camp, et.ID, name, id[:8], tam, nullable(owner))
		return id
	}
	tess, vex, sela := mk("Tess", tam), mk("Vex", tam), mk("Sela", bob)
	repo := NewEntityRepository(db).(*entityRepository)

	if ok, err := repo.ReleaseOne(ctx, other, tess, tam); err != nil || ok {
		t.Errorf("another campaign: %v %v", ok, err)
	}
	if ok, err := repo.ReleaseOne(ctx, camp, sela, tam); err != nil || ok {
		t.Errorf("someone else's page: %v %v", ok, err)
	}
	if ok, err := repo.ReleaseOne(ctx, camp, tess, tam); err != nil || !ok {
		t.Fatalf("release: %v %v", ok, err)
	}
	var owner *string
	var playable bool
	if err := db.QueryRow(`SELECT owner_user_id, playable FROM entities WHERE id = ?`, tess).Scan(&owner, &playable); err != nil {
		t.Fatal(err)
	}
	if owner != nil || !playable {
		t.Errorf("released page: owner=%v playable=%v", owner, playable)
	}
	var vexOwner *string
	if err := db.QueryRow(`SELECT owner_user_id FROM entities WHERE id = ?`, vex).Scan(&vexOwner); err != nil || vexOwner == nil || *vexOwner != tam {
		t.Errorf("their other page was touched: %v %v", vexOwner, err)
	}
	if ok, _ := repo.ReleaseOne(ctx, camp, tess, tam); ok {
		t.Error("releasing twice should report nothing released")
	}
}
