package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestGameNightsTarget(t *testing.T) {
	tests := []struct {
		name                 string
		cal, sid, date, want string
	}{
		{"no calendar, the list", "", "", "", "/campaigns/c1/sessions"},
		{"no calendar, the session's own page", "", "s1", "2026-10-08", "/campaigns/c1/sessions/s1"},
		{"a night opens its day", "k1", "s1", "2026-10-08", "/campaigns/c1/calendars/k1/view?date=2026-10-08&night=s1"},
		{"no night named opens the next one", "k1", "", "", "/campaigns/c1/calendars/k1/view?night=next"},
		{"a session with no date opens its own page", "k1", "s1", "", "/campaigns/c1/sessions/s1"},
		{"a bad date opens the session's own page", "k1", "s1", "Thursday", "/campaigns/c1/sessions/s1"},
		{"ids are escaped", "k/1", "s&x", "2026-10-08", "/campaigns/c1/calendars/k%2F1/view?date=2026-10-08&night=s%26x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gameNightsTarget("c1", tt.cal, tt.sid, tt.date); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSidebarNightDate(t *testing.T) {
	tests := []struct {
		name string
		s    Session
		want string
	}{
		{"one-off", Session{ScheduledDate: strp("2026-10-08")}, "2026-10-08"},
		{"no date", Session{}, ""},
		{"a series opens its next night", Session{ScheduledDate: strp("2026-09-24"), IsRecurring: true, RecurrenceType: strp(RecurrenceWeekly)}, "2026-10-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sidebarNightDate(tt.s, "2026-10-03"); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionPlanFrom(t *testing.T) {
	tests := []struct {
		name            string
		date, clock, tz string
		want            SessionPlan
	}{
		{"whole plan", "2026-10-08", "19:00", "America/Chicago", SessionPlan{"2026-10-08", "19:00", "America/Chicago"}},
		{"no date, no plan", "", "19:00", "America/Chicago", SessionPlan{}},
		{"bad date, no plan", "Thursday", "19:00", "", SessionPlan{}},
		{"bad time dropped", "2026-10-08", "7pm", "", SessionPlan{Date: "2026-10-08"}},
		{"unknown zone dropped", "2026-10-08", "19:00", "Mars/Olympus", SessionPlan{Date: "2026-10-08", Time: "19:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionPlanFrom(tt.date, tt.clock, tt.tz); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// stubCalendarFinder answers RealWorldCalendarID with a fixed id.
type stubCalendarFinder struct{ id string }

func (s stubCalendarFinder) RealWorldCalendarID(context.Context, string, int, string) (string, error) {
	return s.id, nil
}

// With no real-world calendar, the owner is offered one rather than being
// sent to the Sessions page; everyone else keeps the Sessions page.
func TestGameNightsLink_NoRealWorldCalendar(t *testing.T) {
	tests := []struct {
		name, calID, query string
		role               campaigns.Role
		want               string
	}{
		{"the owner is offered a real-world calendar", "", "", campaigns.RoleOwner, "/campaigns/c1/calendars/wizard/reallife"},
		{"a player keeps the Sessions page", "", "", campaigns.RolePlayer, "/campaigns/c1/sessions"},
		{"a named session still opens its own page", "", "?session=s1&date=2026-10-08", campaigns.RoleOwner, "/campaigns/c1/sessions/s1"},
		{"with a calendar, the calendar", "k1", "", campaigns.RoleOwner, "/campaigns/c1/calendars/k1/view?night=next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewSessionService(&mockSessionRepo{}, nil, nil))
			h.SetCalendarFinder(stubCalendarFinder{id: tt.calID})
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/game-nights"+tt.query, nil), rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tt.role})
			if err := h.GameNightsLink(c); err != nil {
				t.Fatalf("GameNightsLink: %v", err)
			}
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Fatalf("Location = %q, want %q", got, tt.want)
			}
		})
	}
}
