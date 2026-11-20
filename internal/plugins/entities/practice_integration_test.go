package entities

import (
	"context"
	"sync"
	"testing"
)

// TestPracticeRepository_Integration runs the practice table for real: the
// cap that two simultaneous requests cannot get past, the single taking of a
// waiting hero, scoping by campaign and maker, the guest merge and the
// campaign cascade.
func TestPracticeRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openStandingDB(t)
	ctx := context.Background()
	repo := NewPracticeRepository(db)

	pat, sam, camp, other := testUUID(t), testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{pat, sam} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, u, "pr-"+u+"@example.test", "Practice Test", "x")
	}
	for _, c := range []string{camp, other} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Practice", "pr-"+c[:8], pat)
	}
	mk := func(campaign, user, name string) *PracticeHero {
		return &PracticeHero{ID: testUUID(t), CampaignID: campaign, UserID: user, Name: name,
			Picks: map[string]HeroPick{"class": {Name: "Fury", Options: []string{"Edge"}}}}
	}

	t.Run("round trip keeps the picks, level and copy name", func(t *testing.T) {
		lvl := 3
		h := mk(camp, pat, "Tess as a Fury")
		h.Level, h.CopiedFromName = &lvl, "Tess"
		if ok, err := repo.Insert(ctx, h, 3); err != nil || !ok {
			t.Fatalf("insert: %v %v", ok, err)
		}
		got, err := repo.Find(ctx, camp, h.ID)
		if err != nil || got == nil {
			t.Fatalf("find: %v %v", got, err)
		}
		if got.Name != h.Name || got.Picks["class"].Name != "Fury" || got.Picks["class"].Options[0] != "Edge" ||
			got.Level == nil || *got.Level != 3 || got.CopiedFromName != "Tess" || got.AskStatus != "" {
			t.Errorf("got %+v", got)
		}
		if other, _ := repo.Find(ctx, other, h.ID); other != nil {
			t.Error("another campaign found it")
		}
		_, _ = repo.Delete(ctx, camp, h.ID, pat)
	})

	t.Run("the cap holds, in sequence and under a race", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if ok, err := repo.Insert(ctx, mk(camp, sam, "S"), 3); err != nil || !ok {
				t.Fatalf("insert %d: %v %v", i, ok, err)
			}
		}
		if ok, err := repo.Insert(ctx, mk(camp, sam, "S4"), 3); err != nil || ok {
			t.Fatalf("a fourth was saved: %v %v", ok, err)
		}
		// Another campaign has its own count.
		if ok, _ := repo.Insert(ctx, mk(other, sam, "Elsewhere"), 3); !ok {
			t.Error("the cap leaked across campaigns")
		}
		_, _ = repo.DeleteForUser(ctx, camp, sam)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = repo.Insert(ctx, mk(camp, pat, "R"), 3)
			}()
		}
		wg.Wait()
		list, err := repo.ListByUser(ctx, camp, pat)
		if err != nil || len(list) > 3 || len(list) == 0 {
			t.Errorf("after the race: %d heroes, %v", len(list), err)
		}
		_, _ = repo.DeleteForUser(ctx, camp, pat)
	})

	t.Run("ask, decline, replace and taking a waiting hero", func(t *testing.T) {
		h := mk(camp, pat, "Ask")
		_, _ = repo.Insert(ctx, h, 3)
		if ok, _ := repo.SetAsk(ctx, camp, h.ID, sam); ok {
			t.Error("someone else asked")
		}
		if ok, err := repo.SetAsk(ctx, camp, h.ID, pat); err != nil || !ok {
			t.Fatalf("ask: %v %v", ok, err)
		}
		if ok, _ := repo.SetAsk(ctx, camp, h.ID, pat); ok {
			t.Error("asking again while waiting should change nothing")
		}
		if ok, _ := repo.Decline(ctx, camp, h.ID, "Next arc."); !ok {
			t.Fatal("decline")
		}
		if got, _ := repo.Find(ctx, camp, h.ID); got.AskStatus != "declined" || got.AskNote != "Next arc." {
			t.Errorf("declined = %+v", got)
		}
		if ok, _ := repo.Decline(ctx, camp, h.ID, "x"); ok {
			t.Error("declined twice")
		}
		_, _ = repo.SetAsk(ctx, camp, h.ID, pat)
		h.Name = "Renamed"
		if ok, err := repo.Replace(ctx, h); err != nil || !ok {
			t.Fatalf("replace: %v %v", ok, err)
		}
		if got, _ := repo.Find(ctx, camp, h.ID); got.Name != "Renamed" || got.AskStatus != "" || got.AskNote != "" || got.AskedAt != nil {
			t.Errorf("replaced = %+v", got)
		}
		// Replacing with the same content still counts as a hit.
		if ok, err := repo.Replace(ctx, h); err != nil || !ok {
			t.Errorf("same-content replace: %v %v", ok, err)
		}
		_, _ = repo.SetAsk(ctx, camp, h.ID, pat)

		var wg sync.WaitGroup
		var mu sync.Mutex
		won := 0
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ok, _ := repo.TakeWaiting(ctx, camp, h.ID); ok {
					mu.Lock()
					won++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if won != 1 {
			t.Errorf("%d callers took the same hero", won)
		}
		if got, _ := repo.Find(ctx, camp, h.ID); got != nil {
			t.Error("a taken hero is still there")
		}
		if err := repo.Restore(ctx, h); err != nil {
			t.Fatal(err)
		}
		if got, _ := repo.Find(ctx, camp, h.ID); got == nil || got.AskStatus != "" {
			t.Errorf("restored = %+v", got)
		}
		_, _ = repo.DeleteForUser(ctx, camp, pat)
	})

	t.Run("lists: the campaign's puts waiting ones first", func(t *testing.T) {
		a, b := mk(camp, pat, "Quiet"), mk(camp, sam, "Waiting")
		_, _ = repo.Insert(ctx, a, 3)
		_, _ = repo.Insert(ctx, b, 3)
		_, _ = repo.SetAsk(ctx, camp, b.ID, sam)
		all, err := repo.ListByCampaign(ctx, camp)
		if err != nil || len(all) != 2 || all[0].ID != b.ID {
			t.Errorf("order = %+v %v", all, err)
		}
		mine, _ := repo.ListByUser(ctx, camp, pat)
		if len(mine) != 1 || mine[0].ID != a.ID {
			t.Errorf("mine = %+v", mine)
		}
		_, _ = repo.DeleteForUserEverywhere(ctx, pat)
		_, _ = repo.DeleteForUserEverywhere(ctx, sam)
	})

	t.Run("a merged guest's heroes move and the newest three stay", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			_, _ = repo.Insert(ctx, mk(camp, sam, "guest"), 3)
		}
		for i := 0; i < 2; i++ {
			_, _ = repo.Insert(ctx, mk(camp, pat, "target"), 3)
		}
		if err := repo.MoveUser(ctx, camp, sam, pat, 3); err != nil {
			t.Fatal(err)
		}
		if g, _ := repo.ListByUser(ctx, camp, sam); len(g) != 0 {
			t.Errorf("guest still holds %d", len(g))
		}
		if m, _ := repo.ListByUser(ctx, camp, pat); len(m) != 3 {
			t.Errorf("target holds %d, want 3", len(m))
		}
		_, _ = repo.DeleteForUser(ctx, camp, pat)
	})

	t.Run("deleting the campaign takes its practice heroes with it", func(t *testing.T) {
		h := mk(other, pat, "Gone with the table")
		_, _ = repo.Insert(ctx, h, 3)
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, other)
		if got, _ := repo.Find(ctx, other, h.ID); got != nil {
			t.Error("the cascade left a practice hero behind")
		}
	})
}
