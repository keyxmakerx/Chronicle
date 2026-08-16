package sessions

import (
	"context"
	"testing"
)

func TestSaveMyAvailability_InvalidTimezone(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.SaveMyAvailability(context.Background(), "camp-1", "u1", SaveAvailabilityRequest{
		TZ:     "Not/AZone",
		Blocks: []AvailabilityBlockDTO{{DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: AvailAvailable}},
	})
	assertAppError(t, err, 400)
}

func TestSaveMyAvailability_InvalidRange(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.SaveMyAvailability(context.Background(), "camp-1", "u1", SaveAvailabilityRequest{
		TZ:     "America/New_York",
		Blocks: []AvailabilityBlockDTO{{DayOfWeek: 2, StartMinute: 1200, EndMinute: 1080, State: AvailAvailable}}, // end < start
	})
	assertAppError(t, err, 400)
}

func TestSaveMyAvailability_InvalidState(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.SaveMyAvailability(context.Background(), "camp-1", "u1", SaveAvailabilityRequest{
		TZ:     "America/New_York",
		Blocks: []AvailabilityBlockDTO{{DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: "unavailable"}}, // not allowed for recurring
	})
	assertAppError(t, err, 400)
}

func TestSaveMyAvailability_ConvertsAndDedupes(t *testing.T) {
	var saved []AvailabilityBlock
	var savedTZ string
	repo := &mockSessionRepo{
		replaceUserAvailabilityFn: func(_ context.Context, campaignID, userID, _ string, blocks []AvailabilityBlock) error {
			if campaignID != "camp-1" || userID != "u1" {
				t.Errorf("wrong scope: %s / %s", campaignID, userID)
			}
			saved = blocks
			if len(blocks) > 0 {
				savedTZ = blocks[0].TZ
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)
	err := svc.SaveMyAvailability(context.Background(), "camp-1", "u1", SaveAvailabilityRequest{
		TZ: "America/New_York",
		Blocks: []AvailabilityBlockDTO{
			{DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: AvailAvailable},
			{DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: AvailPreferred}, // same key → last wins
			{DayOfWeek: 5, StartMinute: 720, EndMinute: 1080, State: AvailAvailable},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 deduped blocks, got %d", len(saved))
	}
	if savedTZ != "America/New_York" {
		t.Errorf("expected tz stamped on rows, got %q", savedTZ)
	}
	// The Tuesday block should carry the last-written state (preferred).
	for _, b := range saved {
		if b.DayOfWeek == 2 && b.State != AvailPreferred {
			t.Errorf("dedupe should keep last state (preferred), got %q", b.State)
		}
	}
}

func TestGetMyAvailability_MapsRows(t *testing.T) {
	repo := &mockSessionRepo{
		listUserAvailabilityFn: func(_ context.Context, _, _ string) ([]AvailabilityBlock, error) {
			return []AvailabilityBlock{
				{DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: AvailAvailable, TZ: "America/New_York"},
			}, nil
		},
	}
	svc := newTestSessionService(repo)
	resp, err := svc.GetMyAvailability(context.Background(), "camp-1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TZ != "America/New_York" || len(resp.Blocks) != 1 {
		t.Fatalf("resp = %+v, want tz + 1 block", resp)
	}
	if resp.Blocks[0].DayOfWeek != 2 || resp.Blocks[0].State != AvailAvailable {
		t.Errorf("block = %+v", resp.Blocks[0])
	}
}

func TestAddMyException_InvalidDate(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.AddMyException(context.Background(), "camp-1", "u1", AddExceptionRequest{
		OnDate: "07/16/2026", StartMinute: 0, EndMinute: 1440, State: AvailUnavailable, TZ: "America/New_York",
	})
	assertAppError(t, err, 400)
}

// RE-POINTED, NOT WEAKENED. This used to assert that AddMyException reached
// repo.AddException — a single-row insert. That was the defect: exception rows
// REPLACE the recurring pattern for their date, so the one row inserted became
// the member's whole day and silently deleted the rest of it. AddMyException now
// composes the day and writes it atomically through ReplaceDayExceptions, so the
// same scope assertions (campaign, user, date) are made against that call
// instead. The write path changed; nothing it checks was dropped.
func TestAddMyException_Valid(t *testing.T) {
	called := false
	repo := &mockSessionRepo{
		replaceDayExceptionsFn: func(_ context.Context, campaignID, userID, onDate string, excs []AvailabilityException) error {
			called = true
			if campaignID != "camp-1" || userID != "u1" || onDate != "2026-07-16" {
				t.Errorf("exception scope/date wrong: campaign=%q user=%q date=%q", campaignID, userID, onDate)
			}
			for _, e := range excs {
				if e.TZ != "America/New_York" {
					t.Errorf("row written in %q, want the requested America/New_York: %+v", e.TZ, e)
				}
			}
			return nil
		},
		addExceptionFn: func(_ context.Context, e *AvailabilityException) error {
			t.Errorf("AddMyException took the single-row insert path again (%+v) — "+
				"that path erases the rest of the member's day", e)
			return nil
		},
	}
	svc := newTestSessionService(repo)
	err := svc.AddMyException(context.Background(), "camp-1", "u1", AddExceptionRequest{
		OnDate: "2026-07-16", StartMinute: 0, EndMinute: 1440, State: AvailUnavailable, TZ: "America/New_York",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected the composed day to be written through ReplaceDayExceptions")
	}
}

func TestBuildOverlay_InvalidWeek(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	_, err := svc.BuildOverlay(context.Background(), "camp-1", nil, "not-a-date", "UTC", true)
	assertAppError(t, err, 400)
}

func TestBuildOverlay_SnapsToMondayAndAggregates(t *testing.T) {
	repo := &mockSessionRepo{
		listCampaignAvailabilityFn: func(_ context.Context, _ string) ([]AvailabilityBlock, error) {
			return []AvailabilityBlock{
				{UserID: "u1", DayOfWeek: 2, StartMinute: 1080, EndMinute: 1260, State: AvailAvailable, TZ: "UTC"},
			}, nil
		},
	}
	svc := newTestSessionService(repo)
	// Request a Wednesday (2026-07-15) — must snap back to Monday 2026-07-13.
	ov, err := svc.BuildOverlay(context.Background(), "camp-1",
		[]overlayMemberInput{{UserID: "u1", Name: "Alex"}}, "2026-07-15", "UTC", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ov.Days) != 7 || ov.Days[0].Date != "2026-07-13" {
		t.Fatalf("week did not snap to Monday: Days[0] = %s", ov.Days[0].Date)
	}
	if ov.Days[1].Hours[18].Free != 1 {
		t.Errorf("Tue 18:00 UTC Free = %d, want 1", ov.Days[1].Hours[18].Free)
	}
}
