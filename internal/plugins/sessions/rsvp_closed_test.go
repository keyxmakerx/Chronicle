package sessions

// A finished or cancelled night is a record of who came: members can't answer
// it from the page or from a link mailed before it ended, while an import
// still restores the attendance of past nights.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestRespondRSVP_RefusesFinishedNights(t *testing.T) {
	tests := []struct {
		status   string
		wantCode int
	}{
		{StatusPlanned, 0},
		{"", 0},
		{StatusCompleted, http.StatusConflict},
		{StatusCancelled, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run("status "+tt.status, func(t *testing.T) {
			wrote := false
			repo := &mockSessionRepo{
				updateAttendeeStatusFn: func(context.Context, string, string, string) error { wrote = true; return nil },
			}
			svc := NewSessionService(repo, nil, nil)
			err := svc.RespondRSVP(context.Background(), &Session{ID: "s1", Status: tt.status}, "u1", RSVPAccepted, nil, patch.Field[string]{})
			if tt.wantCode == 0 {
				if err != nil || !wrote {
					t.Fatalf("an open night must record the answer: err=%v wrote=%v", err, wrote)
				}
				return
			}
			if apperror.SafeCode(err) != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", apperror.SafeCode(err), err, tt.wantCode)
			}
			if wrote {
				t.Error("a finished night must not record an answer")
			}
		})
	}
}

func TestUpdateRSVP_ImportStillRestoresPastAttendance(t *testing.T) {
	wrote := false
	repo := &mockSessionRepo{
		findByIDFn: func(context.Context, string) (*Session, error) {
			return &Session{ID: "s1", Status: StatusCompleted}, nil
		},
		updateAttendeeStatusFn: func(context.Context, string, string, string) error { wrote = true; return nil },
	}
	if err := NewSessionService(repo, nil, nil).UpdateRSVP(context.Background(), "s1", "u1", RSVPAccepted); err != nil || !wrote {
		t.Fatalf("import path must still write: err=%v wrote=%v", err, wrote)
	}
}

func TestValidateRSVPToken_FinishedNight(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	for _, tt := range []struct {
		status string
		ok     bool
	}{{StatusPlanned, true}, {StatusCompleted, false}, {StatusCancelled, false}} {
		repo := &mockSessionRepo{
			findRSVPTokenFn: func(context.Context, string) (*RSVPToken, error) {
				return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
			},
			findByIDFn: func(context.Context, string) (*Session, error) { return &Session{ID: "s1", Status: tt.status}, nil },
		}
		_, err := NewSessionService(repo, nil, nil).ValidateRSVPToken(context.Background(), "rt")
		if (err == nil) != tt.ok {
			t.Errorf("status %q: err = %v, want ok=%v", tt.status, err, tt.ok)
		}
	}
}

// A planned night whose date has passed has been played, though nothing ever
// marked it complete: it takes no answer from the page's endpoint or from a
// mailed link. The date is judged where the day ends last, so nobody is
// refused while the night's own date is still under way for them.
func TestPlayedNight_TakesNoAnswer(t *testing.T) {
	day := func(d string) *string { return &d }
	evening := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	// 05:00 UTC on the 18th is still the 17th in the last zone on Earth.
	earlyMorning := time.Date(2026, 11, 18, 5, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		now   time.Time
		night Session
		open  bool
	}{
		{"last night's game", evening, Session{Status: StatusPlanned, ScheduledDate: day("2026-11-17")}, false},
		{"a game a week ago", evening, Session{Status: StatusPlanned, ScheduledDate: day("2026-11-11")}, false},
		{"tonight's game", evening, Session{Status: StatusPlanned, ScheduledDate: day("2026-11-18")}, true},
		{"last night's game while its date lasts somewhere", earlyMorning, Session{Status: StatusPlanned, ScheduledDate: day("2026-11-17")}, true},
		{"a game still ahead", evening, Session{Status: StatusPlanned, ScheduledDate: day("2026-11-20")}, true},
		{"a game with no date", evening, Session{Status: StatusPlanned}, true},
		{"a repeating game that began long ago", evening, Session{Status: StatusPlanned, IsRecurring: true, ScheduledDate: day("2026-10-02")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			night := tt.night
			night.ID = "s1"
			wrote := false
			future := tt.now.Add(time.Hour)
			if real := time.Now().UTC().Add(time.Hour); real.After(future) {
				future = real
			}
			repo := &mockSessionRepo{
				updateAttendeeStatusFn: func(context.Context, string, string, string) error { wrote = true; return nil },
				findRSVPTokenFn: func(context.Context, string) (*RSVPToken, error) {
					return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
				},
				findByIDFn: func(context.Context, string) (*Session, error) { return &night, nil },
			}
			svc := &sessionService{repo: repo, clock: func() time.Time { return tt.now }}

			err := svc.RespondRSVP(context.Background(), &night, "u1", RSVPAccepted, nil, patch.Field[string]{})
			if tt.open {
				if err != nil || !wrote {
					t.Errorf("an open night must record the answer: err=%v wrote=%v", err, wrote)
				}
			} else {
				if apperror.SafeCode(err) != http.StatusConflict {
					t.Errorf("code = %d (%v), want %d", apperror.SafeCode(err), err, http.StatusConflict)
				}
				if wrote {
					t.Error("a played night recorded an answer")
				}
			}

			if _, err := svc.ValidateRSVPToken(context.Background(), "rt"); (err == nil) != tt.open {
				t.Errorf("mailed link: err = %v, want it to work = %v", err, tt.open)
			}
		})
	}
}
