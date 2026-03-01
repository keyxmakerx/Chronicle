package calendar

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the calendar plugin.
type Handler struct {
	svc      CalendarService
	addonSvc addons.AddonService
}

// NewHandler creates a new calendar Handler.
func NewHandler(svc CalendarService) *Handler {
	return &Handler{svc: svc}
}

// SetAddonService sets the addon service for auto-enabling the calendar addon
// when a calendar is created. Called after all plugins are wired.
func (h *Handler) SetAddonService(svc addons.AddonService) {
	h.addonSvc = svc
}

// Show renders the calendar page (monthly grid view).
// GET /campaigns/:id/calendar
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// If no calendar exists, show setup page.
	if cal == nil {
		csrfToken := middleware.GetCSRFToken(c)
		if c.Request().Header.Get("HX-Request") != "" {
			return middleware.Render(c, http.StatusOK, CalendarSetupFragment(cc, csrfToken))
		}
		return middleware.Render(c, http.StatusOK, CalendarSetupPage(cc, csrfToken))
	}

	// Parse optional year/month query params, default to current date.
	year := cal.CurrentYear
	month := cal.CurrentMonth
	if q := c.QueryParam("year"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			year = v
		}
	}
	if q := c.QueryParam("month"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= len(cal.Months) {
			month = v
		}
	}

	role := int(cc.MemberRole)
	events, err := h.svc.ListEventsForMonth(ctx, cal.ID, year, month, role)
	if err != nil {
		return err
	}

	data := CalendarViewData{
		Calendar:   cal,
		Year:       year,
		MonthIndex: month,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	if c.Request().Header.Get("HX-Request") != "" {
		return middleware.Render(c, http.StatusOK, CalendarGridFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, CalendarPage(cc, data))
}

// CreateCalendar handles calendar creation from the setup form.
// POST /campaigns/:id/calendar
func (h *Handler) CreateCalendar(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	mode := c.FormValue("mode")
	name := c.FormValue("name")
	if name == "" {
		name = "Campaign Calendar"
	}
	epochName := c.FormValue("epoch_name")
	startYear, _ := strconv.Atoi(c.FormValue("start_year"))
	if startYear == 0 {
		startYear = 1
	}

	var epoch *string
	if epochName != "" {
		epoch = &epochName
	}

	// For real-life mode, set Gregorian defaults.
	input := CreateCalendarInput{
		Mode:        mode,
		Name:        name,
		EpochName:   epoch,
		CurrentYear: startYear,
	}
	if mode == ModeRealLife {
		now := time.Now().UTC()
		input.CurrentYear = now.Year()
		input.HoursPerDay = 24
		input.MinutesPerHour = 60
		input.SecondsPerMinute = 60
		input.LeapYearEvery = 4
		input.LeapYearOffset = 0
		if name == "" || name == "Campaign Calendar" {
			input.Name = "Session Calendar"
		}
		ad := "AD"
		input.EpochName = &ad
	}

	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	// Seed months and weekdays based on mode.
	if mode == ModeRealLife {
		// Gregorian months with correct day counts.
		gregorianMonths := []MonthInput{
			{Name: "January", Days: 31, SortOrder: 0},
			{Name: "February", Days: 28, SortOrder: 1, LeapYearDays: 1},
			{Name: "March", Days: 31, SortOrder: 2},
			{Name: "April", Days: 30, SortOrder: 3},
			{Name: "May", Days: 31, SortOrder: 4},
			{Name: "June", Days: 30, SortOrder: 5},
			{Name: "July", Days: 31, SortOrder: 6},
			{Name: "August", Days: 31, SortOrder: 7},
			{Name: "September", Days: 30, SortOrder: 8},
			{Name: "October", Days: 31, SortOrder: 9},
			{Name: "November", Days: 30, SortOrder: 10},
			{Name: "December", Days: 31, SortOrder: 11},
		}
		if err := h.svc.SetMonths(ctx, cal.ID, gregorianMonths); err != nil {
			return err
		}
		gregorianWeekdays := []WeekdayInput{
			{Name: "Sunday", SortOrder: 0},
			{Name: "Monday", SortOrder: 1},
			{Name: "Tuesday", SortOrder: 2},
			{Name: "Wednesday", SortOrder: 3},
			{Name: "Thursday", SortOrder: 4},
			{Name: "Friday", SortOrder: 5},
			{Name: "Saturday", SortOrder: 6},
		}
		if err := h.svc.SetWeekdays(ctx, cal.ID, gregorianWeekdays); err != nil {
			return err
		}
		// Set current date/time from wall clock.
		now := time.Now().UTC()
		if err := h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
			Name:             cal.Name,
			EpochName:        cal.EpochName,
			CurrentYear:      now.Year(),
			CurrentMonth:     int(now.Month()),
			CurrentDay:       now.Day(),
			CurrentHour:      now.Hour(),
			CurrentMinute:    now.Minute(),
			HoursPerDay:      24,
			MinutesPerHour:   60,
			SecondsPerMinute: 60,
			LeapYearEvery:    4,
			LeapYearOffset:   0,
		}); err != nil {
			return err
		}
	} else {
		// Fantasy defaults: 12 months, 30 days each, 7 generic weekdays.
		defaultMonths := []MonthInput{
			{Name: "Month 1", Days: 30, SortOrder: 0},
			{Name: "Month 2", Days: 30, SortOrder: 1},
			{Name: "Month 3", Days: 30, SortOrder: 2},
			{Name: "Month 4", Days: 30, SortOrder: 3},
			{Name: "Month 5", Days: 30, SortOrder: 4},
			{Name: "Month 6", Days: 30, SortOrder: 5},
			{Name: "Month 7", Days: 30, SortOrder: 6},
			{Name: "Month 8", Days: 30, SortOrder: 7},
			{Name: "Month 9", Days: 30, SortOrder: 8},
			{Name: "Month 10", Days: 30, SortOrder: 9},
			{Name: "Month 11", Days: 30, SortOrder: 10},
			{Name: "Month 12", Days: 30, SortOrder: 11},
		}
		if err := h.svc.SetMonths(ctx, cal.ID, defaultMonths); err != nil {
			return err
		}
		defaultWeekdays := []WeekdayInput{
			{Name: "Day 1", SortOrder: 0},
			{Name: "Day 2", SortOrder: 1},
			{Name: "Day 3", SortOrder: 2},
			{Name: "Day 4", SortOrder: 3},
			{Name: "Day 5", SortOrder: 4},
			{Name: "Day 6", SortOrder: 5},
			{Name: "Day 7", SortOrder: 6},
		}
		if err := h.svc.SetWeekdays(ctx, cal.ID, defaultWeekdays); err != nil {
			return err
		}
	}

	// Auto-enable the calendar addon for this campaign so dashboard/entity
	// blocks render immediately without manual extension toggling.
	if h.addonSvc != nil {
		addon, aErr := h.addonSvc.GetBySlug(ctx, "calendar")
		if aErr == nil && addon != nil {
			userID := auth.GetUserID(c)
			if eErr := h.addonSvc.EnableForCampaign(ctx, cc.Campaign.ID, addon.ID, userID); eErr != nil {
				slog.Warn("auto-enable calendar addon failed", slog.Any("error", eErr))
			}
		}
	}

	// Redirect to settings for fantasy mode so users can immediately customize
	// months, weekdays, etc. Real-life mode goes straight to the calendar.
	if mode == ModeRealLife {
		return c.Redirect(http.StatusSeeOther,
			fmt.Sprintf("/campaigns/%s/calendar", cc.Campaign.ID))
	}
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendar/settings", cc.Campaign.ID))
}

// UpdateCalendarAPI updates calendar settings.
// PUT /campaigns/:id/calendar/settings
func (h *Handler) UpdateCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req struct {
		Name             string  `json:"name"`
		Description      *string `json:"description"`
		EpochName        *string `json:"epoch_name"`
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
		Name:             req.Name,
		Description:      req.Description,
		EpochName:        req.EpochName,
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
	})
}

// UpdateMonthsAPI replaces all months.
// PUT /campaigns/:id/calendar/months
func (h *Handler) UpdateMonthsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var months []MonthInput
	if err := c.Bind(&months); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.SetMonths(ctx, cal.ID, months)
}

// UpdateWeekdaysAPI replaces all weekdays.
// PUT /campaigns/:id/calendar/weekdays
func (h *Handler) UpdateWeekdaysAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var weekdays []WeekdayInput
	if err := c.Bind(&weekdays); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.SetWeekdays(ctx, cal.ID, weekdays)
}

// UpdateMoonsAPI replaces all moons.
// PUT /campaigns/:id/calendar/moons
func (h *Handler) UpdateMoonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var moons []MoonInput
	if err := c.Bind(&moons); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.SetMoons(ctx, cal.ID, moons)
}

// CreateEventAPI creates a new event.
// POST /campaigns/:id/calendar/events
func (h *Handler) CreateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		EntityID       *string `json:"entity_id"`
		Year           int     `json:"year"`
		Month          int     `json:"month"`
		Day            int     `json:"day"`
		StartHour      *int    `json:"start_hour"`
		StartMinute    *int    `json:"start_minute"`
		EndYear        *int    `json:"end_year"`
		EndMonth       *int    `json:"end_month"`
		EndDay         *int    `json:"end_day"`
		EndHour        *int    `json:"end_hour"`
		EndMinute      *int    `json:"end_minute"`
		IsRecurring    bool    `json:"is_recurring"`
		RecurrenceType *string `json:"recurrence_type"`
		Visibility     string  `json:"visibility"`
		Category       *string `json:"category"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Get user ID from session context.
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}

	evt, err := h.svc.CreateEvent(ctx, cal.ID, CreateEventInput{
		Name:           req.Name,
		Description:    req.Description,
		EntityID:       req.EntityID,
		Year:           req.Year,
		Month:          req.Month,
		Day:            req.Day,
		StartHour:      req.StartHour,
		StartMinute:    req.StartMinute,
		EndYear:        req.EndYear,
		EndMonth:       req.EndMonth,
		EndDay:         req.EndDay,
		EndHour:        req.EndHour,
		EndMinute:      req.EndMinute,
		IsRecurring:    req.IsRecurring,
		RecurrenceType: req.RecurrenceType,
		Visibility:     req.Visibility,
		Category:       req.Category,
		CreatedBy:      userID,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, evt)
}

// requireEventInCampaign fetches an event and verifies its calendar belongs to
// the given campaign. Returns 404 for cross-campaign IDOR attempts.
func (h *Handler) requireEventInCampaign(c echo.Context, eventID, campaignID string) (*Event, error) {
	ctx := c.Request().Context()
	evt, err := h.svc.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	// Verify event's calendar belongs to this campaign.
	cal, err := h.svc.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "event not found")
	}
	return evt, nil
}

// UpdateEventAPI updates an existing event.
// PUT /campaigns/:id/calendar/events/:eid
func (h *Handler) UpdateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		EntityID       *string `json:"entity_id"`
		Year           int     `json:"year"`
		Month          int     `json:"month"`
		Day            int     `json:"day"`
		StartHour      *int    `json:"start_hour"`
		StartMinute    *int    `json:"start_minute"`
		EndYear        *int    `json:"end_year"`
		EndMonth       *int    `json:"end_month"`
		EndDay         *int    `json:"end_day"`
		EndHour        *int    `json:"end_hour"`
		EndMinute      *int    `json:"end_minute"`
		IsRecurring    bool    `json:"is_recurring"`
		RecurrenceType *string `json:"recurrence_type"`
		Visibility     string  `json:"visibility"`
		Category       *string `json:"category"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.UpdateEvent(ctx, eventID, UpdateEventInput{
		Name:           req.Name,
		Description:    req.Description,
		EntityID:       req.EntityID,
		Year:           req.Year,
		Month:          req.Month,
		Day:            req.Day,
		StartHour:      req.StartHour,
		StartMinute:    req.StartMinute,
		EndYear:        req.EndYear,
		EndMonth:       req.EndMonth,
		EndDay:         req.EndDay,
		EndHour:        req.EndHour,
		EndMinute:      req.EndMinute,
		IsRecurring:    req.IsRecurring,
		RecurrenceType: req.RecurrenceType,
		Visibility:     req.Visibility,
		Category:       req.Category,
	})
}

// DeleteEventAPI deletes an event.
// DELETE /campaigns/:id/calendar/events/:eid
func (h *Handler) DeleteEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteEvent(ctx, eventID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// UpdateSeasonsAPI replaces all seasons.
// PUT /campaigns/:id/calendar/seasons
func (h *Handler) UpdateSeasonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var seasons []Season
	if err := c.Bind(&seasons); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.SetSeasons(ctx, cal.ID, seasons)
}

// UpdateErasAPI replaces all eras.
// PUT /campaigns/:id/calendar/eras
func (h *Handler) UpdateErasAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var eras []EraInput
	if err := c.Bind(&eras); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.SetEras(ctx, cal.ID, eras)
}

// DeleteCalendarAPI removes the calendar and all its data.
// DELETE /campaigns/:id/calendar
func (h *Handler) DeleteCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	if err := h.svc.DeleteCalendar(ctx, cal.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ShowSettings renders the calendar settings page.
// GET /campaigns/:id/calendar/settings
func (h *Handler) ShowSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if cal == nil {
		return c.Redirect(http.StatusSeeOther,
			fmt.Sprintf("/campaigns/%s/calendar", cc.Campaign.ID))
	}

	csrfToken := middleware.GetCSRFToken(c)
	if c.Request().Header.Get("HX-Request") != "" {
		return middleware.Render(c, http.StatusOK, CalendarSettingsFragment(cc, cal, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, CalendarSettingsPage(cc, cal, csrfToken))
}

// AdvanceDateAPI moves the current date forward by N days.
// POST /campaigns/:id/calendar/advance
func (h *Handler) AdvanceDateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req struct {
		Days int `json:"days"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Days < 1 || req.Days > 3650 {
		return echo.NewHTTPError(http.StatusBadRequest, "days must be between 1 and 3650")
	}

	return h.svc.AdvanceDate(ctx, cal.ID, req.Days)
}

// AdvanceTimeAPI moves the current time forward by hours and/or minutes,
// rolling over into days as needed.
// POST /campaigns/:id/calendar/advance-time
func (h *Handler) AdvanceTimeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	var req struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Hours < 0 || req.Minutes < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "hours and minutes must be non-negative")
	}
	if req.Hours == 0 && req.Minutes == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "must advance by at least 1 minute or 1 hour")
	}
	if req.Hours > 87600 { // ~10 years of 24-hour days
		return echo.NewHTTPError(http.StatusBadRequest, "hours must be at most 87600")
	}

	return h.svc.AdvanceTime(ctx, cal.ID, req.Hours, req.Minutes)
}

// EntityEventsFragment returns a small HTMX fragment listing calendar events
// linked to a specific entity. Loaded lazily from entity show pages.
// GET /campaigns/:id/calendar/entity-events/:eid
func (h *Handler) EntityEventsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	entityID := c.Param("eid")

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if cal == nil {
		// No calendar = no events section.
		return c.NoContent(http.StatusOK)
	}

	role := int(cc.MemberRole)
	events, err := h.svc.ListEventsForEntity(ctx, entityID, role)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return c.NoContent(http.StatusOK)
	}

	return middleware.Render(c, http.StatusOK, EntityEventsSection(cc, cal, events))
}

// UpcomingEventsFragment returns an HTMX fragment with upcoming calendar events.
// Used by the calendar_preview dashboard block via lazy-loading.
// GET /campaigns/:id/calendar/upcoming
func (h *Handler) UpcomingEventsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if cal == nil {
		return middleware.Render(c, http.StatusOK, UpcomingEventsEmpty())
	}

	limit := 5
	if q := c.QueryParam("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= 20 {
			limit = v
		}
	}

	role := int(cc.MemberRole)
	events, err := h.svc.ListUpcomingEvents(ctx, cal.ID, limit, role)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, UpcomingEventsBlock(cc, cal, events))
}

// ShowTimeline renders the timeline (list) view of calendar events.
// GET /campaigns/:id/calendar/timeline
func (h *Handler) ShowTimeline(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	if cal == nil {
		csrfToken := middleware.GetCSRFToken(c)
		if c.Request().Header.Get("HX-Request") != "" {
			return middleware.Render(c, http.StatusOK, CalendarSetupFragment(cc, csrfToken))
		}
		return middleware.Render(c, http.StatusOK, CalendarSetupPage(cc, csrfToken))
	}

	// Default to current year, allow override via query param.
	year := cal.CurrentYear
	if q := c.QueryParam("year"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			year = v
		}
	}

	role := int(cc.MemberRole)
	events, err := h.svc.ListEventsForYear(ctx, cal.ID, year, role)
	if err != nil {
		return err
	}

	data := TimelineViewData{
		Calendar:   cal,
		Year:       year,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	if c.Request().Header.Get("HX-Request") != "" {
		return middleware.Render(c, http.StatusOK, TimelineFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, TimelinePage(cc, data))
}

// ExportCalendarAPI returns the calendar as a downloadable JSON file.
// GET /campaigns/:id/calendar/export
func (h *Handler) ExportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil || cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found")
	}

	// Optionally include events.
	var events []Event
	includeEvents := c.QueryParam("events") == "true"
	if includeEvents {
		events, err = h.svc.ListAllEvents(ctx, cal.ID)
		if err != nil {
			slog.Error("export: failed to list events", slog.Any("error", err))
		}
	}

	export := BuildExport(cal, events, includeEvents)
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-calendar.json"`, cc.Campaign.Slug))
	return c.JSON(http.StatusOK, export)
}

// ImportCalendarAPI handles calendar import from an uploaded JSON file.
// Accepts Simple Calendar, Calendaria, Fantasy-Calendar, and Chronicle formats.
// POST /campaigns/:id/calendar/import
func (h *Handler) ImportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cal, err := h.svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if cal == nil {
		return echo.NewHTTPError(http.StatusNotFound, "calendar not found — create a calendar first")
	}

	// Read uploaded file (multipart form or raw JSON body).
	var data []byte
	file, fileErr := c.FormFile("file")
	if fileErr == nil {
		// Multipart upload.
		src, openErr := file.Open()
		if openErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
		}
		defer src.Close()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024)) // 10MB limit
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
		}
	} else {
		// Try raw JSON body.
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no file uploaded and no JSON body")
		}
	}

	// Parse and detect format.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, parseErr.Error())
	}

	// Check for preview mode — return what would be imported without applying.
	if c.QueryParam("preview") == "true" {
		return c.JSON(http.StatusOK, result)
	}

	// Apply the import to the existing calendar.
	if err := h.svc.ApplyImport(ctx, cal.ID, result); err != nil {
		slog.Error("import: failed to apply", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to apply import")
	}

	// Return JSON response with summary.
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"format":   result.Format,
		"name":     result.CalendarName,
		"months":   len(result.Months),
		"weekdays": len(result.Weekdays),
		"moons":    len(result.Moons),
		"seasons":  len(result.Seasons),
		"eras":     len(result.Eras),
	})
}

// ImportPreviewAPI returns a preview of what would be imported from a JSON file
// without actually applying the changes. Used by the import UI for confirmation.
// POST /campaigns/:id/calendar/import/preview
func (h *Handler) ImportPreviewAPI(c echo.Context) error {
	// Read uploaded file.
	var data []byte
	var err error
	file, fileErr := c.FormFile("file")
	if fileErr == nil {
		src, openErr := file.Open()
		if openErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
		}
		defer src.Close()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
		}
	} else {
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no file uploaded")
		}
	}

	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, parseErr.Error())
	}

	// Return the parsed preview as JSON.
	return c.JSON(http.StatusOK, result)
}

// ImportFromSetupAPI handles import during calendar setup (no existing calendar).
// Creates a new calendar and applies the imported configuration.
// POST /campaigns/:id/calendar/import-setup
func (h *Handler) ImportFromSetupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// Read uploaded file.
	file, fileErr := c.FormFile("file")
	if fileErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "no file uploaded")
	}
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 10*1024*1024))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read uploaded file")
	}

	// Parse the import.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, parseErr.Error())
	}

	// Create a new fantasy calendar with the imported name.
	calName := result.CalendarName
	if calName == "" {
		calName = "Imported Calendar"
	}
	input := CreateCalendarInput{
		Mode:             ModeFantasy,
		Name:             calName,
		EpochName:        result.Settings.EpochName,
		CurrentYear:      result.Settings.CurrentYear,
		HoursPerDay:      result.Settings.HoursPerDay,
		MinutesPerHour:   result.Settings.MinutesPerHour,
		SecondsPerMinute: result.Settings.SecondsPerMinute,
		LeapYearEvery:    result.Settings.LeapYearEvery,
		LeapYearOffset:   result.Settings.LeapYearOffset,
	}

	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	// Apply imported sub-resources.
	if err := h.svc.ApplyImport(ctx, cal.ID, result); err != nil {
		slog.Error("import-setup: failed to apply", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to apply import")
	}

	// Auto-enable the calendar addon.
	if h.addonSvc != nil {
		addon, aErr := h.addonSvc.GetBySlug(ctx, "calendar")
		if aErr == nil && addon != nil {
			userID := auth.GetUserID(c)
			if eErr := h.addonSvc.EnableForCampaign(ctx, cc.Campaign.ID, addon.ID, userID); eErr != nil {
				slog.Warn("auto-enable calendar addon failed", slog.Any("error", eErr))
			}
		}
	}

	// Redirect to settings page so user can review the import.
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendar/settings", cc.Campaign.ID))
}

// Silence unused import warnings for json and io packages.
var _ = json.Marshal
var _ = io.ReadAll

// CalendarViewData holds all data needed to render the calendar grid.
type CalendarViewData struct {
	Calendar   *Calendar
	Year       int
	MonthIndex int // 1-based month index
	Events     []Event
	CampaignID string
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

// CurrentMonthDef returns the month definition for the current view month.
func (d CalendarViewData) CurrentMonthDef() *Month {
	idx := d.MonthIndex - 1
	if idx >= 0 && idx < len(d.Calendar.Months) {
		return &d.Calendar.Months[idx]
	}
	return nil
}

// CurrentMonthDays returns the number of days in the current month,
// accounting for leap years.
func (d CalendarViewData) CurrentMonthDays() int {
	return d.Calendar.MonthDays(d.MonthIndex-1, d.Year)
}

// CurrentSeason returns the season for a given day in the current month, or nil.
func (d CalendarViewData) CurrentSeason(day int) *Season {
	return d.Calendar.SeasonForDate(d.MonthIndex, day)
}

// PrevMonth returns year, month for the previous month (wrapping at year boundary).
func (d CalendarViewData) PrevMonth() (int, int) {
	m := d.MonthIndex - 1
	y := d.Year
	if m < 1 {
		m = len(d.Calendar.Months)
		y--
	}
	return y, m
}

// NextMonth returns year, month for the next month (wrapping at year boundary).
func (d CalendarViewData) NextMonth() (int, int) {
	m := d.MonthIndex + 1
	y := d.Year
	if m > len(d.Calendar.Months) {
		m = 1
		y++
	}
	return y, m
}

// EventsForDay returns events that fall on the given day.
func (d CalendarViewData) EventsForDay(day int) []Event {
	var result []Event
	for _, e := range d.Events {
		if e.Day == day {
			result = append(result, e)
		}
	}
	return result
}

// IsToday returns true if the given day/month/year matches the calendar's current date.
func (d CalendarViewData) IsToday(day int) bool {
	return d.Year == d.Calendar.CurrentYear &&
		d.MonthIndex == d.Calendar.CurrentMonth &&
		day == d.Calendar.CurrentDay
}

// AbsoluteDay calculates the total days from year 0 for moon phase computation.
func (d CalendarViewData) AbsoluteDay(day int) int {
	yearLength := d.Calendar.YearLength()
	total := d.Year * yearLength
	// Add days from months before current month.
	for i := 0; i < d.MonthIndex-1 && i < len(d.Calendar.Months); i++ {
		total += d.Calendar.Months[i].Days
	}
	total += day
	return total
}

// WeekdayIndex returns the weekday index (0-based) for a given day in the current month/year.
func (d CalendarViewData) WeekdayIndex(day int) int {
	wl := d.Calendar.WeekLength()
	if wl == 0 {
		return 0
	}
	absDay := d.AbsoluteDay(day)
	idx := absDay % wl
	if idx < 0 {
		idx += wl
	}
	return idx
}

// StartWeekdayOffset returns how many blank cells to render before day 1
// of the current month in the grid.
func (d CalendarViewData) StartWeekdayOffset() int {
	return d.WeekdayIndex(1)
}

// TimelineViewData holds data for the chronological timeline view.
type TimelineViewData struct {
	Calendar   *Calendar
	Year       int
	Events     []Event
	CampaignID string
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

// MonthName returns the month name for a 1-based month index.
func (d TimelineViewData) MonthName(month int) string {
	if month >= 1 && month <= len(d.Calendar.Months) {
		return d.Calendar.Months[month-1].Name
	}
	return fmt.Sprintf("Month %d", month)
}

// EventsByMonth groups events by their month index for timeline rendering.
func (d TimelineViewData) EventsByMonth() []TimelineMonth {
	monthMap := make(map[int][]Event)
	for _, evt := range d.Events {
		monthMap[evt.Month] = append(monthMap[evt.Month], evt)
	}
	// Produce ordered slice.
	var result []TimelineMonth
	for m := 1; m <= len(d.Calendar.Months); m++ {
		if events, ok := monthMap[m]; ok {
			result = append(result, TimelineMonth{
				Index:  m,
				Name:   d.Calendar.Months[m-1].Name,
				Events: events,
			})
		}
	}
	return result
}

// TimelineMonth groups events under a month header for timeline display.
type TimelineMonth struct {
	Index  int
	Name   string
	Events []Event
}
