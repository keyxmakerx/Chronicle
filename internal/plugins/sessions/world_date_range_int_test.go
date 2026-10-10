package sessions

import (
	"context"
	"reflect"
	"testing"
)

// TestDB_ListPlannedByWorldDateRange pins the in-world date range query the
// calendar's anchor-move warning relies on, against real rows: inclusive
// tuple bounds across month and year edges, planned/live/complete-date
// filtering, campaign scoping, ordering and the cap.
func TestDB_ListPlannedByWorldDateRange(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)

	add := func(camp, user, name, status string, y, m, d *int, deleted bool) {
		t.Helper()
		id := seedSession(t, db, camp, user, name)
		if _, err := db.Exec(`UPDATE sessions SET status = ?, calendar_year = ?, calendar_month = ?, calendar_day = ?,
			deleted_at = IF(?, NOW(), NULL) WHERE id = ?`, status, y, m, d, deleted, id); err != nil {
			t.Fatalf("shape session %s: %v", name, err)
		}
	}
	n := func(v int) *int { return &v }

	add(campID, userID, "before window", StatusPlanned, n(1000), n(1), n(1), false)
	add(campID, userID, "lower bound", StatusPlanned, n(1000), n(2), n(10), false)
	add(campID, userID, "same year later month", StatusPlanned, n(1000), n(12), n(1), false)
	add(campID, userID, "next year early month", StatusPlanned, n(1001), n(1), n(5), false)
	add(campID, userID, "upper bound", StatusPlanned, n(1001), n(3), n(20), false)
	add(campID, userID, "after window", StatusPlanned, n(1001), n(3), n(21), false)
	add(campID, userID, "completed", StatusCompleted, n(1000), n(6), n(1), false)
	add(campID, userID, "cancelled", StatusCancelled, n(1000), n(6), n(2), false)
	add(campID, userID, "soft deleted", StatusPlanned, n(1000), n(6), n(3), true)
	add(campID, userID, "no day", StatusPlanned, n(1000), n(6), nil, false)
	add(campID, userID, "no date at all", StatusPlanned, nil, nil, nil, false)
	add(otherCamp, otherUser, "other campaign", StatusPlanned, n(1000), n(6), n(4), false)

	from, to := WorldDate{1000, 2, 10}, WorldDate{1001, 3, 20}
	names := func(limit int) []string {
		t.Helper()
		got, err := repo.ListPlannedByWorldDateRange(ctx, campID, "", from, to, limit)
		if err != nil {
			t.Fatalf("ListPlannedByWorldDateRange: %v", err)
		}
		var out []string
		for _, s := range got {
			if !s.HasCalendarDate() {
				t.Errorf("%q returned without a full in-world date", s.Name)
			}
			out = append(out, s.Name)
		}
		return out
	}

	want := []string{"lower bound", "same year later month", "next year early month", "upper bound"}
	if got := names(0); !reflect.DeepEqual(got, want) {
		t.Errorf("uncapped = %v, want %v", got, want)
	}
	if got := names(2); !reflect.DeepEqual(got, want[:2]) {
		t.Errorf("limit 2 = %v, want the two soonest %v", got, want[:2])
	}
}

// TestDB_WorldDateCalendarScoping pins the calendar filter on the world-date
// range, the backfill stamp and the stamp's idempotence against real rows.
func TestDB_WorldDateCalendarScoping(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	calA, calB := newDBID(t), newDBID(t)
	for _, c := range []string{calA, calB} {
		if _, err := db.Exec(`INSERT INTO calendars (id, campaign_id, name) VALUES (?, ?, ?)`, c, campID, "cal "+c[:4]); err != nil {
			t.Fatalf("seed calendar: %v", err)
		}
	}
	add := func(name string, cal *string, month int) {
		t.Helper()
		id := seedSession(t, db, campID, userID, name)
		if _, err := db.Exec(`UPDATE sessions SET calendar_id = ?, calendar_year = 1000, calendar_month = ?, calendar_day = 5 WHERE id = ?`,
			cal, month, id); err != nil {
			t.Fatalf("shape session %s: %v", name, err)
		}
	}
	add("on A", &calA, 1)
	add("on B", &calB, 2)
	add("unstamped", nil, 3)

	from, to := WorldDate{1000, 1, 1}, WorldDate{1000, 12, 30}
	names := func(calendarID string) []string {
		t.Helper()
		got, err := repo.ListPlannedByWorldDateRange(ctx, campID, calendarID, from, to, 0)
		if err != nil {
			t.Fatalf("ListPlannedByWorldDateRange: %v", err)
		}
		var out []string
		for _, s := range got {
			out = append(out, s.Name)
		}
		return out
	}
	if got := names(calA); !reflect.DeepEqual(got, []string{"on A"}) {
		t.Errorf("calendar A = %v, want only its own night", got)
	}
	if got := names(""); !reflect.DeepEqual(got, []string{"on A", "on B", "unstamped"}) {
		t.Errorf("no calendar named = %v, want every night", got)
	}

	ids, err := repo.ListCampaignIDsWithUnstampedWorldDates(ctx)
	if err != nil || !reflect.DeepEqual(ids, []string{campID}) {
		t.Fatalf("unstamped campaigns = %v, %v", ids, err)
	}
	if n, err := repo.StampWorldDateCalendar(ctx, campID, calA); err != nil || n != 1 {
		t.Fatalf("stamp = %d, %v; want 1 row", n, err)
	}
	if n, err := repo.StampWorldDateCalendar(ctx, campID, calB); err != nil || n != 0 {
		t.Errorf("second stamp = %d, %v; want 0 (already stamped rows are never rewritten)", n, err)
	}
	if got := names(calA); !reflect.DeepEqual(got, []string{"on A", "unstamped"}) {
		t.Errorf("calendar A after stamp = %v", got)
	}
}
