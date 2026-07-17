package syncapi

// calendar_date_beacon_test.go — C-SYNC-DATE-BEACON. Pins
// RecordCalendarDateBeacon's write-throttle decision (skip iff the last
// beacon is <60s old AND the date is unchanged; a changed date always
// writes) and GetCalendarDateBeacon's plain passthrough. The
// Bearer-vs-session auth gate itself is tested at the handler layer
// (calendar_api_handler_test.go), since that decision reads GetAPIKey(c)
// off the Echo context, not anything in this service.

import (
	"context"
	"testing"
	"time"
)

func TestRecordCalendarDateBeacon_NoExistingBeacon_AlwaysWrites(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	svc := NewSyncAPIService(repo)

	if err := svc.RecordCalendarDateBeacon(context.Background(), "camp-1", 2026, 7, 17); err != nil {
		t.Fatalf("RecordCalendarDateBeacon: %v", err)
	}
	if len(repo.upsertBeaconCalls) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(repo.upsertBeaconCalls))
	}
	got := repo.upsertBeaconCalls[0]
	if got.CampaignID != "camp-1" || got.Year != 2026 || got.Month != 7 || got.Day != 17 {
		t.Errorf("upsert beacon = %+v, want campaign camp-1 2026-07-17", got)
	}
}

func TestRecordCalendarDateBeacon_SameDateFreshBeacon_Throttled(t *testing.T) {
	existing := &CalendarDateBeacon{
		CampaignID: "camp-1", Year: 2026, Month: 7, Day: 17,
		ServedAt: time.Now().UTC().Add(-30 * time.Second), // 30s old, under the 60s throttle
	}
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
			return existing, nil
		},
	}
	svc := NewSyncAPIService(repo)

	if err := svc.RecordCalendarDateBeacon(context.Background(), "camp-1", 2026, 7, 17); err != nil {
		t.Fatalf("RecordCalendarDateBeacon: %v", err)
	}
	if len(repo.upsertBeaconCalls) != 0 {
		t.Fatalf("expected the write to be throttled (0 upsert calls), got %d", len(repo.upsertBeaconCalls))
	}
}

func TestRecordCalendarDateBeacon_SameDateStaleBeacon_Writes(t *testing.T) {
	existing := &CalendarDateBeacon{
		CampaignID: "camp-1", Year: 2026, Month: 7, Day: 17,
		ServedAt: time.Now().UTC().Add(-90 * time.Second), // 90s old, past the 60s throttle
	}
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
			return existing, nil
		},
	}
	svc := NewSyncAPIService(repo)

	if err := svc.RecordCalendarDateBeacon(context.Background(), "camp-1", 2026, 7, 17); err != nil {
		t.Fatalf("RecordCalendarDateBeacon: %v", err)
	}
	if len(repo.upsertBeaconCalls) != 1 {
		t.Fatalf("expected the stale beacon to be refreshed (1 upsert call), got %d", len(repo.upsertBeaconCalls))
	}
}

func TestRecordCalendarDateBeacon_DateChanged_WritesImmediatelyDespiteFreshBeacon(t *testing.T) {
	existing := &CalendarDateBeacon{
		CampaignID: "camp-1", Year: 2026, Month: 7, Day: 16,
		ServedAt: time.Now().UTC().Add(-5 * time.Second), // 5s old — well under the throttle
	}
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
			return existing, nil
		},
	}
	svc := NewSyncAPIService(repo)

	// The date advanced from 16 to 17 — the chip needs the fresher date,
	// not a stale throttled one, so this must write immediately.
	if err := svc.RecordCalendarDateBeacon(context.Background(), "camp-1", 2026, 7, 17); err != nil {
		t.Fatalf("RecordCalendarDateBeacon: %v", err)
	}
	if len(repo.upsertBeaconCalls) != 1 {
		t.Fatalf("expected a date change to write immediately (1 upsert call), got %d", len(repo.upsertBeaconCalls))
	}
	if repo.upsertBeaconCalls[0].Day != 17 {
		t.Errorf("upsert beacon day = %d, want 17", repo.upsertBeaconCalls[0].Day)
	}
}

func TestRecordCalendarDateBeacon_GetError_PropagatesWithoutWriting(t *testing.T) {
	wantErr := context.DeadlineExceeded
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
			return nil, wantErr
		},
	}
	svc := NewSyncAPIService(repo)

	err := svc.RecordCalendarDateBeacon(context.Background(), "camp-1", 2026, 7, 17)
	if err != wantErr {
		t.Fatalf("RecordCalendarDateBeacon error = %v, want %v", err, wantErr)
	}
	if len(repo.upsertBeaconCalls) != 0 {
		t.Errorf("expected no write when the throttle lookup fails, got %d calls", len(repo.upsertBeaconCalls))
	}
}

func TestGetCalendarDateBeacon_DelegatesToRepo(t *testing.T) {
	want := &CalendarDateBeacon{CampaignID: "camp-1", Year: 2026, Month: 7, Day: 17, ServedAt: time.Now().UTC()}
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
			if campaignID != "camp-1" {
				t.Errorf("campaignID = %q, want camp-1", campaignID)
			}
			return want, nil
		},
	}
	svc := NewSyncAPIService(repo)

	got, err := svc.GetCalendarDateBeacon(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("GetCalendarDateBeacon: %v", err)
	}
	if got != want {
		t.Errorf("GetCalendarDateBeacon = %+v, want %+v", got, want)
	}
}

func TestGetCalendarDateBeacon_NoneRecorded_ReturnsNilNoError(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	svc := NewSyncAPIService(repo)

	got, err := svc.GetCalendarDateBeacon(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("GetCalendarDateBeacon: %v", err)
	}
	if got != nil {
		t.Errorf("GetCalendarDateBeacon = %+v, want nil", got)
	}
}
