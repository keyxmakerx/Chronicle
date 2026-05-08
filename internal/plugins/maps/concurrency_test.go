package maps

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestUpdateMarker_StaleConflict locks in the optimistic-concurrency
// contract on UpdateMarker. The relay (C-MAP1) requires this so the
// Foundry editor can detect "remote changed since I started typing"
// without having to refetch on every keystroke.
//
// Three cases are sufficient:
//
//  1. ExpectedUpdatedAt omitted (last-writer-wins fallback) → 200.
//  2. ExpectedUpdatedAt equals the row's UpdatedAt → 200.
//  3. ExpectedUpdatedAt older than row's UpdatedAt → 409.
//
// The same shape applies to drawing/token/layer/fog mutations; one
// representative test on UpdateMarker proves the wiring without
// duplicating across every entity.
func TestUpdateMarker_StaleConflict(t *testing.T) {
	rowUpdated := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	stale := rowUpdated.Add(-time.Minute)

	cases := []struct {
		name         string
		expected     *time.Time
		wantHTTPCode int // 0 means no error expected
	}{
		{"omitted falls back to last-writer-wins", nil, 0},
		{"matching timestamp accepts", &rowUpdated, 0},
		{"stale timestamp rejects with 409", &stale, http.StatusConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockMapRepo{
				getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
					return &Marker{
						ID:        "mk-1",
						MapID:     "map-1",
						Name:      "Old Name",
						Visibility: "everyone",
						UpdatedAt: rowUpdated,
					}, nil
				},
			}
			svc := newTestMapService(repo)

			err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{
				Name:              "New Name",
				X:                 50,
				Y:                 50,
				Icon:              "fa-map-pin",
				Color:             "#3b82f6",
				Visibility:        "everyone",
				ExpectedUpdatedAt: tc.expected,
			})

			if tc.wantHTTPCode == 0 {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				return
			}
			assertAppError(t, err, tc.wantHTTPCode)
		})
	}
}

// TestDeleteMarker_StaleConflict mirrors the update path on the delete
// path — proves the same concurrency check fires on delete too.
func TestDeleteMarker_StaleConflict(t *testing.T) {
	rowUpdated := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	stale := rowUpdated.Add(-time.Minute)

	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", MapID: "map-1", UpdatedAt: rowUpdated}, nil
		},
	}
	svc := newTestMapService(repo)

	if err := svc.DeleteMarker(context.Background(), "mk-1", &stale); err == nil {
		t.Fatal("expected stale-timestamp delete to be rejected with conflict")
	} else {
		assertAppError(t, err, http.StatusConflict)
	}

	if err := svc.DeleteMarker(context.Background(), "mk-1", &rowUpdated); err != nil {
		t.Fatalf("expected matching-timestamp delete to succeed, got: %v", err)
	}
}
