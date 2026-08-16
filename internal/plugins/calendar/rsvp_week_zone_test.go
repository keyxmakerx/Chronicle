package calendar

// "Out this week" must resolve THE MEMBER'S week.
//
// THE DEFECT: rsvpWeekDates anchored on now.UTC() (or on the event's Y/M/D read
// as UTC), so the redeeming member's own zone never entered the calculation —
// and the emailed link is a one-tap action with no week picker. A Pacific/
// Auckland player tapping it over Monday breakfast (2026-08-17 08:30 local =
// 2026-08-16 20:30 UTC) blocked 2026-08-10..2026-08-16, the week that had
// already ended, and stayed fully available for the week they meant. Mirror
// image for Pacific/Pago_Pago late on a Sunday: 2026-08-16 22:00 local =
// 2026-08-17 09:00 UTC, so it blocked a week EARLY. Seven availability_exception
// rows landed on days they never intended, the days they did intend stayed open
// in the Director's overlay, and there is no undo on that page.
//
// The old test could not see any of this: it pinned both branches at 15:04 UTC,
// a moment that is the same civil day in every zone on earth.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRSVPWeekDates_ResolvesTheMembersWeekNotUTC walks the two edges where the
// member's civil date and UTC's disagree.
func TestRSVPWeekDates_ResolvesTheMembersWeekNotUTC(t *testing.T) {
	fantasy := testCalendar(nil) // no real-time mapping: anchor is "now"
	evt := testEvent(nil)

	for _, tc := range []struct {
		name        string
		zone        string
		localMoment string // the member's own wall clock when they tap the link
		wantMonday  string
	}{
		// Monday breakfast in Auckland is still Sunday evening in UTC.
		{"Auckland Monday breakfast", "Pacific/Auckland", "2026-08-17 08:30", "2026-08-17"},
		// Sunday night in Pago Pago is already Monday in UTC.
		{"Pago Pago Sunday night", "Pacific/Pago_Pago", "2026-08-16 22:00", "2026-08-10"},
		// A zone where the two agree must be unchanged.
		{"London midweek", "Europe/London", "2026-08-19 12:00", "2026-08-17"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("tzdata missing for %s", tc.zone)
			}
			local, err := time.ParseInLocation("2006-01-02 15:04", tc.localMoment, loc)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			monday, dates := rsvpWeekDates(fantasy, evt, local.UTC(), tc.zone)
			if monday != tc.wantMonday {
				t.Errorf("member in %s tapping at %s local: blocked the week of %s, "+
					"but their own week starts %s", tc.zone, tc.localMoment, monday, tc.wantMonday)
			}
			if len(dates) != 7 || dates[0] != tc.wantMonday {
				t.Errorf("week = %v, want 7 days from %s", dates, tc.wantMonday)
			}
			// And the member's own local date must be inside the blocked week —
			// the whole point of "this week".
			today := local.Format("2006-01-02")
			var covered bool
			for _, d := range dates {
				if d == today {
					covered = true
				}
			}
			if !covered {
				t.Errorf("the member's own today (%s) is not in the blocked week %v", today, dates)
			}
		})
	}
}

// TestRSVPWeekDates_NoZoneFallsBackToUTC pins the honest fallback: a member who
// has set no zone anywhere gets the UTC reading (there is nothing else), and the
// caller discloses it — see TestApplyOutThisWeek_NamesTheZone.
func TestRSVPWeekDates_NoZoneFallsBackToUTC(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	monday, _ := rsvpWeekDates(testCalendar(nil), testEvent(nil), now, "")
	if monday != "2026-08-17" {
		t.Errorf("no-zone fallback monday = %q, want the UTC week 2026-08-17", monday)
	}
	// An unknown zone string must not panic or silently shift the week either.
	badMonday, _ := rsvpWeekDates(testCalendar(nil), testEvent(nil), now, "Mars/Olympus_Mons")
	if badMonday != monday {
		t.Errorf("an unresolvable zone changed the week: %q vs %q", badMonday, monday)
	}
}

// TestApplyOutThisWeek_WritesTheMembersWeek is the end-to-end statement: the
// handler must reach the member's zone and write THAT week's dates.
func TestApplyOutThisWeek_WritesTheMembersWeek(t *testing.T) {
	if _, err := time.LoadLocation("Pacific/Auckland"); err != nil {
		t.Skip("tzdata missing for Pacific/Auckland")
	}
	restore := nowUTC
	// 2026-08-17 08:30 Auckland.
	nowUTC = func() time.Time { return time.Date(2026, 8, 16, 20, 30, 0, 0, time.UTC) }
	defer func() { nowUTC = restore }()

	avail := &mockAvailability{zone: "Pacific/Auckland"}
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	h.SetAvailabilityWriter(avail)

	msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "player-9")

	if len(avail.written) != 7 {
		t.Fatalf("expected all 7 days written, got %d: %v", len(avail.written), avail.written)
	}
	if avail.written[0] != "2026-08-17" {
		t.Errorf("blocked the week of %s; the member's Monday is 2026-08-17 — they tapped the "+
			"link over Monday breakfast and got last week", avail.written[0])
	}
	if !strings.Contains(msg, "2026-08-17") {
		t.Errorf("the message names a different week from the one written: %q", msg)
	}
	if !strings.Contains(msg, "Pacific/Auckland") {
		t.Errorf("the message does not name the zone the week was resolved in: %q", msg)
	}
}

// TestApplyOutThisWeek_NamesTheZone pins the disclosure in both directions. A
// week label reads as a fact; the member has no other way to learn it was a UTC
// reading of their Monday morning.
func TestApplyOutThisWeek_NamesTheZone(t *testing.T) {
	restore := nowUTC
	nowUTC = func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = restore }()

	for _, tc := range []struct {
		name, zone string
		wantIn     []string
	}{
		{"zone set", "Europe/London", []string{"Europe/London"}},
		{"no zone anywhere", "", []string{"UTC", "haven't set a timezone"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			avail := &mockAvailability{zone: tc.zone}
			h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
			h.SetAvailabilityWriter(avail)
			msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "player-9")
			for _, want := range tc.wantIn {
				if !strings.Contains(msg, want) {
					t.Errorf("message %q does not contain %q", msg, want)
				}
			}
		})
	}
}
