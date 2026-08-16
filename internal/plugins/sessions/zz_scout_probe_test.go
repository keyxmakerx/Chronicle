package sessions

// SCOUTING PROBES — not fixes, not guards. Each test here asserts the CURRENT
// (suspected-wrong) behaviour so the report can point at a green/red run rather
// than at an argument. Delete or re-point when the defects are fixed.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// --- fake repo just wide enough for AddMyAvailableWindows -------------------

type probeWrite struct {
	onDate string
	rows   []AvailabilityException
}

// probeOfferRepo wires the existing mockSessionRepo (service_test.go) with just
// the four methods AddMyAvailableWindows touches, and records the writes.
func probeOfferRepo(recurring []AvailabilityBlock, exc []AvailabilityException, out *[]probeWrite) *mockSessionRepo {
	return &mockSessionRepo{
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
			*out = append(*out, probeWrite{onDate: onDate, rows: excs})
			return nil
		},
	}
}

// PROBE 1 — AddMyAvailableWindows re-labels the member's EXISTING hours with the
// caller-supplied zone.
//
// Setup mirrors production exactly: the member painted their weekly grid from a
// New York browser (member_availability.tz = America/New_York, Tue 18:00–22:00
// local). They then click "Suggest another time" in an RSVP email and offer one
// window. calendarAvailabilityAdapter.OfferAvailableWindows passes
// users.timezone, which is NULL for this member, so the tz that arrives is
// "UTC".
//
// The claim: the rows written back carry tz="UTC" while still carrying the
// New-York MINUTE NUMBERS, i.e. the member's usual Tuesday evening silently
// moves 4 hours earlier in real time.
func TestProbe_OfferedWindowsRelabelExistingHoursIntoTheCallersZone(t *testing.T) {
	recurring := []AvailabilityBlock{{
		DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "America/New_York",
	}}
	var written []probeWrite
	svc := &sessionService{repo: probeOfferRepo(recurring, nil, &written)}

	// The adapter's UTC fallback (users.timezone IS NULL).
	err := svc.AddMyAvailableWindows(context.Background(), "camp-1", "u1", "UTC",
		[]AvailabilityWindowDTO{{OnDate: "2026-08-04", StartMinute: 22 * 60, EndMinute: 23 * 60}})
	if err != nil {
		t.Fatalf("AddMyAvailableWindows: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 day written, got %d", len(written))
	}
	w := written[0]
	t.Logf("rows written for %s:", w.onDate)
	for _, r := range w.rows {
		t.Logf("  %d..%d state=%s tz=%q", r.StartMinute, r.EndMinute, r.State, r.TZ)
	}

	// The composed block covers the member's original 18:00 start...
	if w.rows[0].StartMinute != 18*60 {
		t.Fatalf("composed start = %d, want %d (the member's own recurring start)", w.rows[0].StartMinute, 18*60)
	}
	// ...but the zone it is now expressed in is the CALLER'S, not the one the
	// minutes were authored in. THIS IS THE DEFECT.
	if w.rows[0].TZ != "UTC" {
		t.Fatalf("probe stale: composed row tz = %q, expected the caller's %q", w.rows[0].TZ, "UTC")
	}
	if w.rows[0].TZ == recurring[0].TZ {
		t.Fatalf("no relabel happened — nothing to report")
	}
	t.Logf("CONFIRMED: block authored in %q rewritten as %q with the same minute numbers",
		recurring[0].TZ, w.rows[0].TZ)
}

// PROBE 1b — the same relabel, measured where the operator sees it: the DM's
// week overlay. Before the offer the member is free 18:00–22:00 New York
// (= 22:00–02:00 UTC). After it they are free 18:00–22:00 UTC
// (= 14:00–18:00 New York). Same member, same day, no edit by them.
func TestProbe_RelabelMovesTheMemberInTheDMOverlay(t *testing.T) {
	member := []overlayMemberInput{{UserID: "u1", Name: "Ana"}}
	weekStart, err := timeutil.ParseCivilDate("2026-08-03") // Monday
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	utc := time.UTC

	// BEFORE: recurring Tue 18:00–22:00 America/New_York.
	before := buildWeekOverlay(member,
		map[string][]AvailabilityBlock{"u1": {{
			DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60,
			State: AvailAvailable, TZ: "America/New_York",
		}}}, nil, weekStart, utc, "UTC", true)

	// AFTER: the composed exception rows the probe above produced — same
	// minutes, tz "UTC" — for Tuesday 2026-08-04.
	after := buildWeekOverlay(member, nil,
		map[string][]AvailabilityException{"u1": {{
			OnDate: "2026-08-04", StartMinute: 18 * 60, EndMinute: 23 * 60,
			State: AvailAvailable, TZ: "UTC",
		}}}, weekStart, utc, "UTC", true)

	freeHours := func(o WeekOverlay) []int {
		var out []int
		for col, d := range o.Days {
			for h, cell := range d.Hours {
				if cell.Free > 0 {
					out = append(out, col*100+h)
				}
			}
		}
		return out
	}
	t.Logf("BEFORE (viewer UTC) free col*100+hour: %v", freeHours(before))
	t.Logf("AFTER  (viewer UTC) free col*100+hour: %v", freeHours(after))

	// BEFORE: 18:00–22:00 New York on Tuesday = 22:00 Tue UTC through 02:00 Wed
	// UTC. So the member IS free at 01:00 Wednesday UTC and is NOT free at
	// 18:00 Tuesday UTC (that is 14:00 for them — the middle of a workday).
	if before.Days[2].Hours[1].Free != 1 {
		t.Fatalf("setup wrong: expected free at 01:00 UTC Wednesday, got %d", before.Days[2].Hours[1].Free)
	}
	if before.Days[1].Hours[18].Free != 0 {
		t.Fatalf("setup wrong: expected NOT free at 18:00 UTC Tuesday, got %d", before.Days[1].Hours[18].Free)
	}

	// AFTER the relabel the two facts invert, with no edit by the member: the
	// hours the Director sees have slid four hours earlier in real time.
	if after.Days[2].Hours[1].Free != 0 {
		t.Fatalf("probe stale: still free at 01:00 UTC Wednesday after the relabel")
	}
	if after.Days[1].Hours[18].Free != 1 {
		t.Fatalf("probe stale: not free at 18:00 UTC Tuesday after the relabel")
	}
	t.Log("CONFIRMED: the member's stated evening moved four hours earlier in real time; " +
		"the Director's overlay now shows them free during their working afternoon " +
		"and busy during the hours they actually painted")
}

// PROBE 2 — the session RSVP token flow (/rsvp/:token) never re-checks
// membership, so a member REMOVED from the campaign can still write into
// session_attendees for up to the token's 7-day life.
//
// The comparison that makes this a defect rather than a design choice is inside
// this same package: proposals_handler.go:153,178 call isCampaignMember on BOTH
// halves of /proposals/respond/:token, and calendar/rsvp_handler.go:937 does the
// same for /calendar-rsvp/:token ("a link cannot outlive the access that
// justified it"). /rsvp/:token is the one that does not.
func TestProbe_SessionRSVPTokenIgnoresRemovedMember(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	var appliedFor string
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "ex-member", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, sid, uid, status string) error {
			appliedFor = uid + ":" + status
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error { return nil },
	}
	h := &Handler{svc: NewSessionService(repo, nil)}
	// The roster no longer contains "ex-member" — they were removed after the
	// invite mail went out. (Even a nil lister would do: nothing consults it.)
	h.SetMemberLister(&stubMemberLister{members: []campaigns.CampaignMember{{UserID: "still-here"}}})

	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/rsvp/rt", nil), httptest.NewRecorder())
	c.SetParamNames("token")
	c.SetParamValues("rt")
	if err := h.ApplyRSVPToken(c); err != nil {
		t.Fatalf("ApplyRSVPToken: %v", err)
	}

	if appliedFor == "" {
		t.Fatal("probe stale: the removed member's token was refused — the gap may be closed")
	}
	t.Logf("CONFIRMED: an ex-member's emailed link still wrote %q (proposals + calendar-rsvp both refuse this)", appliedFor)
}

// PROBE 3 — the sessions token consume is NOT atomic, unlike its two siblings.
//
// calendar/rsvp_repository.go:220 consumes with `... WHERE token = ? AND
// used_at IS NULL` and treats RowsAffected==0 as "already spent". The sessions
// repository's MarkRSVPTokenUsed (repository.go:497) has no such predicate, and
// ApplyRSVPToken (service.go:484) does validate-then-write with nothing in
// between — so two submits that both validate before either consumes BOTH
// apply. Modelled here as the exact interleaving a double-tap produces.
func TestProbe_SessionRSVPTokenConsumeIsNotAtomic(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	var applies, consumes int
	used := false
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			tok := &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}
			if used {
				now := time.Now().UTC()
				tok.UsedAt = &now
			}
			return tok, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error { applies++; return nil },
		markRSVPTokenUsedFn:    func(_ context.Context, _ string) error { consumes++; used = true; return nil },
	}
	svc := &sessionService{repo: repo}

	// Both requests validate first (this is the read half of ApplyRSVPToken).
	tokA, errA := svc.ValidateRSVPToken(context.Background(), "rt")
	tokB, errB := svc.ValidateRSVPToken(context.Background(), "rt")
	if errA != nil || errB != nil || tokA == nil || tokB == nil {
		t.Fatalf("both validations passed in production too: %v / %v", errA, errB)
	}

	// Then both proceed to the write half, exactly as ApplyRSVPToken does after
	// its own ValidateRSVPToken returns.
	for _, tok := range []*RSVPToken{tokA, tokB} {
		if err := repo.UpdateAttendeeStatus(context.Background(), tok.SessionID, tok.UserID, tok.Action); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if err := repo.MarkRSVPTokenUsed(context.Background(), tok.Token); err != nil {
			t.Fatalf("consume: %v", err)
		}
	}

	if applies != 2 || consumes != 2 {
		t.Fatalf("probe stale: applies=%d consumes=%d — a guard may have been added", applies, consumes)
	}
	t.Log("CONFIRMED: one single-use token applied twice and consumed twice; " +
		"MarkRSVPTokenUsed has no `used_at IS NULL` predicate and reports no RowsAffected")
}

// PROBE 9 — splitToViewerDays does not terminate for a viewer in a zone whose
// LOCAL MIDNIGHT DOES NOT EXIST on the spring-forward day.
//
// availability_overlay.go:212 computes the end of the current local day as
//
//	nextMidnight := time.Date(y, mo, d, 0,0,0,0, loc).AddDate(0, 0, 1)
//
// In America/Havana and America/Santiago the clocks jump 00:00 → 01:00, so
// time.Date normalises 00:00 BACKWARDS to 23:00 on the PREVIOUS day, and
// AddDate then lands on 23:00 of the SAME day. `nextMidnight` is therefore one
// hour BEFORE the local day really ends. Any segment still running at 23:00
// takes the `segEnd = nextMidnight` branch, and the loop's only advance is
// `cur = segEnd` — which assigns cur the value it already holds.
//
// This test does the arithmetic without running the loop, so it is safe to keep
// in the suite. The live repro is TestProbe9Live below, which is opt-in because
// it hangs the binary (which is the point).
func TestProbe_SplitToViewerDaysFixedPoint(t *testing.T) {
	for _, tc := range []struct{ zone, day string }{
		{"America/Havana", "2026-03-08"},
		{"America/Santiago", "2026-09-06"},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("tzdata missing for %s", tc.zone)
			}
			d, _ := time.Parse("2006-01-02", tc.day)

			// A member's ordinary late-evening block, 22:00 → local midnight.
			start := time.Date(d.Year(), d.Month(), d.Day(), 0, 22*60, 0, 0, loc)
			end := time.Date(d.Year(), d.Month(), d.Day(), 0, 24*60, 0, 0, loc)
			le := end.In(loc)

			// Iterate the loop body BY HAND, bounded, and watch cur stop moving.
			cur := start.In(loc)
			var seen []string
			for i := 0; i < 4 && cur.Before(le); i++ {
				y, mo, dd := cur.Date()
				nextMidnight := time.Date(y, mo, dd, 0, 0, 0, 0, loc).AddDate(0, 0, 1)
				segEnd := le
				if !nextMidnight.After(le) {
					segEnd = nextMidnight
				}
				seen = append(seen, fmt.Sprintf("cur=%s nextMidnight=%s → segEnd=%s",
					cur.Format("2006-01-02 15:04 MST"),
					nextMidnight.Format("2006-01-02 15:04 MST"),
					segEnd.Format("2006-01-02 15:04 MST")))
				prev := cur
				cur = segEnd
				if cur.Equal(prev) {
					for _, s := range seen {
						t.Log("  " + s)
					}
					t.Logf("CONFIRMED: cur did not advance on iteration %d and le=%s is still ahead of it — "+
						"buildWeekOverlay cannot terminate for a viewer in %s on %s",
						i+1, le.Format("2006-01-02 15:04 MST"), tc.zone, tc.day)
					return
				}
			}
			for _, s := range seen {
				t.Log("  " + s)
			}
			t.Fatalf("probe stale: the loop advanced past the transition in %s — a guard may exist now", tc.zone)
		})
	}
}

// TestProbe9Live is the end-to-end repro and it HANGS ON PURPOSE. Run it alone:
//
//	CHRONICLE_SCOUT_HANG=1 go test ./internal/plugins/sessions/ \
//	    -run TestProbe9Live -timeout 20s
//
// `go test` kills the binary at the deadline and prints the stack, which lands
// inside splitToViewerDays. Opt-in so it never wedges CI.
func TestProbe9Live(t *testing.T) {
	if os.Getenv("CHRONICLE_SCOUT_HANG") == "" {
		t.Skip("set CHRONICLE_SCOUT_HANG=1 to run the hanging repro (it does not return)")
	}
	loc, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skip("tzdata missing")
	}
	weekStart, _ := timeutil.ParseCivilDate("2026-03-02") // Monday; 03-08 is the Sunday
	overlay := buildWeekOverlay(
		[]overlayMemberInput{{UserID: "u1", Name: "Ana"}},
		map[string][]AvailabilityBlock{"u1": {{
			DayOfWeek: int(time.Sunday), StartMinute: 22 * 60, EndMinute: 24 * 60,
			State: AvailAvailable, TZ: "America/Havana",
		}}}, nil, weekStart, loc, "America/Havana", true)
	t.Fatalf("unreachable if the defect is present; got %d days", len(overlay.Days))
}
