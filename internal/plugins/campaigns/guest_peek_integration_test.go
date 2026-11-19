package campaigns

import (
	"context"
	"testing"
	"time"
)

// TestGuestCodePeek_Integration checks the door's read against real MariaDB:
// a live code shows its campaign without being used, and a used, expired or
// wrong code, or one whose campaign is in the Trash, shows nothing.
func TestGuestCodePeek_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	svc := NewGuestCodeService(NewGuestCodeRepository(db), NewCampaignRepository(db))
	start := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return start }

	owner, campaignID := navPinsUUID(t), navPinsUUID(t)
	navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "peek-"+owner+"@example.test", "Peek owner", "x")
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Peek", "peek-"+campaignID[:8], owner)

	live, err := svc.Create(ctx, campaignID, owner, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := svc.Peek(ctx, live.Code); !ok || id != campaignID {
		t.Fatalf("Peek(live) = %q %v", id, ok)
	}
	// Looking does not use the code.
	if _, _, err := svc.ClaimGuestCode(ctx, live.Code); err != nil {
		t.Fatalf("Claim after Peek: %v", err)
	}
	if _, ok := svc.Peek(ctx, live.Code); ok {
		t.Fatal("a used code still shows")
	}
	if _, ok := svc.Peek(ctx, "ZZZZ-ZZZZ"); ok {
		t.Fatal("a wrong code shows")
	}

	old, _ := svc.Create(ctx, campaignID, owner, "", 0)
	svc.now = func() time.Time { return start.Add(guestCodeLife + time.Minute) }
	if _, ok := svc.Peek(ctx, old.Code); ok {
		t.Fatal("an expired code shows")
	}
	svc.now = func() time.Time { return start }

	trashed, _ := svc.Create(ctx, campaignID, owner, "", 0)
	navPinsExec(t, db, `UPDATE campaigns SET deleted_at = NOW() WHERE id = ?`, campaignID)
	if _, ok := svc.Peek(ctx, trashed.Code); ok {
		t.Fatal("a code of a trashed campaign shows")
	}
}
