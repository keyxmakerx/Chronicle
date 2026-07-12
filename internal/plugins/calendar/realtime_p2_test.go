// realtime_p2_test.go — C-REAL-CALENDAR-P2. Pins the four mandatory-opener fixes
// (F1 ListCalendars seam, F2 worldstate content keys on the live date, F3
// ApplyImport guarded as W8, F4 PutDate guard surfaces as 4xx) plus the P2 enable
// flow (RC-2 zone-required/valid, HoursPerDay==24, RC-1 reallife-only), the W6
// name-only-save footgun fix, the disable + no-clobber semantics, and the export
// round-trip. In-package so the fake clock is injected directly onto
// calendarService.now and the guard apperror type is asserted by string.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// rtServiceRepo wires a calendarService with the fake clock over a caller-built
// repo, so P2 tests can inject listByCampaignIDFn / updateFn / applyImportFn.
func rtServiceRepo(repo *mockCalendarRepo, clock time.Time) *calendarService {
	return &calendarService{repo: repo, events: NoopCalendarEventPublisher{}, now: rtClock(clock)}
}

// --- F1: ListCalendars (the sixth loader) seams live dates ---

// TestRealTimeP2_ListCalendarsSeamsLiveDate pins F1: the base list loader — read
// directly by the syncapi list endpoint, the owner dashboard, and the timeline —
// overwrites a real-time calendar's stale stored date with the live wall-clock
// date, while leaving a reallife-but-manual calendar's stored date untouched.
func TestRealTimeP2_ListCalendarsSeamsLiveDate(t *testing.T) {
	// 2024-02-29 12:00 UTC → America/New_York (EST) = 2024-02-29 07:00.
	clock := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)

	rt := rtCalendar("America/New_York") // RT, stored 2000-01-01
	manual := rtCalendar("America/New_York")
	manual.ID = "cal-manual"
	manual.TracksRealTime = false
	manual.CurrentYear, manual.CurrentMonth, manual.CurrentDay = 1500, 6, 6

	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return []Calendar{*rt, *manual}, nil
		},
	}
	svc := rtServiceRepo(repo, clock)

	cals, err := svc.ListCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(cals) != 2 {
		t.Fatalf("got %d calendars, want 2", len(cals))
	}
	// [0] real-time → live date.
	if cals[0].CurrentYear != 2024 || cals[0].CurrentMonth != 2 || cals[0].CurrentDay != 29 || cals[0].CurrentHour != 7 {
		t.Errorf("real-time list entry = %04d-%02d-%02d %02d:xx, want live 2024-02-29 07",
			cals[0].CurrentYear, cals[0].CurrentMonth, cals[0].CurrentDay, cals[0].CurrentHour)
	}
	// [1] manual → stored date unchanged (stop-and-flag #2 discipline).
	if cals[1].CurrentYear != 1500 || cals[1].CurrentMonth != 6 || cals[1].CurrentDay != 6 {
		t.Errorf("manual list entry = %04d-%02d-%02d, want stored 1500-06-06 unchanged",
			cals[1].CurrentYear, cals[1].CurrentMonth, cals[1].CurrentDay)
	}
}

// --- F2: worldstate content authoring keys on the live date ---

// TestRealTimeP2_WorldstateContentKeysOnLiveDate pins F2: authoring "weather for
// today" on a real-time calendar upserts onto the LIVE date, not the stale stored
// Current*. Without the seam the write would land on 2000-01-01.
func TestRealTimeP2_WorldstateContentKeysOnLiveDate(t *testing.T) {
	clock := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC) // → 2024-02-29 EST
	cal := rtCalendar("America/New_York")                  // stored 2000-01-01
	var gotY, gotM, gotD int
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		setDayWeatherFn: func(_ context.Context, _ string, y, m, d int, _ string) error {
			gotY, gotM, gotD = y, m, d
			return nil
		},
	}
	svc := rtServiceRepo(repo, clock)

	if err := svc.SetWorldState(context.Background(), "cal-rt",
		WorldStateUpdateInput{Weather: rtStrPtr("rain")}); err != nil {
		t.Fatalf("weather authoring must stay editable on a real-time calendar (RC-5): %v", err)
	}
	if gotY != 2024 || gotM != 2 || gotD != 29 {
		t.Errorf("weather keyed on %04d-%02d-%02d, want the live 2024-02-29 (not stored 2000-01-01)", gotY, gotM, gotD)
	}
}

// --- F3: ApplyImport guarded as W8 ---

// TestRealTimeP2_ApplyImportGuardedOnRealTime pins F3: an import rewrites Current*
// unconditionally, so it is rejected on a real-time calendar and never reaches the
// repo write. A reallife-but-manual calendar still imports normally (flag-gated).
func TestRealTimeP2_ApplyImportGuardedOnRealTime(t *testing.T) {
	// Real-time → rejected, no write.
	rt := rtCalendar("America/New_York")
	applied := false
	rtRepo := &mockCalendarRepo{
		getByIDFn:     func(_ context.Context, _ string) (*Calendar, error) { return rt, nil },
		applyImportFn: func(_ context.Context, _ *Calendar, _ *ImportResult) error { applied = true; return nil },
	}
	err := rtServiceRepo(rtRepo, rtFixedClock).ApplyImport(context.Background(), "cal-rt", &ImportResult{})
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("ApplyImport on real-time calendar: err = %v, want validation", err)
	}
	if applied {
		t.Error("ApplyImport must not write on a real-time calendar (W8)")
	}

	// Reallife-but-manual → import proceeds to the repo write.
	manual := rtCalendar("America/New_York")
	manual.TracksRealTime = false
	manualApplied := false
	manualRepo := &mockCalendarRepo{
		getByIDFn:     func(_ context.Context, _ string) (*Calendar, error) { return manual, nil },
		applyImportFn: func(_ context.Context, _ *Calendar, _ *ImportResult) error { manualApplied = true; return nil },
	}
	if err := rtServiceRepo(manualRepo, rtFixedClock).ApplyImport(context.Background(), "cal-rt", &ImportResult{}); err != nil {
		t.Fatalf("manual reallife import should succeed: %v", err)
	}
	if !manualApplied {
		t.Error("manual reallife import should reach the repo write (guard is flag-gated)")
	}
}

// --- F4: PutDate guard rejection surfaces as 4xx, not 500 ---

// TestRealTimeP2_PutDateGuardSurfacesAs4xx pins F4: the W-guard's apperror carries
// Type "validation_error", so before the fix the handler's isAppErrorType(…,
// "validation") check missed and a date-push on a real-time calendar 500'd. After
// the fix it is a clean 400 validation response.
func TestRealTimeP2_PutDateGuardSurfacesAs4xx(t *testing.T) {
	svc, _ := rtService(rtCalendar("America/New_York"), rtFixedClock)
	h := NewAPIHandler(svc, &stubVerifier{})

	e := echo.New()
	body := `{"year":2030,"month":1,"day":1,"hour":0,"minute":0}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cid")
	c.SetParamValues("camp-1")
	c.QueryParams().Set("token", "ok-token")

	if err := h.PutDate(c); err != nil {
		t.Fatalf("PutDate returned a raw error (should respond via JSON): %v", err)
	}
	// The guard's apperror is a validation error → 422 (Unprocessable Entity). The
	// F4 point is that it is a 4xx validation response, NOT the pre-fix 500-class
	// APIErrInternal that the "validation" (never-matching) check fell through to.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("PutDate on a real-time calendar returned %d, want 422 (validation, not a 500-class error); body=%s",
			rec.Code, rec.Body.String())
	}
	if rec.Code >= 500 {
		t.Errorf("guard rejection must not be a 5xx (F4 regression); got %d", rec.Code)
	}
}

// --- P2: enable-flow validation (RC-1 / RC-2 / 24h) ---

// TestRealTimeP2_EnableRequiresPreconditions pins that turning ON real-time
// tracking is rejected unless the calendar is reallife (RC-1), an IANA anchor zone
// is supplied and loadable (RC-2), and the day is 24 hours. Each failure is a
// validation error (→ 4xx) and never writes.
func TestRealTimeP2_EnableRequiresPreconditions(t *testing.T) {
	tr := true
	valid := "America/New_York"
	bad := "Not/AZone"

	cases := []struct {
		name    string
		mode    string
		zone    *string
		hours   int
		wantErr bool
	}{
		{"fantasy mode rejected", ModeFantasy, &valid, 24, true},
		{"missing zone rejected", ModeRealLife, nil, 24, true},
		{"blank zone rejected", ModeRealLife, rtStrPtr("   "), 24, true},
		{"invalid zone rejected", ModeRealLife, &bad, 24, true},
		{"non-24-hour day rejected", ModeRealLife, &valid, 23, true},
		{"valid enable accepted", ModeRealLife, &valid, 24, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := rtCalendar("America/New_York")
			base.Mode = tc.mode
			base.TracksRealTime = false // enabling from a manual base
			base.RealTimeZone = nil
			wrote := false
			repo := &mockCalendarRepo{
				getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return base, nil },
				updateFn:  func(_ context.Context, _ *Calendar) error { wrote = true; return nil },
			}
			svc := rtServiceRepo(repo, rtFixedClock)
			err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
				Name: "Session", SetRealTime: &tr, RealTimeZone: tc.zone,
				HoursPerDay: tc.hours, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
			})
			if tc.wantErr {
				if !isAppErrorType(err, "validation_error") {
					t.Fatalf("err = %v, want validation_error", err)
				}
				if wrote {
					t.Error("a rejected enable must not write")
				}
			} else if err != nil {
				t.Fatalf("valid enable should succeed: %v", err)
			}
		})
	}
}

// TestRealTimeP2_EnablePersistsFlagAndZone pins that a valid enable persists
// tracks_real_time=true and the trimmed anchor zone.
func TestRealTimeP2_EnablePersistsFlagAndZone(t *testing.T) {
	base := rtCalendar("America/New_York")
	base.TracksRealTime = false
	base.RealTimeZone = nil
	var persisted *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return base, nil },
		updateFn:  func(_ context.Context, c *Calendar) error { persisted = c; return nil },
	}
	svc := rtServiceRepo(repo, rtFixedClock)
	tr := true
	if err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Session", SetRealTime: &tr, RealTimeZone: rtStrPtr("  Europe/London  "),
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected a repo write on enable")
	}
	if !persisted.TracksRealTime {
		t.Error("tracks_real_time should persist true")
	}
	if persisted.RealTimeZone == nil || *persisted.RealTimeZone != "Europe/London" {
		t.Errorf("anchor zone = %v, want trimmed 'Europe/London'", persisted.RealTimeZone)
	}
}

// --- P2: W6 name-only-save footgun fix ---

// TestRealTimeP2_SettingsSaveOnRealTimePreservesDate pins the footgun fix: a
// settings-form save (SetRealTime non-nil) on a real-time calendar that posts a
// DIFFERENT date (the seamed live value the form holds) does NOT trip the guard —
// the stored date is preserved and the non-date edit (name) is saved.
func TestRealTimeP2_SettingsSaveOnRealTimePreservesDate(t *testing.T) {
	cal := rtCalendar("America/New_York") // RT, stored 2000-01-01
	var persisted *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		updateFn:  func(_ context.Context, c *Calendar) error { persisted = c; return nil },
	}
	svc := rtServiceRepo(repo, rtFixedClock)
	tr := true
	err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Renamed", SetRealTime: &tr, RealTimeZone: rtStrPtr("America/New_York"),
		// The form posts the LIVE date (differs from stored 2000-01-01).
		CurrentYear: 2024, CurrentMonth: 2, CurrentDay: 29, CurrentHour: 7, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	})
	if err != nil {
		t.Fatalf("settings save on a real-time calendar must succeed (footgun): %v", err)
	}
	if persisted == nil {
		t.Fatal("expected a repo write")
	}
	if persisted.CurrentYear != 2000 || persisted.CurrentMonth != 1 || persisted.CurrentDay != 1 {
		t.Errorf("stored date = %04d-%02d-%02d, want preserved 2000-01-01 (not the posted live date)",
			persisted.CurrentYear, persisted.CurrentMonth, persisted.CurrentDay)
	}
	if persisted.Name != "Renamed" {
		t.Errorf("name = %q, want the non-date edit saved", persisted.Name)
	}
}

// TestRealTimeP2_DatePushStillRejectsWhenSetRealTimeNil pins the other half of the
// footgun fix: a date-PUSH caller (PutDate) leaves SetRealTime nil, so its explicit
// date change on a real-time calendar still reaches the W6 guard and is rejected —
// preservation is scoped to settings-form saves, not every write.
func TestRealTimeP2_DatePushStillRejectsWhenSetRealTimeNil(t *testing.T) {
	cal := rtCalendar("America/New_York") // stored 2000-01-01
	wrote := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		updateFn:  func(_ context.Context, _ *Calendar) error { wrote = true; return nil },
	}
	svc := rtServiceRepo(repo, rtFixedClock)
	err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name:        "Session", // SetRealTime nil = date-push path
		CurrentYear: 2030, CurrentMonth: 1, CurrentDay: 1, CurrentHour: 0, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	})
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("date-push on a real-time calendar: err = %v, want validation", err)
	}
	if wrote {
		t.Error("a rejected date-push must not write")
	}
}

// --- P2: disable + no-clobber ---

// TestRealTimeP2_DisableClearsFlagAndZone pins that disabling reverts to a manual
// stored-date calendar and clears the now-meaningless anchor zone.
func TestRealTimeP2_DisableClearsFlagAndZone(t *testing.T) {
	cal := rtCalendar("America/New_York") // RT with zone
	var persisted *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		updateFn:  func(_ context.Context, c *Calendar) error { persisted = c; return nil },
	}
	svc := rtServiceRepo(repo, rtFixedClock)
	f := false
	if err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Session", SetRealTime: &f,
		CurrentYear: 2000, CurrentMonth: 1, CurrentDay: 1, CurrentHour: 0, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected a repo write on disable")
	}
	if persisted.TracksRealTime {
		t.Error("tracks_real_time should be cleared on disable")
	}
	if persisted.RealTimeZone != nil {
		t.Errorf("anchor zone should be cleared on disable, got %v", persisted.RealTimeZone)
	}
}

// TestRealTimeP2_NilSetRealTimePreservesRTFields pins that a caller that does NOT
// manage the flag (SetRealTime nil — PutDate, worldstate, seed/create) never
// clobbers the stored real-time settings when it round-trips the calendar.
func TestRealTimeP2_NilSetRealTimePreservesRTFields(t *testing.T) {
	cal := rtCalendar("America/New_York") // RT with zone
	var persisted *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		updateFn:  func(_ context.Context, c *Calendar) error { persisted = c; return nil },
	}
	svc := rtServiceRepo(repo, rtFixedClock)
	// A non-date-changing update (input date == stored) with SetRealTime nil.
	if err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Same", CurrentYear: 2000, CurrentMonth: 1, CurrentDay: 1, CurrentHour: 0, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	}); err != nil {
		t.Fatalf("non-date settings save: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected a repo write")
	}
	if !persisted.TracksRealTime {
		t.Error("SetRealTime nil must preserve the stored tracks_real_time=true")
	}
	if persisted.RealTimeZone == nil || *persisted.RealTimeZone != "America/New_York" {
		t.Errorf("SetRealTime nil must preserve the stored zone, got %v", persisted.RealTimeZone)
	}
}

// --- P2: export round-trip ---

// TestRealTimeP2_ExportCarriesRTFields pins that the calendar export carries both
// real-time fields so an export/re-import preserves the setting; a manual calendar
// exports tracks_real_time=false with the zone omitted.
func TestRealTimeP2_ExportCarriesRTFields(t *testing.T) {
	rt := rtCalendar("America/New_York")
	exp := BuildExport(rt, nil, false)
	if !exp.Calendar.TracksRealTime {
		t.Error("export should carry tracks_real_time=true")
	}
	if exp.Calendar.RealTimeZone == nil || *exp.Calendar.RealTimeZone != "America/New_York" {
		t.Errorf("export zone = %v, want 'America/New_York'", exp.Calendar.RealTimeZone)
	}

	manual := rtCalendar("America/New_York")
	manual.TracksRealTime = false
	manual.RealTimeZone = nil
	exp2 := BuildExport(manual, nil, false)
	if exp2.Calendar.TracksRealTime {
		t.Error("manual calendar export should carry tracks_real_time=false")
	}
	if exp2.Calendar.RealTimeZone != nil {
		t.Errorf("manual calendar export should omit the zone, got %v", exp2.Calendar.RealTimeZone)
	}
}
