// anchor_preview_test.go: table-driven tests for CalendarService.PreviewAnchorMove
// — the read-only warning-box preview for moving a calendar's real-date
// anchor (see service.go's "Real-date anchor (Part C: preview only)"
// section). Uses a fake GameNightsAffectedByAnchorMove, mirroring this
// file's other fake-repo/fake-gate patterns (mocks_test.go,
// service_integration_test.go's testEntityGate).
package calendar

import (
	"context"
	"testing"
	"time"
)

// fakeGameNightsGate is a minimal, function-injection fake for
// GameNightsAffectedByAnchorMove: nil fn returns no sessions, and every call
// records the limit it was invoked with so a test can pin the cap.
type fakeGameNightsGate struct {
	fn              func(ctx context.Context, campaignID string, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit int) ([]AffectedSession, error)
	called          bool
	calledWithLimit int
}

func (g *fakeGameNightsGate) SessionsInWorldDateRange(ctx context.Context, campaignID string, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit int) ([]AffectedSession, error) {
	g.called = true
	g.calledWithLimit = limit
	if g.fn != nil {
		return g.fn(ctx, campaignID, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit)
	}
	return nil, nil
}

// anchoredTestCalendar returns a fantasy calendar (12 months of 30 days, no
// leap years) anchored at (1000,1,1) <-> 2020-01-01, and the fakeCalendarRepo
// serving it, for PreviewAnchorMove tests.
func anchoredTestCalendar(t *testing.T) (*fakeCalendarRepo, Calendar) {
	t.Helper()
	anchorYear, anchorMonth, anchorDay := 1000, 1, 1
	anchorReal := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	cal := Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy,
		CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 1,
		AnchorYear: &anchorYear, AnchorMonth: &anchorMonth, AnchorDay: &anchorDay, AnchorRealDate: &anchorReal,
	}
	months := make([]Month, 12)
	for i := range months {
		months[i] = Month{Days: 30}
	}
	repo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			c := cal
			return &c, nil
		},
		getMonthsFn:   func(_ context.Context, _ string) ([]Month, error) { return months, nil },
		getWeekdaysFn: func(_ context.Context, _ string) ([]Weekday, error) { return make([]Weekday, 7), nil },
	}
	return repo, cal
}

func TestPreviewAnchorMove_NoAnchorYet(t *testing.T) {
	repo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Mode: ModeFantasy}, nil
		},
	}
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	gate := &fakeGameNightsGate{}
	cs.SetGameNightsAffectedByAnchorMove(gate)

	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA, 1, 1, 1, time.Now())
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if !got.FirstTimeSet {
		t.Errorf("FirstTimeSet = false, want true for a calendar with no anchor yet")
	}
	if got.DeltaDays != 0 || len(got.Affected) != 0 {
		t.Errorf("a first-time set must report no delta and no affected sessions, got %+v", got)
	}
	if gate.called {
		t.Errorf("the sessions gate must not be queried for a first-time anchor set")
	}
}

func TestPreviewAnchorMove_PositiveShift(t *testing.T) {
	repo, cal := anchoredTestCalendar(t)
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	gate := &fakeGameNightsGate{
		fn: func(_ context.Context, _ string, _, _, _, _, _, _, _ int) ([]AffectedSession, error) {
			return []AffectedSession{{Name: "Session A", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 5}}, nil
		},
	}
	cs.SetGameNightsAffectedByAnchorMove(gate)

	// Same in-world anchor date, real date moved 10 days later.
	newReal := cal.AnchorRealDate.AddDate(0, 0, 10)
	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA,
		*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay, newReal)
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if got.FirstTimeSet {
		t.Errorf("an already-anchored calendar must not report FirstTimeSet")
	}
	if got.DeltaDays != 10 {
		t.Errorf("DeltaDays = %d, want 10 (the real anchor alone moved 10 days later)", got.DeltaDays)
	}
	if len(got.Affected) != 1 {
		t.Fatalf("expected 1 affected session, got %d", len(got.Affected))
	}
	a := got.Affected[0]
	// Session A's in-world date (1000,1,5) is 4 days after the anchor
	// (1000,1,1), so its OLD real date is 2020-01-01 + 4 = 2020-01-05, and
	// its NEW real date is that plus the 10-day delta: 2020-01-15.
	if a.Name != "Session A" || a.OldYear != 2020 || a.OldMonth != 1 || a.OldDay != 5 {
		t.Errorf("old date = %+v, want 2020-01-05", a)
	}
	if a.NewYear != 2020 || a.NewMonth != 1 || a.NewDay != 15 {
		t.Errorf("new date = %+v, want 2020-01-15", a)
	}
}

func TestPreviewAnchorMove_NegativeShift(t *testing.T) {
	repo, cal := anchoredTestCalendar(t)
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	gate := &fakeGameNightsGate{
		fn: func(_ context.Context, _ string, _, _, _, _, _, _, _ int) ([]AffectedSession, error) {
			return []AffectedSession{{Name: "Session B", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 1}}, nil
		},
	}
	cs.SetGameNightsAffectedByAnchorMove(gate)

	newReal := cal.AnchorRealDate.AddDate(0, 0, -5)
	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA,
		*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay, newReal)
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if got.DeltaDays != -5 {
		t.Errorf("DeltaDays = %d, want -5", got.DeltaDays)
	}
	if len(got.Affected) != 1 {
		t.Fatalf("expected 1 affected session, got %d", len(got.Affected))
	}
	a := got.Affected[0]
	if a.OldYear != 2020 || a.OldMonth != 1 || a.OldDay != 1 {
		t.Errorf("old date = %+v, want 2020-01-01 (session sits exactly on the anchor)", a)
	}
	if a.NewYear != 2019 || a.NewMonth != 12 || a.NewDay != 27 {
		t.Errorf("new date = %+v, want 2019-12-27 (5 days earlier)", a)
	}
}

func TestPreviewAnchorMove_WorldAndRealBothMoveTheSameAmountIsANoOp(t *testing.T) {
	repo, cal := anchoredTestCalendar(t)
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	gate := &fakeGameNightsGate{}
	cs.SetGameNightsAffectedByAnchorMove(gate)

	// Re-pinning the anchor 3 in-world days later AND 3 real days later
	// changes nothing about the mapping — every other day's real date is
	// unaffected, so DeltaDays is 0 and the gate is never even queried.
	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA,
		*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay+3, cal.AnchorRealDate.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if got.DeltaDays != 0 {
		t.Errorf("DeltaDays = %d, want 0 (both ends of the anchor moved by the same amount)", got.DeltaDays)
	}
	if gate.called {
		t.Errorf("a no-op move (DeltaDays 0) must not query the sessions gate")
	}
}

func TestPreviewAnchorMove_CapsAtThreeAffected(t *testing.T) {
	repo, cal := anchoredTestCalendar(t)
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	gate := &fakeGameNightsGate{
		fn: func(_ context.Context, _ string, _, _, _, _, _, _, limit int) ([]AffectedSession, error) {
			all := []AffectedSession{
				{Name: "S1", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 1},
				{Name: "S2", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 2},
				{Name: "S3", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 3},
				{Name: "S4", OldWorldYear: 1000, OldWorldMonth: 1, OldWorldDay: 4},
			}
			if limit > 0 && limit < len(all) {
				all = all[:limit]
			}
			return all, nil
		},
	}
	cs.SetGameNightsAffectedByAnchorMove(gate)

	newReal := cal.AnchorRealDate.AddDate(0, 0, 1)
	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA,
		*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay, newReal)
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if gate.calledWithLimit != maxAnchorMovePreviewAffected {
		t.Errorf("gate called with limit %d, want %d", gate.calledWithLimit, maxAnchorMovePreviewAffected)
	}
	if len(got.Affected) != maxAnchorMovePreviewAffected {
		t.Errorf("got %d affected sessions, want the %d-item cap", len(got.Affected), maxAnchorMovePreviewAffected)
	}
}

func TestPreviewAnchorMove_UnwiredGateStillReturnsTheDelta(t *testing.T) {
	repo, cal := anchoredTestCalendar(t)
	svc := newTestCalendarService(repo, nil, nil, nil)
	cs := svc.(*calendarService)
	// No SetGameNightsAffectedByAnchorMove call: cs.gameNights stays nil,
	// mirroring EntityVisibilityGate's own nil-safe wiring.

	newReal := cal.AnchorRealDate.AddDate(0, 0, 7)
	got, err := cs.PreviewAnchorMove(context.Background(), "cal-1", testCampaignA,
		*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay, newReal)
	if err != nil {
		t.Fatalf("PreviewAnchorMove: %v", err)
	}
	if got.DeltaDays != 7 {
		t.Errorf("DeltaDays = %d, want 7", got.DeltaDays)
	}
	if len(got.Affected) != 0 {
		t.Errorf("an unwired gate must yield an empty Affected list, not an error, got %+v", got.Affected)
	}
}
