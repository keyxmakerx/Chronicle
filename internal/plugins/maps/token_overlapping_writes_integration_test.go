package maps

import (
	"context"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// seedTokenForOverlap inserts the user, campaign, map and token rows a token
// write needs and returns the repo, map id and token id.
func seedTokenForOverlap(t *testing.T) (DrawingRepository, string, string) {
	t.Helper()
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewDrawingRepository(db)

	userID, campaignID, mapID := newMapsDBID(t), newMapsDBID(t), newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Token Overlap Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Token Overlap Test", campaignID, userID)
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`,
		mapID, campaignID, "Overlap Map")

	tok := &Token{ID: newMapsDBID(t), MapID: mapID, Name: "Goblin", X: 10, Y: 10, Width: 1, Height: 1, Scale: 1}
	if err := repo.CreateToken(ctx, tok); err != nil {
		t.Fatalf("create token: %v", err)
	}
	return repo, mapID, tok.ID
}

// A write that names only some columns must not put back the values it read
// for the others. Deterministic form of the race: the stale copy is read
// before another save lands.
func TestUpdateToken_StaleReadKeepsUntouchedColumns_Integration(t *testing.T) {
	repo, _, id := seedTokenForOverlap(t)
	ctx := context.Background()

	stale, err := repo.GetToken(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	hp, hpMax := 7, 20
	fresh := *stale
	fresh.Bar1Value, fresh.Bar1Max = &hp, &hpMax
	if err := repo.UpdateToken(ctx, &fresh, []TokenColumn{"bar1_value", "bar1_max"}); err != nil {
		t.Fatal(err)
	}

	stale.X, stale.Y = 55, 66
	if err := repo.UpdateToken(ctx, stale, []TokenColumn{"x", "y"}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetToken(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.X != 55 || got.Y != 66 {
		t.Errorf("position = %v,%v, want 55,66", got.X, got.Y)
	}
	if got.Bar1Value == nil || *got.Bar1Value != 7 || got.Bar1Max == nil || *got.Bar1Max != 20 {
		t.Errorf("bars were overwritten by the stale copy: %v/%v", got.Bar1Value, got.Bar1Max)
	}
}

// A position patch and a bar patch sent together through the service must both
// survive on every iteration.
func TestUpdateToken_ConcurrentPositionAndBar_Integration(t *testing.T) {
	repo, mapID, id := seedTokenForOverlap(t)
	ctx := context.Background()
	svc := NewDrawingService(repo)

	for i := 1; i <= 40; i++ {
		x, y, hp := float64(i), float64(100-i), i
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs[0] = svc.UpdateToken(ctx, id, mapID, true, UpdateTokenInput{X: patch.Of(x), Y: patch.Of(y)})
		}()
		go func() {
			defer wg.Done()
			errs[1] = svc.UpdateToken(ctx, id, mapID, true, UpdateTokenInput{Bar1Value: patch.Of(hp)})
		}()
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("iteration %d errors: %v, %v", i, errs[0], errs[1])
		}
		got, err := repo.GetToken(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.X != x || got.Y != y || got.Bar1Value == nil || *got.Bar1Value != hp {
			t.Fatalf("iteration %d lost an update: x,y=%v,%v bar1=%v, want %v,%v,%d", i, got.X, got.Y, got.Bar1Value, x, y, hp)
		}
	}
}
