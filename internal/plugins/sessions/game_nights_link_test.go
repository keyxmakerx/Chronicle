package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// A date poll's page and the polls list open on the Game nights calendar
// when the campaign has one, and stay as pages when it doesn't.
func TestDatePollPagesOpenOnTheCalendar(t *testing.T) {
	start := time.Date(2099, 10, 17, 1, 30, 0, 0, time.UTC)
	repo := &mockSessionRepo{
		getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
			return &SlotProposal{ID: "p1", CampaignID: "c1", Title: "Next arc", Status: ProposalOpen},
				[]SlotProposalOption{{ID: "o1", ProposalID: "p1", StartsAtUTC: start, EndsAtUTC: start.Add(4 * time.Hour)}}, nil
		},
	}
	tests := []struct {
		name, calID, path string
		want              string
	}{
		{"a poll opens on its next date", "k1", "/campaigns/c1/proposals/p1?tz=America/Chicago", "/campaigns/c1/calendars/k1/view?date=2099-10-16&poll=p1"},
		{"the list opens the calendar", "k1", "/campaigns/c1/proposals", "/campaigns/c1/calendars/k1/view?night=next"},
		{"without a calendar a poll keeps its page", "", "/campaigns/c1/proposals/p1", ""},
		{"without a calendar the list keeps its page", "", "/campaigns/c1/proposals", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewSessionService(repo, nil, nil))
			h.SetCalendarFinder(stubCalendarFinder{id: tt.calID})
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, tt.path, nil), rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true})
			c.SetParamNames("pid")
			c.SetParamValues("p1")
			var err error
			if strings.HasSuffix(c.Request().URL.Path, "/proposals") {
				err = h.ListProposals(c)
			} else {
				err = h.ShowProposal(c)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Fatalf("Location = %q, want %q", got, tt.want)
			}
			if tt.want == "" && rec.Code != http.StatusOK {
				t.Fatalf("status %d, want the page", rec.Code)
			}
		})
	}
}
