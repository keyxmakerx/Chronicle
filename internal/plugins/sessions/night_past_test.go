package sessions

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func newNightMailHandler(svc SessionService, mailer *listMailer) *Handler {
	return &Handler{svc: svc, mailer: mailer, baseURL: "https://x.test",
		memberLister: &stubMemberLister{members: []campaigns.CampaignMember{{UserID: "p", Email: "p@x.test"}}}}
}

// serviceOn is a session service whose clock reads noon UTC on day.
func serviceOn(repo SessionRepository, day string) SessionService {
	at, err := time.Parse("2006-01-02 15:04", day+" 12:00")
	if err != nil {
		panic(err)
	}
	svc := NewSessionService(repo, nil, nil).(*sessionService)
	svc.clock = func() time.Time { return at }
	return svc
}

func weeklyFrom(date string) *Session {
	return &Session{ID: "s1", Name: "Night", ScheduledDate: strp(date), IsRecurring: true, RecurrenceType: strp("weekly"), RecurrenceInterval: 1}
}

func TestUpcomingNight(t *testing.T) {
	ended := weeklyFrom("2026-10-03")
	ended.RecurrenceEndDate = strp("2026-10-31")
	tests := []struct {
		name     string
		s        *Session
		today    string
		wantDate string
		wantOK   bool
	}{
		{"undated is always announced", &Session{}, "2026-11-11", "", true},
		{"one-off in the past", &Session{ScheduledDate: strp("2026-11-07")}, "2026-11-11", "2026-11-07", false},
		{"one-off today", &Session{ScheduledDate: strp("2026-11-11")}, "2026-11-11", "2026-11-11", true},
		{"one-off ahead", &Session{ScheduledDate: strp("2026-11-20")}, "2026-11-11", "2026-11-20", true},
		{"series names its next night", weeklyFrom("2026-10-03"), "2026-11-11", "2026-11-14", true},
		{"series whose end date has passed", ended, "2026-11-11", "", false},
		{"series not started yet", weeklyFrom("2026-12-05"), "2026-11-11", "2026-12-05", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, ok := upcomingNight(tt.s, tt.today)
			if date != tt.wantDate || ok != tt.wantOK {
				t.Errorf("upcomingNight = %q, %v; want %q, %v", date, ok, tt.wantDate, tt.wantOK)
			}
		})
	}
}

func TestNightsToday_WaitsForEveryZone(t *testing.T) {
	at := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	if got := nightsToday(at); got != "2026-11-10" {
		t.Errorf("nightsToday(03:00 UTC) = %q; the 10th is still under way in the Americas", got)
	}
}

func TestNightChangeAudience(t *testing.T) {
	tests := []struct {
		name     string
		s        *Session
		wantNil  bool
		wantDate string
		wantFrom string
	}{
		{"a night already played tells nobody", &Session{ID: "s1", ScheduledDate: strp("2026-11-07")}, true, "", ""},
		{"an upcoming one-off keeps its date", &Session{ID: "s1", ScheduledDate: strp("2026-11-20")}, false, "2026-11-20", "2026-11-11"},
		{"a series is named by its next night and counts only later answers", weeklyFrom("2026-10-03"), false, "2026-11-14", "2026-11-11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var from string
			orig := *tt.s.ScheduledDate
			repo := &mockSessionRepo{listGoingOrMaybeUserIDsFn: func(_ context.Context, _, fromDate string) ([]string, error) {
				from = fromDate
				return []string{"u1"}, nil
			}}
			aud, err := serviceOn(repo, "2026-11-11").NightChangeAudience(context.Background(), tt.s)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantNil {
				if aud != nil {
					t.Fatalf("audience = %+v, want none", aud)
				}
				return
			}
			if aud == nil || len(aud.UserIDs) != 1 || *aud.Night.ScheduledDate != tt.wantDate {
				t.Fatalf("audience = %+v, want u1 and night %s", aud, tt.wantDate)
			}
			if from != tt.wantFrom {
				t.Errorf("answers read from %q, want %q", from, tt.wantFrom)
			}
			if *tt.s.ScheduledDate != orig {
				t.Error("the stored session was modified instead of a copy")
			}
		})
	}
}

func TestDeleteSession_PastNightTellsNobody(t *testing.T) {
	tests := []struct {
		name string
		s    *Session
		want int
	}{
		{"past one-off", &Session{ID: "s1", ScheduledDate: strp("2026-11-07")}, 0},
		{"series with nights left", weeklyFrom("2026-10-03"), 1},
		{"upcoming one-off", &Session{ID: "s1", ScheduledDate: strp("2026-11-20")}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted := false
			repo := &mockSessionRepo{
				findByIDFn:             func(context.Context, string) (*Session, error) { return tt.s, nil },
				listRespondedUserIDsFn: func(context.Context, string) ([]string, error) { return []string{"u1"}, nil },
				softDeleteSessionFn:    func(context.Context, string) error { deleted = true; return nil },
			}
			got, err := serviceOn(repo, "2026-11-11").DeleteSession(context.Background(), "s1")
			if err != nil || len(got) != tt.want || !deleted {
				t.Errorf("responded = %v, err = %v, deleted = %v; want %d to tell and the night deleted", got, err, deleted, tt.want)
			}
		})
	}
}

func TestRestoreAndAnnounce_PastNightAndLostRace(t *testing.T) {
	tests := []struct {
		name      string
		s         *Session
		changed   bool
		wantErr   bool
		wantAsked bool // whether anyone was looked up to be told
	}{
		{"upcoming night is announced", &Session{ID: "s1", CampaignID: "c1", ScheduledDate: strp("2026-11-20")}, true, false, true},
		{"past night is restored quietly", &Session{ID: "s1", CampaignID: "c1", ScheduledDate: strp("2026-11-07")}, true, false, false},
		{"a restore that lost the race announces nothing", &Session{ID: "s1", CampaignID: "c1", ScheduledDate: strp("2026-11-20")}, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restored, asked := false, false
			repo := &mockSessionRepo{
				findByIDIncludingDeletedFn: func(context.Context, string) (*Session, error) { return tt.s, nil },
				// Both racers saw the night in the Trash; the other already
				// restored it by the time ours writes.
				findByIDFn: func(context.Context, string) (*Session, error) {
					if !restored {
						return nil, apperror.NewNotFound("session not found")
					}
					return tt.s, nil
				},
				restoreSessionFn: func(context.Context, string) (bool, error) { restored = true; return tt.changed, nil },
				listRespondedUserIDsFn: func(context.Context, string) ([]string, error) {
					asked = true
					return nil, nil
				},
			}
			_, err := serviceOn(repo, "2026-11-11").RestoreAndAnnounce(context.Background(), "c1", "s1")
			if tt.wantErr {
				assertAppError(t, err, http.StatusNotFound)
			} else if err != nil {
				t.Fatal(err)
			}
			if asked != tt.wantAsked {
				t.Errorf("looked up who to tell = %v, want %v", asked, tt.wantAsked)
			}
		})
	}
}

func TestEmailNightChange_PastNightSendsNothing(t *testing.T) {
	mailer := &listMailer{}
	repo := &mockSessionRepo{listGoingOrMaybeUserIDsFn: func(context.Context, string, string) ([]string, error) {
		return []string{"p"}, nil
	}}
	h := newNightMailHandler(serviceOn(repo, "2026-11-11"), mailer)
	h.emailNightChange(context.Background(), mailCampaign{ID: "c1", Name: "Therin"}, nightCancelled, nil,
		&Session{ID: "s1", Name: "Night", ScheduledDate: strp("2026-11-07")}, "")
	if len(mailer.sent) != 0 {
		t.Errorf("a night already played emailed %d people", len(mailer.sent))
	}
}

func TestEmailNightChange_SeriesNamesNextNight(t *testing.T) {
	mailer := &listMailer{}
	repo := &mockSessionRepo{listGoingOrMaybeUserIDsFn: func(context.Context, string, string) ([]string, error) {
		return []string{"p"}, nil
	}}
	h := newNightMailHandler(serviceOn(repo, "2026-11-11"), mailer)
	h.emailNightChange(context.Background(), mailCampaign{ID: "c1", Name: "Therin"}, nightCancelled, nil,
		weeklyFrom("2026-10-03"), "")
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(mailer.sent))
	}
	if got := mailer.sent[0].plain; !strings.Contains(got, "Sat, Nov 14, 2026") || strings.Contains(got, "Oct 3") {
		t.Errorf("a cancelled series should name its next night, not its first:\n%s", got)
	}
}
