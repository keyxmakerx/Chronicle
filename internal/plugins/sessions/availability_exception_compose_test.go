package sessions

// POST /campaigns/:id/availability/exceptions must MARK A WINDOW, not replace
// the day.
//
// THE DEFECT: AddMyException inserted one row via repo.AddException. Exception
// rows fully REPLACE the recurring pattern for their date (effectiveBlocks),
// so that single row became the member's entire day. A member whose recurring
// Tuesday was 09:00–23:00 and who posted "I'm ALSO free 07:00–08:00" ended up
// available for one hour at 7am and busy every evening — fourteen hours gone
// from the Director's overlay and from the derived best-window, with their own
// grid still showing 09:00–23:00 so nothing on screen told them.
//
// The compose-the-day rule that prevents this already existed; it had been
// applied to the RSVP-offer path and to the client-side editor, and never to
// this endpoint, which is a documented Player+ route.

import (
	"context"
	"testing"
	"time"
)

// aTuesday returns a Tuesday inside the today±1y exception window.
func aTuesday(t *testing.T) string {
	t.Helper()
	d := time.Now().UTC().AddDate(0, 0, 14)
	d = d.AddDate(0, 0, (int(time.Tuesday)-int(d.Weekday())+7)%7)
	return d.Format("2006-01-02")
}

// TestAddMyException_KeepsTheRestOfTheDay is the headline regression, measured
// on the rows the endpoint writes.
func TestAddMyException_KeepsTheRestOfTheDay(t *testing.T) {
	date := aTuesday(t)
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 9 * 60, EndMinute: 23 * 60,
		State: AvailAvailable, TZ: "UTC",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)

	// "I'm ALSO free 07:00–08:00."
	if err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: date, StartMinute: 7 * 60, EndMinute: 8 * 60, State: AvailAvailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want one day written, got %d: %+v", len(written), written)
	}
	got := written[0].rows
	if len(got) != 2 {
		t.Fatalf("composed day = %+v, want the added hour AND the member's usual 09:00–23:00", got)
	}
	if got[0].StartMinute != 7*60 || got[0].EndMinute != 8*60 {
		t.Errorf("first block = %d..%d, want 420..480", got[0].StartMinute, got[0].EndMinute)
	}
	if got[1].StartMinute != 9*60 || got[1].EndMinute != 23*60 {
		t.Errorf("second block = %d..%d, want the untouched 540..1380 — the rest of the "+
			"member's day was erased", got[1].StartMinute, got[1].EndMinute)
	}
}

// TestAddMyException_UnavailablePunchesAHoleNotAWholeDay is the other direction:
// "I'm busy 19:00–20:00" must leave the member available either side.
func TestAddMyException_UnavailablePunchesAHoleNotAWholeDay(t *testing.T) {
	date := aTuesday(t)
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 23 * 60,
		State: AvailAvailable, TZ: "UTC",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)

	if err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: date, StartMinute: 19 * 60, EndMinute: 20 * 60, State: AvailUnavailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want one day written, got %+v", written)
	}
	want := []struct {
		start, end int
		state      string
	}{
		{18 * 60, 19 * 60, AvailAvailable},
		{19 * 60, 20 * 60, AvailUnavailable},
		{20 * 60, 23 * 60, AvailAvailable},
	}
	got := written[0].rows
	if len(got) != len(want) {
		t.Fatalf("composed day = %+v, want three blocks (available / busy / available)", got)
	}
	for i, w := range want {
		if got[i].StartMinute != w.start || got[i].EndMinute != w.end || got[i].State != w.state {
			t.Errorf("block %d = %d..%d %s, want %d..%d %s",
				i, got[i].StartMinute, got[i].EndMinute, got[i].State, w.start, w.end, w.state)
		}
	}
}

// TestAddMyException_OverridesAnExplicitPreference pins the keepPreferred=false
// choice: unlike a generic RSVP offer, this endpoint is the member SAYING what
// a window is, so it must be able to overwrite an earlier "preferred".
func TestAddMyException_OverridesAnExplicitPreference(t *testing.T) {
	date := aTuesday(t)
	existing := []AvailabilityException{{
		OnDate: date, StartMinute: 19 * 60, EndMinute: 21 * 60, State: AvailPreferred, TZ: "UTC",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(nil, existing, &written)

	if err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: date, StartMinute: 19 * 60, EndMinute: 20 * 60, State: AvailUnavailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}
	got := written[0].rows
	if len(got) != 2 || got[0].State != AvailUnavailable || got[1].State != AvailPreferred {
		t.Fatalf("composed day = %+v, want 19–20 busy then 20–21 still preferred", got)
	}
}

// TestAddMyException_KeepsTheAuthoredZone applies the same zone rule the RSVP
// offer path uses: the day is written in the zone its own rows were authored in
// and the incoming window is CONVERTED, never the other way round.
func TestAddMyException_KeepsTheAuthoredZone(t *testing.T) {
	// A Tuesday in the member's New York zone.
	date := aTuesday(t)
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)

	// A client posting in UTC: 23:00–23:30 UTC on that Tuesday is 19:00–19:30
	// New York, inside the member's evening.
	if err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: date, StartMinute: 23 * 60, EndMinute: 23*60 + 30, State: AvailUnavailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want one day written, got %+v", written)
	}
	if written[0].tz != "America/New_York" {
		t.Fatalf("day written with tz %q — the caller's zone was stamped over the member's own",
			written[0].tz)
	}
	if written[0].onDate != date {
		t.Fatalf("day written = %s, want %s (23:00 UTC is still the same evening in New York)",
			written[0].onDate, date)
	}
	var sawHole bool
	for _, r := range written[0].rows {
		if r.State == AvailUnavailable && r.StartMinute == 19*60 && r.EndMinute == 19*60+30 {
			sawHole = true
		}
	}
	if !sawHole {
		t.Errorf("the busy window was not converted into the member's zone: %+v", written[0].rows)
	}
}

// TestDB_AddExceptionKeepsTheRestOfTheDay is the row-level statement of the same
// fix, against real MariaDB — because the original claim was about rows and
// about what the projection then reports, and a mock cannot answer either.
func TestDB_AddExceptionKeepsTheRestOfTheDay(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, userID := seedCampaign(t, db)
	svc := NewSessionService(NewSessionRepository(db), nil)
	ctx := context.Background()

	// The member's usual Tuesday, painted in the grid.
	if err := svc.SaveMyAvailability(ctx, campID, userID, SaveAvailabilityRequest{
		TZ: "UTC",
		Blocks: []AvailabilityBlockDTO{
			{DayOfWeek: int(time.Tuesday), StartMinute: 9 * 60, EndMinute: 23 * 60, State: AvailAvailable},
		},
	}); err != nil {
		t.Fatalf("SaveMyAvailability: %v", err)
	}

	date := aTuesday(t)
	if err := svc.AddMyException(ctx, campID, userID, AddExceptionRequest{
		OnDate: date, StartMinute: 7 * 60, EndMinute: 8 * 60, State: AvailAvailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM availability_exceptions
	    WHERE campaign_id=? AND user_id=? AND on_date=?`, campID, userID, date).Scan(&n); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if n != 2 {
		t.Errorf("availability_exceptions holds %d row(s) for %s, want 2 "+
			"(the added hour AND the member's usual evening)", n, date)
	}

	// What the Director actually sees.
	overlay, err := svc.BuildOverlay(ctx, campID,
		[]overlayMemberInput{{UserID: userID, Name: "Ana"}}, date, "UTC", true)
	if err != nil {
		t.Fatalf("BuildOverlay: %v", err)
	}
	var freeAt7, freeAt20 int
	for _, day := range overlay.Days {
		if day.Date == date {
			freeAt7 = day.Hours[7].Free
			freeAt20 = day.Hours[20].Free
		}
	}
	if freeAt7 != 1 {
		t.Errorf("free at 07:00 = %d, want 1 — the added window is missing", freeAt7)
	}
	if freeAt20 != 1 {
		t.Errorf("free at 20:00 = %d, want 1 — the member's recurring evening was deleted "+
			"by adding one morning hour", freeAt20)
	}
}
