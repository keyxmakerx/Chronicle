package sessions

import (
	"context"
	"testing"
)

// TestDB_RemapMonthPositions proves, against real rows, that a swap moves
// every dated session exactly once, and that other calendars, other
// campaigns, unmoved months and soft-deleted rows are handled as documented.
func TestDB_RemapMonthPositions(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)
	calA, calB, calOther := newDBID(t), newDBID(t), newDBID(t)
	for _, c := range []struct{ id, camp string }{{calA, campID}, {calB, campID}, {calOther, otherCamp}} {
		if _, err := db.Exec(`INSERT INTO calendars (id, campaign_id, name) VALUES (?, ?, ?)`, c.id, c.camp, "cal"); err != nil {
			t.Fatalf("seed calendar: %v", err)
		}
	}
	add := func(camp, user, name, cal string, month int, deleted bool) string {
		t.Helper()
		id := seedSession(t, db, camp, user, name)
		if _, err := db.Exec(`UPDATE sessions SET calendar_id = ?, calendar_year = 1000, calendar_month = ?, calendar_day = 7,
			deleted_at = IF(?, NOW(), NULL) WHERE id = ?`, cal, month, deleted, id); err != nil {
			t.Fatalf("shape %s: %v", name, err)
		}
		return id
	}
	three := add(campID, userID, "month 3", calA, 3, false)
	four := add(campID, userID, "month 4", calA, 4, false)
	five := add(campID, userID, "month 5 stays", calA, 5, false)
	deleted := add(campID, userID, "deleted month 3", calA, 3, true)
	onB := add(campID, userID, "other calendar month 3", calB, 3, false)
	elsewhere := add(otherCamp, otherUser, "other campaign", calOther, 3, false)

	n, err := repo.RemapMonthPositions(ctx, campID, calA, map[int]int{3: 4, 4: 3})
	if err != nil {
		t.Fatalf("RemapMonthPositions: %v", err)
	}
	if n != 3 {
		t.Errorf("rows changed = %d, want 3 (both swapped nights and the soft-deleted one)", n)
	}
	month := func(id string) int {
		t.Helper()
		var m int
		if err := db.QueryRow(`SELECT calendar_month FROM sessions WHERE id = ?`, id).Scan(&m); err != nil {
			t.Fatalf("read month: %v", err)
		}
		return m
	}
	for _, c := range []struct {
		name string
		id   string
		want int
	}{
		{"3 -> 4", three, 4}, {"4 -> 3", four, 3}, {"unmoved month", five, 5},
		{"soft-deleted follows its month", deleted, 4},
		{"other calendar untouched", onB, 3}, {"other campaign untouched", elsewhere, 3},
	} {
		if got := month(c.id); got != c.want {
			t.Errorf("%s: month = %d, want %d", c.name, got, c.want)
		}
	}
}
