package syncapi

// calendar_confirm_date_test.go — C-SYNC-APPLIED-BEACON. Pins
// syncAPIService.ConfirmCalendarDate: a plain (never throttled) passthrough
// to the repository's create-or-update, since a confirm is a deliberate
// one-shot module action rather than a value re-read on every poll (unlike
// RecordCalendarDateBeacon, calendar_date_beacon_test.go). The
// Bearer-vs-session auth gate itself is tested at the handler layer
// (calendar_confirm_date_handler_test.go).

import (
	"context"
	"testing"
	"time"
)

func TestConfirmCalendarDate_AlwaysWrites_NoThrottle(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	svc := NewSyncAPIService(repo)

	if err := svc.ConfirmCalendarDate(context.Background(), "camp-1", 2026, 7, 18); err != nil {
		t.Fatalf("ConfirmCalendarDate: %v", err)
	}
	if len(repo.confirmBeaconCalls) != 1 {
		t.Fatalf("expected 1 confirm call, got %d", len(repo.confirmBeaconCalls))
	}
	got := repo.confirmBeaconCalls[0]
	if got.campaignID != "camp-1" || got.year != 2026 || got.month != 7 || got.day != 18 {
		t.Errorf("confirm call = %+v, want campaign camp-1 2026-07-18", got)
	}
	if got.appliedAt.IsZero() {
		t.Error("expected a non-zero appliedAt timestamp")
	}
}

// A second confirm immediately after the first must still write — unlike
// RecordCalendarDateBeacon there is no throttle window to skip.
func TestConfirmCalendarDate_RepeatedCallsAllWrite(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	svc := NewSyncAPIService(repo)

	for i := 0; i < 3; i++ {
		if err := svc.ConfirmCalendarDate(context.Background(), "camp-1", 2026, 7, 18); err != nil {
			t.Fatalf("ConfirmCalendarDate call %d: %v", i, err)
		}
	}
	if len(repo.confirmBeaconCalls) != 3 {
		t.Fatalf("expected 3 confirm calls (no throttle), got %d", len(repo.confirmBeaconCalls))
	}
}

func TestConfirmCalendarDate_RepoError_Propagates(t *testing.T) {
	wantErr := context.DeadlineExceeded
	repo := &mockSyncAPIRepo{
		confirmBeaconFn: func(_ context.Context, _ string, _, _, _ int, _ time.Time) error {
			return wantErr
		},
	}
	svc := NewSyncAPIService(repo)

	err := svc.ConfirmCalendarDate(context.Background(), "camp-1", 2026, 7, 18)
	if err != wantErr {
		t.Fatalf("ConfirmCalendarDate error = %v, want %v", err, wantErr)
	}
}
