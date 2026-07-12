// realtime_test.go — C-REAL-CALENDAR-P1. Pins the real-time (wall-clock) mode
// mechanism with a fake clock: the applyRealTime seam (live date in the anchor
// zone, DST edges, minute rollover, Feb-29 vs Feb-2100 month geometry), the
// zone-load fail-safe (bad/blank zone → serve the stored date, never 500), the
// provably-zero-change behavior for non-real-time calendars (stop-and-flag #2),
// each W1–W7 manual-date-write guard, and a wire + Player-role pin that a viewer
// receives the COMPUTED live date. All in-package so the fake clock is injected
// directly onto calendarService.now.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- fixtures ---

// rtClock returns a fixed-instant clock for deterministic real-time assertions.
func rtClock(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

// rtGregorianMonths mirrors seedDefaults' Gregorian months (Feb carries the
// +1 leap day) so a manual reallife calendar's OLD geometry path is exercised.
func rtGregorianMonths() []Month {
	names := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	days := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	leap := []int{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	out := make([]Month, 12)
	for i := range names {
		out[i] = Month{Name: names[i], Days: days[i], SortOrder: i, LeapYearDays: leap[i]}
	}
	return out
}

// rtCalendar builds a flagged real-time calendar anchored to zone, with a
// deliberately STALE stored date (2000-01-01) the seam must overwrite on read.
func rtCalendar(zone string) *Calendar {
	z := zone
	return &Calendar{
		ID: "cal-rt", CampaignID: "camp-1", Name: "Session Calendar",
		Mode: ModeRealLife, TracksRealTime: true, RealTimeZone: &z,
		Visibility:    "everyone",
		HoursPerDay:   24, MinutesPerHour: 60, SecondsPerMinute: 60,
		LeapYearEvery: 4, LeapYearOffset: 0,
		CurrentYear:   2000, CurrentMonth: 1, CurrentDay: 1, CurrentHour: 0, CurrentMinute: 0,
		Months:        rtGregorianMonths(),
	}
}

// rtService wires a calendarService with the fake clock over a mock repo whose
// GetByID/GetByCampaignID return cal.
func rtService(cal *Calendar, clock time.Time) (*calendarService, *mockCalendarRepo) {
	repo := &mockCalendarRepo{
		getByIDFn:         func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
	}
	return &calendarService{repo: repo, events: NoopCalendarEventPublisher{}, now: rtClock(clock)}, repo
}

// --- seam: live date in the anchor zone ---

// TestRealTime_SeamComputesLiveDateInZone pins that a loader overwrites Current*
// with the wall-clock date converted into the calendar's IANA anchor zone — the
// stale stored 2000-01-01 must NOT survive the read.
func TestRealTime_SeamComputesLiveDateInZone(t *testing.T) {
	// 2024-02-29 12:00 UTC → America/New_York (EST, UTC-5) = 2024-02-29 07:00.
	clock := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	svc, _ := rtService(rtCalendar("America/New_York"), clock)

	got, err := svc.GetCalendarByID(context.Background(), "cal-rt")
	if err != nil {
		t.Fatalf("GetCalendarByID: %v", err)
	}
	if got.CurrentYear != 2024 || got.CurrentMonth != 2 || got.CurrentDay != 29 ||
		got.CurrentHour != 7 || got.CurrentMinute != 0 {
		t.Errorf("live date = %04d-%02d-%02d %02d:%02d, want 2024-02-29 07:00",
			got.CurrentYear, got.CurrentMonth, got.CurrentDay, got.CurrentHour, got.CurrentMinute)
	}
}

// TestRealTime_SeamDSTSpringForward pins that In(loc) skips the 02:00–02:59 gap:
// 2024-03-10 07:00 UTC is 03:00 EDT (the 2 a.m. hour does not exist that day).
func TestRealTime_SeamDSTSpringForward(t *testing.T) {
	clock := time.Date(2024, 3, 10, 7, 0, 0, 0, time.UTC)
	svc, _ := rtService(rtCalendar("America/New_York"), clock)
	got, _ := svc.GetCalendarByID(context.Background(), "cal-rt")
	if got.CurrentMonth != 3 || got.CurrentDay != 10 || got.CurrentHour != 3 || got.CurrentMinute != 0 {
		t.Errorf("spring-forward local = %02d-%02d %02d:%02d, want 03-10 03:00",
			got.CurrentMonth, got.CurrentDay, got.CurrentHour, got.CurrentMinute)
	}
}

// TestRealTime_SeamDSTFallBack pins the fall-back instant: 2024-11-03 06:30 UTC
// is 01:30 EST (the SECOND 1:30 that day, after clocks fall back from EDT).
func TestRealTime_SeamDSTFallBack(t *testing.T) {
	clock := time.Date(2024, 11, 3, 6, 30, 0, 0, time.UTC)
	svc, _ := rtService(rtCalendar("America/New_York"), clock)
	got, _ := svc.GetCalendarByID(context.Background(), "cal-rt")
	if got.CurrentMonth != 11 || got.CurrentDay != 3 || got.CurrentHour != 1 || got.CurrentMinute != 30 {
		t.Errorf("fall-back local = %02d-%02d %02d:%02d, want 11-03 01:30",
			got.CurrentMonth, got.CurrentDay, got.CurrentHour, got.CurrentMinute)
	}
}

// TestRealTime_SeamMinuteRollover pins minute/hour extraction across a boundary:
// two adjacent clocks one minute apart roll 08:59 → 09:00 in EDT.
func TestRealTime_SeamMinuteRollover(t *testing.T) {
	cal := rtCalendar("America/New_York")
	before, _ := rtService(cal, time.Date(2024, 6, 15, 12, 59, 30, 0, time.UTC)) // 08:59 EDT
	got, _ := before.GetCalendarByID(context.Background(), "cal-rt")
	if got.CurrentHour != 8 || got.CurrentMinute != 59 {
		t.Errorf("pre-rollover = %02d:%02d, want 08:59", got.CurrentHour, got.CurrentMinute)
	}
	after, _ := rtService(cal, time.Date(2024, 6, 15, 13, 0, 0, 0, time.UTC)) // 09:00 EDT
	got2, _ := after.GetCalendarByID(context.Background(), "cal-rt")
	if got2.CurrentHour != 9 || got2.CurrentMinute != 0 {
		t.Errorf("post-rollover = %02d:%02d, want 09:00", got2.CurrentHour, got2.CurrentMinute)
	}
}

// --- month geometry (stdlib 4/100/400) ---

// TestRealTime_MonthGeometryGregorian pins the scope-item-5 geometry: a
// real-time calendar renders Feb 2100 as 28 days (stdlib, correct) and Feb 2024
// as 29 (leap) — NOT the broken LeapYearEvery=4 result.
func TestRealTime_MonthGeometryGregorian(t *testing.T) {
	cal := rtCalendar("America/New_York") // real-time flag ON
	if got := cal.MonthDays(1, 2100); got != 28 {
		t.Errorf("real-time Feb 2100 = %d days, want 28 (stdlib 4/100/400)", got)
	}
	if got := cal.MonthDays(1, 2024); got != 29 {
		t.Errorf("real-time Feb 2024 = %d days, want 29 (leap)", got)
	}
	if got := cal.MonthDays(0, 2100); got != 31 {
		t.Errorf("real-time Jan 2100 = %d days, want 31", got)
	}
}

// TestRealTime_ManualReallifeGeometryUnchanged is the stop-and-flag #2 pin: a
// reallife-but-MANUAL calendar (tracks_real_time=0) keeps its configured
// geometry byte-for-byte — Feb 2100 still renders the (wrong-but-unchanged) 29
// from LeapYearEvery=4. The migration is provably zero-change for such rows.
func TestRealTime_ManualReallifeGeometryUnchanged(t *testing.T) {
	manual := rtCalendar("America/New_York")
	manual.TracksRealTime = false // reallife, but manual
	if got := manual.MonthDays(1, 2100); got != 29 {
		t.Errorf("manual reallife Feb 2100 = %d, want 29 unchanged (configured LeapYearEvery=4)", got)
	}
}

// --- zero-change seam (stop-and-flag #2) ---

// TestRealTime_SeamNoOpForNonRealTime pins that the seam NEVER touches a
// non-real-time calendar's stored date: a reallife-manual calendar and a
// fantasy calendar both come back with their stored Current* intact.
func TestRealTime_SeamNoOpForNonRealTime(t *testing.T) {
	clock := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)

	manual := rtCalendar("America/New_York")
	manual.TracksRealTime = false
	manual.CurrentYear, manual.CurrentMonth, manual.CurrentDay = 1492, 8, 21
	svc, _ := rtService(manual, clock)
	got, _ := svc.GetCalendarByID(context.Background(), "cal-rt")
	if got.CurrentYear != 1492 || got.CurrentMonth != 8 || got.CurrentDay != 21 {
		t.Errorf("manual reallife stored date changed: %04d-%02d-%02d, want 1492-08-21",
			got.CurrentYear, got.CurrentMonth, got.CurrentDay)
	}

	fantasy := &Calendar{ID: "cal-f", Mode: ModeFantasy, CurrentYear: 998, CurrentMonth: 3, CurrentDay: 7}
	svc2, _ := rtService(fantasy, clock)
	gotF, _ := svc2.GetCalendarByID(context.Background(), "cal-f")
	if gotF.CurrentYear != 998 || gotF.CurrentMonth != 3 || gotF.CurrentDay != 7 {
		t.Errorf("fantasy stored date changed: %04d-%02d-%02d, want 998-03-07",
			gotF.CurrentYear, gotF.CurrentMonth, gotF.CurrentDay)
	}
}

// --- zone-load fail-safe (dispatch scope item 6) ---

// TestRealTime_BadZoneServesStoredDate pins that an invalid or blank stored zone
// falls back to the STORED date (best-effort) and never errors/500s.
func TestRealTime_BadZoneServesStoredDate(t *testing.T) {
	clock := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)

	bad := rtCalendar("Not/AZone")
	bad.CurrentYear, bad.CurrentMonth, bad.CurrentDay = 2011, 5, 4
	svc, _ := rtService(bad, clock)
	got, err := svc.GetCalendarByID(context.Background(), "cal-rt")
	if err != nil {
		t.Fatalf("bad zone must not error: %v", err)
	}
	if got.CurrentYear != 2011 || got.CurrentMonth != 5 || got.CurrentDay != 4 {
		t.Errorf("bad zone should serve stored date, got %04d-%02d-%02d, want 2011-05-04",
			got.CurrentYear, got.CurrentMonth, got.CurrentDay)
	}

	blank := rtCalendar("")
	blank.RealTimeZone = nil
	blank.CurrentYear, blank.CurrentMonth, blank.CurrentDay = 2011, 5, 4
	svc2, _ := rtService(blank, clock)
	got2, err := svc2.GetCalendarByID(context.Background(), "cal-rt")
	if err != nil {
		t.Fatalf("nil zone must not error: %v", err)
	}
	if got2.CurrentDay != 4 {
		t.Errorf("nil zone should serve stored date, got day %d, want 4", got2.CurrentDay)
	}
}

// --- manual-date-write guards (W1–W7) ---

func rtStrPtr(s string) *string { return &s }

// trackingService is rtService plus an updateCalled flag so guard tests can
// assert the repo write was NEVER reached.
func trackingService(cal *Calendar, clock time.Time) (*calendarService, *bool) {
	called := false
	repo := &mockCalendarRepo{
		getByIDFn:         func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		updateFn:          func(_ context.Context, _ *Calendar) error { called = true; return nil },
		getMonthsFn:       func(_ context.Context, _ string) ([]Month, error) { return rtGregorianMonths(), nil },
		setDayWeatherFn:   func(_ context.Context, _ string, _, _, _ int, _ string) error { return nil },
	}
	return &calendarService{repo: repo, events: NoopCalendarEventPublisher{}, now: rtClock(clock)}, &called
}

var rtFixedClock = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

// TestRealTime_GuardAdvanceDate — W1: a real-time calendar rejects AdvanceDate
// with a validation error and never writes.
func TestRealTime_GuardAdvanceDate(t *testing.T) {
	svc, updated := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	err := svc.AdvanceDate(context.Background(), "cal-rt", 1)
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("AdvanceDate on real-time calendar: err = %v, want validation", err)
	}
	if *updated {
		t.Error("AdvanceDate must not write on a real-time calendar")
	}
}

// TestRealTime_GuardAdvanceTime — W2.
func TestRealTime_GuardAdvanceTime(t *testing.T) {
	svc, updated := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	err := svc.AdvanceTime(context.Background(), "cal-rt", 1, 0)
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("AdvanceTime: err = %v, want validation", err)
	}
	if *updated {
		t.Error("AdvanceTime must not write on a real-time calendar")
	}
}

// TestRealTime_GuardSetDate — W3, the external-sync (Foundry/Calendaria)
// date-push writer. The validation apperror is what the syncapi handler
// surfaces to the module as a clean wire error.
func TestRealTime_GuardSetDate(t *testing.T) {
	svc, updated := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	err := svc.SetDate(context.Background(), "cal-rt", 2030, 1, 1, 0, 0)
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("SetDate: err = %v, want validation", err)
	}
	if *updated {
		t.Error("SetDate must not write on a real-time calendar")
	}
}

// TestRealTime_GuardUpdateCalendarDateChange — W6: a settings save that CHANGES
// the date is rejected, but a save that leaves the date untouched (name-only)
// still writes — non-date settings stay editable.
func TestRealTime_GuardUpdateCalendarDateChange(t *testing.T) {
	// Date change → rejected.
	svc, updated := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	err := svc.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Session Calendar", CurrentYear: 2030, CurrentMonth: 1, CurrentDay: 1,
		CurrentHour: 0, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	})
	if !isAppErrorType(err, "validation_error") {
		t.Fatalf("UpdateCalendar date change: err = %v, want validation", err)
	}
	if *updated {
		t.Error("UpdateCalendar must not write a date change on a real-time calendar")
	}

	// Same date, different name → allowed (stored date is 2000-01-01 00:00).
	svc2, updated2 := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	err = svc2.UpdateCalendar(context.Background(), "cal-rt", UpdateCalendarInput{
		Name: "Renamed", CurrentYear: 2000, CurrentMonth: 1, CurrentDay: 1,
		CurrentHour: 0, CurrentMinute: 0,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
	})
	if err != nil {
		t.Fatalf("non-date settings save must succeed on a real-time calendar: %v", err)
	}
	if !*updated2 {
		t.Error("non-date settings save should write")
	}
}

// TestRealTime_GuardWorldstateAdvanceAndTime — W4/W5: the GM panel's Advance and
// absolute set-time verbs are rejected on a real-time calendar.
func TestRealTime_GuardWorldstateAdvanceAndTime(t *testing.T) {
	svc, updated := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	if err := svc.SetWorldState(context.Background(), "cal-rt",
		WorldStateUpdateInput{Advance: &WorldStateAdvance{Days: 1}}); !isAppErrorType(err, "validation_error") {
		t.Fatalf("worldstate Advance: err = %v, want validation", err)
	}
	yr := 2030
	if err := svc.SetWorldState(context.Background(), "cal-rt",
		WorldStateUpdateInput{Time: &WorldStateTimeSet{Year: &yr}}); !isAppErrorType(err, "validation_error") {
		t.Fatalf("worldstate Time: err = %v, want validation", err)
	}
	if *updated {
		t.Error("worldstate date verbs must not write on a real-time calendar")
	}
}

// TestRealTime_WorldstateContentAuthoringAllowed — RC-5: weather/celestial
// content authoring onto the live "today" stays editable on a real-time
// calendar; only the date-moving verbs are guarded.
func TestRealTime_WorldstateContentAuthoringAllowed(t *testing.T) {
	svc, _ := trackingService(rtCalendar("America/New_York"), rtFixedClock)
	if err := svc.SetWorldState(context.Background(), "cal-rt",
		WorldStateUpdateInput{Weather: rtStrPtr("rain")}); err != nil {
		t.Errorf("weather authoring must stay editable on a real-time calendar (RC-5): %v", err)
	}
}

// TestRealTime_ManualWritersAllowed — the guards are FLAG-gated: a
// reallife-but-manual calendar still advances and sets its date normally.
func TestRealTime_ManualWritersAllowed(t *testing.T) {
	manual := rtCalendar("America/New_York")
	manual.TracksRealTime = false
	svc, updated := trackingService(manual, rtFixedClock)
	if err := svc.AdvanceDate(context.Background(), "cal-rt", 1); err != nil {
		t.Fatalf("manual reallife AdvanceDate should succeed: %v", err)
	}
	if !*updated {
		t.Error("manual reallife AdvanceDate should write")
	}
}

// --- wire + Player-role pins: the viewer receives the COMPUTED live date ---

// TestRealTime_WirePublicSnapshotServesLiveDate drives the public Foundry-facing
// APIHandler.GetCalendar backed by the REAL seam-bearing service and asserts the
// JSON snapshot carries the computed live date, not the stale stored one. This
// is also the syncapi GET-path coverage (both public GETs route through
// svc.GetCalendar).
func TestRealTime_WirePublicSnapshotServesLiveDate(t *testing.T) {
	// 2024-02-29 12:00 UTC → 07:00 EST.
	svc, _ := rtService(rtCalendar("America/New_York"), time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC))
	h := NewAPIHandler(svc, &stubVerifier{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/calendar", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cid")
	c.SetParamValues("camp-1")
	c.QueryParams().Set("token", "ok-token")

	if err := h.GetCalendar(c); err != nil {
		t.Fatalf("GetCalendar: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got apiCalendarSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if got.CurrentYear != 2024 || got.CurrentMonth != 2 || got.CurrentDay != 29 || got.CurrentHour != 7 {
		t.Errorf("wire snapshot = %04d-%02d-%02d %02d:xx, want live 2024-02-29 07",
			got.CurrentYear, got.CurrentMonth, got.CurrentDay, got.CurrentHour)
	}
}

// TestRealTime_PlayerRoleSeesLiveDate — the composed visibility pin (RC-3): a
// Player-role viewer resolving through GetActiveVisibleCalendar receives the
// live computed date on an everyone-visible real-time calendar.
func TestRealTime_PlayerRoleSeesLiveDate(t *testing.T) {
	svc, _ := rtService(rtCalendar("America/New_York"), time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC))
	got, err := svc.GetActiveVisibleCalendar(context.Background(), "camp-1", int(campaigns.RolePlayer), "user-1")
	if err != nil {
		t.Fatalf("GetActiveVisibleCalendar: %v", err)
	}
	if got == nil {
		t.Fatal("Player should see the everyone-visible calendar")
	}
	if got.CurrentYear != 2024 || got.CurrentMonth != 2 || got.CurrentDay != 29 || got.CurrentHour != 7 {
		t.Errorf("Player-visible date = %04d-%02d-%02d %02d:xx, want live 2024-02-29 07",
			got.CurrentYear, got.CurrentMonth, got.CurrentDay, got.CurrentHour)
	}
}
