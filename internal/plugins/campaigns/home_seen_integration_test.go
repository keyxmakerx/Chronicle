package campaigns

import (
	"context"
	"testing"
	"time"
)

// A member with no marker reads from the day they joined; Mark moves only the
// caller's own row, never backwards, and refuses someone who is not a member.
func TestHomeSeenStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	store := NewHomeSeenStore(db)

	owner, p1, p2, stranger := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	campaignID := navPinsUUID(t)
	for _, u := range []string{owner, p1, p2, stranger} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "homeseen-"+u+"@example.test", "Home Seen", "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Home Seen", "home-seen-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player'), (?, ?, 'player')`,
		campaignID, owner, campaignID, p1, campaignID, p2)

	seen, ok, err := store.Get(ctx, campaignID, p1)
	if err != nil || !ok || seen.SeenAt != nil || seen.JoinedAt.IsZero() || !seen.SeenUpTo().Equal(seen.JoinedAt) {
		t.Fatalf("a new member reads from the join time: %+v ok=%v err=%v", seen, ok, err)
	}
	if _, ok, _ := store.Get(ctx, campaignID, stranger); ok {
		t.Error("a non-member has no marker")
	}

	at := time.Now().UTC().Truncate(time.Second)
	if ok, err := store.Mark(ctx, campaignID, p1, at); err != nil || !ok {
		t.Fatalf("Mark: ok=%v err=%v", ok, err)
	}
	seen, _, _ = store.Get(ctx, campaignID, p1)
	if seen.SeenAt == nil || !seen.SeenAt.Equal(at) {
		t.Errorf("marker = %v, want %v", seen.SeenAt, at)
	}
	if other, _, _ := store.Get(ctx, campaignID, p2); other.SeenAt != nil {
		t.Error("marking one member moved another's marker")
	}
	// Never backwards, and an unchanged write is still a member's success.
	if ok, err := store.Mark(ctx, campaignID, p1, at.Add(-time.Hour)); err != nil || !ok {
		t.Fatalf("older Mark: ok=%v err=%v", ok, err)
	}
	if seen, _, _ = store.Get(ctx, campaignID, p1); !seen.SeenAt.Equal(at) {
		t.Errorf("an older Mark moved the marker back to %v", seen.SeenAt)
	}
	if ok, err := store.Mark(ctx, campaignID, stranger, at); err != nil || ok {
		t.Errorf("a non-member Mark: ok=%v err=%v", ok, err)
	}
	// The marker goes with the membership: rejoining starts from the join time.
	navPinsExec(t, db, `DELETE FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, campaignID, p1)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'player')`, campaignID, p1)
	if seen, _, _ = store.Get(ctx, campaignID, p1); seen.SeenAt != nil {
		t.Error("a rejoined member kept the old marker")
	}
}
