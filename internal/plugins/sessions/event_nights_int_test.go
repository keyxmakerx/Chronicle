package sessions

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
		return eventsWithoutNight([]SessionEvent{ev}, nights, nil, nil)
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

// seedEvent inserts a calendar and one event on it, returning both ids.
func seedEvent(t *testing.T, db *sql.DB, campID, name string) (calID, eventID string) {
	t.Helper()
	calID, eventID = newDBID(t), newDBID(t)
	if _, err := db.Exec(`INSERT INTO calendars (id, campaign_id, name) VALUES (?, ?, ?)`, calID, campID, "story"); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO calendar_events (id, calendar_id, name, year, month, day) VALUES (?, ?, ?, 1492, 3, 14)`,
		eventID, calID, name); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return calID, eventID
}

// A night made from an event keeps its link through a rename and through the
// Trash, and the link is read back by the list the strip uses.
func TestDB_EventLinkSurvivesRenameAndTrash(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campID, userID := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campID, "Session 1")

	id := seedSession(t, db, campID, userID, "Session 1")
	if _, err := db.Exec(`UPDATE sessions SET calendar_event_id = ? WHERE id = ?`, eventID, id); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET name = 'The siege' WHERE id = ?`, id); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil || got.CalendarEventID == nil || *got.CalendarEventID != eventID {
		t.Fatalf("FindByID link = %v, err %v", got, err)
	}
	listed, err := repo.ListByCampaign(ctx, campID)
	if err != nil || len(listed) != 1 || listed[0].CalendarEventID == nil || *listed[0].CalendarEventID != eventID {
		t.Fatalf("ListByCampaign link = %+v, err %v", listed, err)
	}

	if err := repo.SoftDeleteSession(ctx, id); err != nil {
		t.Fatalf("trash: %v", err)
	}
	linked, err := repo.ListLinkedEventIDs(ctx, campID)
	if err != nil || len(linked) != 1 || linked[0] != eventID {
		t.Fatalf("a night in the Trash must keep holding its event: %v, err %v", linked, err)
	}
}

// The unique key is the whole race fix: several simultaneous makes of one
// event leave exactly one night, and every loser is told it is a conflict.
func TestDB_SimultaneousMakesForOneEventLeaveOneNight(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil, nil)
	ctx := context.Background()
	campID, userID := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campID, "Session 1")
	ev := SessionEvent{ID: eventID, Name: "Session 1", Year: 1492, Month: 3, Day: 14}

	const n = 6
	var wg sync.WaitGroup
	var made, conflicts, other atomic.Int32
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := makeNightFromEvent(ctx, svc, nil, campID, userID, ev)
			var ae *apperror.AppError
			switch {
			case err == nil:
				made.Add(1)
			case errors.As(err, &ae) && ae.Code == http.StatusConflict:
				conflicts.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if made.Load() != 1 || conflicts.Load() != n-1 || other.Load() != 0 {
		t.Fatalf("made %d, conflicts %d, other %d; want 1 night and %d conflicts", made.Load(), conflicts.Load(), other.Load(), n-1)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE calendar_event_id = ?`, eventID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("nights for the event = %d, err %v", rows, err)
	}
}

// Deleting the event unlinks its night; it does not take the night with it.
func TestDB_DeletingTheEventUnlinksTheNight(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	campID, userID := seedCampaign(t, db)
	_, eventID := seedEvent(t, db, campID, "Session 1")
	id := seedSession(t, db, campID, userID, "Session 1")
	if _, err := db.Exec(`UPDATE sessions SET calendar_event_id = ? WHERE id = ?`, eventID, id); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM calendar_events WHERE id = ?`, eventID); err != nil {
		t.Fatalf("delete event: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil || got.CalendarEventID != nil {
		t.Fatalf("night after its event was deleted = %+v, err %v; want it kept and unlinked", got, err)
	}
}

// The migration applies twice over itself and reverts cleanly: the scratch DB
// already ran it once, so run it again, then the down, then the up.
func TestDB_EventLinkMigrationIsIdempotentAndReverts(t *testing.T) {
	db := newScratchDB(t)
	up, err := MigrationsFS.ReadFile("migrations/012_event_night_link.up.sql")
	if err != nil {
		t.Fatalf("read up: %v", err)
	}
	down, err := MigrationsFS.ReadFile("migrations/012_event_night_link.down.sql")
	if err != nil {
		t.Fatalf("read down: %v", err)
	}
	run := func(label string, sqlText []byte) {
		t.Helper()
		for _, stmt := range strings.Split(string(sqlText), ";\n") {
			if strings.TrimSpace(stripSQLComments(stmt)) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v\n%s", label, err, stmt)
			}
		}
	}
	run("up again", up)
	run("down", down)
	var cols int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'sessions' AND column_name = 'calendar_event_id'`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("column after down = %d, err %v", cols, err)
	}
	run("down again", down)
	run("up after down", up)
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'sessions' AND index_name = 'uq_sessions_calendar_event'`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("unique index after up = %d, err %v", cols, err)
	}
}

// stripSQLComments drops whole-line "--" comments so a chunk that is only
// comments is recognised as empty.
func stripSQLComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}
