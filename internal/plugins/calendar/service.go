package calendar

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
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
	ListCalendars(ctx context.Context, campaignID string) ([]Calendar, error)
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error

	// Sub-resource bulk updates (replace all).
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error
	SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error
	GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error)

	// Weather.
	GetWeather(ctx context.Context, calendarID string) (*Weather, error)
	SetWeather(ctx context.Context, calendarID string, input WeatherInput) error

	// Cycles.
	GetCycles(ctx context.Context, calendarID string) ([]Cycle, error)
	SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error

	// Festivals.
	GetFestivals(ctx context.Context, calendarID string) ([]Festival, error)
	SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error

	// Events.
	CreateEvent(ctx context.Context, calendarID string, input CreateEventInput) (*Event, error)
	GetEvent(ctx context.Context, eventID string) (*Event, error)
	UpdateEvent(ctx context.Context, eventID string, input UpdateEventInput) error
	DeleteEvent(ctx context.Context, eventID string) error
	UpdateEventVisibility(ctx context.Context, eventID string, input UpdateEventVisibilityInput) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int, userID string) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int, userID string) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, limit int, role int, userID string) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int, userID string) ([]Event, error)
	ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int, userID string) ([]Event, error)

	// Search.
	SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)

	// Date/time helpers.
	AdvanceDate(ctx context.Context, calendarID string, days int) error
	AdvanceTime(ctx context.Context, calendarID string, hours, minutes int) error
	SetDate(ctx context.Context, calendarID string, year, month, day, hour, minute int) error

	// Import/export.
	ApplyImport(ctx context.Context, calendarID string, result *ImportResult) error
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)

	// Wiring.
	SetEventPublisher(pub CalendarEventPublisher)
}

// CalendarEventPublisher emits domain events when calendar data changes.
// Implemented by the WebSocket EventBus adapter in routes.go.
type CalendarEventPublisher interface {
	PublishCalendarEvent(eventType, campaignID, resourceID string, payload any)
}

// NoopCalendarEventPublisher is a no-op implementation for tests.
type NoopCalendarEventPublisher struct{}

func (NoopCalendarEventPublisher) PublishCalendarEvent(string, string, string, any) {}

// calendarService is the default CalendarService implementation.
type calendarService struct {
	repo   CalendarRepository
	events CalendarEventPublisher
}

// NewCalendarService creates a CalendarService backed by the given repository.
func NewCalendarService(repo CalendarRepository) CalendarService {
	return &calendarService{repo: repo, events: NoopCalendarEventPublisher{}}
}

// SetEventPublisher sets the event publisher for real-time sync.
func (s *calendarService) SetEventPublisher(pub CalendarEventPublisher) {
	s.events = pub
}

// CreateCalendar creates a new calendar for a campaign with default months and
// weekdays seeded based on the mode. Multiple calendars per campaign are supported.
// The first calendar created for a campaign is automatically set as the default.
//
// For real-life mode: seeds Gregorian months (with correct day counts), standard
// weekdays, 24/60/60 time system, leap year every 4, and syncs current date/time
// from the wall clock (UTC).
//
// For fantasy mode: seeds 12 generic months (30 days each) and 7 generic weekdays
// with 24/60/60 time system defaults.
func (s *calendarService) CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error) {
	// Check if any calendars already exist to determine default status.
	existing, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("check existing calendars: %w", err)
	}
	isFirst := len(existing) == 0

	if input.Name == "" {
		input.Name = "Campaign Calendar"
	}
	// Validate and default mode.
	if input.Mode != ModeRealLife {
		input.Mode = ModeFantasy
	}

	// For real-life mode, override defaults with Gregorian settings.
	if input.Mode == ModeRealLife {
		now := time.Now().UTC()
		input.CurrentYear = now.Year()
		input.HoursPerDay = 24
		input.MinutesPerHour = 60
		input.SecondsPerMinute = 60
		input.LeapYearEvery = 4
		input.LeapYearOffset = 0
		if input.Name == "" || input.Name == "Campaign Calendar" {
			input.Name = "Session Calendar"
		}
		ad := "AD"
		input.EpochName = &ad
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
		SortOrder:        len(existing), // append after existing calendars
		IsDefault:        isFirst,       // first calendar is the default
	}

	if err := s.repo.Create(ctx, cal); err != nil {
		return nil, fmt.Errorf("create calendar: %w", err)
	}

	// Seed default months and weekdays based on mode.
	if err := s.seedDefaults(ctx, cal); err != nil {
		return nil, fmt.Errorf("seeding calendar defaults: %w", err)
	}

	return cal, nil
}

// seedDefaults populates a newly created calendar with mode-appropriate months,
// weekdays, and time settings. For real-life mode, also syncs wall clock time.
func (s *calendarService) seedDefaults(ctx context.Context, cal *Calendar) error {
	if cal.Mode == ModeRealLife {
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
		if err := s.SetMonths(ctx, cal.ID, gregorianMonths); err != nil {
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
		if err := s.SetWeekdays(ctx, cal.ID, gregorianWeekdays); err != nil {
			return err
		}
		// Seed default event categories.
		if err := s.SetEventCategories(ctx, cal.ID, DefaultEventCategories()); err != nil {
			return err
		}
		// Sync current date/time from wall clock.
		now := time.Now().UTC()
		return s.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
			Name:             cal.Name,
			Description:      cal.Description,
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
		})
	}

	// Fantasy mode: 12 generic months and 7 generic weekdays.
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
	if err := s.SetMonths(ctx, cal.ID, defaultMonths); err != nil {
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
	if err := s.SetWeekdays(ctx, cal.ID, defaultWeekdays); err != nil {
		return err
	}
	return s.SetEventCategories(ctx, cal.ID, DefaultEventCategories())
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
	if cal.EventCategories, err = s.repo.GetEventCategories(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get event categories: %w", err)
	}
	if cal.Cycles, err = s.repo.GetCycles(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get cycles: %w", err)
	}
	if cal.Festivals, err = s.repo.GetFestivals(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get festivals: %w", err)
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

	// Validate time system values to prevent division by zero and invalid state.
	if input.HoursPerDay < 1 {
		return apperror.NewValidation("hours_per_day must be at least 1")
	}
	if input.MinutesPerHour < 1 {
		return apperror.NewValidation("minutes_per_hour must be at least 1")
	}
	if input.SecondsPerMinute < 1 {
		return apperror.NewValidation("seconds_per_minute must be at least 1")
	}
	if input.CurrentMonth < 1 {
		return apperror.NewValidation("current_month must be at least 1")
	}
	if input.CurrentDay < 1 {
		return apperror.NewValidation("current_day must be at least 1")
	}
	if input.CurrentHour < 0 || input.CurrentHour >= input.HoursPerDay {
		return apperror.NewValidation("current_hour must be between 0 and hours_per_day - 1")
	}
	if input.CurrentMinute < 0 || input.CurrentMinute >= input.MinutesPerHour {
		return apperror.NewValidation("current_minute must be between 0 and minutes_per_hour - 1")
	}
	if input.LeapYearEvery < 0 {
		return apperror.NewValidation("leap_year_every must not be negative")
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

// DeleteCalendar removes a calendar and all its data. If the deleted calendar
// was the default, the first remaining calendar (by sort_order) becomes the new default.
func (s *calendarService) DeleteCalendar(ctx context.Context, calendarID string) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}
	wasDefault := cal.IsDefault
	campaignID := cal.CampaignID

	if err := s.repo.Delete(ctx, calendarID); err != nil {
		return err
	}

	// If the deleted calendar was the default, promote the first remaining one.
	if wasDefault {
		remaining, err := s.repo.ListByCampaignID(ctx, campaignID)
		if err != nil {
			return fmt.Errorf("list remaining calendars: %w", err)
		}
		if len(remaining) > 0 {
			if err := s.repo.SetDefault(ctx, campaignID, remaining[0].ID); err != nil {
				return fmt.Errorf("set new default: %w", err)
			}
		}
	}
	return nil
}

// ListCalendars returns all calendars for a campaign, ordered by sort_order then name.
func (s *calendarService) ListCalendars(ctx context.Context, campaignID string) ([]Calendar, error) {
	cals, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	return cals, nil
}

// SetDefaultCalendar marks a calendar as the campaign's default, unsetting all others.
func (s *calendarService) SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error {
	// Verify the calendar belongs to this campaign (IDOR protection).
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}
	if cal.CampaignID != campaignID {
		return apperror.NewForbidden("calendar does not belong to this campaign")
	}
	return s.repo.SetDefault(ctx, campaignID, calendarID)
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
	if err := s.repo.SetMonths(ctx, calendarID, months); err != nil {
		return err
	}
	s.publishStructureUpdated(ctx, calendarID)
	return nil
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
	if err := s.repo.SetWeekdays(ctx, calendarID, weekdays); err != nil {
		return err
	}
	s.publishStructureUpdated(ctx, calendarID)
	return nil
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
	if err := s.repo.SetMoons(ctx, calendarID, moons); err != nil {
		return err
	}
	s.publishStructureUpdated(ctx, calendarID)
	return nil
}

// SetSeasons replaces all seasons.
func (s *calendarService) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	for i, s := range seasons {
		if s.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("season %d: name is required", i+1))
		}
	}
	if err := s.repo.SetSeasons(ctx, calendarID, seasons); err != nil {
		return err
	}
	s.publishStructureUpdated(ctx, calendarID)
	return nil
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
	if err := s.repo.SetEras(ctx, calendarID, eras); err != nil {
		return err
	}
	s.publishStructureUpdated(ctx, calendarID)
	return nil
}

// SetEventCategories replaces all event categories. Validates names and slugs.
func (s *calendarService) SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error {
	for i, c := range cats {
		if c.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("category %d: name is required", i+1))
		}
		if c.Slug == "" {
			return apperror.NewValidation(fmt.Sprintf("category %d: slug is required", i+1))
		}
		if c.Color == "" {
			cats[i].Color = "#6b7280"
		}
	}
	return s.repo.SetEventCategories(ctx, calendarID, cats)
}

// GetEventCategories returns all event categories for a calendar.
func (s *calendarService) GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error) {
	return s.repo.GetEventCategories(ctx, calendarID)
}

// GetWeather returns the current weather state for a calendar.
func (s *calendarService) GetWeather(ctx context.Context, calendarID string) (*Weather, error) {
	return s.repo.GetWeather(ctx, calendarID)
}

// SetWeather sets the current weather state for a calendar.
func (s *calendarService) SetWeather(ctx context.Context, calendarID string, input WeatherInput) error {
	if err := s.repo.SetWeather(ctx, calendarID, input); err != nil {
		return fmt.Errorf("set weather: %w", err)
	}
	// Publish weather change event.
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("weather.changed", cal.CampaignID, calendarID, input)
	}
	return nil
}

// GetCycles returns all cycles for a calendar.
func (s *calendarService) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	return s.repo.GetCycles(ctx, calendarID)
}

// SetCycles replaces all cycles for a calendar.
func (s *calendarService) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	if err := s.repo.SetCycles(ctx, calendarID, cycles); err != nil {
		return fmt.Errorf("set cycles: %w", err)
	}
	// Publish structure updated event.
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("structure.updated", cal.CampaignID, calendarID, nil)
	}
	return nil
}

// GetFestivals returns all festivals for a calendar.
func (s *calendarService) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	return s.repo.GetFestivals(ctx, calendarID)
}

// SetFestivals replaces all festivals for a calendar.
func (s *calendarService) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	if err := s.repo.SetFestivals(ctx, calendarID, festivals); err != nil {
		return fmt.Errorf("set festivals: %w", err)
	}
	// Publish structure updated event.
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("structure.updated", cal.CampaignID, calendarID, nil)
	}
	return nil
}

// CreateEvent creates a new calendar event.
func (s *calendarService) CreateEvent(ctx context.Context, calendarID string, input CreateEventInput) (*Event, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("event name is required")
	}
	if len(input.Name) > 255 {
		return nil, apperror.NewValidation("event name must be 255 characters or less")
	}
	if input.Month < 1 {
		return nil, apperror.NewValidation("month must be at least 1")
	}
	if input.Day < 1 {
		return nil, apperror.NewValidation("day must be at least 1")
	}
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return nil, err
	}

	// Sanitize HTML if provided (rich text descriptions from TipTap editor).
	var descHTML *string
	if input.DescriptionHTML != nil && *input.DescriptionHTML != "" {
		sanitized := sanitize.HTML(*input.DescriptionHTML)
		descHTML = &sanitized
	}

	evt := &Event{
		ID:                       generateID(),
		CalendarID:               calendarID,
		EntityID:                 input.EntityID,
		Name:                     input.Name,
		Description:              input.Description,
		DescriptionHTML:          descHTML,
		Year:                     input.Year,
		Month:                    input.Month,
		Day:                      input.Day,
		StartHour:                input.StartHour,
		StartMinute:              input.StartMinute,
		EndYear:                  input.EndYear,
		EndMonth:                 input.EndMonth,
		EndDay:                   input.EndDay,
		EndHour:                  input.EndHour,
		EndMinute:                input.EndMinute,
		IsRecurring:              input.IsRecurring,
		RecurrenceType:           input.RecurrenceType,
		RecurrenceInterval:       input.RecurrenceInterval,
		RecurrenceEndYear:        input.RecurrenceEndYear,
		RecurrenceEndMonth:       input.RecurrenceEndMonth,
		RecurrenceEndDay:         input.RecurrenceEndDay,
		RecurrenceMaxOccurrences: input.RecurrenceMaxOccurrences,
		Visibility:               input.Visibility,
		VisibilityRules:          input.VisibilityRules,
		Category:                 input.Category,
		Color:                    input.Color,
		Icon:                     input.Icon,
		AllDay:                   input.AllDay,
		CreatedBy:                &input.CreatedBy,
	}

	if err := s.repo.CreateEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	// Resolve campaign ID for event publishing.
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("event.created", cal.CampaignID, evt.ID, evt)
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

	// Validate visibility (same rules as CreateEvent).
	if input.Visibility == "" {
		input.Visibility = evt.Visibility // preserve existing if not provided
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return err
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
	evt.RecurrenceInterval = input.RecurrenceInterval
	evt.RecurrenceEndYear = input.RecurrenceEndYear
	evt.RecurrenceEndMonth = input.RecurrenceEndMonth
	evt.RecurrenceEndDay = input.RecurrenceEndDay
	evt.RecurrenceMaxOccurrences = input.RecurrenceMaxOccurrences
	evt.Visibility = input.Visibility
	evt.VisibilityRules = input.VisibilityRules
	evt.Category = input.Category
	evt.Color = input.Color
	evt.Icon = input.Icon
	evt.AllDay = input.AllDay

	if err := s.repo.UpdateEvent(ctx, evt); err != nil {
		return err
	}
	if cal, err := s.repo.GetByID(ctx, evt.CalendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("event.updated", cal.CampaignID, evt.ID, evt)
	}
	return nil
}

// DeleteEvent removes an event.
func (s *calendarService) DeleteEvent(ctx context.Context, eventID string) error {
	// Fetch event before deletion for event publishing.
	evt, _ := s.repo.GetEvent(ctx, eventID)
	if err := s.repo.DeleteEvent(ctx, eventID); err != nil {
		return err
	}
	if evt != nil {
		if cal, err := s.repo.GetByID(ctx, evt.CalendarID); err == nil && cal != nil {
			s.events.PublishCalendarEvent("event.deleted", cal.CampaignID, eventID, evt)
		}
	}
	return nil
}

// ListEventsForMonth returns events for a given month/year, filtered by role and per-user rules.
func (s *calendarService) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int, userID string) ([]Event, error) {
	events, err := s.repo.ListEventsForMonth(ctx, calendarID, year, month, role)
	if err != nil {
		return nil, err
	}
	return filterEventsByUser(events, role, userID), nil
}

// ListEventsForEntity returns all events linked to a specific entity, filtered by per-user rules.
func (s *calendarService) ListEventsForEntity(ctx context.Context, entityID string, role int, userID string) ([]Event, error) {
	events, err := s.repo.ListEventsForEntity(ctx, entityID, role)
	if err != nil {
		return nil, err
	}
	return filterEventsByUser(events, role, userID), nil
}

// ListEventsForYear returns all events for a given year, filtered by per-user rules.
func (s *calendarService) ListEventsForYear(ctx context.Context, calendarID string, year int, role int, userID string) ([]Event, error) {
	events, err := s.repo.ListEventsForYear(ctx, calendarID, year, role)
	if err != nil {
		return nil, err
	}
	return filterEventsByUser(events, role, userID), nil
}

// ListEventsForDateRange returns events within a date range for a given year.
func (s *calendarService) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int, userID string) ([]Event, error) {
	events, err := s.repo.ListEventsForDateRange(ctx, calendarID, year, startMonth, startDay, endMonth, endDay, role)
	if err != nil {
		return nil, err
	}
	return filterEventsByUser(events, role, userID), nil
}

// UpdateEventVisibility updates the base visibility and per-user rules for a calendar event.
func (s *calendarService) UpdateEventVisibility(ctx context.Context, eventID string, input UpdateEventVisibilityInput) error {
	evt, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}
	if evt == nil {
		return apperror.NewNotFound("event not found")
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return err
	}
	return s.repo.UpdateEventVisibility(ctx, eventID, input.Visibility, input.VisibilityRules)
}

// ListUpcomingEvents returns the next N events from the calendar's current date.
// Fetches the calendar to determine the current date, then delegates to the repo.
func (s *calendarService) ListUpcomingEvents(ctx context.Context, calendarID string, limit int, role int, userID string) ([]Event, error) {
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
	events, err := s.repo.ListUpcomingEvents(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, limit)
	if err != nil {
		return nil, err
	}
	return filterEventsByUser(events, role, userID), nil
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

	// Snapshot before state for change detection.
	beforeSeason, beforeEra, beforeMoonPhases := s.snapshotState(ctx, cal)

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
	if err := s.repo.Update(ctx, cal); err != nil {
		return err
	}
	s.events.PublishCalendarEvent("date.advanced", cal.CampaignID, calendarID, map[string]int{
		"year":  cal.CurrentYear,
		"month": cal.CurrentMonth,
		"day":   cal.CurrentDay,
	})
	// Detect and publish state changes.
	s.publishStateChanges(ctx, cal, beforeSeason, beforeEra, beforeMoonPhases)
	return nil
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

// SetDate sets the calendar's current date/time to an absolute value.
// Unlike AdvanceDate (which moves forward by N days), this sets exact values.
// Used by external sync tools (Foundry/Calendaria) that send absolute dates.
func (s *calendarService) SetDate(ctx context.Context, calendarID string, year, month, day, hour, minute int) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	if month < 1 {
		return apperror.NewValidation("month must be at least 1")
	}
	if day < 1 {
		return apperror.NewValidation("day must be at least 1")
	}

	hpd := cal.HoursPerDay
	if hpd <= 0 {
		hpd = 24
	}
	mph := cal.MinutesPerHour
	if mph <= 0 {
		mph = 60
	}
	if hour < 0 || hour >= hpd {
		return apperror.NewValidation("hour out of range for this calendar")
	}
	if minute < 0 || minute >= mph {
		return apperror.NewValidation("minute out of range for this calendar")
	}

	// Eager-load sub-resources for change detection.
	months, _ := s.repo.GetMonths(ctx, calendarID)
	cal.Months = months

	// Snapshot before state for change detection.
	beforeSeason, beforeEra, beforeMoonPhases := s.snapshotState(ctx, cal)

	cal.CurrentYear = year
	cal.CurrentMonth = month
	cal.CurrentDay = day
	cal.CurrentHour = hour
	cal.CurrentMinute = minute

	if err := s.repo.Update(ctx, cal); err != nil {
		return fmt.Errorf("set date: %w", err)
	}

	s.events.PublishCalendarEvent("date.advanced", cal.CampaignID, calendarID, map[string]int{
		"year":   cal.CurrentYear,
		"month":  cal.CurrentMonth,
		"day":    cal.CurrentDay,
		"hour":   cal.CurrentHour,
		"minute": cal.CurrentMinute,
	})
	// Detect and publish state changes.
	s.publishStateChanges(ctx, cal, beforeSeason, beforeEra, beforeMoonPhases)
	return nil
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

// --- Visibility Helpers ---

// filterEventsByUser applies per-user visibility rules to a slice of events.
// Owners always see everything and are not filtered.
func filterEventsByUser(events []Event, role int, userID string) []Event {
	if permissions.CanSeeDmOnly(role) || userID == "" {
		return events
	}
	filtered := events[:0]
	for _, e := range events {
		if canUserView(e.Visibility, e.VisibilityRules, role, userID) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// canUserView checks whether a user can see an event based on its base visibility
// and per-user JSON rules. Owners always see everything and should be checked
// before calling this function.
func canUserView(baseVisibility string, visRulesJSON *string, role int, userID string) bool {
	// Base visibility: dm_only requires Owner role.
	if baseVisibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
		return false
	}

	// Parse per-user JSON rules if present.
	if visRulesJSON == nil || *visRulesJSON == "" {
		return true
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*visRulesJSON), &rules); err != nil {
		slog.Warn("unparseable visibility_rules JSON, failing open", slog.Any("error", err))
		return true // Fail open for existing items — validated on write path.
	}

	// AllowedUsers whitelist takes precedence.
	if len(rules.AllowedUsers) > 0 {
		for _, uid := range rules.AllowedUsers {
			if uid == userID {
				return true
			}
		}
		return false
	}

	// DeniedUsers blacklist.
	if len(rules.DeniedUsers) > 0 {
		for _, uid := range rules.DeniedUsers {
			if uid == userID {
				return false
			}
		}
	}

	return true
}

// validateVisibilityRules checks that a visibility_rules JSON string is
// well-formed if present. Returns a validation error on bad JSON.
func validateVisibilityRules(rulesJSON *string) error {
	if rulesJSON == nil || *rulesJSON == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*rulesJSON), &rules); err != nil {
		return apperror.NewValidation("visibility_rules must be valid JSON: " + err.Error())
	}
	return nil
}

// SearchCalendarEvents returns calendar events matching a query for the quick search system.
// Searches across all calendars in the campaign, including the calendar name in results
// when the campaign has multiple calendars.
func (s *calendarService) SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error) {
	cals, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("search calendar events: %w", err)
	}
	if len(cals) == 0 {
		return nil, nil
	}

	hasMultiple := len(cals) > 1
	var results []map[string]string

	for _, cal := range cals {
		events, err := s.repo.SearchEvents(ctx, cal.ID, query, role)
		if err != nil {
			return nil, fmt.Errorf("search calendar events: %w", err)
		}
		for _, e := range events {
			typeName := "Event"
			if hasMultiple {
				typeName = fmt.Sprintf("Event [%s]", cal.Name)
			}
			results = append(results, map[string]string{
				"id":         e.ID,
				"name":       e.Name,
				"type_name":  typeName,
				"type_icon":  "fa-calendar",
				"type_color": "#f59e0b",
				"url":        fmt.Sprintf("/campaigns/%s/calendars/%s", campaignID, cal.ID),
			})
		}
	}
	return results, nil
}

// publishStructureUpdated fires a structure.updated WS event.
func (s *calendarService) publishStructureUpdated(ctx context.Context, calendarID string) {
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("structure.updated", cal.CampaignID, calendarID, nil)
	}
}

// snapshotState captures the current season, era, and moon phase names before a date change.
// Used for change detection to publish WS events.
func (s *calendarService) snapshotState(ctx context.Context, cal *Calendar) (string, string, map[int]string) {
	// Load seasons/eras/moons if not already loaded.
	if cal.Seasons == nil {
		cal.Seasons, _ = s.repo.GetSeasons(ctx, cal.ID)
	}
	if cal.Eras == nil {
		cal.Eras, _ = s.repo.GetEras(ctx, cal.ID)
	}
	if cal.Moons == nil {
		cal.Moons, _ = s.repo.GetMoons(ctx, cal.ID)
	}

	var seasonName, eraName string
	if season := cal.CurrentSeason(); season != nil {
		seasonName = season.Name
	}
	if era := cal.CurrentEra(); era != nil {
		eraName = era.Name
	}

	absDay := cal.CurrentAbsoluteDay()
	moonPhases := make(map[int]string, len(cal.Moons))
	for i := range cal.Moons {
		moonPhases[cal.Moons[i].ID] = cal.Moons[i].MoonPhaseName(absDay)
	}

	return seasonName, eraName, moonPhases
}

// publishStateChanges compares before/after state and publishes WS events for changes.
func (s *calendarService) publishStateChanges(ctx context.Context, cal *Calendar, beforeSeason, beforeEra string, beforeMoonPhases map[int]string) {
	// Season change detection.
	if afterSeason := cal.CurrentSeason(); afterSeason != nil {
		if afterSeason.Name != beforeSeason {
			s.events.PublishCalendarEvent("season.changed", cal.CampaignID, cal.ID, map[string]any{
				"id":    afterSeason.ID,
				"name":  afterSeason.Name,
				"color": afterSeason.Color,
			})
		}
	} else if beforeSeason != "" {
		// Left a season without entering a new one.
		s.events.PublishCalendarEvent("season.changed", cal.CampaignID, cal.ID, nil)
	}

	// Era change detection.
	if afterEra := cal.CurrentEra(); afterEra != nil {
		if afterEra.Name != beforeEra {
			s.events.PublishCalendarEvent("era.changed", cal.CampaignID, cal.ID, map[string]any{
				"id":           afterEra.ID,
				"name":         afterEra.Name,
				"color":        afterEra.Color,
			})
		}
	}

	// Moon phase change detection.
	absDay := cal.CurrentAbsoluteDay()
	for i := range cal.Moons {
		moon := &cal.Moons[i]
		afterPhase := moon.MoonPhaseName(absDay)
		if afterPhase != beforeMoonPhases[moon.ID] {
			s.events.PublishCalendarEvent("moon.phase_changed", cal.CampaignID, cal.ID, map[string]any{
				"moon_id":        moon.ID,
				"moon_name":      moon.Name,
				"phase_name":     afterPhase,
				"phase_position": moon.MoonPhase(absDay),
			})
		}
	}
}
