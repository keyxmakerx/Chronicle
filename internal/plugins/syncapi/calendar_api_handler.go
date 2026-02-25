package syncapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// GetCalendar returns the full calendar structure for a campaign.
// GET /api/v1/campaigns/:id/calendar
func (h *CalendarAPIHandler) GetCalendar(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to get calendar", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get calendar")
	}
	if cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no calendar configured for this campaign")
	}

	return c.JSON(http.StatusOK, cal)
}

// GetCurrentDate returns only the current in-game date.
// GET /api/v1/campaigns/:id/calendar/date
func (h *CalendarAPIHandler) GetCurrentDate(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"year":  cal.CurrentYear,
		"month": cal.CurrentMonth,
		"day":   cal.CurrentDay,
	})
}

// --- Events Read ---

// ListEvents returns events for a month or year.
// GET /api/v1/campaigns/:id/calendar/events?year=N&month=M
func (h *CalendarAPIHandler) ListEvents(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	role := h.resolveRole(c)
	year, _ := strconv.Atoi(c.QueryParam("year"))
	month, _ := strconv.Atoi(c.QueryParam("month"))

	if year == 0 {
		year = cal.CurrentYear
	}

	var events []calendar.Event

	if month > 0 {
		events, err = h.calendarSvc.ListEventsForMonth(ctx, cal.ID, year, month, role)
	} else {
		// No month specified — return events for the entity if entity_id is provided.
		entityID := c.QueryParam("entity_id")
		if entityID != "" {
			events, err = h.calendarSvc.ListEventsForEntity(ctx, entityID, role)
		} else {
			events, err = h.calendarSvc.ListEventsForMonth(ctx, cal.ID, year, cal.CurrentMonth, role)
		}
	}

	if err != nil {
		slog.Error("api: failed to list events", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list events")
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
	ctx := c.Request().Context()

	evt, err := h.calendarSvc.GetEvent(ctx, eventID)
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.JSON(http.StatusOK, evt)
}

// --- Events Write ---

// apiCreateEventRequest is the JSON body for creating a calendar event via the API.
type apiCreateEventRequest struct {
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	EntityID       *string `json:"entity_id"`
	Year           int     `json:"year"`
	Month          int     `json:"month"`
	Day            int     `json:"day"`
	EndYear        *int    `json:"end_year"`
	EndMonth       *int    `json:"end_month"`
	EndDay         *int    `json:"end_day"`
	IsRecurring    bool    `json:"is_recurring"`
	RecurrenceType *string `json:"recurrence_type"`
	Visibility     string  `json:"visibility"`
	Category       *string `json:"category"`
}

// CreateEvent creates a new calendar event.
// POST /api/v1/campaigns/:id/calendar/events
func (h *CalendarAPIHandler) CreateEvent(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "api key required")
	}

	campaignID := c.Param("id")
	ctx := c.Request().Context()

	cal, err := h.calendarSvc.GetCalendar(ctx, campaignID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req apiCreateEventRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	evt, err := h.calendarSvc.CreateEvent(ctx, cal.ID, calendar.CreateEventInput{
		Name:           req.Name,
		Description:    req.Description,
		EntityID:       req.EntityID,
		Year:           req.Year,
		Month:          req.Month,
		Day:            req.Day,
		EndYear:        req.EndYear,
		EndMonth:       req.EndMonth,
		EndDay:         req.EndDay,
		IsRecurring:    req.IsRecurring,
		RecurrenceType: req.RecurrenceType,
		Visibility:     req.Visibility,
		Category:       req.Category,
		CreatedBy:      key.UserID,
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.JSON(http.StatusCreated, evt)
}

// apiUpdateEventRequest is the JSON body for updating a calendar event.
type apiUpdateEventRequest struct {
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	EntityID       *string `json:"entity_id"`
	Year           int     `json:"year"`
	Month          int     `json:"month"`
	Day            int     `json:"day"`
	EndYear        *int    `json:"end_year"`
	EndMonth       *int    `json:"end_month"`
	EndDay         *int    `json:"end_day"`
	IsRecurring    bool    `json:"is_recurring"`
	RecurrenceType *string `json:"recurrence_type"`
	Visibility     string  `json:"visibility"`
	Category       *string `json:"category"`
}

// UpdateEvent updates an existing calendar event.
// PUT /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) UpdateEvent(c echo.Context) error {
	eventID := c.Param("eventID")
	ctx := c.Request().Context()

	var req apiUpdateEventRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.calendarSvc.UpdateEvent(ctx, eventID, calendar.UpdateEventInput{
		Name:           req.Name,
		Description:    req.Description,
		EntityID:       req.EntityID,
		Year:           req.Year,
		Month:          req.Month,
		Day:            req.Day,
		EndYear:        req.EndYear,
		EndMonth:       req.EndMonth,
		EndDay:         req.EndDay,
		IsRecurring:    req.IsRecurring,
		RecurrenceType: req.RecurrenceType,
		Visibility:     req.Visibility,
		Category:       req.Category,
	}); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEvent removes a calendar event.
// DELETE /api/v1/campaigns/:id/calendar/events/:eventID
func (h *CalendarAPIHandler) DeleteEvent(c echo.Context) error {
	eventID := c.Param("eventID")
	ctx := c.Request().Context()

	if err := h.calendarSvc.DeleteEvent(ctx, eventID); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Date Management ---

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
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req apiAdvanceDateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Days < 1 || req.Days > 3650 {
		return echo.NewHTTPError(http.StatusBadRequest, "days must be between 1 and 3650")
	}

	if err := h.calendarSvc.AdvanceDate(ctx, cal.ID, req.Days); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		EpochName      *string `json:"epoch_name"`
		CurrentYear    int     `json:"current_year"`
		CurrentMonth   int     `json:"current_month"`
		CurrentDay     int     `json:"current_day"`
		LeapYearEvery  int     `json:"leap_year_every"`
		LeapYearOffset int     `json:"leap_year_offset"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.calendarSvc.UpdateCalendar(ctx, cal.ID, calendar.UpdateCalendarInput{
		Name:           req.Name,
		Description:    req.Description,
		EpochName:      req.EpochName,
		CurrentYear:    req.CurrentYear,
		CurrentMonth:   req.CurrentMonth,
		CurrentDay:     req.CurrentDay,
		LeapYearEvery:  req.LeapYearEvery,
		LeapYearOffset: req.LeapYearOffset,
	}); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var months []calendar.MonthInput
	if err := c.Bind(&months); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.calendarSvc.SetMonths(ctx, cal.ID, months); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var weekdays []calendar.WeekdayInput
	if err := c.Bind(&weekdays); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.calendarSvc.SetWeekdays(ctx, cal.ID, weekdays); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var moons []calendar.MoonInput
	if err := c.Bind(&moons); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.calendarSvc.SetMoons(ctx, cal.ID, moons); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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
