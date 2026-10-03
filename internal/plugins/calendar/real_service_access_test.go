// real_service_access_test.go: full-stack route tests through
// RegisterRoutes wired to the REAL calendarService (NewCalendarService over
// mocks_test.go's in-memory repo fakes), not access_test.go's fakeCalendarSvc.
//
// fakeCalendarSvc exists so route-gating tests don't have to care about
// business logic, but that also means every earlier access_test.go
// assertion about visibility filtering was only ever checking fakeCalendarSvc's
// own hand-rolled behavior (see its secretCalendarID special case), never the
// real service's calendarInCampaignForViewer/filterCalendarsByUser rules.
// This file closes that gap: it proves, through the real HTTP router and
// the real service, that a Director-only calendar is excluded from a
// Player's list and calendar-page responses, and that a calendar id belonging to a
// different campaign 404s instead of leaking.
package calendar

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newRealServiceTestRouter wires RegisterRoutes to a genuine calendarService
// (over in-memory fakeCalendarRepo/fakeEventRepo/fakeEventKindRepo/
// fakeWeatherRepo), so a test here exercises the same visibility filtering
// production traffic does — unlike access_test.go's fakeCalendarSvc, which
// answers from its own hardcoded rules.
func newRealServiceTestRouter(calRepo *fakeCalendarRepo, roles map[string]campaigns.Role) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	// Same minimal error translation access_test.go uses: apperror.AppError
	// -> its own status code, so a NotFound really comes back as HTTP 404.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := NewCalendarService(calRepo, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	h := NewHandler(svc)
	campaignSvc := guardCampaignSvc{roles: roles}
	RegisterRoutes(e, h, campaignSvc, guardAuthSvc{}, guardAddonSvc{enabled: true})
	return e
}

// TestRealService_DirectorOnlyCalendarExcludedFromPlayerList proves the
// calendars LIST page (Index), through the real service, omits a dm_only
// calendar for a Player while an Owner sees it — not a fakeCalendarSvc
// stand-in's behavior, the genuine filterCalendarsByUser/
// GetCalendarForViewer path list_handler.go actually calls.
func TestRealService_DirectorOnlyCalendarExcludedFromPlayerList(t *testing.T) {
	const campaignID = "camp-real-a"
	visible := Calendar{ID: "cal-visible", CampaignID: campaignID, Name: "Visible Calendar",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	secret := Calendar{ID: "cal-directors-eyes-only", CampaignID: campaignID, Name: "Directors Eyes Only",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "dm_only"}

	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, cid string) ([]Calendar, error) {
			if cid != campaignID {
				return nil, nil
			}
			return []Calendar{visible, secret}, nil
		},
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			switch id {
			case visible.ID:
				v := visible
				return &v, nil
			case secret.ID:
				s := secret
				return &s, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}

	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer, "u-owner": campaigns.RoleOwner}
	e := newRealServiceTestRouter(calRepo, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars", "u-player")
	if rec.Code != http.StatusOK {
		t.Fatalf("Player Index: got %d, body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret.Name) {
		t.Errorf("Player's calendars list must not name the dm_only calendar, body:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), visible.Name) {
		t.Errorf("Player's calendars list must still show the visible calendar, body:\n%s", rec.Body.String())
	}

	rec = doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner Index: got %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), secret.Name) {
		t.Errorf("Owner's calendars list must show the dm_only calendar, body:\n%s", rec.Body.String())
	}
}

// TestRealService_DirectorOnlyCalendarPage404sForPlayer is the same
// invariant at the calendar's own page, which a card opens (directly, or
// fetched in place by calendar_open.js): a Player fetching the dm_only
// calendar's page directly (not just omitted from the list) must get a real
// NotFound, and an Owner must succeed.
func TestRealService_DirectorOnlyCalendarPage404sForPlayer(t *testing.T) {
	const campaignID = "camp-real-b"
	secret := Calendar{ID: "cal-secret-page", CampaignID: campaignID, Name: "Directors Eyes Only",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "dm_only"}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == secret.ID {
				s := secret
				return &s, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}
	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer, "u-owner": campaigns.RoleOwner}
	e := newRealServiceTestRouter(calRepo, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars/"+secret.ID+"/view", "u-player")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Player opening a dm_only calendar: got %d, want 404, body: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars/"+secret.ID+"/view", "u-owner")
	if rec.Code != http.StatusOK {
		t.Errorf("Owner opening the same calendar: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

// TestRealService_AllowListedCalendarHidesFromNonAllowedPlayer covers the
// other half of "Director-only or allow-listed": an "everyone" calendar
// whose visibility_rules names specific users must stay invisible to a
// Player NOT on that list, and visible to one who is.
func TestRealService_AllowListedCalendarHidesFromNonAllowedPlayer(t *testing.T) {
	const campaignID = "camp-real-c"
	rules := `{"allowed_users":["u-ally"]}`
	restricted := Calendar{ID: "cal-allow-listed", CampaignID: campaignID, Name: "Inner Circle Calendar",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		Visibility: "everyone", VisibilityRules: &rules}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == restricted.ID {
				r := restricted
				return &r, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}
	roles := map[string]campaigns.Role{"u-outsider": campaigns.RolePlayer, "u-ally": campaigns.RolePlayer}
	e := newRealServiceTestRouter(calRepo, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars/"+restricted.ID+"/view", "u-outsider")
	if rec.Code != http.StatusNotFound {
		t.Errorf("a Player not on the allow-list: got %d, want 404, body: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(e, http.MethodGet, "/campaigns/"+campaignID+"/calendars/"+restricted.ID+"/view", "u-ally")
	if rec.Code != http.StatusOK {
		t.Errorf("the allow-listed Player: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

// TestRealService_CalendarFromDifferentCampaign404sNotLeak proves a calid
// that genuinely exists, but under a DIFFERENT campaign than the one named
// in the URL, 404s exactly like a nonexistent id — never a redirect or a
// body that confirms the calendar exists elsewhere.
func TestRealService_CalendarFromDifferentCampaign404sNotLeak(t *testing.T) {
	const requestedCampaign = "camp-real-d"
	const actualCampaign = "camp-real-other"
	elsewhere := Calendar{ID: "cal-owned-elsewhere", CampaignID: actualCampaign, Name: "Someone Else's Calendar",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == elsewhere.ID {
				e := elsewhere
				return &e, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e := newRealServiceTestRouter(calRepo, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/"+requestedCampaign+"/calendars/"+elsewhere.ID+"/view", "u-owner")
	if rec.Code != http.StatusNotFound {
		t.Errorf("a calendar id from a different campaign: got %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), elsewhere.Name) {
		t.Errorf("the 404 must not leak the calendar's name, body:\n%s", rec.Body.String())
	}

	// Control: the SAME id through its real campaign succeeds.
	rec = doRequest(e, http.MethodGet, "/campaigns/"+actualCampaign+"/calendars/"+elsewhere.ID+"/view", "u-owner")
	if rec.Code != http.StatusOK {
		t.Errorf("the same calendar through its own campaign: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}
