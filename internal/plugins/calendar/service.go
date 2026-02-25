package calendar

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

	// Events.
	CreateEvent(ctx context.Context, calendarID string, input CreateEventInput) (*Event, error)
	GetEvent(ctx context.Context, eventID string) (*Event, error)
	UpdateEvent(ctx context.Context, eventID string, input UpdateEventInput) error
	DeleteEvent(ctx context.Context, eventID string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)

	// Date helpers.
	AdvanceDate(ctx context.Context, calendarID string, days int) error
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
	if input.CurrentYear == 0 {
		input.CurrentYear = 1
	}

	cal := &Calendar{
		ID:             generateID(),
		CampaignID:     campaignID,
		Name:           input.Name,
		Description:    input.Description,
		EpochName:      input.EpochName,
		CurrentYear:    input.CurrentYear,
		CurrentMonth:   1,
		CurrentDay:     1,
		LeapYearEvery:  input.LeapYearEvery,
		LeapYearOffset: input.LeapYearOffset,
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

	evt := &Event{
		ID:             generateID(),
		CalendarID:     calendarID,
		EntityID:       input.EntityID,
		Name:           input.Name,
		Description:    input.Description,
		Year:           input.Year,
		Month:          input.Month,
		Day:            input.Day,
		EndYear:        input.EndYear,
		EndMonth:       input.EndMonth,
		EndDay:         input.EndDay,
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
	evt.EntityID = input.EntityID
	evt.Year = input.Year
	evt.Month = input.Month
	evt.Day = input.Day
	evt.EndYear = input.EndYear
	evt.EndMonth = input.EndMonth
	evt.EndDay = input.EndDay
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
