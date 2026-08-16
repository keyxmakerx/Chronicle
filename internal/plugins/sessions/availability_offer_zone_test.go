package sessions

// The ZONE half of the offered-availability contract (the compose-the-day half
// lives in availability_offer_test.go).
//
// THE DEFECT THESE PIN: AddMyAvailableWindows composes a whole day from the
// member's own rows and writes it back through ReplaceMyDayExceptions, which
// stamps ONE zone on every row. It used to stamp the CALLER'S zone —
// users.timezone, "UTC" whenever the member never set an account zone, which is
// the default — while the minutes on the canvas had been authored in the zone
// the availability page's "Your timezone" control writes into
// member_availability.tz and nowhere else.
//
// The two are one click apart, so a member who painted 18:00–22:00 from a New
// York browser and then answered "Suggest another time" in an RSVP email had
// their whole stated evening RELABELLED as UTC: same minute numbers, four hours
// earlier in real time, showing in the Director's overlay as free during their
// working afternoon and busy during the hours they actually painted. They never
// edited anything and nothing on screen said so.

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// zoneWrite records one ReplaceMyDayExceptions call.
type zoneWrite struct {
	onDate string
	tz     string
	rows   []AvailabilityException
}

// offerSvcWithRows wires a service whose repo returns the given stored rows and
// records every day-replace the offer produces.
func offerSvcWithRows(recurring []AvailabilityBlock, exc []AvailabilityException, out *[]zoneWrite) *sessionService {
	return &sessionService{repo: &mockSessionRepo{
		listUserAvailabilityFn: func(ctx context.Context, campaignID, userID string) ([]AvailabilityBlock, error) {
			return recurring, nil
		},
		listUserExceptionsFn: func(ctx context.Context, campaignID, userID string) ([]AvailabilityException, error) {
			return exc, nil
		},
		countUserExceptionsFn: func(ctx context.Context, campaignID, userID string) (int, error) {
			return len(exc), nil
		},
		replaceDayExceptionsFn: func(ctx context.Context, campaignID, userID, onDate string, excs []AvailabilityException) error {
			tz := ""
			if len(excs) > 0 {
				tz = excs[0].TZ
			}
			*out = append(*out, zoneWrite{onDate: onDate, tz: tz, rows: excs})
			return nil
		},
	}}
}

// TestOfferedWindows_KeepsTheZoneTheMembersRowsWereAuthoredIn is the headline
// regression. Production shape exactly: recurring pattern painted in
// America/New_York, account zone unset so the adapter passes "UTC".
func TestOfferedWindows_KeepsTheZoneTheMembersRowsWereAuthoredIn(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)

	// 22:00–23:00 UTC on the Tuesday = 18:00–19:00 New York, inside the hours
	// the member already offered.
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "UTC",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 22 * 60, EndMinute: 23 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 day written, got %d: %+v", len(written), written)
	}
	w := written[0]
	if w.tz != "America/New_York" {
		t.Fatalf("composed day written with tz %q — the caller's zone was stamped over the "+
			"zone the member's own rows were authored in (%q)", w.tz, "America/New_York")
	}
	if w.onDate != "2026-08-04" {
		t.Errorf("day written = %s, want 2026-08-04", w.onDate)
	}
	// The member's own 18:00–22:00 must still be there, unmoved, and the offer
	// must have been CONVERTED into their zone rather than pasted in raw. Raw
	// 22:00–23:00 would show up as a second block (1320..1380).
	if len(w.rows) != 1 || w.rows[0].StartMinute != 18*60 || w.rows[0].EndMinute != 22*60 {
		t.Fatalf("composed rows = %+v, want a single 1080..1320 block (the offer converts to "+
			"18:00–19:00 NY, which is already inside the member's evening)", w.rows)
	}
}

// TestOfferedWindows_DoNotMoveTheMemberInTheDirectorsOverlay measures the same
// fix where the operator actually sees it: the projection. Before and after the
// offer the member must be free at the SAME REAL HOURS.
//
// This is the assertion that would still catch the defect if someone "fixed"
// the zone field alone without converting the offer.
func TestOfferedWindows_DoNotMoveTheMemberInTheDirectorsOverlay(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "UTC",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 22 * 60, EndMinute: 23 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}

	member := []overlayMemberInput{{UserID: "u1", Name: "Ana"}}
	weekStart, err := timeutil.ParseCivilDate("2026-08-03") // Monday
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	before := buildWeekOverlay(member,
		map[string][]AvailabilityBlock{"u1": recurring}, nil, weekStart, time.UTC, "UTC", true)

	var after WeekOverlay
	{
		rows := make([]AvailabilityException, 0, 4)
		for _, w := range written {
			for _, r := range w.rows {
				r.OnDate = w.onDate
				rows = append(rows, r)
			}
		}
		after = buildWeekOverlay(member, map[string][]AvailabilityBlock{"u1": recurring},
			map[string][]AvailabilityException{"u1": rows}, weekStart, time.UTC, "UTC", true)
	}

	freeSet := func(o WeekOverlay) map[int]bool {
		out := map[int]bool{}
		for col, d := range o.Days {
			for h, cell := range d.Hours {
				if cell.Free > 0 {
					out[col*100+h] = true
				}
			}
		}
		return out
	}
	b, a := freeSet(before), freeSet(after)

	// 18:00–22:00 New York on Tuesday = 22:00 Tue UTC through 02:00 Wed UTC.
	for _, key := range []int{1*100 + 22, 1*100 + 23, 2*100 + 0, 2*100 + 1} {
		if !b[key] {
			t.Fatalf("setup wrong: expected free at col*100+hour %d BEFORE the offer", key)
		}
		if !a[key] {
			t.Errorf("the member stopped being free at col*100+hour %d after offering a window "+
				"INSIDE hours they had already given — their evening was relabelled", key)
		}
	}
	// And they must NOT have acquired their working afternoon.
	if a[1*100+18] && !b[1*100+18] {
		t.Errorf("the member became free at 18:00 UTC Tuesday (14:00 their time) — " +
			"their hours moved four hours earlier in real time")
	}
}

// TestOfferedWindows_ConvertAcrossAMidnightBoundary covers the case the
// conversion makes possible: an offer whose real hours fall on a DIFFERENT
// calendar date in the member's zone. The window has to be written on the date
// it really lands on, and the member's other day must not be touched.
func TestOfferedWindows_ConvertAcrossAMidnightBoundary(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)

	// 02:00–04:00 UTC on Wednesday 2026-08-05 = 22:00–00:00 New York on
	// TUESDAY 2026-08-04.
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "UTC",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-05", StartMinute: 2 * 60, EndMinute: 4 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 day written, got %d: %+v", len(written), written)
	}
	if written[0].onDate != "2026-08-04" {
		t.Fatalf("window written to %s; in the member's own zone those hours are the "+
			"evening of 2026-08-04", written[0].onDate)
	}
	if written[0].tz != "America/New_York" {
		t.Errorf("tz = %q, want America/New_York", written[0].tz)
	}
	// Their Tuesday evening extended to midnight: 18:00 → 24:00.
	if len(written[0].rows) != 1 || written[0].rows[0].StartMinute != 18*60 ||
		written[0].rows[0].EndMinute != timeutil.MinutesPerDay {
		t.Errorf("composed rows = %+v, want one 1080..1440 block", written[0].rows)
	}
}

// TestOfferedWindows_NoStoredZoneUsesTheCallers pins the no-regression case: a
// member with no rows at all has no authored zone, so the caller's is the only
// honest answer and the offer is written verbatim.
func TestOfferedWindows_NoStoredZoneUsesTheCallers(t *testing.T) {
	var written []zoneWrite
	svc := offerSvcWithRows(nil, nil, &written)
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "Europe/London",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 19 * 60, EndMinute: 21 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 || written[0].tz != "Europe/London" {
		t.Fatalf("want the caller's zone on a member with nothing stored, got %+v", written)
	}
	if written[0].rows[0].StartMinute != 19*60 || written[0].rows[0].EndMinute != 21*60 {
		t.Errorf("offer was converted when there was nothing to convert against: %+v", written[0].rows)
	}
}

// TestOfferedWindows_SameZoneIsUnchanged is the "did the fix disturb the common
// path" pin: when the offer's zone already equals the member's, nothing shifts.
func TestOfferedWindows_SameZoneIsUnchanged(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, nil, &written)
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "America/New_York",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 22 * 60, EndMinute: 23 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 || written[0].onDate != "2026-08-04" || written[0].tz != "America/New_York" {
		t.Fatalf("unexpected write: %+v", written)
	}
	if len(written[0].rows) != 1 || written[0].rows[0].StartMinute != 18*60 || written[0].rows[0].EndMinute != 23*60 {
		t.Errorf("composed rows = %+v, want one 1080..1380 block (18:00–23:00 local)", written[0].rows)
	}
}

// TestOfferedWindows_DayWithExistingExceptionsKeepsTHATZone pins the precedence
// rule: exception rows for a date REPLACE the recurring pattern, so those rows
// are the day and the day must be rewritten in their zone — not in the pattern's.
func TestOfferedWindows_DayWithExistingExceptionsKeepsTHATZone(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	// The member already hand-authored this Tuesday from a London browser.
	existing := []AvailabilityException{{
		OnDate: "2026-08-04", StartMinute: 20 * 60, EndMinute: 21 * 60,
		State: AvailPreferred, TZ: "Europe/London",
	}}
	var written []zoneWrite
	svc := offerSvcWithRows(recurring, existing, &written)

	// Offer 19:00–20:00 Europe/London, same zone the day is authored in.
	if err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "Europe/London",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 19 * 60, EndMinute: 20 * 60}}); err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 day written, got %+v", written)
	}
	if written[0].tz != "Europe/London" {
		t.Fatalf("day written with tz %q; the date's own exception rows are authored in "+
			"Europe/London and they ARE the day", written[0].tz)
	}
	// The preferred hour survives the generic offer (paint keeps preferred).
	var sawPreferred bool
	for _, r := range written[0].rows {
		if r.State == AvailPreferred && r.StartMinute == 20*60 && r.EndMinute == 21*60 {
			sawPreferred = true
		}
	}
	if !sawPreferred {
		t.Errorf("the member's explicit preference was lost: %+v", written[0].rows)
	}
}
