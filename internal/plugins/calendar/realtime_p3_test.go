// realtime_p3_test.go — C-REAL-CALENDAR-P3 item 0. Pins the three mandatory
// openers: 0a the Gregorian-JDN weekday path (no leap drift for real-time
// calendars, provably zero-change for everything else), 0b the RT invariant that
// closes the syncapi mode-walk bypass, and 0c import consuming the real-time
// fields through the enable-flow validation. In-package so the fake clock is
// injected directly onto calendarService.now and the guard apperror type is
// asserted by string.
package calendar

import (
	"context"
	"encoding/json"
	"testing"
)

// --- 0a: Gregorian-JDN weekday path (no leap drift) ---

// rtWeekdayCal is a flagged real-time (UsesRealTime) Gregorian calendar with the
// standard Sun..Sat week, so v2WeekdayIndexFor takes the JDN branch.
func rtWeekdayCal() *Calendar {
	c := rtCalendar("America/New_York") // reallife + TracksRealTime + 24h + Gregorian months
	c.Weekdays = []Weekday{
		{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
		{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"},
	}
	return c
}

// TestRealTimeP3_WeekdayJDNNoLeapDrift pins 0a: a real-time calendar computes the
// weekday column from a proleptic-Gregorian JDN, so it agrees with the old
// constant-length formula in 2026 (pre-drift) AND stays correct past the 2028
// leap boundary where that formula drifts +1. Fixtures per the dispatch:
// 2028-03-01 is a Wednesday, 2032-07-04 is a Sunday (Sun=index 0).
func TestRealTimeP3_WeekdayJDNNoLeapDrift(t *testing.T) {
	cal := rtWeekdayCal()
	cases := []struct {
		y, m, d int
		want    int
		note    string
	}{
		{2026, 1, 1, 4, "Thu — matches the 2026-correct old formula"},
		{2026, 6, 8, 1, "Mon — the #428 align-fix headline date, unchanged"},
		{2028, 3, 1, 3, "Wed — the first post-drift fixture (old formula wrongly says Tue)"},
		{2032, 7, 4, 0, "Sun — second post-drift fixture"},
	}
	for _, tc := range cases {
		if got := v2WeekdayIndexFor(cal, tc.y, tc.m, tc.d); got != tc.want {
			t.Errorf("v2WeekdayIndexFor(RT, %04d-%02d-%02d) = %d; want %d (%s)",
				tc.y, tc.m, tc.d, got, tc.want, tc.note)
		}
	}
}

// TestRealTimeP3_WeekdayNonRTIsZeroChange is the stop-and-flag control: the JDN
// branch is entered ONLY for UsesRealTime() calendars. A reallife-but-manual /
// fantasy-shaped Gregorian calendar (UsesRealTime()=false) keeps the constant-
// length formula byte-for-byte — including its 2028 drift — proving the branch
// changed nothing for non-real-time calendars.
func TestRealTimeP3_WeekdayNonRTIsZeroChange(t *testing.T) {
	nonRT := gregorian2026() // Mode/TracksRealTime unset → UsesRealTime()=false

	// Agreement in 2026, divergence in 2028: the RT twin corrects the drift; the
	// non-RT original keeps the (now-wrong) constant-length value.
	rt := rtWeekdayCal()
	if a, b := v2WeekdayIndexFor(rt, 2026, 6, 8), v2WeekdayIndexFor(nonRT, 2026, 6, 8); a != b || a != 1 {
		t.Errorf("2026-06-08: RT=%d nonRT=%d; want both 1 (agree pre-drift)", a, b)
	}
	if rtIdx, oldIdx := v2WeekdayIndexFor(rt, 2028, 3, 1), v2WeekdayIndexFor(nonRT, 2028, 3, 1); rtIdx != 3 || oldIdx != 2 {
		t.Errorf("2028-03-01: RT=%d (want 3, corrected) nonRT=%d (want 2, the preserved drift)", rtIdx, oldIdx)
	}

	// The non-RT path equals the exact pre-change constant-length formula for
	// every shape, including a fantasy 10-day week — a direct zero-change proof.
	fantasy := &Calendar{
		Months:   []Month{{Name: "Trien", Days: 30}, {Name: "Eldar", Days: 32}, {Name: "Vast", Days: 28}},
		Weekdays: make([]Weekday, 10),
	}
	for _, tc := range []struct{ y, m, d int }{{100, 1, 6}, {100, 2, 15}, {2028, 3, 1}, {2033, 3, 3}} {
		if got, want := v2WeekdayIndexFor(fantasy, tc.y, tc.m, tc.d), constLenWeekday(fantasy, tc.y, tc.m, tc.d); got != want {
			t.Errorf("fantasy weekday %04d-%02d-%02d = %d; want %d (constant-length path unchanged)", tc.y, tc.m, tc.d, got, want)
		}
	}
}

// constLenWeekday reproduces the pre-P3 constant-YearLength weekday formula so the
// zero-change control can assert the non-RT path is byte-identical to it.
func constLenWeekday(cal *Calendar, year, month, day int) int {
	wl := cal.WeekLength()
	if wl == 0 {
		return 0
	}
	abs := year * cal.YearLength()
	for i := 0; i < month-1 && i < len(cal.Months); i++ {
		abs += cal.Months[i].Days
	}
	abs += day
	idx := abs % wl
	if idx < 0 {
		idx += wl
	}
	return idx
}

// --- 0b: RT invariant closes the mode-walk / hours-walk bypass ---

// TestRealTimeP3_InvariantClosesModeWalk pins 0b: any settings write whose RESULT
// still tracks real time must satisfy reallife + 24h (+ loadable zone), else it
// is a validation error and never persists. Each case echoes the stored date so
// the W6 date-guard is NOT what trips — isolating the invariant. A clean disable
// alongside the mode change still succeeds.
func TestRealTimeP3_InvariantClosesModeWalk(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*UpdateCalendarInput)
		wantErr bool
	}{
		{"mode-walk to fantasy (SetRealTime nil) rejected", func(in *UpdateCalendarInput) {
			in.Mode = ModeFantasy // syncapi binds mode; SetRealTime stays nil
		}, true},
		{"hours-walk to non-24 (SetRealTime nil) rejected", func(in *UpdateCalendarInput) {
			in.Mode = ModeRealLife
			in.HoursPerDay = 10
		}, true},
		{"explicit disable + mode change accepted", func(in *UpdateCalendarInput) {
			f := false
			in.SetRealTime = &f
			in.Mode = ModeFantasy
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cal := rtCalendar("America/New_York") // RT, stored 2000-01-01, 24h
			var persisted *Calendar
			repo := &mockCalendarRepo{
				getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
				updateFn:  func(_ context.Context, c *Calendar) error { persisted = c; return nil },
			}
			svc := rtServiceRepo(repo, rtFixedClock)
			in := UpdateCalendarInput{
				Name:        "Session",
				CurrentYear: 2000, CurrentMonth: 1, CurrentDay: 1, CurrentHour: 0, CurrentMinute: 0,
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
			}
			tc.mutate(&in)
			err := svc.UpdateCalendar(context.Background(), "cal-rt", in)
			if tc.wantErr {
				if !isAppErrorType(err, "validation_error") {
					t.Fatalf("err = %v, want validation_error (invariant rejection)", err)
				}
				if persisted != nil {
					t.Error("a rejected settings write must not persist")
				}
				return
			}
			if err != nil {
				t.Fatalf("explicit disable should succeed: %v", err)
			}
			if persisted == nil {
				t.Fatal("expected a repo write on disable")
			}
			if persisted.TracksRealTime {
				t.Error("explicit disable should clear tracks_real_time")
			}
			if persisted.Mode != ModeFantasy {
				t.Errorf("mode = %q, want the fantasy mode change applied", persisted.Mode)
			}
		})
	}
}

// --- 0c: import consumes the real-time fields ---

// TestRealTimeP3_ImportRoundTripPreservesRT pins the 0c round-trip: an RT
// calendar exported and re-imported onto a NON-RT (fantasy) target comes out
// real-time with the same anchor zone + reallife mode — the export.go round-trip
// claim is now true, not just serialized-and-dropped.
func TestRealTimeP3_ImportRoundTripPreservesRT(t *testing.T) {
	src := rtCalendar("Europe/London")
	raw, err := json.Marshal(BuildExport(src, nil, false))
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	result, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// parseChronicle must carry the fields (the previously-dropped step).
	if !result.Settings.TracksRealTime {
		t.Fatal("parseChronicle dropped tracks_real_time")
	}
	if result.Settings.RealTimeZone == nil || *result.Settings.RealTimeZone != "Europe/London" {
		t.Fatalf("parsed zone = %v, want Europe/London", result.Settings.RealTimeZone)
	}

	target := &Calendar{
		ID: "cal-target", Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
	}
	var applied *Calendar
	repo := &mockCalendarRepo{
		getByIDFn:     func(_ context.Context, _ string) (*Calendar, error) { return target, nil },
		applyImportFn: func(_ context.Context, cal *Calendar, _ *ImportResult) error { applied = cal; return nil },
	}
	if err := rtServiceRepo(repo, rtFixedClock).ApplyImport(context.Background(), "cal-target", result); err != nil {
		t.Fatalf("import onto a fantasy target should succeed: %v", err)
	}
	if applied == nil {
		t.Fatal("expected repo.ApplyImport to be called")
	}
	if !applied.TracksRealTime {
		t.Error("import should enable real-time on the target")
	}
	if applied.Mode != ModeRealLife {
		t.Errorf("import should force reallife mode for RT, got %q", applied.Mode)
	}
	if applied.RealTimeZone == nil || *applied.RealTimeZone != "Europe/London" {
		t.Errorf("import zone = %v, want Europe/London", applied.RealTimeZone)
	}
}

// TestRealTimeP3_ImportBadZoneRejected pins that an import claiming real-time with
// an invalid zone is a NAMED validation error (through the same check as enable),
// never a silent drop, and never writes.
func TestRealTimeP3_ImportBadZoneRejected(t *testing.T) {
	result := &ImportResult{
		Format:       FormatChronicle,
		CalendarName: "X",
		Months:       []MonthInput{{Name: "M", Days: 30}},
		Settings: ImportedSettings{
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			TracksRealTime: true, RealTimeZone: rtStrPtr("Not/AZone"),
		},
	}
	target := &Calendar{ID: "cal-t", Mode: ModeFantasy, HoursPerDay: 24}
	wrote := false
	repo := &mockCalendarRepo{
		getByIDFn:     func(_ context.Context, _ string) (*Calendar, error) { return target, nil },
		applyImportFn: func(_ context.Context, _ *Calendar, _ *ImportResult) error { wrote = true; return nil },
	}
	err := rtServiceRepo(repo, rtFixedClock).ApplyImport(context.Background(), "cal-t", result)
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("err = %v, want validation_error naming the zone", err)
	}
	if wrote {
		t.Error("a bad-zone real-time import must not write")
	}
}

// TestRealTimeP3_ImportOntoRealTimeStillRejected pins that W8 stands: importing
// (even a valid RT payload) onto an already-real-time calendar is rejected
// wholesale, so the live wall-clock date is never stomped.
func TestRealTimeP3_ImportOntoRealTimeStillRejected(t *testing.T) {
	target := rtCalendar("America/New_York") // already RT
	wrote := false
	repo := &mockCalendarRepo{
		getByIDFn:     func(_ context.Context, _ string) (*Calendar, error) { return target, nil },
		applyImportFn: func(_ context.Context, _ *Calendar, _ *ImportResult) error { wrote = true; return nil },
	}
	result := &ImportResult{
		Settings: ImportedSettings{TracksRealTime: true, RealTimeZone: rtStrPtr("Europe/London"), HoursPerDay: 24},
	}
	err := rtServiceRepo(repo, rtFixedClock).ApplyImport(context.Background(), "cal-rt", result)
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("err = %v, want validation_error (W8 reject-onto-RT)", err)
	}
	if wrote {
		t.Error("import onto a real-time calendar must not write (W8)")
	}
}
