package sessions

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// Undo removes only this campaign's dismissal, twice is harmless, and an event
// that was never dismissed is not an error.
func TestDB_RestoreEventIsScopedAndIdempotent(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campA, userA := seedCampaign(t, db)
	campB, _ := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campA, "Session 1")
	if err := repo.DismissEvent(ctx, campA, eventID, userA); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	// Another campaign's Undo for the same id leaves A's dismissal alone.
	if err := repo.RestoreEvent(ctx, campB, eventID); err != nil {
		t.Fatalf("restore from B: %v", err)
	}
	if ids, _ := repo.ListDismissedEventIDs(ctx, campA); len(ids) != 1 {
		t.Fatalf("B's undo removed A's dismissal: %v", ids)
	}
	for i := 0; i < 2; i++ {
		if err := repo.RestoreEvent(ctx, campA, eventID); err != nil {
			t.Fatalf("restore #%d: %v", i+1, err)
		}
	}
	if ids, err := repo.ListDismissedEventIDs(ctx, campA); err != nil || len(ids) != 0 {
		t.Fatalf("dismissals after undo = %v, err %v", ids, err)
	}
}

// An event deleted between the handler's read and the insert is a 404 for the
// dismissal and for a night made from it, never a raw foreign-key error.
func TestDB_VanishedEventIsNotFound(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campID, userID := seedCampaign(t, db)

	err := repo.DismissEvent(ctx, campID, "no-such-event", userID)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("dismiss of a vanished event = %v, want a 404", err)
	}

	missing := "no-such-event"
	err = repo.Create(ctx, campID, &Session{ID: "s-vanished", Name: "Session 1", Status: StatusPlanned, CalendarEventID: &missing})
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("night from a vanished event = %v, want a 404", err)
	}
}
