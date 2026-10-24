package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type fakeLastNight struct {
	night *sessions.Session
	today string
}

func (f *fakeLastNight) LastNightToWriteUp(_ context.Context, _, today string) (*sessions.Session, error) {
	f.today = today
	return f.night, nil
}

// "Last night" is the viewer's own last night: 01:30 UTC on the 24th is
// still the 23rd in New York, so the 22nd is last night there.
func TestBuildLastNight(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, time.October, 24, 1, 30, 0, 0, time.UTC)
	date := "2026-10-22"
	tests := []struct {
		name      string
		loc       *time.Location
		wantToday string
		wantLead  string
	}{
		{"New York", ny, "2026-10-23", "Last night"},
		{"no zone reads UTC", nil, "2026-10-24", "Game night, Thu, Oct 22, 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLastNight{night: &sessions.Session{ID: "s2", Name: "Nine Dark Nights", ScheduledDate: &date}}
			v, ok, err := buildLastNight(context.Background(), f, "c1", now, tt.loc)
			if err != nil || !ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			if f.today != tt.wantToday || v.Lead != tt.wantLead {
				t.Errorf("today %q lead %q, want %q %q", f.today, v.Lead, tt.wantToday, tt.wantLead)
			}
			if v.Href != "/campaigns/c1/sessions/s2?write=1#session-recap" {
				t.Errorf("href = %q", v.Href)
			}
		})
	}
}

func TestLastNightHandler_OnlyRecapWriters(t *testing.T) {
	date := "2026-10-23"
	both := fakeComingUpAddons{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true}
	tests := []struct {
		name   string
		role   campaigns.Role
		member bool
		addons fakeComingUpAddons
		want   bool
	}{
		{"scribe", campaigns.RoleScribe, true, both, true},
		{"owner", campaigns.RoleOwner, true, both, true},
		{"player", campaigns.RolePlayer, true, both, false},
		{"not a member", campaigns.RoleScribe, false, both, false},
		{"game nights off", campaigns.RoleScribe, true, fakeComingUpAddons{calendar.PluginSlug: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &lastNightHandler{
				nights: &fakeLastNight{night: &sessions.Session{ID: "s2", Name: "Nine Dark Nights", ScheduledDate: &date}},
				addons: tt.addons,
				now:    func() time.Time { return time.Date(2026, time.October, 24, 15, 0, 0, 0, time.UTC) },
			}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/last-night", nil), rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tt.role, IsMember: tt.member})
			c.Set("auth_user_id", "me")
			if err := h.Show(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, "Write it up"); got != tt.want {
				t.Errorf("card shown = %v, want %v: %q", got, tt.want, body)
			}
			if tt.want && !strings.Contains(body, "Last night: Nine Dark Nights") {
				t.Errorf("body = %q", body)
			}
		})
	}
}
