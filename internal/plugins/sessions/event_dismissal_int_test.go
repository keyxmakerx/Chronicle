package sessions

import (
	"context"
	"database/sql"
	"testing"
)

// Deleting the event removes its dismissal row, so nothing dangles.
func TestDB_DeletingTheEventRemovesItsDismissal(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campID, userID := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campID, "Session 1")
	if err := repo.DismissEvent(ctx, campID, eventID, userID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM calendar_events WHERE id = ?`, eventID); err != nil {
		t.Fatalf("delete event: %v", err)
	}
	if ids, err := repo.ListDismissedEventIDs(ctx, campID); err != nil || len(ids) != 0 {
		t.Fatalf("dismissals after the event was deleted = %v, err %v", ids, err)
	}
}

// "Not this one" is kept per campaign, shared by everyone, and a second press
// is harmless.
func TestDB_DismissalsAreKeptPerCampaign(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campA, userA := seedCampaign(t, db)
	campB, _ := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campA, "Session 1")

	for i := 0; i < 2; i++ {
		if err := repo.DismissEvent(ctx, campA, eventID, userA); err != nil {
			t.Fatalf("dismiss #%d: %v", i+1, err)
		}
	}
	if ids, err := repo.ListDismissedEventIDs(ctx, campA); err != nil || len(ids) != 1 || ids[0] != eventID {
		t.Fatalf("campaign A dismissals = %v, err %v", ids, err)
	}
	if ids, err := repo.ListDismissedEventIDs(ctx, campB); err != nil || len(ids) != 0 {
		t.Fatalf("campaign B must not see A's dismissals: %v, err %v", ids, err)
	}
	var by sql.NullString
	if err := db.QueryRow(`SELECT dismissed_by FROM session_event_dismissals WHERE campaign_id = ?`, campA).Scan(&by); err != nil || by.String != userA {
		t.Fatalf("dismissed_by = %v, err %v", by, err)
	}
}
