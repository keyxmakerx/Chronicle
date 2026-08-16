package sessions

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// This file guards the overlay projection against the DST-midnight zones —
// the ones where local 00:00 does not exist on the transition day, because the
// clocks jump straight from 00:00 to 01:00.
//
// Before the fix, buildWeekOverlay could not RETURN for a viewer in one of
// those zones on the transition week: splitToViewerDays computed the end of the
// local day as `time.Date(y,mo,d,0,0,0,0,loc).AddDate(0,0,1)`, Go normalised the
// nonexistent midnight backwards to 23:00 of the previous day, AddDate landed on
// 23:00 of the SAME day, and the loop's only advance (`cur = segEnd`) assigned
// cur the value it already held — while appending a LaneSegment every iteration.
//
// Reachability was not theoretical: GET /campaigns/:id/availability/overlay
// takes an arbitrary ?tz= from any Player, and the Bench / /schedule project in
// the viewer's own stored profile zone, so it also fired by accident twice a
// year for Cuban, Chilean and Azorean tables. Measured live at 41 MB → 2.6 GB
// RSS in ten seconds, still climbing after the client disconnected.
//
// EVERY TEST HERE RUNS THE PROJECTION UNDER A DEADLINE. A plain call would hang
// the test binary instead of failing it, and a suite that hangs gets its
// timeout raised, not its bug fixed.

// dstMidnightZones are the zones (in the shipped tzdata) whose DST transition
// lands on midnight in the 2026-2028 window, plus the ordinary zones that must
// keep working exactly as before.
var dstMidnightZones = []struct {
	zone string
	// weekStart is a Monday; the transition falls inside that week.
	weekStart string
	// transition is the local date on which midnight does not exist.
	transition string
}{
	{"America/Havana", "2026-03-02", "2026-03-08"},
	{"America/Santiago", "2026-08-31", "2026-09-06"},
	{"Atlantic/Azores", "2026-03-23", "2026-03-29"},
}

// runOverlayWithDeadline calls buildWeekOverlay on a watchdog. It returns the
// overlay, or fails the test with a diagnosis if the projection did not return.
func runOverlayWithDeadline(t *testing.T, d time.Duration, fn func() WeekOverlay) WeekOverlay {
	t.Helper()
	done := make(chan WeekOverlay, 1)
	go func() { done <- fn() }()
	select {
	case o := <-done:
		return o
	case <-time.After(d):
		t.Fatalf("buildWeekOverlay did not return within %v — the viewer-day split is not "+
			"terminating. In production this goroutine keeps ALLOCATING until the OOM killer "+
			"takes the whole instance out, and it does not stop when the client disconnects.", d)
		return WeekOverlay{}
	}
}

// TestOverlay_TerminatesInMidnightJumpZones is the direct regression: a member
// free 22:00–24:00 local on the transition Sunday, rendered for a viewer in the
// same zone. This is the exact shape the live repro used.
func TestOverlay_TerminatesInMidnightJumpZones(t *testing.T) {
	for _, tc := range dstMidnightZones {
		t.Run(tc.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("tzdata missing for %s", tc.zone)
			}
			trans, err := time.Parse("2006-01-02", tc.transition)
			if err != nil {
				t.Fatalf("bad transition date: %v", err)
			}
			weekStart, err := timeutil.ParseCivilDate(tc.weekStart)
			if err != nil {
				t.Fatalf("bad week start: %v", err)
			}

			overlay := runOverlayWithDeadline(t, 10*time.Second, func() WeekOverlay {
				return buildWeekOverlay(
					[]overlayMemberInput{{UserID: "u1", Name: "Ana"}},
					map[string][]AvailabilityBlock{"u1": {{
						DayOfWeek: int(trans.Weekday()), StartMinute: 22 * 60, EndMinute: 24 * 60,
						State: AvailAvailable, TZ: tc.zone,
					}}}, nil, weekStart, loc, tc.zone, true)
			})

			// Not just "it returned" — it has to have rendered the member's
			// evening. A fuse that returns an EMPTY overlay would satisfy a
			// termination-only assertion while silently dropping availability.
			var col = -1
			for i, d := range overlay.Days {
				if d.Date == tc.transition {
					col = i
				}
			}
			if col < 0 {
				t.Fatalf("the transition date %s is not one of the 7 columns (%v)", tc.transition, overlay.Days[0].Date)
			}
			if got := overlay.Days[col].Hours[22].Free; got != 1 {
				t.Errorf("free at 22:00 on %s = %d, want 1 — the member's evening was dropped", tc.transition, got)
			}
			if got := overlay.Days[col].Hours[23].Free; got != 1 {
				t.Errorf("free at 23:00 on %s = %d, want 1 — the member's evening was dropped", tc.transition, got)
			}
			if len(overlay.Members) != 1 || len(overlay.Members[0].Lanes) == 0 {
				t.Fatalf("no lanes rendered for the member on the transition week")
			}
			// The runaway loop appended one LaneSegment per iteration. A member
			// with one weekly block across a 7-day window can never legitimately
			// produce a large lane list; this catches "terminated, but only
			// because of the fuse".
			if n := len(overlay.Members[0].Lanes); n > 4 {
				t.Errorf("member has %d lane segments for a single weekly block — "+
					"the split is emitting spurious segments", n)
			}
		})
	}
}

// TestOverlay_HostileViewerZoneCannotWedgeTheRequest is the security-shaped
// half: ?tz= is attacker-chosen (any Player, sessions/routes.go RolePlayer) and
// ?week= is too. Every hour of the transition day is walked as a candidate
// block so no single lucky offset can pass for coverage.
func TestOverlay_HostileViewerZoneCannotWedgeTheRequest(t *testing.T) {
	for _, tc := range dstMidnightZones {
		loc, err := time.LoadLocation(tc.zone)
		if err != nil {
			t.Skipf("tzdata missing for %s", tc.zone)
		}
		trans, _ := time.Parse("2006-01-02", tc.transition)
		weekStart, _ := timeutil.ParseCivilDate(tc.weekStart)

		for startHour := 0; startHour < 24; startHour++ {
			for _, span := range []int{1, 2, 6, 24} {
				endMin := (startHour + span) * 60
				if endMin > timeutil.MinutesPerDay {
					endMin = timeutil.MinutesPerDay
				}
				if endMin <= startHour*60 {
					continue
				}
				overlay := runOverlayWithDeadline(t, 10*time.Second, func() WeekOverlay {
					return buildWeekOverlay(
						[]overlayMemberInput{{UserID: "u1", Name: "Ana"}},
						map[string][]AvailabilityBlock{"u1": {{
							DayOfWeek: int(trans.Weekday()), StartMinute: startHour * 60, EndMinute: endMin,
							State: AvailAvailable, TZ: tc.zone,
						}}}, nil, weekStart, loc, tc.zone, true)
					// A hostile caller can also cross zones; the member's own
					// zone above is the innocent case, covered below.
				})
				if len(overlay.Days) != 7 {
					t.Fatalf("%s %02d:00+%dh: overlay came back malformed", tc.zone, startHour, span)
				}
			}
		}

		// Cross-zone: member elsewhere, viewer in the midnight-jump zone. This
		// is the Bench/`/schedule` shape — the viewer zone is the profile zone,
		// the members are wherever they are.
		overlay := runOverlayWithDeadline(t, 10*time.Second, func() WeekOverlay {
			return buildWeekOverlay(
				[]overlayMemberInput{{UserID: "u1", Name: "Ana"}, {UserID: "u2", Name: "Bo"}},
				map[string][]AvailabilityBlock{
					"u1": {{DayOfWeek: int(trans.Weekday()), StartMinute: 0, EndMinute: 24 * 60,
						State: AvailAvailable, TZ: "Pacific/Kiritimati"}},
					"u2": {{DayOfWeek: int(trans.Weekday()), StartMinute: 0, EndMinute: 24 * 60,
						State: AvailPreferred, TZ: "Pacific/Pago_Pago"}},
				}, nil, weekStart, loc, tc.zone, true)
		})
		if len(overlay.Days) != 7 {
			t.Fatalf("%s cross-zone: overlay came back malformed", tc.zone)
		}
	}
}

// TestSplitToViewerDays_AdvancesAcrossAMidnightJump measures the unit directly
// so a regression is diagnosed in one line rather than as "the suite hangs".
func TestSplitToViewerDays_AdvancesAcrossAMidnightJump(t *testing.T) {
	loc, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skip("tzdata missing for America/Havana")
	}
	// 22:00 on 2026-03-08 through local midnight (= 00:00 on the 9th).
	start := timeutil.WallClockInstant(loc, 2026, time.March, 8, 22*60)
	end := timeutil.WallClockInstant(loc, 2026, time.March, 8, 24*60)

	type res struct{ segs []viewerSeg }
	done := make(chan res, 1)
	go func() { done <- res{splitToViewerDays(start, end, loc)} }()
	var segs []viewerSeg
	select {
	case r := <-done:
		segs = r.segs
	case <-time.After(5 * time.Second):
		t.Fatal("splitToViewerDays did not return — cur is not advancing past the midnight jump")
	}

	if len(segs) != 1 {
		t.Fatalf("want 1 segment (the block ends exactly at local midnight), got %d: %+v", len(segs), segs)
	}
	if segs[0].date != "2026-03-08" {
		t.Errorf("segment date = %s, want 2026-03-08", segs[0].date)
	}
	if segs[0].startMin != 22*60 || segs[0].endMin != timeutil.MinutesPerDay {
		t.Errorf("segment = %d..%d, want %d..%d", segs[0].startMin, segs[0].endMin, 22*60, timeutil.MinutesPerDay)
	}
}

// TestSplitToViewerDays_CrossesTheJumpDayBoundary covers the case that actually
// STRADDLES the boundary: a block starting the evening before the transition
// and running into the transition day. The second segment must start at the
// hour the local day really begins (01:00 in Havana — 00:00 does not exist),
// not at 00:00.
func TestSplitToViewerDays_CrossesTheJumpDayBoundary(t *testing.T) {
	loc, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skip("tzdata missing for America/Havana")
	}
	// 2026-03-07 23:00 local through 2026-03-08 03:00 local.
	start := timeutil.WallClockInstant(loc, 2026, time.March, 7, 23*60)
	end := timeutil.WallClockInstant(loc, 2026, time.March, 8, 3*60)

	done := make(chan []viewerSeg, 1)
	go func() { done <- splitToViewerDays(start, end, loc) }()
	var segs []viewerSeg
	select {
	case segs = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("splitToViewerDays did not return across the jump-day boundary")
	}

	if len(segs) != 2 {
		t.Fatalf("want 2 segments (one per local day), got %d: %+v", len(segs), segs)
	}
	if segs[0].date != "2026-03-07" || segs[0].startMin != 23*60 || segs[0].endMin != timeutil.MinutesPerDay {
		t.Errorf("first segment = %+v, want 2026-03-07 %d..%d", segs[0], 23*60, timeutil.MinutesPerDay)
	}
	// The local day begins at 01:00 because 00:00 does not exist. Reporting 0
	// here would claim the member was free during an hour the clock skipped.
	if segs[1].date != "2026-03-08" || segs[1].startMin != 60 || segs[1].endMin != 3*60 {
		t.Errorf("second segment = %+v, want 2026-03-08 60..180 (the day starts at 01:00)", segs[1])
	}
}
