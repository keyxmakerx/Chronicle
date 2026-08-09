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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

// worldStateCoord is ONE date/time coordinate on the world-state PUT, and it
// exists because a plain `*int` could express only two of the three states a
// coordinate actually has.
//
// THE DEFECT IT CLOSES. A GM cleared the Year box on Set date and submitted;
// the world moved to year 0, HTTP 200, stored. Both browser writers read the
// field with `parseInt(el.value, 10)` and a fallback of **0**, so "empty"
// reached the server as a number the GM never typed — and year 0 is a
// legitimate year on a fantasy calendar (see BuildWorldStateSeed's header and
// the weather-on-date bounds check, which both say so explicitly), so no
// value-based rule can tell the two apart. The fix is to keep zero settable and
// make EMPTY expressible instead:
//
//	absent / null   → Set=false, Blank=false → preserve the stored value
//	a JSON number   → Set=true                → write it, zero and negative included
//	a numeric string→ Set=true                → write it (an <input> holds strings)
//	"" / "  " / junk→ Blank=true              → REFUSE, naming the field
//
// UnmarshalJSON never returns an error, deliberately. An error here aborts
// Echo's Bind and the handler can only answer a generic "invalid request",
// which is the same dead end for the GM as the silent zero was: it does not
// say which box was empty. Recording Blank lets the handler name the
// coordinate.
type worldStateCoord struct {
	// Set is true when a usable number arrived.
	Set bool
	// Blank is true when the key arrived but held nothing usable — an empty
	// input, whitespace, or a value that is not a number at all.
	Blank bool
	// Value is meaningful only when Set.
	Value int
}

// UnmarshalJSON implements the absent/number/blank split described on the type.
func (c *worldStateCoord) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil // absent-equivalent: preserve
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var raw string
		if err := json.Unmarshal(b, &raw); err != nil {
			c.Blank = true
			return nil
		}
		s = strings.TrimSpace(raw)
		if s == "" {
			c.Blank = true
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			c.Blank = true
			return nil
		}
		c.Set, c.Value = true, n
		return nil
	}
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		c.Blank = true
		return nil
	}
	c.Set, c.Value = true, n
	return nil
}

// Ptr renders the coordinate as the service's partial-set pointer: nil when the
// caller did not name it, a value when they did.
func (c worldStateCoord) Ptr() *int {
	if !c.Set {
		return nil
	}
	v := c.Value
	return &v
}

// putWorldStateBody is the PUT request shape. All sections are optional —
// send mood without time or vice-versa. The date/time fields are
// worldStateCoords, which distinguish "set to this value" from "leave
// unchanged" AND from "the GM's box was empty".
type putWorldStateBody struct {
	Mood *struct {
		Color     *string `json:"color"`
		Intensity float64 `json:"intensity"`
	} `json:"moodTint"`
	Time *struct {
		Year   worldStateCoord `json:"year"`
		Month  worldStateCoord `json:"month"`
		Day    worldStateCoord `json:"day"`
		Hour   worldStateCoord `json:"hour"`
		Minute worldStateCoord `json:"minute"`
	} `json:"time"`
	// Advance is the GM panel's relative-verb path (+1hr / +1day /
	// +long-rest / step-back). Signed; full rollover server-side.
	Advance *struct {
		Days    int `json:"days"`
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	} `json:"advance"`
	// Weather is the GM panel's current-period weather override (4b).
	Weather *string `json:"weather"`
	// WeatherDate, when present alongside Weather, retargets the override to an
	// arbitrary day (the event drawer's "set weather for this day", C-CAL-EDITOR-
	// EXPANSION PR2). Additive optional field — absent in old clients, so the
	// wire contract is unchanged (omitted = current-date behavior).
	WeatherDate *struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"weatherDate"`
	// TriggerEvent is the GM panel's "trigger world-event" (4c). DmOnly is
	// honored only for capability holders (CanAuthorDmOnly).
	TriggerEvent *struct {
		Type          string `json:"type"`
		Name          string `json:"name"`
		StartHour     int    `json:"start_hour"`
		DurationHours int    `json:"duration_hours"`
		DmOnly        bool   `json:"dm_only"`
	} `json:"triggerEvent"`
	// ClearEvents removes all world-events on the current day (C-CAL-GM-PANEL-
	// REWORK B — the "stuck meteor" off switch). PUT-input only; not a seed field.
	ClearEvents bool `json:"clearEvents"`
	// ClearEventType removes only that type's world-events on the current day
	// (the GM panel's per-event × chip). Additive optional field — absent in
	// old clients, so the wire contract is unchanged.
	ClearEventType string `json:"clearEventType"`
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
		// A coordinate that arrived EMPTY is refused by name before anything is
		// written. Substituting a number here is what moved a campaign to year 0.
		for _, f := range []struct {
			name  string
			coord worldStateCoord
		}{
			{"year", body.Time.Year}, {"month", body.Time.Month}, {"day", body.Time.Day},
			{"hour", body.Time.Hour}, {"minute", body.Time.Minute},
		} {
			if f.coord.Blank {
				return apperror.NewValidation(f.name +
					" must be a number — leave it out to keep the stored value")
			}
		}
		input.Time = &WorldStateTimeSet{
			Year:   body.Time.Year.Ptr(),
			Month:  body.Time.Month.Ptr(),
			Day:    body.Time.Day.Ptr(),
			Hour:   body.Time.Hour.Ptr(),
			Minute: body.Time.Minute.Ptr(),
		}
	}
	if body.Advance != nil {
		input.Advance = &WorldStateAdvance{
			Days:    body.Advance.Days,
			Hours:   body.Advance.Hours,
			Minutes: body.Advance.Minutes,
		}
	}
	if body.Weather != nil {
		input.Weather = body.Weather
		if body.WeatherDate != nil {
			input.WeatherDate = &WorldStateWeatherDate{
				Year:  body.WeatherDate.Year,
				Month: body.WeatherDate.Month,
				Day:   body.WeatherDate.Day,
			}
		}
	}
	if body.TriggerEvent != nil {
		te := body.TriggerEvent
		// dm_only is honored only for capability holders (co-DM authoring);
		// otherwise the triggered event is public. Capability check stays at
		// the handler layer, mirroring the calendar event create path.
		vis := storageVisibilityEveryone
		if te.DmOnly && campaigns.GetCampaignContext(c).CanAuthorDmOnly() {
			vis = storageVisibilityDMOnly
		}
		input.TriggerEvent = &WorldStateTriggerEvent{
			Type:          te.Type,
			Name:          te.Name,
			StartHour:     te.StartHour,
			DurationHours: te.DurationHours,
			Visibility:    vis,
		}
	}
	// B: the GM "clear world-events" off switch (mirrors the mood Clear).
	input.ClearEvents = body.ClearEvents
	input.ClearEventType = body.ClearEventType

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
