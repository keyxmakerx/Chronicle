// create_calendar_redirect_test.go — C-WIDGET-BINDING-QA1 Bug 1. Creating a
// calendar must land on the V2 shell (the old V1 view was missing the V2
// worldstate features/animations). The fantasy path still routes to settings
// (mode-agnostic customize-first), which is not the V1 view.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// createRedirectStub satisfies CalendarService via embedding; only CreateCalendar
// is exercised (returns a fixed calendar so we can assert the redirect target).
type createRedirectStub struct {
	CalendarService
	created *Calendar
}

func (s *createRedirectStub) CreateCalendar(_ context.Context, _ string, _ CreateCalendarInput) (*Calendar, error) {
	return s.created, nil
}

func invokeFormCreate(t *testing.T, mode string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(&createRedirectStub{created: &Calendar{ID: "cal-9", CampaignID: "camp-1", Name: "C", Mode: mode}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/calendars",
		strings.NewReader("mode="+mode+"&name=C"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	if err := h.CreateCalendar(c); err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	return rec
}

func TestCreateCalendar_RealLifeRedirectsToV2(t *testing.T) {
	rec := invokeFormCreate(t, ModeRealLife)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/campaigns/camp-1/calendar/v2/cal-9" {
		t.Errorf("real-life create must land on the V2 shell; Location=%q", loc)
	}
}

func TestCreateCalendar_FantasyStillRoutesToSettings(t *testing.T) {
	rec := invokeFormCreate(t, ModeFantasy)
	// Fantasy → settings (mode-agnostic customize-first), NOT the V1 view.
	if loc := rec.Header().Get("Location"); loc != "/campaigns/camp-1/calendars/cal-9/settings" {
		t.Errorf("fantasy create should route to settings; Location=%q", loc)
	}
}
