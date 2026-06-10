// worldstate_w2_test.go — C-CAL-GM-PANEL-REWORK. The clear-world-events write
// path (B), the deferred-audit persistence guards (D — advance clamp + set-time
// bounds), and the players-never-get-the-panel gate.
package calendar

import (
	"context"
	"strings"
	"testing"
)

func TestSetWorldState_ClearEvents(t *testing.T) {
	cal := sampleSeedCalendar()
	var gotCal string
	var gy, gm, gd int
	called := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		clearCelestialEventsFn: func(_ context.Context, calID string, y, m, d int) error {
			called, gotCal, gy, gm, gd = true, calID, y, m, d
			return nil
		},
	}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(&recordingPublisher{})

	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{ClearEvents: true}); err != nil {
		t.Fatalf("SetWorldState{ClearEvents}: %v", err)
	}
	if !called {
		t.Fatal("ClearEvents must call ClearCelestialEvents")
	}
	if gotCal != "cal-1" || gy != cal.CurrentYear || gm != cal.CurrentMonth || gd != cal.CurrentDay {
		t.Errorf("cleared (%s %d-%d-%d); want cal-1 %d-%d-%d (the CURRENT day)",
			gotCal, gy, gm, gd, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	}
}

func TestSetWorldState_AdvanceClamped(t *testing.T) {
	cal := sampleSeedCalendar()
	advanced := false
	repo := &mockCalendarRepo{
		getByIDFn:   func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return cal.Months, nil },
		updateFn:    func(_ context.Context, _ *Calendar) error { advanced = true; return nil },
	}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(&recordingPublisher{})

	// An absurd advance is rejected BEFORE the (unbounded) advanceClock loop.
	err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Advance: &WorldStateAdvance{Days: 2000000000}})
	if err == nil {
		t.Error("an out-of-range advance must be rejected (the deferred DoS guard)")
	}
	if advanced {
		t.Error("the clock must not be written for a rejected advance")
	}

	// A normal GM verb still applies.
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Advance: &WorldStateAdvance{Days: 1}}); err != nil {
		t.Errorf("a normal +1 day advance should succeed: %v", err)
	}
	if !advanced {
		t.Error("a normal advance should write the clock")
	}
}

func TestSetWorldState_SetTimeOutOfRange(t *testing.T) {
	cal := sampleSeedCalendar()
	wrote := false
	repo := &mockCalendarRepo{
		getByIDFn:   func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return cal.Months, nil },
		updateFn:    func(_ context.Context, _ *Calendar) error { wrote = true; return nil },
	}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(&recordingPublisher{})

	bigMonth := 999
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Time: &WorldStateTimeSet{Month: &bigMonth}}); err == nil {
		t.Error("month=999 must be rejected (set-time upper bound)")
	}
	bigDay := 999
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Time: &WorldStateTimeSet{Day: &bigDay}}); err == nil {
		t.Error("day=999 must be rejected (set-time upper bound)")
	}
	if wrote {
		t.Error("no write should happen for an out-of-range set-time")
	}

	// A valid set-time still works.
	okMonth, okDay := 2, 10
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Time: &WorldStateTimeSet{Month: &okMonth, Day: &okDay}}); err != nil {
		t.Errorf("a valid set-time should succeed: %v", err)
	}
	if !wrote {
		t.Error("a valid set-time should write the date")
	}
}

func TestGMPanel_PlayersGetNothing(t *testing.T) {
	cal := &Calendar{ID: "c", Name: "X", CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1, Months: []Month{{Name: "Jan", Days: 30}}}

	// Player (no world-state control) → no panel markup at all.
	var player strings.Builder
	if err := gmControlPanelV2(CalendarV2ViewData{ActiveCalendar: cal, CanControlWorldState: false}).Render(context.Background(), &player); err != nil {
		t.Fatalf("render (player): %v", err)
	}
	if strings.TrimSpace(player.String()) != "" {
		t.Errorf("players must NOT receive the GM panel markup; got %q", player.String())
	}

	// Owner/co-DM → the panel, including the new clear-world-events button.
	var owner strings.Builder
	if err := gmControlPanelV2(CalendarV2ViewData{ActiveCalendar: cal, CanControlWorldState: true}).Render(context.Background(), &owner); err != nil {
		t.Fatalf("render (owner): %v", err)
	}
	for _, want := range []string{"data-gm-panel", "data-gm-events-clear", "data-gm-sheet-panel"} {
		if !strings.Contains(owner.String(), want) {
			t.Errorf("owner GM panel missing %q", want)
		}
	}
	// A re-anchored overlay, not the old fixed bottom-right float.
	if strings.Contains(owner.String(), "fixed bottom-4 right-4") {
		t.Error("panel must be re-anchored as an in-band overlay, not fixed bottom-right")
	}
}
