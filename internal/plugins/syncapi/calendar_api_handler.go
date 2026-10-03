package syncapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CalendarAPIHandler serves the calendar REST surface the Foundry module's
// calendar sync uses. Every route works on the campaign's default calendar
// (or its first, when none is marked default) and goes through calendar.CalendarService's (calendarID, campaignID)
// methods with the caller's Viewer, so a read or write can never reach
// another campaign's calendar and a player never receives what the
// calendar pages would hide from them.
//
// The pre-V5 structure, settings, advance, world-state, event-category,
// export and import routes are retired (retiredCalendarRoute in routes.go).
type CalendarAPIHandler struct {
	syncSvc     SyncAPIService
	calendarSvc calendar.CalendarService
	campaignSvc campaigns.CampaignService
}

// NewCalendarAPIHandler creates the calendar API handler.
func NewCalendarAPIHandler(syncSvc SyncAPIService, calendarSvc calendar.CalendarService, campaignSvc campaigns.CampaignService) *CalendarAPIHandler {
	return &CalendarAPIHandler{syncSvc: syncSvc, calendarSvc: calendarSvc, campaignSvc: campaignSvc}
}

// retiredCalendarRoute answers a pre-V5 calendar route that the new calendar
// has no API for. 410, not 404, so a script still calling it learns the route
// is gone for good and what to use instead. The Foundry module calls none of
// them.
func retiredCalendarRoute(instead string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusGone, map[string]string{
			"error":   "calendar_route_retired",
			"message": "This calendar API route was retired. " + instead,
		})
	}
}

// What each retired route says to use instead.
const (
	retiredStructure       = "Calendar structure and settings are edited in Chronicle's calendar. Foundry's Import button creates a campaign's first calendar; after that dates and events sync."
	retiredEventCategories = "Event kinds replaced event categories; edit them in Chronicle's calendar."
	retiredAdvance         = "Set the date with PUT /calendar/date."
)

// --- Caller identity ---

// viewer builds the caller's Viewer the way the calendar pages do
// (CampaignContext.VisibilityRole): the key owner's live campaign role, or
// Owner for a co-DM grant. A stored Bearer key's owner is a live Owner
// (RequireKeyOwnerStillOwner); the session door carries the signed-in
// member's own role. An unresolved member fails closed to RoleNone.
func (h *CalendarAPIHandler) viewer(c echo.Context) permissions.Viewer {
	key := GetAPIKey(c)
	if key == nil {
		return permissions.RequestViewer(int(campaigns.RoleNone), "")
	}
	ctx := c.Request().Context()
	member, err := h.campaignSvc.GetMember(ctx, key.CampaignID, key.UserID)
	if err != nil || member == nil {
		return permissions.RequestViewer(int(campaigns.RoleNone), key.UserID)
	}
	role := member.Role
	// The grant only counts for a member, as on the web.
	if role < campaigns.RoleOwner {
		if granted, err := h.campaignSvc.IsUserDmGranted(ctx, key.CampaignID, key.UserID); err == nil && granted {
			role = campaigns.RoleOwner
		}
	}
	return permissions.RequestViewer(int(role), key.UserID)
}

// requireOwner mirrors the calendar plugin's RequireRole(Owner) web routes
// (settings, event delete) for the same actions over the API, where
// RequirePermission(PermWrite) alone would also admit a Scribe.
func (h *CalendarAPIHandler) requireOwner(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewForbidden("owner role required")
	}
	member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
	if err != nil || member == nil || member.Role < campaigns.RoleOwner {
		return apperror.NewForbidden("owner role required")
	}
	return nil
}

// defaultCalendar loads the one calendar Foundry follows for v (the default,
// or the first when none is marked default), with its sub-resources already
// filtered for v. A campaign with no calendar, or one v cannot see, is the
// same 404 the module reads as "no calendar".
func (h *CalendarAPIHandler) defaultCalendar(c echo.Context, v permissions.Viewer) (*calendar.Calendar, error) {
	return h.calendarSvc.GetPrimaryCalendarForViewer(c.Request().Context(), c.Param("id"), v)
}

// --- Visibility on the wire ---

// Chronicle stores "dm_only"; the sync wire contract says "gm-only". The
// module treats anything but "gm-only"/"gm_only" as public, so a stored
// value leaking out would show a GM-only event to players in Foundry.
const (
	wireVisibilityGMOnly = "gm-only"
	storedVisibilityDM   = "dm_only"
)

// visibilityFromWire maps an inbound wire visibility to the stored value.
// Unknown values pass through for the service to validate.
func visibilityFromWire(v string) string {
	switch v {
	case wireVisibilityGMOnly, "gm_only":
		return storedVisibilityDM
	}
	return v
}

// eventForWire returns a response copy of e with its visibility in the wire
// form.
func eventForWire(e calendar.Event) calendar.Event {
	if e.Visibility == storedVisibilityDM {
		e.Visibility = wireVisibilityGMOnly
	}
	return e
}

// --- Calendar reads ---

// ListCalendars returns the campaign's calendars the caller can see.
// GET /api/v1/campaigns/:id/calendars
func (h *CalendarAPIHandler) ListCalendars(c echo.Context) error {
	cals, err := h.calendarSvc.ListCalendars(c.Request().Context(), c.Param("id"), h.viewer(c))
	if err != nil {
		return err
	}
	if cals == nil {
		cals = []calendar.Calendar{}
	}
	return c.JSON(http.StatusOK, map[string]any{"data": cals, "total": len(cals)})
}

// GetCalendar returns the default calendar with its structure.
// GET /api/v1/campaigns/:id/calendar
func (h *CalendarAPIHandler) GetCalendar(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, cal)
}

// GetCurrentDate returns the current date with the season, era, moon phases
// and weather for it. A read by a real Bearer key (the module) records the
// served-date beacon; a member browsing over the session door never does.
// GET /api/v1/campaigns/:id/calendar/date
func (h *CalendarAPIHandler) GetCurrentDate(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	h.recordCalendarDateBeaconIfModule(c, cal.CampaignID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)

	result := map[string]any{
		"mode":   cal.Mode,
		"year":   cal.CurrentYear,
		"month":  cal.CurrentMonth,
		"day":    cal.CurrentDay,
		"hour":   cal.CurrentHour,
		"minute": cal.CurrentMinute,
		// The module pauses its date push when this is true; the effective
		// predicate, so it can never disagree with SetCurrentDate's refusal.
		"tracks_real_time": cal.UsesRealTime(),
	}
	if season := cal.CurrentSeason(); season != nil {
		result["current_season"] = map[string]any{"id": season.ID, "name": season.Name, "color": season.Color}
	}
	// Eras are stripped for players by the service, so this is nil for them.
	if era := cal.CurrentEra(); era != nil {
		result["current_era"] = map[string]any{
			"id": era.ID, "name": era.Name, "start_year": era.StartYear, "color": era.Color,
		}
	}
	if len(cal.Moons) > 0 {
		absDay := cal.CurrentDayIndex() // the counter the calendar pages read moons from
		phases := make([]map[string]any, 0, len(cal.Moons))
		for i := range cal.Moons {
			m := &cal.Moons[i]
			phases = append(phases, map[string]any{
				"moon_id":        m.ID,
				"moon_name":      m.Name,
				"phase_name":     m.MoonPhaseName(absDay),
				"phase_position": m.MoonPhase(absDay),
				"phase_icon":     m.MoonPhaseIcon(absDay),
			})
		}
		result["current_moon_phases"] = phases
	}
	if cal.Weather != nil {
		result["current_weather"] = cal.Weather
	}
	return c.JSON(http.StatusOK, result)
}

// confirmDateRequest is the JSON body for POST /calendar/date/confirm.
type confirmDateRequest struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// ConfirmDate records the date the module actually applied in Foundry. Its
// whole purpose is the write, so a session-door caller (never the module)
// gets 403 rather than a misleading 204.
// POST /api/v1/campaigns/:id/calendar/date/confirm
func (h *CalendarAPIHandler) ConfirmDate(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil || key.ID == synthKeySessionID {
		return apperror.NewForbidden("calendar date confirmation requires a real API key")
	}
	var req confirmDateRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.syncSvc.ConfirmCalendarDate(c.Request().Context(), key.CampaignID, req.Year, req.Month, req.Day); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Sub-resource reads (already filtered for the caller by the service) ---

// GetSeasons returns the default calendar's seasons.
// GET /api/v1/campaigns/:id/calendar/seasons
func (h *CalendarAPIHandler) GetSeasons(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNil(cal.Seasons)})
}

// GetMoons returns the default calendar's moons, without hidden ones for a
// player.
// GET /api/v1/campaigns/:id/calendar/moons
func (h *CalendarAPIHandler) GetMoons(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNil(cal.Moons)})
}

// GetEras returns the default calendar's eras as the viewer may see them:
// for a player, eras hidden until they begin are withheld and Director
// notes stripped (the calendar service's viewer read does both).
// GET /api/v1/campaigns/:id/calendar/eras
func (h *CalendarAPIHandler) GetEras(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNil(cal.Eras)})
}

// GetStructure returns the months, weekdays, time system and leap rule.
// GET /api/v1/campaigns/:id/calendar/structure
func (h *CalendarAPIHandler) GetStructure(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"id":                 cal.ID,
		"name":               cal.Name,
		"mode":               cal.Mode,
		"hours_per_day":      cal.HoursPerDay,
		"minutes_per_hour":   cal.MinutesPerHour,
		"seconds_per_minute": cal.SecondsPerMinute,
		"epoch_name":         cal.EpochName,
		"months":             nonNil(cal.Months),
		"weekdays":           nonNil(cal.Weekdays),
		"leap_year":          map[string]any{"every": cal.LeapYearEvery, "offset": cal.LeapYearOffset},
	})
}

// GetWeather returns today's weather, or {} when none is set.
// GET /api/v1/campaigns/:id/calendar/weather
func (h *CalendarAPIHandler) GetWeather(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	if cal.Weather == nil {
		return c.JSON(http.StatusOK, map[string]any{})
	}
	return c.JSON(http.StatusOK, cal.Weather)
}

// GetCycles returns the default calendar's cycles.
// GET /api/v1/campaigns/:id/calendar/cycles
func (h *CalendarAPIHandler) GetCycles(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNil(cal.Cycles)})
}

// GetFestivals returns the default calendar's festivals.
// GET /api/v1/campaigns/:id/calendar/festivals
func (h *CalendarAPIHandler) GetFestivals(c echo.Context) error {
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNil(cal.Festivals)})
}

// nonNil returns s, or an empty slice so the wire carries [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// --- Event reads ---

// ListEvents returns the events in one month (default: the current one),
// filtered for the caller exactly as the calendar pages filter them. A
// repeating event carries its dates in the month as `occurrences` (skips and
// moves applied), the only way a rule event's dates reach a client.
// TODO(#930): the module places events by their own date and ignores
// `occurrences`, so a rule event shows on its start date only in Foundry.
// GET /api/v1/campaigns/:id/calendar/events?year=N&month=M
func (h *CalendarAPIHandler) ListEvents(c echo.Context) error {
	v := h.viewer(c)
	cal, err := h.defaultCalendar(c, v)
	if err != nil {
		return err
	}
	year, month := cal.CurrentYear, cal.CurrentMonth
	if y, err := strconv.Atoi(c.QueryParam("year")); err == nil {
		year = y
	}
	if m, err := strconv.Atoi(c.QueryParam("month")); err == nil && m > 0 {
		month = m
	}
	events, err := h.calendarSvc.ListEventsForMonth(c.Request().Context(), cal.ID, cal.CampaignID, year, month, v)
	if err != nil {
		return err
	}
	out := make([]calendar.Event, 0, len(events))
	for _, e := range events {
		out = append(out, eventForWire(e))
	}
	sanitizeCalendarEventsHTMLForEgress(out)
	return c.JSON(http.StatusOK, map[string]any{"data": out, "total": len(out)})
}

// GetEvent returns one event of the default calendar.
// GET /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) GetEvent(c echo.Context) error {
	v := h.viewer(c)
	cal, err := h.defaultCalendar(c, v)
	if err != nil {
		return err
	}
	evt, err := h.calendarSvc.GetEventForViewer(c.Request().Context(), c.Param("eventID"), cal.ID, cal.CampaignID, v)
	if err != nil {
		return err
	}
	out := eventForWire(*evt)
	sanitizeCalendarEventHTMLForEgress(&out)
	return c.JSON(http.StatusOK, out)
}

// --- Event writes ---

// apiCreateEventRequest is the JSON body for POST /calendar/events. Field
// names match the calendar plugin's own event API.
type apiCreateEventRequest struct {
	Name                     string  `json:"name"`
	Description              *string `json:"description"`
	DescriptionHTML          *string `json:"description_html"`
	EntityID                 *string `json:"entity_id"`
	Year                     int     `json:"year"`
	Month                    int     `json:"month"`
	Day                      int     `json:"day"`
	StartHour                *int    `json:"start_hour"`
	StartMinute              *int    `json:"start_minute"`
	EndYear                  *int    `json:"end_year"`
	EndMonth                 *int    `json:"end_month"`
	EndDay                   *int    `json:"end_day"`
	EndHour                  *int    `json:"end_hour"`
	EndMinute                *int    `json:"end_minute"`
	IsRecurring              bool    `json:"is_recurring"`
	RecurrenceType           *string `json:"recurrence_type"`
	RecurrenceInterval       *int    `json:"recurrence_interval"`
	RecurrenceEndYear        *int    `json:"recurrence_end_year"`
	RecurrenceEndMonth       *int    `json:"recurrence_end_month"`
	RecurrenceEndDay         *int    `json:"recurrence_end_day"`
	RecurrenceMaxOccurrences *int    `json:"recurrence_max_occurrences"`
	Visibility               string  `json:"visibility"`
	VisibilityRules          *string `json:"visibility_rules"`
	KindID                   *int    `json:"kind_id"`
	Announced                *string `json:"announced"`
	Tier                     *string `json:"tier"`
	Color                    *string `json:"color"`
	Icon                     *string `json:"icon"`
	AllDay                   bool    `json:"all_day"`
}

// CreateEvent creates an event on the default calendar. PermWrite is
// Scribe+, matching the web route; the service refuses a gm-only event from
// a caller who may not author one.
// POST /api/v1/campaigns/:id/calendar/events
func (h *CalendarAPIHandler) CreateEvent(c echo.Context) error {
	v := h.viewer(c)
	cal, err := h.defaultCalendar(c, v)
	if err != nil {
		return err
	}
	var req apiCreateEventRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	evt, err := h.calendarSvc.CreateEvent(c.Request().Context(), cal.ID, cal.CampaignID, calendar.CreateEventInput{
		CanAuthorDmOnly:          v.SkipsPerUserRules(),
		Name:                     req.Name,
		Description:              req.Description,
		DescriptionHTML:          req.DescriptionHTML,
		EntityID:                 req.EntityID,
		Year:                     req.Year,
		Month:                    req.Month,
		Day:                      req.Day,
		StartHour:                req.StartHour,
		StartMinute:              req.StartMinute,
		EndYear:                  req.EndYear,
		EndMonth:                 req.EndMonth,
		EndDay:                   req.EndDay,
		EndHour:                  req.EndHour,
		EndMinute:                req.EndMinute,
		IsRecurring:              req.IsRecurring,
		RecurrenceType:           req.RecurrenceType,
		RecurrenceInterval:       req.RecurrenceInterval,
		RecurrenceEndYear:        req.RecurrenceEndYear,
		RecurrenceEndMonth:       req.RecurrenceEndMonth,
		RecurrenceEndDay:         req.RecurrenceEndDay,
		RecurrenceMaxOccurrences: req.RecurrenceMaxOccurrences,
		Visibility:               visibilityFromWire(req.Visibility),
		VisibilityRules:          req.VisibilityRules,
		KindID:                   req.KindID,
		Announced:                req.Announced,
		Tier:                     req.Tier,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
		CreatedBy:                v.UserID(),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, eventForWire(*evt))
}

// apiUpdateEventRequest is the JSON body for PUT /calendar/events/:id.
// PARTIAL: absent preserves, null clears, a value replaces — the module
// sends only the fields it changed.
type apiUpdateEventRequest struct {
	Name                     patch.Field[string] `json:"name"`
	Description              patch.Field[string] `json:"description"`
	DescriptionHTML          patch.Field[string] `json:"description_html"`
	EntityID                 patch.Field[string] `json:"entity_id"`
	Year                     patch.Field[int]    `json:"year"`
	Month                    patch.Field[int]    `json:"month"`
	Day                      patch.Field[int]    `json:"day"`
	StartHour                patch.Field[int]    `json:"start_hour"`
	StartMinute              patch.Field[int]    `json:"start_minute"`
	EndYear                  patch.Field[int]    `json:"end_year"`
	EndMonth                 patch.Field[int]    `json:"end_month"`
	EndDay                   patch.Field[int]    `json:"end_day"`
	EndHour                  patch.Field[int]    `json:"end_hour"`
	EndMinute                patch.Field[int]    `json:"end_minute"`
	IsRecurring              patch.Field[bool]   `json:"is_recurring"`
	RecurrenceType           patch.Field[string] `json:"recurrence_type"`
	RecurrenceInterval       patch.Field[int]    `json:"recurrence_interval"`
	RecurrenceEndYear        patch.Field[int]    `json:"recurrence_end_year"`
	RecurrenceEndMonth       patch.Field[int]    `json:"recurrence_end_month"`
	RecurrenceEndDay         patch.Field[int]    `json:"recurrence_end_day"`
	RecurrenceMaxOccurrences patch.Field[int]    `json:"recurrence_max_occurrences"`
	Visibility               patch.Field[string] `json:"visibility"`
	VisibilityRules          patch.Field[string] `json:"visibility_rules"`
	KindID                   patch.Field[int]    `json:"kind_id"`
	Announced                patch.Field[string] `json:"announced"`
	Tier                     patch.Field[string] `json:"tier"`
	Color                    patch.Field[string] `json:"color"`
	Icon                     patch.Field[string] `json:"icon"`
	AllDay                   patch.Field[bool]   `json:"all_day"`
}

// UpdateEvent applies a partial update to an event of the default calendar.
// The service answers 404 for an event the caller cannot see and 403 for a
// visibility change the caller may not make.
// PUT /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) UpdateEvent(c echo.Context) error {
	v := h.viewer(c)
	cal, err := h.defaultCalendar(c, v)
	if err != nil {
		return err
	}
	var req apiUpdateEventRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	visibility := req.Visibility
	if s, ok := visibility.Get(); ok {
		visibility = patch.Of(visibilityFromWire(s))
	}
	if err := h.calendarSvc.UpdateEvent(c.Request().Context(), c.Param("eventID"), cal.ID, cal.CampaignID, calendar.UpdateEventInput{
		Name:                     req.Name,
		Description:              req.Description,
		DescriptionHTML:          req.DescriptionHTML,
		EntityID:                 req.EntityID,
		Year:                     req.Year,
		Month:                    req.Month,
		Day:                      req.Day,
		StartHour:                req.StartHour,
		StartMinute:              req.StartMinute,
		EndYear:                  req.EndYear,
		EndMonth:                 req.EndMonth,
		EndDay:                   req.EndDay,
		EndHour:                  req.EndHour,
		EndMinute:                req.EndMinute,
		IsRecurring:              req.IsRecurring,
		RecurrenceType:           req.RecurrenceType,
		RecurrenceInterval:       req.RecurrenceInterval,
		RecurrenceEndYear:        req.RecurrenceEndYear,
		RecurrenceEndMonth:       req.RecurrenceEndMonth,
		RecurrenceEndDay:         req.RecurrenceEndDay,
		RecurrenceMaxOccurrences: req.RecurrenceMaxOccurrences,
		Visibility:               visibility,
		VisibilityRules:          req.VisibilityRules,
		KindID:                   req.KindID,
		Announced:                req.Announced,
		Tier:                     req.Tier,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
	}, v); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEvent removes an event of the default calendar. Owner only, like the
// web route.
// DELETE /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) DeleteEvent(c echo.Context) error {
	if err := h.requireOwner(c); err != nil {
		return err
	}
	v := h.viewer(c)
	cal, err := h.defaultCalendar(c, v)
	if err != nil {
		return err
	}
	if err := h.calendarSvc.DeleteEvent(c.Request().Context(), c.Param("eventID"), cal.ID, cal.CampaignID, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Date ---

// apiSetDateRequest is the JSON body for PUT /calendar/date.
type apiSetDateRequest struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// SetDate moves the default calendar to an absolute date and time. Owner
// only, like the calendar settings page. A real-time calendar answers 422,
// which the module reads as "dates are read-only here".
// PUT /api/v1/campaigns/:id/calendar/date
func (h *CalendarAPIHandler) SetDate(c echo.Context) error {
	if err := h.requireOwner(c); err != nil {
		return err
	}
	cal, err := h.defaultCalendar(c, h.viewer(c))
	if err != nil {
		return err
	}
	var req apiSetDateRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.calendarSvc.SetCurrentDate(c.Request().Context(), cal.ID, cal.CampaignID,
		req.Year, req.Month, req.Day, req.Hour, req.Minute); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"year":   req.Year,
		"month":  req.Month,
		"day":    req.Day,
		"hour":   req.Hour,
		"minute": req.Minute,
	})
}

// maxFoundryImportBytes caps the Calendaria payload. A real calendar is a few
// kilobytes; the cap keeps a hostile body from being decoded whole.
const maxFoundryImportBytes = 1 << 20

// CreateCalendar is the module's "Import into Chronicle" button: it creates
// the campaign's first calendar from a Calendaria calendar. Owner only, like
// creating a calendar on the web. A campaign that already has a calendar
// gets 409 so a repeated click never makes a second copy.
// POST /api/v1/campaigns/:id/calendar
func (h *CalendarAPIHandler) CreateCalendar(c echo.Context) error {
	if err := h.requireOwner(c); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxFoundryImportBytes+1))
	if err != nil {
		return apperror.NewBadRequest("could not read the calendar")
	}
	if len(body) > maxFoundryImportBytes {
		return apperror.NewBadRequest("the calendar sent from Foundry is too large")
	}
	cal, warnings, err := h.calendarSvc.ImportFoundryCalendar(c.Request().Context(), c.Param("id"), body)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{
		"created":  cal,
		"warnings": nonNil(warnings),
	})
}

// --- Helpers ---

// recordCalendarDateBeaconIfModule records the served-date beacon only for a
// real Bearer key: the session door's synthetic key (synthKeySessionID) is a
// member browsing, not the module. Fire-and-forget on a background context
// so the GET does not wait on the throttle read and upsert.
func (h *CalendarAPIHandler) recordCalendarDateBeaconIfModule(c echo.Context, campaignID string, year, month, day int) {
	key := GetAPIKey(c)
	if key == nil || key.ID == synthKeySessionID {
		return
	}
	go func() {
		if err := h.syncSvc.RecordCalendarDateBeacon(context.Background(), campaignID, year, month, day); err != nil {
			slog.Warn("calendar date beacon record failed",
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
		}
	}()
}
