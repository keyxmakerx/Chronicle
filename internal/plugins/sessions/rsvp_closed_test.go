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
