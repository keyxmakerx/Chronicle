package entities

import (
	"context"
	"sync"
	"testing"
)

// Two players claiming the same free character must not both win: the write
// is guarded by the current owner, so exactly one claim takes it and the
// other learns it lost. A repeat claim by the winner still reports success.
func TestEntityRepository_ClaimOwner_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	alice, bob, campaignID := testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{alice, bob} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "claim-int-"+u+"@example.test", "Claim Int Test", "x")
	}
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Claim Int Test", "claim-int-"+campaignID[:8], alice)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, alice, bob)
	}()

	et := &EntityType{
		CampaignID: campaignID, Slug: "claim-party", Name: "Party", NamePlural: "Party",
		Icon: "fa-user", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
	}
	mustCreate(t, NewEntityTypeRepository(db), ctx, et)
	repo := NewEntityRepository(db)
	mk := func(slug string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, campaignID, et.ID, slug, slug, alice)
		return id
	}

	t.Run("second claim loses and the first owner stays", func(t *testing.T) {
		id := mk("claim-seq")
		if won, err := repo.ClaimOwner(ctx, id, alice); err != nil || !won {
			t.Fatalf("alice: won=%v err=%v", won, err)
		}
		if won, err := repo.ClaimOwner(ctx, id, bob); err != nil || won {
			t.Fatalf("bob: won=%v err=%v, want a loss", won, err)
		}
		if won, err := repo.ClaimOwner(ctx, id, alice); err != nil || !won {
			t.Fatalf("alice again: won=%v err=%v, want success", won, err)
		}
		e, _ := repo.FindByID(ctx, id)
		if e.OwnerUserID == nil || *e.OwnerUserID != alice || !e.Playable {
			t.Fatalf("owner=%v playable=%v, want alice and playable", e.OwnerUserID, e.Playable)
		}
	})

	t.Run("simultaneous claims have exactly one winner", func(t *testing.T) {
		id := mk("claim-race")
		var wg sync.WaitGroup
		wins := make([]bool, 2)
		errs := make([]error, 2)
		for i, u := range []string{alice, bob} {
			wg.Add(1)
			go func(i int, u string) {
				defer wg.Done()
				wins[i], errs[i] = repo.ClaimOwner(ctx, id, u)
			}(i, u)
		}
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("errors: %v, %v", errs[0], errs[1])
		}
		if wins[0] == wins[1] {
			t.Fatalf("wins = %v, want exactly one", wins)
		}
		e, _ := repo.FindByID(ctx, id)
		want := alice
		if wins[1] {
			want = bob
		}
		if e.OwnerUserID == nil || *e.OwnerUserID != want {
			t.Fatalf("owner=%v, want the winner %s", e.OwnerUserID, want)
		}
	})
}
