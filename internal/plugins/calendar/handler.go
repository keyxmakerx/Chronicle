package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SessionLister provides session data for the calendar grid display.
// Implemented by the sessions service. Defined here to avoid import cycles.
type SessionLister interface {
	ListSessionsForDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]CalendarSession, error)
	// ListAllSessions returns all planned sessions for a campaign (for the sessions modal).
	ListAllSessions(ctx context.Context, campaignID, userID string) ([]CalendarSession, error)
}

// CalendarSession is a lightweight session representation for calendar display.
// Avoids importing the sessions package directly.
type CalendarSession struct {
	ID            string
	Name          string
	ScheduledDate string // YYYY-MM-DD
	Status        string
	IsRecurring   bool
	RecurrenceLabel string
	AcceptedCount int
	TotalCount    int
	UserRSVP      string // Current user's RSVP status (empty if not attendee).
}

// Handler processes HTTP requests for the calendar plugin.
type Handler struct {
	svc           CalendarService
	addonSvc      addons.AddonService
	sessionLister SessionLister
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

// SetSessionLister wires the session service for calendar grid display.
func (h *Handler) SetSessionLister(sl SessionLister) {
	h.sessionLister = sl
}

// requireCalendarInCampaign fetches a calendar by ID and verifies it belongs
// to the given campaign. Returns 404 if not found or mismatched, preventing
// cross-campaign IDOR attacks.
func (h *Handler) requireCalendarInCampaign(c echo.Context, calendarID, campaignID string) (*Calendar, error) {
	return middleware.RequireInCampaign(c.Request().Context(), h.svc.GetCalendarByID, calendarID, campaignID, "calendar")
}

// Index lists all calendars for a campaign. If none exist, shows the setup page.
// If exactly one exists, redirects to that calendar's detail view.
// GET /campaigns/:id/calendars
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cals, err := h.svc.ListCalendars(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// No calendars: show setup page.
	if len(cals) == 0 {
		csrfToken := middleware.GetCSRFToken(c)
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, CalendarSetupFragment(cc, csrfToken))
		}
		return middleware.Render(c, http.StatusOK, CalendarSetupPage(cc, csrfToken))
	}

	// Single calendar: redirect directly to it.
	if len(cals) == 1 {
		return c.Redirect(http.StatusSeeOther,
			fmt.Sprintf("/campaigns/%s/calendars/%s", cc.Campaign.ID, cals[0].ID))
	}

	// Multiple calendars: show list page.
	isOwner := cc.MemberRole >= campaigns.RoleOwner
	csrfToken := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarListFragment(cc, cals, isOwner, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, CalendarListPage(cc, cals, isOwner, csrfToken))
}

// Show renders the calendar page (monthly grid view).
// GET /campaigns/:id/calendars/:calId
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
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
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForMonth(ctx, cal.ID, year, month, role, userID)
	if err != nil {
		return err
	}

	data := CalendarViewData{
		Calendar:   cal,
		Year:       year,
		MonthIndex: month,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	// For real-life calendars, fetch sessions that fall in this month.
	if cal.Mode == "reallife" && h.sessionLister != nil {
		startDate := fmt.Sprintf("%04d-%02d-01", year, month)
		endDate := fmt.Sprintf("%04d-%02d-%02d", year, month, daysInGregorianMonth(year, month))
		sessions, err := h.sessionLister.ListSessionsForDateRange(ctx, cc.Campaign.ID, startDate, endDate)
		if err == nil {
			data.Sessions = sessions
		}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarGridFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, CalendarPage(cc, data))
}

// EmbedCalendar returns a compact calendar grid fragment for dashboard embedding.
// Accepts calId as a route param or query param for dashboard block config.
// GET /campaigns/:id/calendars/:calId/embed
func (h *Handler) EmbedCalendar(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// Support both route param and query param for backward compat with dashboard blocks.
	calID := c.Param("calId")
	if calID == "" {
		calID = c.QueryParam("calendarId")
	}

	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return middleware.Render(c, http.StatusOK, CalendarEmbedEmpty(cc))
		}
	} else {
		// No calendar specified: use the default/first calendar.
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
	}

	// No calendar exists — return a setup prompt.
	if cal == nil {
		return middleware.Render(c, http.StatusOK, CalendarEmbedEmpty(cc))
	}

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
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForMonth(ctx, cal.ID, year, month, role, userID)
	if err != nil {
		return err
	}

	data := CalendarViewData{
		Calendar:   cal,
		Year:       year,
		MonthIndex: month,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	return middleware.Render(c, http.StatusOK, CalendarEmbedFragment(cc, data))
}

// ShowWeek renders the calendar week view.
// GET /campaigns/:id/calendars/:calId/week
func (h *Handler) ShowWeek(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Parse year/month/day query params, default to current date.
	year := cal.CurrentYear
	month := cal.CurrentMonth
	day := cal.CurrentDay
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
	if q := c.QueryParam("day"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 {
			day = v
		}
	}

	// Snap day to week start (find the most recent weekday 0).
	weekLen := cal.WeekLength()
	if weekLen == 0 {
		weekLen = 7
	}
	// Calculate absolute day and find the week start.
	yearLength := cal.YearLength()
	absDay := year*yearLength
	for i := 0; i < month-1 && i < len(cal.Months); i++ {
		absDay += cal.Months[i].Days
		if cal.IsLeapYear(year) {
			absDay += cal.Months[i].LeapYearDays
		}
	}
	absDay += day
	weekdayIdx := absDay % weekLen
	if weekdayIdx < 0 {
		weekdayIdx += weekLen
	}
	// Move back to weekday 0.
	startAbsDay := absDay - weekdayIdx

	// Convert absolute day back to year/month/day.
	startYear := year
	startMonth := month
	startDay := day - weekdayIdx
	// Walk backwards if we went before day 1.
	for startDay < 1 {
		startMonth--
		if startMonth < 1 {
			startMonth = len(cal.Months)
			startYear--
		}
		startDay += cal.MonthDays(startMonth-1, startYear)
	}

	// Calculate end date for the week.
	endYear := startYear
	endMonth := startMonth
	endDay := startDay + weekLen - 1
	monthDays := cal.MonthDays(endMonth-1, endYear)
	for endDay > monthDays {
		endDay -= monthDays
		endMonth++
		if endMonth > len(cal.Months) {
			endMonth = 1
			endYear++
		}
		monthDays = cal.MonthDays(endMonth-1, endYear)
	}

	role := int(cc.MemberRole)
	userID := auth.GetUserID(c)

	// Fetch events for the date range. If range crosses months, fetch both.
	var events []Event
	if startMonth == endMonth && startYear == endYear {
		events, err = h.svc.ListEventsForDateRange(ctx, cal.ID, startYear, startMonth, startDay, endMonth, endDay, role, userID)
	} else {
		// Cross-month week: fetch from startMonth to end of start month, and from month 1 to endDay of endMonth.
		events1, err1 := h.svc.ListEventsForDateRange(ctx, cal.ID, startYear, startMonth, startDay, startMonth, cal.MonthDays(startMonth-1, startYear), role, userID)
		if err1 != nil {
			return err1
		}
		events2, err2 := h.svc.ListEventsForDateRange(ctx, cal.ID, endYear, endMonth, 1, endMonth, endDay, role, userID)
		if err2 != nil {
			return err2
		}
		events = append(events1, events2...)
	}
	if err != nil {
		return err
	}

	_ = startAbsDay // used for calculation only

	data := WeekViewData{
		Calendar:   cal,
		Year:       startYear,
		MonthIndex: startMonth,
		StartDay:   startDay,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	// For real-life calendars, fetch sessions.
	if cal.Mode == "reallife" && h.sessionLister != nil {
		startDate := fmt.Sprintf("%04d-%02d-%02d", startYear, startMonth, startDay)
		endDate := fmt.Sprintf("%04d-%02d-%02d", endYear, endMonth, endDay)
		sessions, err := h.sessionLister.ListSessionsForDateRange(ctx, cc.Campaign.ID, startDate, endDate)
		if err == nil {
			data.Sessions = sessions
		}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, WeekFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, WeekPage(cc, data))
}

// ShowDay renders the calendar day view for a single day.
// GET /campaigns/:id/calendars/:calId/day
func (h *Handler) ShowDay(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Parse year/month/day query params, default to current date.
	year := cal.CurrentYear
	month := cal.CurrentMonth
	day := cal.CurrentDay
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
	if q := c.QueryParam("day"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 {
			day = v
		}
	}

	// Clamp day to valid range for the month.
	maxDays := cal.MonthDays(month-1, year)
	if day > maxDays {
		day = maxDays
	}

	role := int(cc.MemberRole)
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForDateRange(ctx, cal.ID, year, month, day, month, day, role, userID)
	if err != nil {
		return err
	}

	data := DayViewData{
		Calendar:   cal,
		Year:       year,
		MonthIndex: month,
		Day:        day,
		Events:     events,
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	// For real-life calendars, fetch sessions for this day.
	if cal.Mode == "reallife" && h.sessionLister != nil {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		sessions, err := h.sessionLister.ListSessionsForDateRange(ctx, cc.Campaign.ID, dateStr, dateStr)
		if err == nil {
			data.Sessions = sessions
		}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, DayFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, DayPage(cc, data))
}

// SessionsFragment returns the sessions modal content as an HTMX fragment.
// Used to refresh the session list inside the calendar's sessions overlay.
// GET /campaigns/:id/calendars/:calId/sessions-fragment
func (h *Handler) SessionsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	data := CalendarViewData{
		Calendar:   cal,
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	// Fetch all planned sessions for the campaign.
	if h.sessionLister != nil {
		sessions, err := h.sessionLister.ListAllSessions(ctx, cc.Campaign.ID, userID)
		if err == nil {
			data.Sessions = sessions
		}
	}

	return middleware.Render(c, http.StatusOK, sessionsModalContent(data))
}

// CreateCalendar handles calendar creation from the setup form.
// POST /campaigns/:id/calendars
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

	// Service handles mode-specific defaults and seeds months/weekdays.
	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, CreateCalendarInput{
		Mode:        mode,
		Name:        name,
		EpochName:   epoch,
		CurrentYear: startYear,
	})
	if err != nil {
		return err
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
			fmt.Sprintf("/campaigns/%s/calendars/%s", cc.Campaign.ID, cal.ID))
	}
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendars/%s/settings", cc.Campaign.ID, cal.ID))
}

// UpdateCalendarAPI updates calendar settings.
// PUT /campaigns/:id/calendars/:calId/settings
func (h *Handler) UpdateCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
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
		return apperror.NewBadRequest("invalid request")
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
// PUT /campaigns/:id/calendars/:calId/months
func (h *Handler) UpdateMonthsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var months []MonthInput
	if err := c.Bind(&months); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetMonths(ctx, cal.ID, months)
}

// UpdateWeekdaysAPI replaces all weekdays.
// PUT /campaigns/:id/calendars/:calId/weekdays
func (h *Handler) UpdateWeekdaysAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var weekdays []WeekdayInput
	if err := c.Bind(&weekdays); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetWeekdays(ctx, cal.ID, weekdays)
}

// UpdateMoonsAPI replaces all moons.
// PUT /campaigns/:id/calendars/:calId/moons
func (h *Handler) UpdateMoonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var moons []MoonInput
	if err := c.Bind(&moons); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetMoons(ctx, cal.ID, moons)
}

// CreateEventAPI creates a new event.
// POST /campaigns/:id/calendars/:calId/events
func (h *Handler) CreateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return err
		}
	} else {
		// No calendar specified: use the default calendar (for dashboard widget quick-add).
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
		if cal == nil {
			return apperror.NewNotFound("no default calendar found")
		}
	}

	var req struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		DescriptionHTML *string `json:"description_html"`
		EntityID        *string `json:"entity_id"`
		Year            int     `json:"year"`
		Month           int     `json:"month"`
		Day             int     `json:"day"`
		StartHour       *int    `json:"start_hour"`
		StartMinute     *int    `json:"start_minute"`
		EndYear         *int    `json:"end_year"`
		EndMonth        *int    `json:"end_month"`
		EndDay          *int    `json:"end_day"`
		EndHour         *int    `json:"end_hour"`
		EndMinute       *int    `json:"end_minute"`
		IsRecurring     bool    `json:"is_recurring"`
		RecurrenceType  *string `json:"recurrence_type"`
		Visibility      string  `json:"visibility"`
		VisibilityRules *string `json:"visibility_rules"`
		Category        *string `json:"category"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Get user ID from session context.
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}

	evt, err := h.svc.CreateEvent(ctx, cal.ID, CreateEventInput{
		Name:            req.Name,
		Description:     req.Description,
		DescriptionHTML: req.DescriptionHTML,
		EntityID:        req.EntityID,
		Year:            req.Year,
		Month:           req.Month,
		Day:             req.Day,
		StartHour:       req.StartHour,
		StartMinute:     req.StartMinute,
		EndYear:         req.EndYear,
		EndMonth:        req.EndMonth,
		EndDay:          req.EndDay,
		EndHour:         req.EndHour,
		EndMinute:       req.EndMinute,
		IsRecurring:     req.IsRecurring,
		RecurrenceType:  req.RecurrenceType,
		Visibility:      req.Visibility,
		VisibilityRules: req.VisibilityRules,
		Category:        req.Category,
		CreatedBy:       userID,
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
		return nil, apperror.NewNotFound("event not found")
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
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		DescriptionHTML *string `json:"description_html"`
		EntityID        *string `json:"entity_id"`
		Year            int     `json:"year"`
		Month           int     `json:"month"`
		Day             int     `json:"day"`
		StartHour       *int    `json:"start_hour"`
		StartMinute     *int    `json:"start_minute"`
		EndYear         *int    `json:"end_year"`
		EndMonth        *int    `json:"end_month"`
		EndDay          *int    `json:"end_day"`
		EndHour         *int    `json:"end_hour"`
		EndMinute       *int    `json:"end_minute"`
		IsRecurring     bool    `json:"is_recurring"`
		RecurrenceType  *string `json:"recurrence_type"`
		Visibility      string  `json:"visibility"`
		VisibilityRules *string `json:"visibility_rules"`
		Category        *string `json:"category"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateEvent(ctx, eventID, UpdateEventInput{
		Name:            req.Name,
		Description:     req.Description,
		DescriptionHTML: req.DescriptionHTML,
		EntityID:        req.EntityID,
		Year:            req.Year,
		Month:           req.Month,
		Day:             req.Day,
		StartHour:       req.StartHour,
		StartMinute:     req.StartMinute,
		EndYear:         req.EndYear,
		EndMonth:        req.EndMonth,
		EndDay:          req.EndDay,
		EndHour:         req.EndHour,
		EndMinute:       req.EndMinute,
		IsRecurring:     req.IsRecurring,
		RecurrenceType:  req.RecurrenceType,
		Visibility:      req.Visibility,
		VisibilityRules: req.VisibilityRules,
		Category:        req.Category,
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

// UpdateEventVisibilityAPI updates the visibility and per-user rules for a calendar event.
// PUT /campaigns/:id/calendar/events/:eid/visibility
func (h *Handler) UpdateEventVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}

	var input UpdateEventVisibilityInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateEventVisibility(ctx, eventID, input); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateSeasonsAPI replaces all seasons.
// PUT /campaigns/:id/calendars/:calId/seasons
func (h *Handler) UpdateSeasonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var seasons []Season
	if err := c.Bind(&seasons); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetSeasons(ctx, cal.ID, seasons)
}

// UpdateErasAPI replaces all eras.
// PUT /campaigns/:id/calendars/:calId/eras
func (h *Handler) UpdateErasAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var eras []EraInput
	if err := c.Bind(&eras); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetEras(ctx, cal.ID, eras)
}

// UpdateEventCategoriesAPI replaces all event categories.
// PUT /campaigns/:id/calendars/:calId/event-categories
func (h *Handler) UpdateEventCategoriesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var cats []EventCategoryInput
	if err := c.Bind(&cats); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.SetEventCategories(ctx, cal.ID, cats)
}

// GetEventCategoriesAPI returns all event categories for a calendar.
// GET /campaigns/:id/calendars/:calId/event-categories
func (h *Handler) GetEventCategoriesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	cats, err := h.svc.GetEventCategories(ctx, cal.ID)
	if err != nil {
		return err
	}
	if cats == nil {
		cats = []EventCategory{}
	}
	return c.JSON(http.StatusOK, cats)
}

// DeleteCalendarAPI removes the calendar and all its data.
// DELETE /campaigns/:id/calendars/:calId
func (h *Handler) DeleteCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	if _, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteCalendar(ctx, calID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ShowSettings renders the calendar settings page.
// GET /campaigns/:id/calendars/:calId/settings
func (h *Handler) ShowSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarSettingsFragment(cc, cal, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, CalendarSettingsPage(cc, cal, csrfToken))
}

// AdvanceDateAPI moves the current date forward by N days.
// POST /campaigns/:id/calendars/:calId/advance
func (h *Handler) AdvanceDateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Days int `json:"days"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.Days < 1 || req.Days > 3650 {
		return apperror.NewBadRequest("days must be between 1 and 3650")
	}

	return h.svc.AdvanceDate(ctx, cal.ID, req.Days)
}

// AdvanceTimeAPI moves the current time forward by hours and/or minutes,
// rolling over into days as needed.
// POST /campaigns/:id/calendars/:calId/advance-time
func (h *Handler) AdvanceTimeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.Hours < 0 || req.Minutes < 0 {
		return apperror.NewBadRequest("hours and minutes must be non-negative")
	}
	if req.Hours == 0 && req.Minutes == 0 {
		return apperror.NewBadRequest("must advance by at least 1 minute or 1 hour")
	}
	if req.Hours > 87600 { // ~10 years of 24-hour days
		return apperror.NewBadRequest("hours must be at most 87600")
	}

	return h.svc.AdvanceTime(ctx, cal.ID, req.Hours, req.Minutes)
}

// EntityEventsFragment returns a small HTMX fragment listing calendar events
// linked to a specific entity. Loaded lazily from entity show pages.
// Uses the default calendar for the campaign since entity pages don't know which
// calendar context to use; events from all calendars are included.
// GET /campaigns/:id/calendars/entity-events/:eid
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
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForEntity(ctx, entityID, role, userID)
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
// Supports both /calendars/:calId/upcoming and /calendars/upcoming (default calendar).
// GET /campaigns/:id/calendars/:calId/upcoming
func (h *Handler) UpcomingEventsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	calID := c.Param("calId")
	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return middleware.Render(c, http.StatusOK, UpcomingEventsEmpty())
		}
	} else {
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
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
	userID := auth.GetUserID(c)
	events, err := h.svc.ListUpcomingEvents(ctx, cal.ID, limit, role, userID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, UpcomingEventsBlock(cc, cal, events))
}

// ShowTimeline renders the timeline (list) view of calendar events.
// GET /campaigns/:id/calendars/:calId/timeline
func (h *Handler) ShowTimeline(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Default to current year, allow override via query param.
	year := cal.CurrentYear
	if q := c.QueryParam("year"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			year = v
		}
	}

	role := int(cc.MemberRole)
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForYear(ctx, cal.ID, year, role, userID)
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

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, TimelineFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, TimelinePage(cc, data))
}

// ExportCalendarAPI returns the calendar as a downloadable JSON file.
// GET /campaigns/:id/calendars/:calId/export
func (h *Handler) ExportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
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
// POST /campaigns/:id/calendars/:calId/import
func (h *Handler) ImportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Read uploaded file (multipart form or raw JSON body).
	var data []byte
	file, fileErr := c.FormFile("file")
	if fileErr == nil {
		// Multipart upload.
		src, openErr := file.Open()
		if openErr != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
		defer func() { _ = src.Close() }()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024)) // 10MB limit
		if err != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
	} else {
		// Try raw JSON body.
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return apperror.NewBadRequest("no file uploaded and no JSON body")
		}
	}

	// Parse and detect format.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
	}

	// Check for preview mode — return what would be imported without applying.
	if c.QueryParam("preview") == "true" {
		return c.JSON(http.StatusOK, result)
	}

	// Apply the import to the existing calendar.
	if err := h.svc.ApplyImport(ctx, cal.ID, result); err != nil {
		slog.Error("import: failed to apply", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to apply import"))
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
			return apperror.NewBadRequest("could not read uploaded file")
		}
		defer func() { _ = src.Close() }()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024))
		if err != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
	} else {
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return apperror.NewBadRequest("no file uploaded")
		}
	}

	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
	}

	// Return the parsed preview as JSON.
	return c.JSON(http.StatusOK, result)
}

// ImportFromSetupAPI handles import during calendar setup (no existing calendar).
// Creates a new calendar and applies the imported configuration.
// POST /campaigns/:id/calendars/import-setup
func (h *Handler) ImportFromSetupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// Read uploaded file.
	file, fileErr := c.FormFile("file")
	if fileErr != nil {
		return apperror.NewBadRequest("no file uploaded")
	}
	src, err := file.Open()
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, 10*1024*1024))
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}

	// Parse the import.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
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
		return apperror.NewInternal(fmt.Errorf("failed to apply import"))
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
		fmt.Sprintf("/campaigns/%s/calendars/%s/settings", cc.Campaign.ID, cal.ID))
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
	Sessions   []CalendarSession // Sessions to display on real-life calendar grids.
	CampaignID string
	UserID     string
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

// SessionsForDay returns sessions scheduled on the given day.
// Day is 1-based for the current month being viewed.
func (d CalendarViewData) SessionsForDay(day int) []CalendarSession {
	target := fmt.Sprintf("%04d-%02d-%02d", d.Year, d.MonthIndex, day)
	var result []CalendarSession
	for _, s := range d.Sessions {
		if s.ScheduledDate == target {
			result = append(result, s)
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

// WeekViewData holds data for the calendar week view.
type WeekViewData struct {
	Calendar   *Calendar
	Year       int
	MonthIndex int // 1-based month of the week's starting day
	StartDay   int // 1-based day of the week's starting day
	Events     []Event
	Sessions   []CalendarSession
	CampaignID string
	UserID     string
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

// WeekDay represents a single day in the week view.
type WeekDay struct {
	Year      int
	Month     int // 1-based month index
	Day       int
	MonthName string
	Events    []Event
	Sessions  []CalendarSession
	IsToday   bool
	Season    *Season
}

// WeekDays returns the list of days in this week, each with their events.
func (d WeekViewData) WeekDays() []WeekDay {
	cal := d.Calendar
	weekLen := cal.WeekLength()
	if weekLen == 0 {
		weekLen = 7
	}

	days := make([]WeekDay, 0, weekLen)
	year := d.Year
	month := d.MonthIndex
	day := d.StartDay

	for i := 0; i < weekLen; i++ {
		// Get month name.
		monthName := ""
		if month >= 1 && month <= len(cal.Months) {
			monthName = cal.Months[month-1].Name
		}

		// Filter events for this specific date.
		var dayEvents []Event
		for _, e := range d.Events {
			if e.Year == year && e.Month == month && e.Day == day {
				dayEvents = append(dayEvents, e)
			}
		}

		// Filter sessions for this date.
		var daySessions []CalendarSession
		target := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		for _, s := range d.Sessions {
			if s.ScheduledDate == target {
				daySessions = append(daySessions, s)
			}
		}

		isToday := year == cal.CurrentYear && month == cal.CurrentMonth && day == cal.CurrentDay
		season := cal.SeasonForDate(month, day)

		days = append(days, WeekDay{
			Year:      year,
			Month:     month,
			Day:       day,
			MonthName: monthName,
			Events:    dayEvents,
			Sessions:  daySessions,
			IsToday:   isToday,
			Season:    season,
		})

		// Advance to next day.
		day++
		monthDays := cal.MonthDays(month-1, year)
		if day > monthDays {
			day = 1
			month++
			if month > len(cal.Months) {
				month = 1
				year++
			}
		}
	}
	return days
}

// WeekdayName returns the weekday name for the i-th day of the week (0-based).
func (d WeekViewData) WeekdayName(i int) string {
	if i >= 0 && i < len(d.Calendar.Weekdays) {
		return d.Calendar.Weekdays[i].Name
	}
	return ""
}

// PrevWeek returns year, month, day for the start of the previous week.
func (d WeekViewData) PrevWeek() (int, int, int) {
	cal := d.Calendar
	weekLen := cal.WeekLength()
	if weekLen == 0 {
		weekLen = 7
	}
	year := d.Year
	month := d.MonthIndex
	day := d.StartDay - weekLen

	for day < 1 {
		month--
		if month < 1 {
			month = len(cal.Months)
			year--
		}
		monthDays := cal.MonthDays(month-1, year)
		day += monthDays
	}
	return year, month, day
}

// NextWeek returns year, month, day for the start of the next week.
func (d WeekViewData) NextWeek() (int, int, int) {
	cal := d.Calendar
	weekLen := cal.WeekLength()
	if weekLen == 0 {
		weekLen = 7
	}
	year := d.Year
	month := d.MonthIndex
	day := d.StartDay + weekLen

	monthDays := cal.MonthDays(month-1, year)
	for day > monthDays {
		day -= monthDays
		month++
		if month > len(cal.Months) {
			month = 1
			year++
		}
		monthDays = cal.MonthDays(month-1, year)
	}
	return year, month, day
}

// EndDate returns the year, month, day of the last day in this week.
func (d WeekViewData) EndDate() (int, int, int) {
	days := d.WeekDays()
	if len(days) == 0 {
		return d.Year, d.MonthIndex, d.StartDay
	}
	last := days[len(days)-1]
	return last.Year, last.Month, last.Day
}

// DayViewData holds data for the calendar day view.
type DayViewData struct {
	Calendar   *Calendar
	Year       int
	MonthIndex int // 1-based month
	Day        int // 1-based day
	Events     []Event
	Sessions   []CalendarSession
	CampaignID string
	UserID     string
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

// MonthName returns the name of the current month.
func (d DayViewData) MonthName() string {
	if d.MonthIndex >= 1 && d.MonthIndex <= len(d.Calendar.Months) {
		return d.Calendar.Months[d.MonthIndex-1].Name
	}
	return ""
}

// WeekdayName returns the weekday name for this day.
func (d DayViewData) WeekdayName() string {
	weekLen := d.Calendar.WeekLength()
	if weekLen == 0 {
		return ""
	}
	yearLength := d.Calendar.YearLength()
	absDay := d.Year*yearLength
	for i := 0; i < d.MonthIndex-1 && i < len(d.Calendar.Months); i++ {
		absDay += d.Calendar.Months[i].Days
		if d.Calendar.IsLeapYear(d.Year) {
			absDay += d.Calendar.Months[i].LeapYearDays
		}
	}
	absDay += d.Day
	idx := absDay % weekLen
	if idx < 0 {
		idx += weekLen
	}
	if idx >= 0 && idx < len(d.Calendar.Weekdays) {
		return d.Calendar.Weekdays[idx].Name
	}
	return ""
}

// IsToday reports whether this day is the calendar's current date.
func (d DayViewData) IsToday() bool {
	return d.Year == d.Calendar.CurrentYear &&
		d.MonthIndex == d.Calendar.CurrentMonth &&
		d.Day == d.Calendar.CurrentDay
}

// Season returns the season for this day, if any.
func (d DayViewData) Season() *Season {
	return d.Calendar.SeasonForDate(d.MonthIndex, d.Day)
}

// PrevDay returns year, month, day for the previous day.
func (d DayViewData) PrevDay() (int, int, int) {
	year, month, day := d.Year, d.MonthIndex, d.Day-1
	if day < 1 {
		month--
		if month < 1 {
			month = len(d.Calendar.Months)
			year--
		}
		day = d.Calendar.MonthDays(month-1, year)
	}
	return year, month, day
}

// NextDay returns year, month, day for the next day.
func (d DayViewData) NextDay() (int, int, int) {
	year, month, day := d.Year, d.MonthIndex, d.Day+1
	maxDays := d.Calendar.MonthDays(month-1, year)
	if day > maxDays {
		day = 1
		month++
		if month > len(d.Calendar.Months) {
			month = 1
			year++
		}
	}
	return year, month, day
}

// daysInGregorianMonth returns the number of days in a Gregorian calendar month.
func daysInGregorianMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}
