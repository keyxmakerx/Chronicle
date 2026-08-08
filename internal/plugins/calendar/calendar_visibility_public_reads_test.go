package calendar

// calendar_visibility_public_reads_test.go — C-SWEEP-R3: the three
// public-group calendar READ routes that resolve a calendar BY ID and were
// still doing it with the bare campaign check.
//
// C-CAL-DASHBOARD-W5a introduced requireVisibleCalendar and scoped the wave to
// /calendar/v2/:calId. EmbedCalendar, UpcomingEventsFragment and ShowTimeline
// live on the same public group (RequireViewAccess — any member at any role),
// take :calId (embed also takes ?calendarId= from a dashboard block config) and
// were never in that wave, so a player who guessed a dm_only calendar's ID got
// the month grid, the upcoming list and the timeline — including every event on
// it marked `everyone`, which per-EVENT filtering happily passes through
// because it never looks at the CALENDAR's visibility.
//
// The gate must be the calendar's, and its refusal must be indistinguishable
// from a missing calendar: the two fragment routes already answer a missing
// calendar with their empty fragment and the timeline with NotFound, so
// requireVisibleCalendar reuses those branches verbatim and leaks no existence.
//
// The owner half of every case is the no-regression pin: the same hidden
// calendar must still render for the DM who owns it.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	hiddenCalName   = "Secret Cult Calendar"
	hiddenEventName = "Blood Moon Ritual"
)

// publicReadHandler serves one calendar whose visibility the caller picks,
// carrying one `everyone` event. The event is deliberately world-visible: the
// leak this pins is precisely that per-event filtering cannot save a calendar
// whose own visibility is never consulted.
func publicReadHandler(visibility string) *Handler {
	cal := &Calendar{
		ID: "cal-secret", CampaignID: "camp-1", Name: hiddenCalName,
		Visibility:  visibility,
		CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
	}
	evt := Event{
		ID: "e1", CalendarID: "cal-secret", Name: hiddenEventName,
		Year: 1, Month: 1, Day: 2, Visibility: "everyone",
	}
	return NewHandler(NewCalendarService(&mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == cal.ID {
				return cal, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
		// Eager-load the structure the month grid indexes.
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) {
			return []Month{{Name: "Hammerforge", Days: 30, SortOrder: 1}}, nil
		},
		getWeekdaysFn: func(_ context.Context, _ string) ([]Weekday, error) {
			return []Weekday{{Name: "Firstday", SortOrder: 1}}, nil
		},
		listEventsForMonthFn: func(_ context.Context, _ string, _, _ int, _ int) ([]Event, error) {
			return []Event{evt}, nil
		},
		listEventsForYearFn: func(_ context.Context, _ string, _ int, _ int) ([]Event, error) {
			return []Event{evt}, nil
		},
		listUpcomingEventsFn: func(_ context.Context, _ string, _, _, _ int, _ int, _ int) ([]Event, error) {
			return []Event{evt}, nil
		},
	}))
}

// servePublicRead drives one of the three routes the way the public group does.
// calIDParam goes on :calId; calIDQuery goes on ?calendarId= (the embed's
// dashboard-block spelling) — exactly one of them is set per case.
func servePublicRead(h *Handler, role campaigns.Role, userID, calIDParam, calIDQuery string,
	call func(*Handler, echo.Context) error,
) (*httptest.ResponseRecorder, error) {
	target := "/campaigns/camp-1/calendars/" + calIDParam + "/embed"
	if calIDQuery != "" {
		target += "?calendarId=" + calIDQuery
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId")
	c.SetParamValues("camp-1", calIDParam)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsMember: true,
	})
	c.Set("auth_user_id", userID)
	return rec, call(h, c)
}

// assertNoLeak fails if a rendered body carries anything only a viewer of the
// hidden calendar should ever see. The body is quoted around the hit rather
// than dumped: ShowTimeline renders a FULL page and the whole layout in a
// failure message buries the one line that matters.
func assertNoLeak(t *testing.T, what, body string) {
	t.Helper()
	for _, secret := range []string{hiddenCalName, hiddenEventName, "e1"} {
		if i := strings.Index(body, secret); i >= 0 {
			t.Errorf("%s leaked %q from a calendar hidden from this player: …%s…",
				what, secret, excerpt(body, i))
		}
	}
}

// excerpt returns ~120 characters of body centred on index i.
func excerpt(body string, i int) string {
	start := max(i-60, 0)
	end := min(i+60, len(body))
	return body[start:end]
}

func TestEmbedCalendar_HiddenCalendarByRouteParamDoesNotLeak(t *testing.T) {
	h := publicReadHandler("dm_only")

	rec, err := servePublicRead(h, campaigns.RolePlayer, "player-1", "cal-secret", "",
		(*Handler).EmbedCalendar)
	if err != nil {
		t.Fatalf("EmbedCalendar(player): %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the empty fragment, same as a missing calendar)", rec.Code)
	}
	assertNoLeak(t, "EmbedCalendar(:calId)", rec.Body.String())
}

// The embed's OTHER door: a dashboard block config passes the id as a query
// param, and it resolves through the same branch — so it takes the same gate.
func TestEmbedCalendar_HiddenCalendarByQueryParamDoesNotLeak(t *testing.T) {
	h := publicReadHandler("dm_only")

	rec, err := servePublicRead(h, campaigns.RolePlayer, "player-1", "", "cal-secret",
		(*Handler).EmbedCalendar)
	if err != nil {
		t.Fatalf("EmbedCalendar(query): %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertNoLeak(t, "EmbedCalendar(?calendarId)", rec.Body.String())
}

func TestUpcomingEventsFragment_HiddenCalendarDoesNotLeak(t *testing.T) {
	h := publicReadHandler("dm_only")

	rec, err := servePublicRead(h, campaigns.RolePlayer, "player-1", "cal-secret", "",
		(*Handler).UpcomingEventsFragment)
	if err != nil {
		t.Fatalf("UpcomingEventsFragment(player): %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the empty fragment)", rec.Code)
	}
	assertNoLeak(t, "UpcomingEventsFragment", rec.Body.String())
}

// The timeline has no empty fragment — it must refuse, and refuse with the
// SAME NotFound a missing calendar gets (ShowV2's behaviour), never a 403 that
// would confirm the calendar exists.
func TestShowTimeline_HiddenCalendarRefusesAsNotFound(t *testing.T) {
	h := publicReadHandler("dm_only")

	rec, err := servePublicRead(h, campaigns.RolePlayer, "player-1", "cal-secret", "",
		(*Handler).ShowTimeline)
	if err == nil {
		t.Fatalf("ShowTimeline(player) rendered %d bytes instead of refusing", rec.Body.Len())
	}
	assertAppError(t, err, http.StatusNotFound)
	if want := "calendar not found"; err.Error() != want && !strings.Contains(err.Error(), want) {
		t.Errorf("refusal %q does not match a missing calendar's %q", err.Error(), want)
	}
	assertNoLeak(t, "ShowTimeline", rec.Body.String())
}

// NO-REGRESSION: the DM the hidden calendar belongs to still gets all three,
// and a plain `everyone` calendar still renders for a player.
func TestPublicCalendarReads_StillRenderForPermittedViewers(t *testing.T) {
	cases := []struct {
		name       string
		visibility string
		role       campaigns.Role
	}{
		{"owner sees the dm_only calendar", "dm_only", campaigns.RoleOwner},
		{"player sees an everyone calendar", "everyone", campaigns.RolePlayer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := publicReadHandler(tc.visibility)

			rec, err := servePublicRead(h, tc.role, "u-1", "cal-secret", "", (*Handler).EmbedCalendar)
			if err != nil {
				t.Fatalf("EmbedCalendar: %v", err)
			}
			if !strings.Contains(rec.Body.String(), hiddenEventName) {
				t.Errorf("EmbedCalendar dropped %q for a permitted viewer", hiddenEventName)
			}

			rec, err = servePublicRead(h, tc.role, "u-1", "cal-secret", "", (*Handler).UpcomingEventsFragment)
			if err != nil {
				t.Fatalf("UpcomingEventsFragment: %v", err)
			}
			if !strings.Contains(rec.Body.String(), hiddenEventName) {
				t.Errorf("UpcomingEventsFragment dropped %q for a permitted viewer", hiddenEventName)
			}

			if _, err := servePublicRead(h, tc.role, "u-1", "cal-secret", "", (*Handler).ShowTimeline); err != nil {
				t.Fatalf("ShowTimeline refused a permitted viewer: %v", err)
			}
		})
	}
}
