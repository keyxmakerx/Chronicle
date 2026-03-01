package calendar

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CalendarService defines business logic for the calendar plugin.
type CalendarService interface {
	// Calendar CRUD.
	CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error)
	GetCalendar(ctx context.Context, campaignID string) (*Calendar, error)
	GetCalendarByID(ctx context.Context, calendarID string) (*Calendar, error)
	UpdateCalendar(ctx context.Context, calendarID string, input UpdateCalendarInput) error
	DeleteCalendar(ctx context.Context, calendarID string) error

	// Sub-resource bulk updates (replace all).
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error

	// Events.
	CreateEvent(ctx context.Context, calendarID string, input CreateEventInput) (*Event, error)
	GetEvent(ctx context.Context, eventID string) (*Event, error)
	UpdateEvent(ctx context.Context, eventID string, input UpdateEventInput) error
	DeleteEvent(ctx context.Context, eventID string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, limit int, role int) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error)

	// Date/time helpers.
	AdvanceDate(ctx context.Context, calendarID string, days int) error
	AdvanceTime(ctx context.Context, calendarID string, hours, minutes int) error

	// Import/export.
	ApplyImport(ctx context.Context, calendarID string, result *ImportResult) error
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)
}

// calendarService is the default CalendarService implementation.
type calendarService struct {
	repo CalendarRepository
}

// NewCalendarService creates a CalendarService backed by the given repository.
func NewCalendarService(repo CalendarRepository) CalendarService {
	return &calendarService{repo: repo}
}

// CreateCalendar creates a new calendar for a campaign. Only one per campaign.
func (s *calendarService) CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error) {
	// Check if calendar already exists.
	existing, err := s.repo.GetByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("check existing calendar: %w", err)
	}
	if existing != nil {
		return nil, apperror.NewValidation("campaign already has a calendar")
	}

	if input.Name == "" {
		input.Name = "Campaign Calendar"
	}
	// Validate and default mode.
	if input.Mode != ModeRealLife {
		input.Mode = ModeFantasy
	}
	if input.CurrentYear == 0 {
		input.CurrentYear = 1
	}
	if input.HoursPerDay <= 0 {
		input.HoursPerDay = 24
	}
	if input.MinutesPerHour <= 0 {
		input.MinutesPerHour = 60
	}
	if input.SecondsPerMinute <= 0 {
		input.SecondsPerMinute = 60
	}

	cal := &Calendar{
		ID:               generateID(),
		CampaignID:       campaignID,
		Mode:             input.Mode,
		Name:             input.Name,
		Description:      input.Description,
		EpochName:        input.EpochName,
		CurrentYear:      input.CurrentYear,
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      input.HoursPerDay,
		MinutesPerHour:   input.MinutesPerHour,
		SecondsPerMinute: input.SecondsPerMinute,
		LeapYearEvery:    input.LeapYearEvery,
		LeapYearOffset:   input.LeapYearOffset,
	}

	if err := s.repo.Create(ctx, cal); err != nil {
		return nil, fmt.Errorf("create calendar: %w", err)
	}
	return cal, nil
}

// GetCalendar returns the full calendar for a campaign with all sub-resources.
func (s *calendarService) GetCalendar(ctx context.Context, campaignID string) (*Calendar, error) {
	cal, err := s.repo.GetByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, nil
	}
	return s.eagerLoad(ctx, cal)
}

// GetCalendarByID returns a calendar by ID with all sub-resources loaded.
func (s *calendarService) GetCalendarByID(ctx context.Context, calendarID string) (*Calendar, error) {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, nil
	}
	return s.eagerLoad(ctx, cal)
}

// eagerLoad populates all sub-resources on a calendar.
func (s *calendarService) eagerLoad(ctx context.Context, cal *Calendar) (*Calendar, error) {
	var err error
	if cal.Months, err = s.repo.GetMonths(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get months: %w", err)
	}
	if cal.Weekdays, err = s.repo.GetWeekdays(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get weekdays: %w", err)
	}
	if cal.Moons, err = s.repo.GetMoons(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get moons: %w", err)
	}
	if cal.Seasons, err = s.repo.GetSeasons(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get seasons: %w", err)
	}
	if cal.Eras, err = s.repo.GetEras(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get eras: %w", err)
	}
	return cal, nil
}

// UpdateCalendar updates the calendar name, description, epoch, and current date.
func (s *calendarService) UpdateCalendar(ctx context.Context, calendarID string, input UpdateCalendarInput) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	cal.Name = input.Name
	cal.Description = input.Description
	cal.EpochName = input.EpochName
	cal.CurrentYear = input.CurrentYear
	cal.CurrentMonth = input.CurrentMonth
	cal.CurrentDay = input.CurrentDay
	cal.CurrentHour = input.CurrentHour
	cal.CurrentMinute = input.CurrentMinute
	cal.HoursPerDay = input.HoursPerDay
	cal.MinutesPerHour = input.MinutesPerHour
	cal.SecondsPerMinute = input.SecondsPerMinute
	cal.LeapYearEvery = input.LeapYearEvery
	cal.LeapYearOffset = input.LeapYearOffset

	if err := s.repo.Update(ctx, cal); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}
	return nil
}

// DeleteCalendar removes a calendar and all its data.
func (s *calendarService) DeleteCalendar(ctx context.Context, calendarID string) error {
	return s.repo.Delete(ctx, calendarID)
}

// SetMonths replaces all months. Validates at least one month exists.
func (s *calendarService) SetMonths(ctx context.Context, calendarID string, months []MonthInput) error {
	if len(months) == 0 {
		return apperror.NewValidation("calendar must have at least one month")
	}
	for i, m := range months {
		if m.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("month %d: name is required", i+1))
		}
		if m.Days < 1 || m.Days > 400 {
			return apperror.NewValidation(fmt.Sprintf("month %q: days must be between 1 and 400", m.Name))
		}
		if m.LeapYearDays < 0 {
			return apperror.NewValidation(fmt.Sprintf("month %q: leap_year_days cannot be negative", m.Name))
		}
	}
	return s.repo.SetMonths(ctx, calendarID, months)
}

// SetWeekdays replaces all weekdays.
func (s *calendarService) SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error {
	if len(weekdays) == 0 {
		return apperror.NewValidation("calendar must have at least one weekday")
	}
	for i, w := range weekdays {
		if w.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("weekday %d: name is required", i+1))
		}
	}
	return s.repo.SetWeekdays(ctx, calendarID, weekdays)
}

// SetMoons replaces all moons.
func (s *calendarService) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	for i, m := range moons {
		if m.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("moon %d: name is required", i+1))
		}
		if m.CycleDays <= 0 {
			return apperror.NewValidation(fmt.Sprintf("moon %q: cycle_days must be positive", m.Name))
		}
	}
	return s.repo.SetMoons(ctx, calendarID, moons)
}

// SetSeasons replaces all seasons.
func (s *calendarService) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	for i, s := range seasons {
		if s.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("season %d: name is required", i+1))
		}
	}
	return s.repo.SetSeasons(ctx, calendarID, seasons)
}

// SetEras replaces all eras. Validates names and year ranges.
func (s *calendarService) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	for i, e := range eras {
		if e.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("era %d: name is required", i+1))
		}
		if e.EndYear != nil && *e.EndYear < e.StartYear {
			return apperror.NewValidation(fmt.Sprintf("era %q: end year cannot be before start year", e.Name))
		}
		if e.Color == "" {
			eras[i].Color = "#6366f1"
		}
	}
	return s.repo.SetEras(ctx, calendarID, eras)
}

// CreateEvent creates a new calendar event.
func (s *calendarService) CreateEvent(ctx context.Context, calendarID string, input CreateEventInput) (*Event, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("event name is required")
	}
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}

	// Sanitize HTML if provided (rich text descriptions from TipTap editor).
	var descHTML *string
	if input.DescriptionHTML != nil && *input.DescriptionHTML != "" {
		sanitized := sanitize.HTML(*input.DescriptionHTML)
		descHTML = &sanitized
	}

	evt := &Event{
		ID:              generateID(),
		CalendarID:      calendarID,
		EntityID:        input.EntityID,
		Name:            input.Name,
		Description:     input.Description,
		DescriptionHTML: descHTML,
		Year:            input.Year,
		Month:          input.Month,
		Day:            input.Day,
		StartHour:      input.StartHour,
		StartMinute:    input.StartMinute,
		EndYear:        input.EndYear,
		EndMonth:       input.EndMonth,
		EndDay:         input.EndDay,
		EndHour:        input.EndHour,
		EndMinute:      input.EndMinute,
		IsRecurring:    input.IsRecurring,
		RecurrenceType: input.RecurrenceType,
		Visibility:     input.Visibility,
		Category:       input.Category,
		CreatedBy:      &input.CreatedBy,
	}

	if err := s.repo.CreateEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	return evt, nil
}

// GetEvent returns an event by ID.
func (s *calendarService) GetEvent(ctx context.Context, eventID string) (*Event, error) {
	evt, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if evt == nil {
		return nil, apperror.NewNotFound("event not found")
	}
	return evt, nil
}

// UpdateEvent updates an existing event.
func (s *calendarService) UpdateEvent(ctx context.Context, eventID string, input UpdateEventInput) error {
	evt, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}
	if evt == nil {
		return apperror.NewNotFound("event not found")
	}

	evt.Name = input.Name
	evt.Description = input.Description
	// Sanitize rich text HTML if provided.
	if input.DescriptionHTML != nil && *input.DescriptionHTML != "" {
		sanitized := sanitize.HTML(*input.DescriptionHTML)
		evt.DescriptionHTML = &sanitized
	} else {
		evt.DescriptionHTML = input.DescriptionHTML
	}
	evt.EntityID = input.EntityID
	evt.Year = input.Year
	evt.Month = input.Month
	evt.Day = input.Day
	evt.StartHour = input.StartHour
	evt.StartMinute = input.StartMinute
	evt.EndYear = input.EndYear
	evt.EndMonth = input.EndMonth
	evt.EndDay = input.EndDay
	evt.EndHour = input.EndHour
	evt.EndMinute = input.EndMinute
	evt.IsRecurring = input.IsRecurring
	evt.RecurrenceType = input.RecurrenceType
	evt.Visibility = input.Visibility
	evt.Category = input.Category

	return s.repo.UpdateEvent(ctx, evt)
}

// DeleteEvent removes an event.
func (s *calendarService) DeleteEvent(ctx context.Context, eventID string) error {
	return s.repo.DeleteEvent(ctx, eventID)
}

// ListEventsForMonth returns events for a given month/year.
func (s *calendarService) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error) {
	return s.repo.ListEventsForMonth(ctx, calendarID, year, month, role)
}

// ListEventsForEntity returns all events linked to a specific entity.
func (s *calendarService) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	return s.repo.ListEventsForEntity(ctx, entityID, role)
}

// ListEventsForYear returns all events for a given year.
func (s *calendarService) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	return s.repo.ListEventsForYear(ctx, calendarID, year, role)
}

// ListUpcomingEvents returns the next N events from the calendar's current date.
// Fetches the calendar to determine the current date, then delegates to the repo.
func (s *calendarService) ListUpcomingEvents(ctx context.Context, calendarID string, limit int, role int) ([]Event, error) {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, nil
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	return s.repo.ListUpcomingEvents(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, limit)
}

// AdvanceDate moves the current date forward by the given number of days,
// rolling over months and years as needed. Accounts for leap years.
func (s *calendarService) AdvanceDate(ctx context.Context, calendarID string, days int) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	months, err := s.repo.GetMonths(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get months: %w", err)
	}
	if len(months) == 0 {
		return apperror.NewValidation("calendar has no months configured")
	}

	// Attach months to calendar for leap year calculations.
	cal.Months = months

	day := cal.CurrentDay
	monthIdx := cal.CurrentMonth - 1 // 0-indexed
	year := cal.CurrentYear

	for i := 0; i < days; i++ {
		day++
		maxDays := cal.MonthDays(monthIdx, year)
		if monthIdx >= 0 && monthIdx < len(months) && day > maxDays {
			day = 1
			monthIdx++
			if monthIdx >= len(months) {
				monthIdx = 0
				year++
			}
		}
	}

	cal.CurrentDay = day
	cal.CurrentMonth = monthIdx + 1
	cal.CurrentYear = year
	return s.repo.Update(ctx, cal)
}

// AdvanceTime moves the current time forward by the given hours and minutes,
// rolling over into days (and subsequently months/years) as needed.
func (s *calendarService) AdvanceTime(ctx context.Context, calendarID string, hours, minutes int) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	months, err := s.repo.GetMonths(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get months: %w", err)
	}
	if len(months) == 0 {
		return apperror.NewValidation("calendar has no months configured")
	}
	cal.Months = months

	hpd := cal.HoursPerDay
	if hpd <= 0 {
		hpd = 24
	}
	mph := cal.MinutesPerHour
	if mph <= 0 {
		mph = 60
	}

	// Add minutes, roll into hours.
	totalMin := cal.CurrentMinute + minutes
	extraHours := totalMin / mph
	cal.CurrentMinute = totalMin % mph

	// Add hours (including rollover from minutes), roll into days.
	totalHours := cal.CurrentHour + hours + extraHours
	extraDays := totalHours / hpd
	cal.CurrentHour = totalHours % hpd

	// Delegate day rollover to the existing date advancement logic.
	if extraDays > 0 {
		day := cal.CurrentDay
		monthIdx := cal.CurrentMonth - 1
		year := cal.CurrentYear

		for i := 0; i < extraDays; i++ {
			day++
			maxDays := cal.MonthDays(monthIdx, year)
			if day > maxDays {
				day = 1
				monthIdx++
				if monthIdx >= len(months) {
					monthIdx = 0
					year++
				}
			}
		}

		cal.CurrentDay = day
		cal.CurrentMonth = monthIdx + 1
		cal.CurrentYear = year
	}

	return s.repo.Update(ctx, cal)
}

// ApplyImport replaces a calendar's configuration with data from an ImportResult.
// Updates the calendar settings and all sub-resources (months, weekdays, moons,
// seasons, eras). This is a destructive operation — existing sub-resources are replaced.
func (s *calendarService) ApplyImport(ctx context.Context, calendarID string, result *ImportResult) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	// Update calendar-level settings from import.
	if result.CalendarName != "" {
		cal.Name = result.CalendarName
	}
	cal.EpochName = result.Settings.EpochName
	if result.Settings.CurrentYear != 0 {
		cal.CurrentYear = result.Settings.CurrentYear
	}
	cal.CurrentMonth = 1
	cal.CurrentDay = 1
	cal.HoursPerDay = result.Settings.HoursPerDay
	cal.MinutesPerHour = result.Settings.MinutesPerHour
	cal.SecondsPerMinute = result.Settings.SecondsPerMinute
	cal.LeapYearEvery = result.Settings.LeapYearEvery
	cal.LeapYearOffset = result.Settings.LeapYearOffset

	if err := s.repo.Update(ctx, cal); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}

	// Apply sub-resources.
	if len(result.Months) > 0 {
		if err := s.SetMonths(ctx, calendarID, result.Months); err != nil {
			return fmt.Errorf("set months: %w", err)
		}
	}
	if len(result.Weekdays) > 0 {
		if err := s.SetWeekdays(ctx, calendarID, result.Weekdays); err != nil {
			return fmt.Errorf("set weekdays: %w", err)
		}
	}
	if err := s.SetMoons(ctx, calendarID, result.Moons); err != nil {
		return fmt.Errorf("set moons: %w", err)
	}
	if err := s.SetSeasons(ctx, calendarID, result.Seasons); err != nil {
		return fmt.Errorf("set seasons: %w", err)
	}
	if err := s.SetEras(ctx, calendarID, result.Eras); err != nil {
		return fmt.Errorf("set eras: %w", err)
	}

	return nil
}

// ListAllEvents returns all events for a calendar (owner visibility, no limit).
// Used for calendar export.
func (s *calendarService) ListAllEvents(ctx context.Context, calendarID string) ([]Event, error) {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, nil
	}
	// Use current year, owner role (3) to get all events including dm_only.
	return s.repo.ListEventsForYear(ctx, calendarID, cal.CurrentYear, 3)
}
