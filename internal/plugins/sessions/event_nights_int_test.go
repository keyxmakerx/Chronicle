package sessions

import (
	"context"
	"testing"
)

// TestDB_EventNightSurvivesRename pins that the strip's same-day match works
// on rows read back through the repository: a night made for a story event
// and then renamed must still stand for that event. The unit test feeds
// eventHasNight a hand-built Session, which cannot catch a list query that
// leaves calendar_id unread.
func TestDB_EventNightSurvivesRename(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	calID := newDBID(t)
	if _, err := db.Exec(`INSERT INTO calendars (id, campaign_id, name) VALUES (?, ?, ?)`, calID, campID, "story"); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	ev := SessionEvent{ID: "e1", Name: "Session 1", CalendarID: calID, Year: 1492, Month: 3, Day: 14}

	// The night as eventNightInput shapes it: in-world date, no real date,
	// calendar stamped as the service does on create.
	id := seedSession(t, db, campID, userID, ev.Name)
	if _, err := db.Exec(`UPDATE sessions SET calendar_id = ?, calendar_year = ?, calendar_month = ?, calendar_day = ? WHERE id = ?`,
		calID, ev.Year, ev.Month, ev.Day, id); err != nil {
		t.Fatalf("shape night: %v", err)
	}

	candidates := func() []SessionEvent {
		t.Helper()
		nights, err := repo.ListByCampaign(ctx, campID)
		if err != nil {
			t.Fatalf("ListByCampaign: %v", err)
		}
		return eventsWithoutNight([]SessionEvent{ev}, nights)
	}

	if got := candidates(); len(got) != 0 {
		t.Fatalf("event offered again right after its night was made: %+v", got)
	}
	if _, err := db.Exec(`UPDATE sessions SET name = ? WHERE id = ?`, "The siege of Hammerfall", id); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := candidates(); len(got) != 0 {
		t.Fatalf("event offered again after its night was renamed: %+v", got)
	}

	// A night on another day of the same calendar must not hide it.
	if _, err := db.Exec(`UPDATE sessions SET calendar_day = ? WHERE id = ?`, ev.Day+1, id); err != nil {
		t.Fatalf("move: %v", err)
	}
	if got := candidates(); len(got) != 1 {
		t.Fatalf("a night on another day hid the event: %+v", got)
	}
}
