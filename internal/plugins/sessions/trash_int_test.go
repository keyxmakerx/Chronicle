package sessions

import (
	"context"
	"testing"
	"time"
)

// TestDB_TrashedSessions proves the Trash SQL against real rows: only this
// campaign's deleted sessions are listed, newest first, and the purge
// removes only those deleted before the cutoff, answers and all.
func TestDB_TrashedSessions(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)
	live := seedSession(t, db, campID, userID, "Live")
	oldGone := seedSession(t, db, campID, userID, "Old")
	newGone := seedSession(t, db, campID, userID, "New")
	elsewhere := seedSession(t, db, otherCamp, otherUser, "Elsewhere")
	if _, err := db.Exec(`UPDATE sessions SET deleted_at = NOW() - INTERVAL 40 DAY WHERE id = ?`, oldGone); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{newGone, elsewhere} {
		if err := repo.SoftDeleteSession(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session_attendees (session_id, user_id, status) VALUES (?, ?, 'accepted')`, oldGone, userID); err != nil {
		t.Fatalf("seed answer: %v", err)
	}

	list, err := repo.ListDeleted(ctx, campID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != newGone || list[1].ID != oldGone || list[0].Name != "New" || list[0].DeletedAt.IsZero() {
		t.Fatalf("listed %+v, want New then Old, this campaign only", list)
	}

	n, err := repo.PurgeDeletedBefore(ctx, time.Now().UTC().AddDate(0, 0, -30))
	if err != nil || n != 1 {
		t.Fatalf("purged %d (err %v), want only the 40-day-old session", n, err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id IN (?, ?, ?, ?)`, live, oldGone, newGone, elsewhere).Scan(&left); err != nil || left != 3 {
		t.Errorf("%d sessions left (err %v), want the other three", left, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM session_attendees WHERE session_id = ?`, oldGone).Scan(&left); err != nil || left != 0 {
		t.Errorf("%d answers left on the purged session (err %v)", left, err)
	}
}
