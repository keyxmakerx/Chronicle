package sessions

import (
	"context"
	"testing"
	"time"
)

// C-SCHED-P2 0d: AddMyException must bound the date to today ±1 year and cap the
// per-user exception count.

func TestAddMyException_DateBound(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil)
	farFuture := time.Now().UTC().AddDate(0, 0, exceptionDateWindowDays+30).Format("2006-01-02")
	err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: farFuture, StartMinute: 60, EndMinute: 120, State: AvailUnavailable, TZ: "UTC",
	})
	if err == nil {
		t.Errorf("expected rejection for a date beyond the today±1y window (%s)", farFuture)
	}
}

func TestAddMyException_PerUserCap(t *testing.T) {
	repo := &mockSessionRepo{countUserExceptionsFn: func(_ context.Context, _, _ string) (int, error) {
		return maxExceptionsPerUser, nil // already at the ceiling
	}}
	svc := NewSessionService(repo, nil)
	err := svc.AddMyException(context.Background(), "c1", "u1", AddExceptionRequest{
		OnDate: time.Now().UTC().Format("2006-01-02"), StartMinute: 60, EndMinute: 120, State: AvailAvailable, TZ: "UTC",
	})
	if err == nil {
		t.Error("expected rejection when the per-user exception cap is reached")
	}
}

// C-SCHED-P2 0c: the day-replace forwards the composed set atomically.

func TestReplaceMyDayExceptions_ForwardsComposedDay(t *testing.T) {
	var gotOnDate string
	var gotExcs []AvailabilityException
	repo := &mockSessionRepo{
		replaceDayExceptionsFn: func(_ context.Context, _, _, onDate string, excs []AvailabilityException) error {
			gotOnDate = onDate
			gotExcs = excs
			return nil
		},
	}
	svc := NewSessionService(repo, nil)
	// A one-hour busy mark (19–20) composed as the rest of the evening staying
	// available (18–19, 20–23) — the whole day is re-sent, not just the hole.
	err := svc.ReplaceMyDayExceptions(context.Background(), "c1", "u1", ReplaceDayExceptionsRequest{
		OnDate: time.Now().UTC().Format("2006-01-02"),
		TZ:     "UTC",
		Blocks: []ExceptionBlockDTO{
			{StartMinute: 18 * 60, EndMinute: 19 * 60, State: AvailAvailable},
			{StartMinute: 20 * 60, EndMinute: 23 * 60, State: AvailAvailable},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOnDate == "" || len(gotExcs) != 2 {
		t.Fatalf("expected 2 composed blocks forwarded for the date, got %d", len(gotExcs))
	}
}

func TestReplaceMyDayExceptions_PerDayBlockCap(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil)
	blocks := make([]ExceptionBlockDTO, maxExceptionBlocksPerDay+1)
	for i := range blocks {
		blocks[i] = ExceptionBlockDTO{StartMinute: 0, EndMinute: 30, State: AvailAvailable}
	}
	err := svc.ReplaceMyDayExceptions(context.Background(), "c1", "u1", ReplaceDayExceptionsRequest{
		OnDate: time.Now().UTC().Format("2006-01-02"), TZ: "UTC", Blocks: blocks,
	})
	if err == nil {
		t.Error("expected rejection when a single day exceeds the per-day block cap")
	}
}

// C-SCHED-P2 0c behavioral pin: a composed day (recurring minus one busy hour)
// leaves the day's other hours VISIBLE in the overlay rather than erasing them.
// The exception rows are the composed available blocks; the overlay renders them
// as the effective day (replace-day semantics, composed input).
func TestBuildWeekOverlay_ComposedDayKeepsOtherHours(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "u1", Name: "Alex"}}
	avail := map[string][]AvailabilityBlock{
		"u1": {block(2 /*Tue*/, 18*60, 22*60, AvailAvailable, "America/New_York")},
	}
	// Tue 2026-07-14: composed exception = available 18–19 and 20–22 (the member
	// marked 19–20 busy through the compose flow, re-sending the rest).
	exc := map[string][]AvailabilityException{
		"u1": {
			{OnDate: "2026-07-14", StartMinute: 18 * 60, EndMinute: 19 * 60, State: AvailAvailable, TZ: "America/New_York"},
			{OnDate: "2026-07-14", StartMinute: 20 * 60, EndMinute: 22 * 60, State: AvailAvailable, TZ: "America/New_York"},
		},
	}
	ov := buildWeekOverlay(members, avail, exc, testWeekStart(), ny, "America/New_York", true)
	tue := ov.Days[1]
	if tue.Hours[18].Free != 1 {
		t.Errorf("hour 18 Free = %d, want 1 (kept)", tue.Hours[18].Free)
	}
	if tue.Hours[19].Free != 0 {
		t.Errorf("hour 19 Free = %d, want 0 (the busy hour)", tue.Hours[19].Free)
	}
	if tue.Hours[20].Free != 1 || tue.Hours[21].Free != 1 {
		t.Errorf("hours 20/21 Free = %d/%d, want 1/1 (rest of day preserved)", tue.Hours[20].Free, tue.Hours[21].Free)
	}
}
