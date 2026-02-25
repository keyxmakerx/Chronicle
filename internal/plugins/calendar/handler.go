package calendar

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the calendar plugin.
type Handler struct {
	svc CalendarService
}

// NewHandler creates a new calendar Handler.
func NewHandler(svc CalendarService) *Handler {
	return &Handler{svc: svc}
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
		Calendar:     cal,
		Year:         year,
		MonthIndex:   month,
		Events:       events,
		CampaignID:   cc.Campaign.ID,
		IsOwner:      cc.MemberRole >= campaigns.RoleOwner,
		CSRFToken:    middleware.GetCSRFToken(c),
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

	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, CreateCalendarInput{
		Name:        name,
		EpochName:   epoch,
		CurrentYear: startYear,
	})
	if err != nil {
		return err
	}

	// Seed default months (12 months, 30 days each) and weekdays (7 days).
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

	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendar", cc.Campaign.ID))
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
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		EpochName    *string `json:"epoch_name"`
		CurrentYear  int     `json:"current_year"`
		CurrentMonth int     `json:"current_month"`
		CurrentDay   int     `json:"current_day"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	return h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
		Name:         req.Name,
		Description:  req.Description,
		EpochName:    req.EpochName,
		CurrentYear:  req.CurrentYear,
		CurrentMonth: req.CurrentMonth,
		CurrentDay:   req.CurrentDay,
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
		IsRecurring    bool    `json:"is_recurring"`
		RecurrenceType *string `json:"recurrence_type"`
		Visibility     string  `json:"visibility"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Get user ID from session context.
	userID := ""
	if session := c.Get("session"); session != nil {
		// Session is set by auth middleware.
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
		IsRecurring:    req.IsRecurring,
		RecurrenceType: req.RecurrenceType,
		Visibility:     req.Visibility,
		CreatedBy:      userID,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, evt)
}

// DeleteEventAPI deletes an event.
// DELETE /campaigns/:id/calendar/events/:eid
func (h *Handler) DeleteEventAPI(c echo.Context) error {
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	if err := h.svc.DeleteEvent(ctx, eventID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
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

// CalendarViewData holds all data needed to render the calendar grid.
type CalendarViewData struct {
	Calendar   *Calendar
	Year       int
	MonthIndex int // 1-based month index
	Events     []Event
	CampaignID string
	IsOwner    bool
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
