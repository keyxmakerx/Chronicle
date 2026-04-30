package syncapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// CalendarAPIHandler serves calendar-related REST API endpoints for external
// tools (Foundry VTT Calendaria sync, etc.). Authenticates via API keys.
type CalendarAPIHandler struct {
	syncSvc     SyncAPIService
	calendarSvc calendar.CalendarService
}

// NewCalendarAPIHandler creates a new calendar API handler.
func NewCalendarAPIHandler(syncSvc SyncAPIService, calendarSvc calendar.CalendarService) *CalendarAPIHandler {
	return &CalendarAPIHandler{
		syncSvc:     syncSvc,
		calendarSvc: calendarSvc,
	}
}

// --- Calendar Read ---

// ListCalendars returns all calendars for a campaign.
// GET /api/v1/campaigns/:id/calendars
func (h *CalendarAPIHandler) ListCalendars(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cals, err := h.calendarSvc.ListCalendars(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to list calendars", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list calendars"))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  cals,
		"total": len(cals),
	})
}

// GetCalendar returns the full calendar structure for a campaign.
// GET /api/v1/campaigns/:id/calendar (default calendar, backward compat)
func (h *CalendarAPIHandler) GetCalendar(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to get calendar", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to get calendar"))
	}
	if cal == nil {
		return apperror.NewNotFound("no calendar configured for this campaign")
	}

	return c.JSON(http.StatusOK, cal)
}

// GetCurrentDate returns the current in-game date with computed state
// (current season, moon phases, era, weather).
// GET /api/v1/campaigns/:id/calendar/date
func (h *CalendarAPIHandler) GetCurrentDate(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	result := map[string]any{
		"mode":   cal.Mode,
		"year":   cal.CurrentYear,
		"month":  cal.CurrentMonth,
		"day":    cal.CurrentDay,
		"hour":   cal.CurrentHour,
		"minute": cal.CurrentMinute,
	}

	// Computed: current season.
	if season := cal.CurrentSeason(); season != nil {
		result["current_season"] = map[string]any{
			"id":    season.ID,
			"name":  season.Name,
			"color": season.Color,
		}
	}

	// Computed: current era.
	if era := cal.CurrentEra(); era != nil {
		result["current_era"] = map[string]any{
			"id":         era.ID,
			"name":       era.Name,
			"start_year": era.StartYear,
			"color":      era.Color,
		}
	}

	// Computed: current moon phases.
	if len(cal.Moons) > 0 {
		absDay := cal.CurrentAbsoluteDay()
		moonPhases := make([]map[string]any, 0, len(cal.Moons))
		for i := range cal.Moons {
			moon := &cal.Moons[i]
			moonPhases = append(moonPhases, map[string]any{
				"moon_id":        moon.ID,
				"moon_name":      moon.Name,
				"phase_name":     moon.MoonPhaseName(absDay),
				"phase_position": moon.MoonPhase(absDay),
				"phase_icon":     moon.MoonPhaseIcon(absDay),
			})
		}
		result["current_moon_phases"] = moonPhases
	}

	// Computed: current weather.
	if weather, err := h.calendarSvc.GetWeather(ctx, cal.ID); err == nil && weather != nil {
		result["current_weather"] = weather
	}

	return c.JSON(http.StatusOK, result)
}

// --- Sub-resource Read ---

// GetSeasons returns all season definitions.
// GET /api/v1/campaigns/:id/calendar/seasons
func (h *CalendarAPIHandler) GetSeasons(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	return c.JSON(http.StatusOK, map[string]any{"data": cal.Seasons})
}

// GetMoons returns all moon definitions.
// GET /api/v1/campaigns/:id/calendar/moons
func (h *CalendarAPIHandler) GetMoons(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	return c.JSON(http.StatusOK, map[string]any{"data": cal.Moons})
}

// GetEras returns all era definitions.
// GET /api/v1/campaigns/:id/calendar/eras
func (h *CalendarAPIHandler) GetEras(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	return c.JSON(http.StatusOK, map[string]any{"data": cal.Eras})
}

// GetEventCategories returns all event category definitions.
// GET /api/v1/campaigns/:id/calendar/event-categories
func (h *CalendarAPIHandler) GetEventCategories(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	cats, err := h.calendarSvc.GetEventCategories(ctx, cal.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to get event categories"))
	}

	return c.JSON(http.StatusOK, map[string]any{"data": cats})
}

// GetStructure returns the calendar structure (months, weekdays, time system, leap year).
// GET /api/v1/campaigns/:id/calendar/structure
func (h *CalendarAPIHandler) GetStructure(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":                 cal.ID,
		"name":               cal.Name,
		"mode":               cal.Mode,
		"hours_per_day":      cal.HoursPerDay,
		"minutes_per_hour":   cal.MinutesPerHour,
		"seconds_per_minute": cal.SecondsPerMinute,
		"epoch_name":         cal.EpochName,
		"months":             cal.Months,
		"weekdays":           cal.Weekdays,
		"leap_year": map[string]any{
			"every":  cal.LeapYearEvery,
			"offset": cal.LeapYearOffset,
		},
	})
}

// GetWeather returns the current weather state.
// GET /api/v1/campaigns/:id/calendar/weather
func (h *CalendarAPIHandler) GetWeather(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	weather, err := h.calendarSvc.GetWeather(ctx, cal.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to get weather"))
	}
	if weather == nil {
		return c.JSON(http.StatusOK, map[string]any{})
	}

	return c.JSON(http.StatusOK, weather)
}

// GetCycles returns all cycle definitions.
// GET /api/v1/campaigns/:id/calendar/cycles
func (h *CalendarAPIHandler) GetCycles(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	cycles, err := h.calendarSvc.GetCycles(ctx, cal.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to get cycles"))
	}

	return c.JSON(http.StatusOK, map[string]any{"data": cycles})
}

// GetFestivals returns all festival definitions.
// GET /api/v1/campaigns/:id/calendar/festivals
func (h *CalendarAPIHandler) GetFestivals(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	festivals, err := h.calendarSvc.GetFestivals(ctx, cal.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to get festivals"))
	}

	return c.JSON(http.StatusOK, map[string]any{"data": festivals})
}

// --- Events Read ---

// ListEvents returns events for a month or year.
// GET /api/v1/campaigns/:id/calendar/events?year=N&month=M
func (h *CalendarAPIHandler) ListEvents(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	role := h.resolveRole(c)
	year, _ := strconv.Atoi(c.QueryParam("year"))
	month, _ := strconv.Atoi(c.QueryParam("month"))

	if year == 0 {
		year = cal.CurrentYear
	}

	var events []calendar.Event

	// Sync API uses API-key auth, not user sessions, so pass empty userID
	// to skip per-user visibility filtering.
	if month > 0 {
		events, err = h.calendarSvc.ListEventsForMonth(ctx, cal.ID, year, month, role, "")
	} else {
		// No month specified — return events for the entity if entity_id is provided.
		entityID := c.QueryParam("entity_id")
		if entityID != "" {
			events, err = h.calendarSvc.ListEventsForEntity(ctx, entityID, role, "")
		} else {
			events, err = h.calendarSvc.ListEventsForMonth(ctx, cal.ID, year, cal.CurrentMonth, role, "")
		}
	}

	if err != nil {
		slog.Error("api: failed to list events", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list events"))
	}

	if events == nil {
		events = []calendar.Event{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  events,
		"total": len(events),
	})
}

// GetEvent returns a single event by ID.
// GET /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) GetEvent(c echo.Context) error {
	eventID := c.Param("eventID")
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	evt, err := h.calendarSvc.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}

	// IDOR protection: verify event belongs to this campaign's calendar.
	cal, err := h.calendarSvc.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return apperror.NewNotFound("event not found")
	}

	// Visibility check: dm_only events require Owner role.
	role := h.resolveRole(c)
	if evt.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
		return apperror.NewNotFound("event not found")
	}

	return c.JSON(http.StatusOK, evt)
}

// --- Events Write ---

// apiCreateEventRequest is the JSON body for creating a calendar event via the API.
type apiCreateEventRequest struct {
	Name                     string   `json:"name"`
	Description              *string  `json:"description"`
	DescriptionHTML          *string  `json:"description_html"`
	EntityID                 *string  `json:"entity_id"`
	Year                     int      `json:"year"`
	Month                    int      `json:"month"`
	Day                      int      `json:"day"`
	StartHour                *int     `json:"start_hour"`
	StartMinute              *int     `json:"start_minute"`
	EndYear                  *int     `json:"end_year"`
	EndMonth                 *int     `json:"end_month"`
	EndDay                   *int     `json:"end_day"`
	EndHour                  *int     `json:"end_hour"`
	EndMinute                *int     `json:"end_minute"`
	IsRecurring              bool     `json:"is_recurring"`
	RecurrenceType           *string  `json:"recurrence_type"`
	RecurrenceInterval       *int     `json:"recurrence_interval"`
	RecurrenceEndYear        *int     `json:"recurrence_end_year"`
	RecurrenceEndMonth       *int     `json:"recurrence_end_month"`
	RecurrenceEndDay         *int     `json:"recurrence_end_day"`
	RecurrenceMaxOccurrences *int     `json:"recurrence_max_occurrences"`
	Visibility               string   `json:"visibility"`
	Category                 *string  `json:"category"`
	Color                    *string  `json:"color"`
	Icon                     *string  `json:"icon"`
	AllDay                   bool     `json:"all_day"`
}

// CreateEvent creates a new calendar event.
// POST /api/v1/campaigns/:id/calendar/events
func (h *CalendarAPIHandler) CreateEvent(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var req apiCreateEventRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	evt, err := h.calendarSvc.CreateEvent(ctx, cal.ID, calendar.CreateEventInput{
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
		Visibility:               req.Visibility,
		Category:                 req.Category,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
		CreatedBy:                key.UserID,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, evt)
}

// apiUpdateEventRequest is the JSON body for updating a calendar event.
type apiUpdateEventRequest struct {
	Name                     string   `json:"name"`
	Description              *string  `json:"description"`
	DescriptionHTML          *string  `json:"description_html"`
	EntityID                 *string  `json:"entity_id"`
	Year                     int      `json:"year"`
	Month                    int      `json:"month"`
	Day                      int      `json:"day"`
	StartHour                *int     `json:"start_hour"`
	StartMinute              *int     `json:"start_minute"`
	EndYear                  *int     `json:"end_year"`
	EndMonth                 *int     `json:"end_month"`
	EndDay                   *int     `json:"end_day"`
	EndHour                  *int     `json:"end_hour"`
	EndMinute                *int     `json:"end_minute"`
	IsRecurring              bool     `json:"is_recurring"`
	RecurrenceType           *string  `json:"recurrence_type"`
	RecurrenceInterval       *int     `json:"recurrence_interval"`
	RecurrenceEndYear        *int     `json:"recurrence_end_year"`
	RecurrenceEndMonth       *int     `json:"recurrence_end_month"`
	RecurrenceEndDay         *int     `json:"recurrence_end_day"`
	RecurrenceMaxOccurrences *int     `json:"recurrence_max_occurrences"`
	Visibility               string   `json:"visibility"`
	Category                 *string  `json:"category"`
	Color                    *string  `json:"color"`
	Icon                     *string  `json:"icon"`
	AllDay                   bool     `json:"all_day"`
}

// UpdateEvent updates an existing calendar event.
// PUT /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) UpdateEvent(c echo.Context) error {
	eventID := c.Param("eventID")
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	// IDOR protection: verify event belongs to this campaign's calendar.
	evt, err := h.calendarSvc.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	cal, err := h.calendarSvc.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return apperror.NewNotFound("event not found")
	}

	var req apiUpdateEventRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.UpdateEvent(ctx, eventID, calendar.UpdateEventInput{
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
		Visibility:               req.Visibility,
		Category:                 req.Category,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
	}); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEvent removes a calendar event.
// DELETE /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) DeleteEvent(c echo.Context) error {
	eventID := c.Param("eventID")
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	// IDOR protection: verify event belongs to this campaign's calendar.
	evt, err := h.calendarSvc.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	cal, err := h.calendarSvc.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return apperror.NewNotFound("event not found")
	}

	if err := h.calendarSvc.DeleteEvent(ctx, eventID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Date Management ---

// apiSetDateRequest is the JSON body for setting an absolute date/time.
type apiSetDateRequest struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// SetDate sets the calendar's current date/time to an absolute value.
// Used by external calendar modules (Calendaria, SimpleCalendar) that
// send absolute dates rather than relative day counts.
// PUT /api/v1/campaigns/:id/calendar/date
func (h *CalendarAPIHandler) SetDate(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var req apiSetDateRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetDate(ctx, cal.ID, req.Year, req.Month, req.Day, req.Hour, req.Minute); err != nil {
		return err
	}

	// Return the confirmed date.
	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"year":   req.Year,
		"month":  req.Month,
		"day":    req.Day,
		"hour":   req.Hour,
		"minute": req.Minute,
	})
}

// apiAdvanceDateRequest is the JSON body for advancing the calendar date.
type apiAdvanceDateRequest struct {
	Days int `json:"days"`
}

// AdvanceDate moves the current date forward by N days.
// POST /api/v1/campaigns/:id/calendar/advance
func (h *CalendarAPIHandler) AdvanceDate(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var req apiAdvanceDateRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if req.Days < 1 || req.Days > 3650 {
		return apperror.NewBadRequest("days must be between 1 and 3650")
	}

	if err := h.calendarSvc.AdvanceDate(ctx, cal.ID, req.Days); err != nil {
		return err
	}

	// Return the updated date.
	updatedCal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || updatedCal == nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"year":   updatedCal.CurrentYear,
		"month":  updatedCal.CurrentMonth,
		"day":    updatedCal.CurrentDay,
		"hour":   updatedCal.CurrentHour,
		"minute": updatedCal.CurrentMinute,
	})
}

// AdvanceTime moves the current time forward by hours and/or minutes.
// POST /api/v1/campaigns/:id/calendar/advance-time
func (h *CalendarAPIHandler) AdvanceTime(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var req struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if req.Hours < 0 || req.Minutes < 0 {
		return apperror.NewBadRequest("hours and minutes must be non-negative")
	}
	if req.Hours == 0 && req.Minutes == 0 {
		return apperror.NewBadRequest("must advance by at least 1 minute or 1 hour")
	}

	if err := h.calendarSvc.AdvanceTime(ctx, cal.ID, req.Hours, req.Minutes); err != nil {
		return err
	}

	// Return the updated date/time.
	updatedCal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || updatedCal == nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"year":   updatedCal.CurrentYear,
		"month":  updatedCal.CurrentMonth,
		"day":    updatedCal.CurrentDay,
		"hour":   updatedCal.CurrentHour,
		"minute": updatedCal.CurrentMinute,
	})
}

// --- Settings Write ---

// UpdateCalendarSettings updates the calendar configuration.
// PUT /api/v1/campaigns/:id/calendar/settings
func (h *CalendarAPIHandler) UpdateCalendarSettings(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var req struct {
		Name             string  `json:"name"`
		Description      *string `json:"description"`
		EpochName        *string `json:"epoch_name"`
		Mode             string  `json:"mode"`
		CurrentYear      int     `json:"current_year"`
		CurrentMonth     int     `json:"current_month"`
		CurrentDay       int     `json:"current_day"`
		CurrentHour      int     `json:"current_hour"`
		CurrentMinute    int     `json:"current_minute"`
		HoursPerDay      int     `json:"hours_per_day"`
		MinutesPerHour   int     `json:"minutes_per_hour"`
		SecondsPerMinute int     `json:"seconds_per_minute"`
		LeapYearEvery    int     `json:"leap_year_every"`
		LeapYearOffset   int     `json:"leap_year_offset"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.UpdateCalendar(ctx, cal.ID, calendar.UpdateCalendarInput{
		Name:             req.Name,
		Description:      req.Description,
		EpochName:        req.EpochName,
		Mode:             req.Mode,
		CurrentYear:      req.CurrentYear,
		CurrentMonth:     req.CurrentMonth,
		CurrentDay:       req.CurrentDay,
		CurrentHour:      req.CurrentHour,
		CurrentMinute:    req.CurrentMinute,
		HoursPerDay:      req.HoursPerDay,
		MinutesPerHour:   req.MinutesPerHour,
		SecondsPerMinute: req.SecondsPerMinute,
		LeapYearEvery:    req.LeapYearEvery,
		LeapYearOffset:   req.LeapYearOffset,
	}); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateMonths replaces all calendar months.
// PUT /api/v1/campaigns/:id/calendar/months
func (h *CalendarAPIHandler) UpdateMonths(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var months []calendar.MonthInput
	if err := c.Bind(&months); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetMonths(ctx, cal.ID, months); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateWeekdays replaces all calendar weekdays.
// PUT /api/v1/campaigns/:id/calendar/weekdays
func (h *CalendarAPIHandler) UpdateWeekdays(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var weekdays []calendar.WeekdayInput
	if err := c.Bind(&weekdays); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetWeekdays(ctx, cal.ID, weekdays); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateMoons replaces all calendar moons.
// PUT /api/v1/campaigns/:id/calendar/moons
func (h *CalendarAPIHandler) UpdateMoons(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var moons []calendar.MoonInput
	if err := c.Bind(&moons); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetMoons(ctx, cal.ID, moons); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEras replaces all calendar eras.
// PUT /api/v1/campaigns/:id/calendar/eras
func (h *CalendarAPIHandler) UpdateEras(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var eras []calendar.EraInput
	if err := c.Bind(&eras); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetEras(ctx, cal.ID, eras); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Sub-resource Write ---

// UpdateSeasons replaces all calendar seasons.
// PUT /api/v1/campaigns/:id/calendar/seasons
func (h *CalendarAPIHandler) UpdateSeasons(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var seasons []calendar.Season
	if err := c.Bind(&seasons); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetSeasons(ctx, cal.ID, seasons); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEventCategories replaces all event categories.
// PUT /api/v1/campaigns/:id/calendar/event-categories
func (h *CalendarAPIHandler) UpdateEventCategories(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var cats []calendar.EventCategoryInput
	if err := c.Bind(&cats); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetEventCategories(ctx, cal.ID, cats); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetWeather sets the current weather state.
// PUT /api/v1/campaigns/:id/calendar/weather
func (h *CalendarAPIHandler) SetWeather(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var input calendar.WeatherInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetWeather(ctx, cal.ID, input); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateCycles replaces all calendar cycles.
// PUT /api/v1/campaigns/:id/calendar/cycles
func (h *CalendarAPIHandler) UpdateCycles(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var cycles []calendar.CycleInput
	if err := c.Bind(&cycles); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetCycles(ctx, cal.ID, cycles); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateFestivals replaces all calendar festivals.
// PUT /api/v1/campaigns/:id/calendar/festivals
func (h *CalendarAPIHandler) UpdateFestivals(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var festivals []calendar.FestivalInput
	if err := c.Bind(&festivals); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.calendarSvc.SetFestivals(ctx, cal.ID, festivals); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Import/Export ---

// ExportCalendar returns the full calendar as a Chronicle JSON export.
// GET /api/v1/campaigns/:id/calendar/export
func (h *CalendarAPIHandler) ExportCalendar(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	var events []calendar.Event
	if c.QueryParam("events") == "true" {
		events, err = h.calendarSvc.ListAllEvents(ctx, cal.ID)
		if err != nil {
			slog.Error("api: failed to list events for export", slog.Any("error", err))
		}
	}

	export := calendar.BuildExport(cal, events, c.QueryParam("events") == "true")
	return c.JSON(http.StatusOK, export)
}

// ImportCalendar imports a calendar configuration from a JSON body.
// If no calendar exists for the campaign yet, one is auto-created from the
// import payload — this lets external bootstrappers (Foundry's Calendaria
// sync) populate a fresh campaign in a single request rather than a
// create-then-import dance.
//
// Mode fallback: the supported import formats (Chronicle / SimpleCalendar /
// Calendaria / Fantasy-Calendar) don't carry a calendar mode, so an
// auto-created calendar defaults to ModeFantasy. The created calendar is
// not pinned to that mode — callers can later flip it via
// PUT /calendar/settings (which now accepts `mode`).
//
// POST /api/v1/campaigns/:id/calendar/import
func (h *CalendarAPIHandler) ImportCalendar(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	data, err := io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
	if err != nil || len(data) == 0 {
		return apperror.NewBadRequest("empty request body")
	}

	result, parseErr := calendar.DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(fmt.Sprintf("parse error: %s", parseErr.Error()))
	}

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to lookup calendar for import", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to lookup calendar"))
	}
	autoCreated := false
	if cal == nil {
		// Auto-create from import payload. Mode fallback is ModeFantasy
		// because no supported import format carries a mode — the
		// CreateCalendar service applies the same default if Mode is empty,
		// but we set it explicitly here so the audit trail is unambiguous.
		newCal, createErr := h.calendarSvc.CreateCalendar(ctx, campaignID, calendar.CreateCalendarInput{
			Mode:             calendar.ModeFantasy,
			Name:             result.CalendarName,
			EpochName:        result.Settings.EpochName,
			CurrentYear:      result.Settings.CurrentYear,
			HoursPerDay:      result.Settings.HoursPerDay,
			MinutesPerHour:   result.Settings.MinutesPerHour,
			SecondsPerMinute: result.Settings.SecondsPerMinute,
			LeapYearEvery:    result.Settings.LeapYearEvery,
			LeapYearOffset:   result.Settings.LeapYearOffset,
		})
		if createErr != nil {
			slog.Error("api: failed to auto-create calendar for import", slog.Any("error", createErr))
			return createErr
		}
		cal = newCal
		autoCreated = true
	}

	if err := h.calendarSvc.ApplyImport(ctx, cal.ID, result); err != nil {
		return err
	}

	status := http.StatusOK
	if autoCreated {
		status = http.StatusCreated
	}
	return c.JSON(status, map[string]any{
		"status":       "ok",
		"format":       result.Format,
		"name":         result.CalendarName,
		"months":       len(result.Months),
		"weekdays":     len(result.Weekdays),
		"moons":        len(result.Moons),
		"seasons":      len(result.Seasons),
		"eras":         len(result.Eras),
		"auto_created": autoCreated,
	})
}

// --- Helpers ---

// resolveRole returns the API key owner's role for privacy filtering.
func (h *CalendarAPIHandler) resolveRole(c echo.Context) int {
	key := GetAPIKey(c)
	if key == nil {
		return 0
	}
	// API keys with sync permission get full visibility (owner-level).
	if key.HasPermission(PermSync) {
		return 3 // RoleOwner
	}
	// Read/write keys get player visibility.
	return 1 // RolePlayer
}
