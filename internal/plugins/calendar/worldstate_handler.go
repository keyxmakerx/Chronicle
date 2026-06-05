// worldstate_handler.go — session-authenticated world-state endpoints
// (C-CAL-WORLDSTATE-SERVER-MODEL).
//
// Route shape: GET/PUT /campaigns/:id/calendar/world-state (registered in
// routes.go). The dispatch sketched an /api/v1/ path, but the GET/PUT here
// are the production-calendar (Phase 2) surface and the GOAL pins their auth
// to session roles + per-event dm_only filtering (VisibilityRole /
// filterEventsByUser) — concepts that only exist on the session-authenticated
// CampaignContext, not the per-campaign-token Foundry API. So these live on
// the session group: GET is Player+, PUT is Owner-only. The later Foundry
// push (Phase 5b) can add a token-auth variant on top of the same service.
// Path/auth choice flagged in the PR + cordinator#27.
package calendar

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// resolveWorldStateCalendar picks the calendar the world-state endpoints act
// on: an explicit ?calendarId= (validated in-campaign) or the user's active
// calendar. Returns nil + a typed not-found when the campaign has no calendar.
func (h *Handler) resolveWorldStateCalendar(c echo.Context) (*Calendar, error) {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	if calID := c.QueryParam("calendarId"); calID != "" {
		return h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	}
	cal, err := h.svc.GetActiveCalendar(ctx, userID, cc.Campaign.ID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return nil, apperror.NewNotFound("no calendar configured for this campaign")
	}
	return cal, nil
}

// GetWorldState — GET /campaigns/:id/calendar/world-state.
//
// Returns the Part-8 world-state seed for the requested (or current) date.
// Player+ (the route gates the role); GM-only celestial events are filtered
// for non-DM viewers via the role passed into the seed builder.
func (h *Handler) GetWorldState(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.resolveWorldStateCalendar(c)
	if err != nil {
		return err
	}

	// Optional date pin via query params; default (0) → current date.
	year := atoiOr(c.QueryParam("year"), 0)
	month := atoiOr(c.QueryParam("month"), 0)
	day := atoiOr(c.QueryParam("day"), 0)

	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)
	seed, err := h.svc.BuildWorldStateSeed(ctx, cal.ID, year, month, day, role, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, seed)
}

// putWorldStateBody is the PUT request shape. All sections are optional —
// send mood without time or vice-versa. Pointer date/time fields distinguish
// "set" from "leave unchanged".
type putWorldStateBody struct {
	Mood *struct {
		Color     *string `json:"color"`
		Intensity float64 `json:"intensity"`
	} `json:"moodTint"`
	Time *struct {
		Year   *int `json:"year"`
		Month  *int `json:"month"`
		Day    *int `json:"day"`
		Hour   *int `json:"hour"`
		Minute *int `json:"minute"`
	} `json:"time"`
	// Advance is the GM panel's relative-verb path (+1hr / +1day /
	// +long-rest / step-back). Signed; full rollover server-side.
	Advance *struct {
		Days    int `json:"days"`
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	} `json:"advance"`
}

// PutWorldState — PUT /campaigns/:id/calendar/world-state.
//
// Sets the live mood + advances/sets time, then emits
// calendar.worldstate.changed. Owner-only for now (route-gated); the service
// seam keeps role policy at the route layer so the Phase-3 co-GM capability
// grant (D6) can widen this without a service rewrite.
func (h *Handler) PutWorldState(c echo.Context) error {
	ctx := c.Request().Context()

	cal, err := h.resolveWorldStateCalendar(c)
	if err != nil {
		return err
	}

	var body putWorldStateBody
	if err := c.Bind(&body); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := WorldStateUpdateInput{}
	if body.Mood != nil {
		input.Mood = &WorldStateMoodTint{Color: body.Mood.Color, Intensity: body.Mood.Intensity}
	}
	if body.Time != nil {
		input.Time = &WorldStateTimeSet{
			Year:   body.Time.Year,
			Month:  body.Time.Month,
			Day:    body.Time.Day,
			Hour:   body.Time.Hour,
			Minute: body.Time.Minute,
		}
	}
	if body.Advance != nil {
		input.Advance = &WorldStateAdvance{
			Days:    body.Advance.Days,
			Hours:   body.Advance.Hours,
			Minutes: body.Advance.Minutes,
		}
	}

	if err := h.svc.SetWorldState(ctx, cal.ID, input); err != nil {
		return err
	}

	// Echo back the freshly-built seed (DM role for the writer = Owner) so the
	// caller can re-render without a follow-up GET.
	cc := campaigns.GetCampaignContext(c)
	seed, err := h.svc.BuildWorldStateSeed(ctx, cal.ID, 0, 0, 0, cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, seed)
}

// atoiOr parses a base-10 int, returning fallback on empty/invalid input.
// Kept local so the world-state handlers don't depend on strconv error
// handling at every call site.
func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		n = -n
	}
	return n
}
