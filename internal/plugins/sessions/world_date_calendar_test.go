package sessions

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// stubResolver answers DefaultCalendarID per campaign.
type stubResolver struct {
	ids  map[string]string
	errs map[string]error
}

func (r stubResolver) DefaultCalendarID(_ context.Context, campaignID string) (string, error) {
	return r.ids[campaignID], r.errs[campaignID]
}

func intp(i int) *int { return &i }

func newStampingService(repo *mockSessionRepo, r DefaultCalendarResolver) *sessionService {
	svc := NewSessionService(repo, nil, nil).(*sessionService)
	if r != nil {
		svc.SetDefaultCalendarResolver(r)
	}
	return svc
}

func TestCreateSession_StampsWorldDateCalendar(t *testing.T) {
	full := func() CreateSessionInput {
		return CreateSessionInput{Name: "Night", CalendarYear: intp(1000), CalendarMonth: intp(3), CalendarDay: intp(4)}
	}
	tests := []struct {
		name     string
		input    CreateSessionInput
		resolver DefaultCalendarResolver
		want     *string
	}{
		{"complete date gets the default calendar", full(), stubResolver{ids: map[string]string{"c1": "cal-1"}}, strp("cal-1")},
		{"no default calendar leaves it unstamped", full(), stubResolver{}, nil},
		{"resolver error does not fail the save", full(), stubResolver{errs: map[string]error{"c1": errors.New("boom")}}, nil},
		{"no resolver wired leaves it unstamped", full(), nil, nil},
		{"no world date means no calendar", CreateSessionInput{Name: "Night"}, stubResolver{ids: map[string]string{"c1": "cal-1"}}, nil},
		{"partial world date means no calendar", CreateSessionInput{Name: "Night", CalendarYear: intp(1000)}, stubResolver{ids: map[string]string{"c1": "cal-1"}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saved *Session
			svc := newStampingService(&mockSessionRepo{createFn: func(_ context.Context, _ string, s *Session) error {
				saved = s
				return nil
			}}, tc.resolver)
			if _, err := svc.CreateSession(context.Background(), "c1", tc.input); err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			if !equalStrPtr(saved.CalendarID, tc.want) {
				t.Errorf("CalendarID = %v, want %v", deref(saved.CalendarID), deref(tc.want))
			}
		})
	}
}

func TestUpdateSession_KeepsClearsAndStampsCalendar(t *testing.T) {
	stored := func(cal *string, y, m, d *int) *Session {
		return &Session{ID: "s1", CampaignID: "c1", Name: "Night", Status: StatusPlanned, CalendarID: cal, CalendarYear: y, CalendarMonth: m, CalendarDay: d}
	}
	resolver := stubResolver{ids: map[string]string{"c1": "default-cal"}}
	tests := []struct {
		name  string
		row   *Session
		input UpdateSessionInput
		want  *string
	}{
		{"edit keeps the recorded calendar, not the default", stored(strp("other-cal"), intp(1), intp(2), intp(3)),
			UpdateSessionInput{CalendarMonth: patch.Of(5)}, strp("other-cal")},
		{"clearing the date clears the calendar", stored(strp("other-cal"), intp(1), intp(2), intp(3)),
			UpdateSessionInput{CalendarYear: patch.Null[int](), CalendarMonth: patch.Null[int](), CalendarDay: patch.Null[int]()}, nil},
		{"setting a date on an undated session stamps the default", stored(nil, nil, nil, nil),
			UpdateSessionInput{CalendarYear: patch.Of(1), CalendarMonth: patch.Of(2), CalendarDay: patch.Of(3)}, strp("default-cal")},
		{"an unrelated edit stamps a dated session the reconciler has not reached yet", stored(nil, intp(1), intp(2), intp(3)),
			UpdateSessionInput{Name: patch.Of("Renamed")}, strp("default-cal")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saved *Session
			svc := newStampingService(&mockSessionRepo{
				findByIDFn: func(context.Context, string) (*Session, error) { return tc.row, nil },
				updateFn:   func(_ context.Context, s *Session) error { saved = s; return nil },
			}, resolver)
			if _, err := svc.UpdateSession(context.Background(), "s1", tc.input); err != nil {
				t.Fatalf("UpdateSession: %v", err)
			}
			if !equalStrPtr(saved.CalendarID, tc.want) {
				t.Errorf("CalendarID = %v, want %v", deref(saved.CalendarID), deref(tc.want))
			}
		})
	}
}

func TestReconcileWorldDateCalendars(t *testing.T) {
	tests := []struct {
		name        string
		campaigns   []string
		resolver    stubResolver
		stampErrFor string
		wantStamped int
		wantCalls   map[string]string // campaign -> calendar stamped
		wantErr     bool
	}{
		{"stamps each campaign with its own default", []string{"a", "b"},
			stubResolver{ids: map[string]string{"a": "cal-a", "b": "cal-b"}}, "", 4,
			map[string]string{"a": "cal-a", "b": "cal-b"}, false},
		{"campaign without a calendar is skipped", []string{"a", "b"},
			stubResolver{ids: map[string]string{"b": "cal-b"}}, "", 2,
			map[string]string{"b": "cal-b"}, false},
		{"nothing to do", nil, stubResolver{}, "", 0, map[string]string{}, false},
		{"one campaign failing does not stop the rest", []string{"a", "b"},
			stubResolver{ids: map[string]string{"a": "cal-a", "b": "cal-b"}, errs: map[string]error{"a": errors.New("boom")}}, "", 2,
			map[string]string{"b": "cal-b"}, true},
		{"a failed stamp is reported and the rest still run", []string{"a", "b"},
			stubResolver{ids: map[string]string{"a": "cal-a", "b": "cal-b"}}, "a", 2,
			map[string]string{"b": "cal-b"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := map[string]string{}
			svc := NewSessionService(&mockSessionRepo{
				listUnstampedCampaignsFn: func(context.Context) ([]string, error) { return tc.campaigns, nil },
				stampWorldDateCalendarFn: func(_ context.Context, camp, cal string) (int64, error) {
					if camp == tc.stampErrFor {
						return 0, errors.New("write failed")
					}
					calls[camp] = cal
					return 2, nil
				},
			}, nil, nil)
			n, err := ReconcileWorldDateCalendars(context.Background(), svc, tc.resolver)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if n != tc.wantStamped {
				t.Errorf("stamped = %d, want %d", n, tc.wantStamped)
			}
			if len(calls) != len(tc.wantCalls) {
				t.Errorf("stamp calls = %v, want %v", calls, tc.wantCalls)
			}
			for k, v := range tc.wantCalls {
				if calls[k] != v {
					t.Errorf("campaign %s stamped %q, want %q", k, calls[k], v)
				}
			}
		})
	}
}

func equalStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
