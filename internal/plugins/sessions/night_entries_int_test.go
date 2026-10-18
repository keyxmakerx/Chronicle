package sessions

import (
	"context"
	"testing"
)

// TestDB_NightEntries proves the entries SQL against real rows: recording an
// entry twice keeps the first, a member's entries come back per session, and
// the by-note read joins the night's name and time while leaving out
// soft-deleted sessions and other campaigns.
func TestDB_NightEntries(t *testing.T) {
	db := newScratchDB(t)
	repo := NewNightEntryRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)
	weekly := seedSession(t, db, campID, userID, "Weekly")
	gone := seedSession(t, db, campID, userID, "Deleted")
	elsewhere := seedSession(t, db, otherCamp, otherUser, "Other table")
	if _, err := db.Exec(`UPDATE sessions SET scheduled_date = '2026-10-03', scheduled_time = '19:00', scheduled_tz = 'Europe/London', is_recurring = 1, recurrence_type = 'weekly' WHERE id = ?`, weekly); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET scheduled_date = '2026-09-01', deleted_at = NOW() WHERE id = ?`, gone); err != nil {
		t.Fatal(err)
	}

	insert := func(r NightEntryRow) {
		t.Helper()
		if err := repo.InsertNightEntry(ctx, r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	insert(NightEntryRow{SessionID: weekly, Date: "2026-10-10", UserID: userID, NoteID: "n-10"})
	insert(NightEntryRow{SessionID: weekly, Date: "2026-10-10", UserID: userID, NoteID: "n-again"}) // kept as it was
	insert(NightEntryRow{SessionID: weekly, Date: "2026-10-17", UserID: userID, NoteID: "n-17"})
	insert(NightEntryRow{SessionID: gone, Date: "2026-09-01", UserID: userID, NoteID: "n-gone"})
	insert(NightEntryRow{SessionID: elsewhere, Date: "2026-09-01", UserID: otherUser, NoteID: "n-else"})

	night, err := repo.ListNightEntries(ctx, weekly, "2026-10-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(night) != 1 || night[0].NoteID != "n-10" || night[0].Date != "2026-10-10" || night[0].UserID != userID {
		t.Errorf("night = %+v, want only n-10", night)
	}

	mine, err := repo.ListUserNightEntries(ctx, userID, []string{weekly})
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 2 {
		t.Errorf("mine = %+v, want both weekly nights", mine)
	}
	if none, err := repo.ListUserNightEntries(ctx, userID, nil); err != nil || len(none) != 0 {
		t.Errorf("no sessions: %v, %v", none, err)
	}

	tagged, err := repo.ListNightEntriesByNote(ctx, campID, []string{"n-10", "n-gone", "n-else", "n-missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 1 || tagged[0].NoteID != "n-10" || tagged[0].Name != "Weekly" || tagged[0].Time != "19:00" || tagged[0].TZ != "Europe/London" {
		t.Errorf("tagged = %+v, want only n-10 with its night", tagged)
	}
}

// TestDB_NightBells proves the bells SQL: a claim succeeds once per night,
// member and kind, and the night list spans campaigns, keeps planned and
// completed nights and repeating series, and leaves out cancelled and
// deleted ones.
func TestDB_NightBells(t *testing.T) {
	db := newScratchDB(t)
	repo := NewNightEntryRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)
	planned := seedSession(t, db, campID, userID, "Planned")
	done := seedSession(t, db, otherCamp, otherUser, "Completed")
	weekly := seedSession(t, db, campID, userID, "Weekly")
	cancelled := seedSession(t, db, campID, userID, "Cancelled")
	gone := seedSession(t, db, campID, userID, "Deleted")
	far := seedSession(t, db, campID, userID, "Far off")
	for _, q := range []struct {
		sql string
		id  string
	}{
		{`UPDATE sessions SET scheduled_date = '2026-10-10', status = 'planned' WHERE id = ?`, planned},
		{`UPDATE sessions SET scheduled_date = '2026-10-09', status = 'completed' WHERE id = ?`, done},
		{`UPDATE sessions SET scheduled_date = '2026-09-05', status = 'planned', is_recurring = 1, recurrence_type = 'weekly' WHERE id = ?`, weekly},
		{`UPDATE sessions SET scheduled_date = '2026-10-10', status = 'cancelled' WHERE id = ?`, cancelled},
		{`UPDATE sessions SET scheduled_date = '2026-10-10', status = 'planned', deleted_at = NOW() WHERE id = ?`, gone},
		{`UPDATE sessions SET scheduled_date = '2026-12-01', status = 'planned' WHERE id = ?`, far},
	} {
		if _, err := db.Exec(q.sql, q.id); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListBellNights(ctx, "2026-10-07", "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range list {
		got[s.ID] = true
	}
	if len(list) != 3 || !got[planned] || !got[done] || !got[weekly] {
		t.Errorf("nights = %v, want planned, completed and the weekly", got)
	}

	for i, want := range []bool{true, false} {
		ok, err := repo.ClaimNightBell(ctx, planned, "2026-10-10", userID, "before")
		if err != nil || ok != want {
			t.Errorf("claim %d = %v, %v; want %v", i, ok, err, want)
		}
	}
	if ok, err := repo.ClaimNightBell(ctx, planned, "2026-10-10", userID, "checkin"); err != nil || !ok {
		t.Errorf("another kind: %v, %v", ok, err)
	}
}
