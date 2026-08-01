package calendar

// schedule_route_test.go — the /schedule page's security review, in executable
// form (C-CALV4-RSVP-P8 Part B).
//
// PART B SPENDS ITS ENTIRE ROUTE BUDGET ON ONE READ:
//
//	GET /campaigns/:id/schedule   Player+   the page
//
// and every other write the page performs rides a route that already shipped —
// the scheduler's own availability PUTs (sessions), the Player+ event-RSVP POST
// (P8A), and P8B's Scribe+ `POST /calendar/ask`. Reuse, not a fork: a second
// availability write path would be a second place the composition invariant
// ("an offer only ever adds") could be got wrong.
//
// Each heading below is one claim from §8.3's review template, and the pattern
// is daycard_route_test.go's / block_prefs_route_test.go's, deliberately — the
// wave's previous single-route slices were reviewed this way and re-authoring
// the shape would fork the discipline.
//
// THE CLAIM THAT MATTERS MOST is the permission one, and it is asserted about
// the PAYLOAD rather than about the HTML: a player's ScheduleData must contain
// no other member's lane, name or Director control at all. A permission
// expressed as a template branch is one refactor away from being lost, which is
// why §4's law is "permission is absence" and why these tests read the built
// data, not the rendered string.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// serveSchedule drives SchedulePage the way the nav would.
func serveSchedule(h *Handler, role campaigns.Role, userID string, query string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	target := "/campaigns/camp-1/schedule"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Embers of the Reach"},
		MemberRole: role, IsMember: true,
	})
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	return rec, h.SchedulePage(c)
}

// scheduleRouteHandler is a Handler with no calendar rows and no schedule seam
// — the degraded floor. Every assertion that is about the ROUTE rather than
// about the data holds here, which is the point: the door's gating cannot
// depend on the reads behind it succeeding.
func scheduleRouteHandler() *Handler {
	return NewHandler(NewCalendarService(&mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return nil, nil
		},
	}))
}

// §8.3 "Who may call it": Player+. The floor is the route's, enforced by
// campaigns.RequireRole in routes.go; the handler itself must not add a second,
// different gate — two gates for one question is how they drift apart.
func TestSchedulePage_RendersForEveryMemberRole(t *testing.T) {
	for _, role := range []campaigns.Role{
		campaigns.RolePlayer, campaigns.RoleScribe, campaigns.RoleOwner,
	} {
		h := scheduleRouteHandler()
		rec, err := serveSchedule(h, role, "u-1", "")
		if err != nil {
			t.Fatalf("role %v: SchedulePage: %v", role, err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("role %v: status = %d, want 200", role, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "cal-schedule") {
			t.Errorf("role %v: the page did not render its own scope root", role)
		}
	}
}

// §8.3 "What it accepts": the campaign id from the PATH (already authorised by
// RequireCampaignAccess) and a handful of display-only query params. Nothing on
// this route names a member, so there is no field by which a caller could ask
// about somebody else — the IDOR probe has no parameter to attack.
func TestSchedulePage_AcceptsNoIdentityParameter(t *testing.T) {
	h := scheduleRouteHandler()
	// The probe: every shape a caller might try to smuggle a subject in.
	for _, q := range []string{
		"as=u-victim", "user=u-victim", "member=u-victim", "user_id=u-victim",
		"campaign=camp-2", "id=camp-2",
	} {
		rec, err := serveSchedule(h, campaigns.RolePlayer, "u-1", q)
		if err != nil {
			t.Fatalf("query %q: SchedulePage: %v", q, err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("query %q: status = %d, want 200 (the param must be ignored, not fatal)", q, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "u-victim") {
			t.Errorf("query %q: a request parameter reached the rendered page", q)
		}
		if strings.Contains(rec.Body.String(), "camp-2") {
			t.Errorf("query %q: a request parameter overrode the authorised campaign", q)
		}
	}
}

// §8.3 "What it leaks on failure": nothing that distinguishes one refusal from
// another. With no seam wired and no calendars the page still renders — a
// degraded read costs the surface its BODY, never its identity, and never an
// error page that says which read failed.
func TestSchedulePage_DegradesWithoutDisclosing(t *testing.T) {
	h := scheduleRouteHandler()
	rec, err := serveSchedule(h, campaigns.RolePlayer, "u-1", "")
	if err != nil {
		t.Fatalf("SchedulePage: %v", err)
	}
	body := rec.Body.String()
	for _, leak := range []string{"sql", "SELECT", "goroutine", "panic:", "camp-1 not found"} {
		if strings.Contains(body, leak) {
			t.Errorf("the degraded page discloses %q", leak)
		}
	}
}

// §8.3 "Missing campaign context": the shared refusal, not a nil dereference.
func TestSchedulePage_RefusesWithoutCampaignContext(t *testing.T) {
	h := scheduleRouteHandler()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/schedule", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.SchedulePage(c); err == nil {
		t.Fatal("SchedulePage accepted a request with no campaign context")
	}
	_ = rec
}

// §8.3 "What it accepts", the zoom half: `?zoom` reaches the BUILDER, not just
// the toggle's own Pressed flag.
//
// This is asserted at the ROUTE and not only in the builder because that is
// exactly where the defect lived: `data.Zoom` round-tripped the query and inked
// `aria-current="true"` while nothing downstream read it, so the two zooms were
// asserted apart and shipped identical. The handler runs on the DEGRADED floor
// here (no schedule seam, so no overlay and no grid), which is why the claim is
// about the resolved display state and the rendered rung rather than about
// column counts — the surfaces that carry columns are proved in
// schedule_dayzoom_test.go.
func TestSchedulePage_ZoomReachesTheBuilderAndTheRungs(t *testing.T) {
	h := scheduleRouteHandler()

	for _, tc := range []struct {
		zoom, want string
	}{
		{"", "week"},
		{"week", "week"},
		{"day", "day"},
		// An unknown zoom is CLAMPED, never passed through: the builder switches
		// on it and an unrecognised value must not become a third column set.
		{"hour", "week"},
		{"DAY", "week"},
	} {
		data := h.buildSchedule(context.Background(), scheduleInput{
			Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Embers of the Reach"},
			UserID:   "u-1",
			Zoom:     tc.zoom,
		})
		if data.Zoom != tc.want {
			t.Errorf("?zoom=%q resolved to %q, want %q", tc.zoom, data.Zoom, tc.want)
		}
		// ?day is always resolved to a real date in the resolved week, whichever
		// zoom is on — the day view has to have a day before it can be asked for.
		if data.Day == "" {
			t.Errorf("?zoom=%q left Day empty", tc.zoom)
		}
	}

	// And the refusal is in the served HTML at every zoom, because the media
	// query — not the server — decides which rung a reader sees.
	for _, q := range []string{"", "zoom=day"} {
		rec, err := serveSchedule(h, campaigns.RoleOwner, "u-1", q)
		if err != nil {
			t.Fatalf("?%s: SchedulePage: %v", q, err)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`week zoom is forced at this width`,
			`sc-rung-narrow`,
			`sc-rung-wide`,
			`disabled`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("?%s: the served page does not contain %q", q, want)
			}
		}
	}
}
