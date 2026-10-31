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
		{"no calendar, the Calendar app", "", "", "", "/campaigns/c1/apps/calendar"},
		{"no calendar, the session's own page", "", "s1", "2026-10-08", "/campaigns/c1/sessions/s1"},
		{"a night opens its day", "k1", "s1", "2026-10-08", "/campaigns/c1/calendars/k1/view?date=2026-10-08&night=s1"},
		{"no night named opens the calendar as it is", "k1", "", "", "/campaigns/c1/calendars/k1/view"},
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

// stubCalendarFinder answers RealWorldCalendarID with a fixed id.
type stubCalendarFinder struct{ id string }

func (s stubCalendarFinder) RealWorldCalendarID(context.Context, string, int, string) (string, error) {
	return s.id, nil
}

// Game nights have no page of their own: with no real-world calendar the
// owner is offered one and other members get the Calendar app, while anyone
// who isn't a member goes to the campaign. The old list address does the same.
func TestGameNightsLink_NoRealWorldCalendar(t *testing.T) {
	tests := []struct {
		name, calID, query string
		role               campaigns.Role
		want               string
	}{
		{"the owner is offered a real-world calendar", "", "", campaigns.RoleOwner, "/campaigns/c1/calendars?wizard=reallife"},
		{"a player gets the Calendar app", "", "", campaigns.RolePlayer, "/campaigns/c1/apps/calendar"},
		{"a visitor goes to the campaign", "k1", "", campaigns.RoleNone, "/campaigns/c1"},
		{"a visitor's named session opens its own page", "k1", "?session=s1&date=2026-10-08", campaigns.RoleNone, "/campaigns/c1/sessions/s1"},
		{"the old list address opens the calendar", "k1", "", campaigns.RolePlayer, "/campaigns/c1/calendars/k1/view"},
		{"a trashed night's Undo notice rides along", "k1", "?trashed=s1&kind=game-night", campaigns.RolePlayer, "/campaigns/c1/calendars/k1/view?kind=game-night&trashed=s1"},
		{"and onto a target that already has a query", "", "?trashed=s1&kind=game-night", campaigns.RoleOwner, "/campaigns/c1/calendars?wizard=reallife&kind=game-night&trashed=s1"},
		{"a named session still opens its own page", "", "?session=s1&date=2026-10-08", campaigns.RoleOwner, "/campaigns/c1/sessions/s1"},
		{"with a calendar, the calendar", "k1", "", campaigns.RoleOwner, "/campaigns/c1/calendars/k1/view"},
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
		{"the list opens the calendar", "k1", "/campaigns/c1/proposals", "/campaigns/c1/calendars/k1/view"},
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
