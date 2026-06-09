package calendar

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
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
	// ListVisibleCalendars is ListCalendars filtered to the calendars the
	// viewer may see (C-CAL-DASHBOARD-W5a). Owner/co-DM see all; others see
	// only calendars whose per-calendar visibility admits them.
	ListVisibleCalendars(ctx context.Context, campaignID string, role int, userID string) ([]Calendar, error)
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error

	// Active-calendar resolution (V2 Wave 1 PR 1 / C-CAL-V2-SHELL-FOUNDATION).
	// GetActiveCalendar returns the user's currently-active calendar:
	// their last-switched calendar via repo lookup, or the campaign's
	// default when no pointer exists. Returns nil if the campaign has
	// zero calendars. SwitchActiveCalendar validates that the target
	// calendar belongs to the campaign before persisting.
	GetActiveCalendar(ctx context.Context, userID, campaignID string) (*Calendar, error)
	// GetActiveVisibleCalendar is GetActiveCalendar that never returns a
	// calendar the viewer may not see (C-CAL-DASHBOARD-W5a): if their active /
	// default calendar is hidden from them, it falls back to the first calendar
	// they can see. Owner/co-DM are unaffected (they see all).
	GetActiveVisibleCalendar(ctx context.Context, campaignID string, role int, userID string) (*Calendar, error)
	SwitchActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error

	// Sidebar pin preference (V2 Wave 1.7A §G). Default TRUE on
	// initial read (matches viewport-aware default + migration 007
	// backfill). SetSidebarPinned upserts via the existing
	// calendar_active row.
	GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error)
	SetSidebarPinned(ctx context.Context, userID, campaignID string, pinned bool) error

	// Sub-resource bulk updates (replace all).
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error
	// Per-era CRUD (V2 Wave 0 PR 2). Complements SetEras bulk-replace
	// for AI Workspace Wave 3 + per-card-edit Wave 1 use cases. All
	// three publish `structure.updated`.
	CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, input EraInput) error
	DeleteEra(ctx context.Context, eraID int) error
	SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error
	GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error)

	// Weather.
	GetWeather(ctx context.Context, calendarID string) (*Weather, error)
	SetWeather(ctx context.Context, calendarID string, input WeatherInput) error

	// Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES).
	// GetWeatherZones returns the active-zone reference plus the full
	// zone catalog. SetWeatherZones replaces the catalog (REPLACE
	// semantics matching SetMonths/SetSeasons/etc.). SetActiveWeatherZone
	// updates only the calendar_weather.zone_id pointer.
	GetWeatherZones(ctx context.Context, calendarID string) (*WeatherZonesState, error)
	SetWeatherZones(ctx context.Context, calendarID string, state WeatherZonesState) error
	SetActiveWeatherZone(ctx context.Context, calendarID, zoneID string) error

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
	// UpdateCalendarVisibility sets a calendar's per-calendar visibility +
	// allow/deny rules (C-CAL-DASHBOARD-W5b). Validates like the event path;
	// bulk-replaces the rule set.
	UpdateCalendarVisibility(ctx context.Context, calendarID string, input UpdateCalendarVisibilityInput) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int, userID string) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int, userID string) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, limit int, role int, userID string) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int, userID string) ([]Event, error)
	ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int, userID string) ([]Event, error)
	// ListAllEventsForCalendar returns every event with no role
	// filter — for the public Foundry API only. See repository
	// comment for the rationale.
	ListAllEventsForCalendar(ctx context.Context, calendarID string) ([]Event, error)

	// Search.
	SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)

	// Date/time helpers.
	AdvanceDate(ctx context.Context, calendarID string, days int) error
	AdvanceTime(ctx context.Context, calendarID string, hours, minutes int) error
	SetDate(ctx context.Context, calendarID string, year, month, day, hour, minute int) error

	// Import/export.
	ApplyImport(ctx context.Context, calendarID string, result *ImportResult) error
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)

	// Entity ties (C-CAL-ENTITY-TIES-DATA-MODEL). Optional M:N both ways
	// with a participation role. This is the cross-plugin surface the
	// entities plugin consumes (via an interface, never a repo import) to
	// render the entity-side Events/Eras sections. Role is validated
	// against the four pinned ParticipationRoles; era roles are optional.
	LinkEntityToEvent(ctx context.Context, entityID, eventID, role string) error
	UnlinkEntityFromEvent(ctx context.Context, entityID, eventID string) error
	LinkEntityToEra(ctx context.Context, entityID string, eraID int, role *string) error
	UnlinkEntityFromEra(ctx context.Context, entityID string, eraID int) error
	EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error)
	ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error)
	EntitiesForEvent(ctx context.Context, eventID string) ([]EntityTieRef, error)
	EntitiesForEra(ctx context.Context, eraID int) ([]EntityTieRef, error)
	// EntitiesForCalendar lists the distinct entities tied to any event/era of
	// a calendar (the Calendars dashboard associations panel, W1).
	EntitiesForCalendar(ctx context.Context, calendarID string) ([]EntityTieRef, error)
	// World-state (C-CAL-WORLDSTATE-SERVER-MODEL). BuildWorldStateSeed
	// assembles the Part-8 seed for a date, filtering GM-only celestial
	// events by role/userID. SetWorldState persists the writable parts
	// (live mood + date/time) and emits calendar.worldstate.changed.
	BuildWorldStateSeed(ctx context.Context, calendarID string, year, month, day, role int, userID string) (*WorldStateSeed, error)
	SetWorldState(ctx context.Context, calendarID string, input WorldStateUpdateInput) error

	// Wiring.
	SetEventPublisher(pub CalendarEventPublisher)
}

// WorldStateUpdateInput is the writable slice of world-state the PUT exposes.
// A nil section is left untouched, so a caller can set mood without touching
// time and vice-versa. timepieceFill / timeControl are intentionally absent —
// they are ephemeral session state, never persisted (CATALOG Part 6).
type WorldStateUpdateInput struct {
	// Mood, when non-nil, writes the live mood-tint columns. Color nil +
	// Intensity 0 clears the wash.
	Mood *WorldStateMoodTint
	// Time, when non-nil, sets the calendar's current date/time. Any nil
	// sub-field preserves the current stored value.
	Time *WorldStateTimeSet
	// Advance, when non-nil, moves the clock RELATIVE to the current value
	// (signed — negative steps back), with full rollover across
	// minute→hour→day→month→year. This is the GM panel's Part-6 verb path
	// (+1hr / +1day / +long-rest / step-back); Time is the absolute
	// set-time/set-date path. Apply Advance OR Time, not both.
	Advance *WorldStateAdvance
	// Weather, when non-nil, sets the CURRENT date's authored weather type
	// (calendar_day_weather, #401). The GM panel's weather override (4b). A
	// thin string slice — no structural editing through it.
	Weather *string
	// TriggerEvent, when non-nil, adds a celestial event on the CURRENT date
	// (calendar_celestial_events). The GM panel's "trigger world-event" (4c).
	// Visibility is resolved by the handler from CanAuthorDmOnly before it
	// reaches the service (capability stays at the route/handler layer).
	TriggerEvent *WorldStateTriggerEvent
}

// WorldStateTriggerEvent is a GM-triggered celestial event (meteor shower /
// eclipse / blood moon) on the current day. Type is validated against the
// known set; Visibility is "everyone" or "dm_only" (already capability-checked
// by the handler).
type WorldStateTriggerEvent struct {
	Type          string
	Name          string
	StartHour     int
	DurationHours int
	Visibility    string
}

// WorldStateAdvance is a signed relative clock move for the GM panel verbs.
type WorldStateAdvance struct {
	Days    int
	Hours   int
	Minutes int
}

// WorldStateTimeSet is a partial date/time set for SetWorldState. Pointer
// fields distinguish "set to this value" from "leave unchanged".
type WorldStateTimeSet struct {
	Year   *int
	Month  *int
	Day    *int
	Hour   *int
	Minute *int
}

// CalendarEventPublisher emits domain events when calendar data changes.
// Implemented by the WebSocket EventBus adapter in routes.go.
type CalendarEventPublisher interface {
	PublishCalendarEvent(eventType, campaignID, resourceID string, payload any)
}

// NoopCalendarEventPublisher is a no-op implementation for tests.
type NoopCalendarEventPublisher struct{}

func (NoopCalendarEventPublisher) PublishCalendarEvent(string, string, string, any) {}

// BindingCleaner sweeps a deleted instance's widget bindings (widget-binding
// framework integrity hook, C-WIDGET-BINDING-P2). Implemented by
// widgetbindings.Service; injected via SetBindingCleaner so the calendar
// plugin doesn't hard-depend on the binding service for its core CRUD and
// tests can mock it. Optional — nil means "no binding framework wired" (the
// render-time guard + Sweep remain the backstop).
type BindingCleaner interface {
	OnInstanceDeleted(ctx context.Context, campaignID, widgetType, instanceID string) (int, error)
}

// calendarService is the default CalendarService implementation.
type calendarService struct {
	repo           CalendarRepository
	events         CalendarEventPublisher
	bindingCleaner BindingCleaner
}

// NewCalendarService creates a CalendarService backed by the given repository.
func NewCalendarService(repo CalendarRepository) CalendarService {
	return &calendarService{repo: repo, events: NoopCalendarEventPublisher{}}
}

// SetBindingCleaner injects the widget-binding cleanup hook (wired at app
// startup once the binding service exists). Reached via a type assertion in
// routes.go so the CalendarService interface stays unchanged.
func (s *calendarService) SetBindingCleaner(c BindingCleaner) { s.bindingCleaner = c }

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
	// Weather is best-effort: a missing row is a normal "no weather
	// set yet" state, not a load failure. Repo returns nil + nil err
	// in that case.
	if cal.Weather, err = s.repo.GetWeather(ctx, cal.ID); err != nil {
		return nil, fmt.Errorf("get weather: %w", err)
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
	// Mode is optional on update. Empty means "leave unchanged"; any non-empty
	// value must be a known mode constant. Mode is mutable post-create — the
	// model itself doesn't pin mode at creation time.
	if input.Mode != "" && input.Mode != ModeFantasy && input.Mode != ModeRealLife {
		return apperror.NewValidation("mode must be 'fantasy' or 'reallife'")
	}

	cal.Name = input.Name
	// Description + EpochName are pointer-typed (nullable in storage); nil
	// from a partial-update bind should mean "preserve current value", not
	// "blank to NULL". Without the guard, any UpdateCalendar call that
	// omits these fields (e.g. an advance-date UI that only sends date
	// fields) silently clears the operator's setup-time description and
	// epoch. Same class of bug C-PERMISSIONS-INLINE-COMPONENT fixed for
	// UpdateEntityInput.IsPrivate via *bool in chronicle#318. See audit at
	// reports/chronicle/2026-05-19-c-cal-null-preserve-audit.md §2.
	if input.Description != nil {
		cal.Description = input.Description
	}
	if input.EpochName != nil {
		cal.EpochName = input.EpochName
	}
	if input.Mode != "" {
		cal.Mode = input.Mode
	}
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

	// Widget-binding delete hook (C-WIDGET-BINDING-P2): a calendar id is the
	// instance for BOTH the "calendar" and "worldstate" widget types, so
	// deleting it must sweep both. Best-effort — the render-time orphan guard +
	// Sweep are the backstop if this misses.
	if s.bindingCleaner != nil {
		_, _ = s.bindingCleaner.OnInstanceDeleted(ctx, campaignID, WidgetTypeCalendar, calendarID)
		_, _ = s.bindingCleaner.OnInstanceDeleted(ctx, campaignID, WidgetTypeWorldstate, calendarID)
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

// ListVisibleCalendars returns only the campaign's calendars the viewer may
// see (C-CAL-DASHBOARD-W5a). Campaign-scoped via ListByCampaignID; visibility
// enforced in the service layer (not UI-only). Owner/co-DM get all.
func (s *calendarService) ListVisibleCalendars(ctx context.Context, campaignID string, role int, userID string) ([]Calendar, error) {
	cals, err := s.ListCalendars(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return filterCalendarsByUser(cals, role, userID), nil
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

// --- Active-calendar resolution (V2 Wave 1 PR 1) ---

// GetActiveCalendar resolves the user's currently-active calendar for
// a campaign. Resolution order:
//  1. Pointer in calendar_active (last switcher choice) — if it resolves to
//     a calendar still in the campaign.
//  2. Campaign default (`is_default = 1`).
//  3. First calendar by sort_order (fallback if no default exists yet).
//  4. nil if the campaign has zero calendars.
//
// Step 1's "still in the campaign" check covers a stale pointer (calendar
// deleted between switches; FK cascade clears the pointer, but during
// race conditions or post-failed-delete states a pointer may briefly
// point at a wrong-campaign calendar — defensively re-verify).
func (s *calendarService) GetActiveCalendar(ctx context.Context, userID, campaignID string) (*Calendar, error) {
	if userID != "" {
		ptrID, err := s.repo.GetActiveCalendarID(ctx, userID, campaignID)
		if err != nil {
			return nil, fmt.Errorf("get active calendar pointer: %w", err)
		}
		if ptrID != "" {
			cal, err := s.repo.GetByID(ctx, ptrID)
			if err == nil && cal != nil && cal.CampaignID == campaignID {
				return cal, nil
			}
			// Stale pointer; fall through to default.
		}
	}
	// Default fallback.
	cal, err := s.repo.GetDefaultByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("get default calendar: %w", err)
	}
	if cal != nil {
		return cal, nil
	}
	// No default set; take first by sort order so a campaign that never
	// got a default flagged still resolves to *something*.
	list, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list campaign calendars: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

// GetActiveVisibleCalendar resolves the viewer's active calendar but never
// returns one they may not see (C-CAL-DASHBOARD-W5a). If their active/default
// calendar is hidden from them, it falls back to the first calendar they can
// see. Owner/co-DM are unaffected (calendarVisibleTo passes for them).
func (s *calendarService) GetActiveVisibleCalendar(ctx context.Context, campaignID string, role int, userID string) (*Calendar, error) {
	cal, err := s.GetActiveCalendar(ctx, userID, campaignID)
	if err != nil {
		return nil, err
	}
	if cal != nil && calendarVisibleTo(cal, role, userID) {
		return cal, nil
	}
	// Active/default calendar is hidden from this viewer — fall back to the
	// first calendar they can see (nil if none).
	visible, err := s.ListVisibleCalendars(ctx, campaignID, role, userID)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, nil
	}
	return &visible[0], nil
}

// SwitchActiveCalendar persists the user's calendar choice. Validates
// IDOR (calendar must belong to campaign) before write. Subsequent
// GetActiveCalendar calls return the switched calendar.
func (s *calendarService) SwitchActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error {
	if userID == "" {
		return apperror.NewValidation("user_id required to switch active calendar")
	}
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
	return s.repo.SetActiveCalendar(ctx, userID, campaignID, calendarID)
}

// GetSidebarPinned returns the user's sidebar pin preference for the
// campaign. Anonymous users (empty user_id) default to TRUE — no
// per-anonymous persistence; just the platform default.
func (s *calendarService) GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error) {
	if userID == "" {
		return true, nil
	}
	return s.repo.GetSidebarPinned(ctx, userID, campaignID)
}

// SetSidebarPinned persists the user's sidebar pin preference.
// Empty user_id rejects — anonymous toggle isn't persisted (operator
// would just see default on next page load).
func (s *calendarService) SetSidebarPinned(ctx context.Context, userID, campaignID string, pinned bool) error {
	if userID == "" {
		return apperror.NewValidation("user_id required to set sidebar pin")
	}
	return s.repo.SetSidebarPinned(ctx, userID, campaignID, pinned)
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

// validateEraInput runs the same validation rules SetEras applies per row.
// Shared between bulk-set + per-era CRUD so AI Workspace Wave 3 imports
// hit identical validation regardless of import shape.
func validateEraInput(in *EraInput) error {
	if in.Name == "" {
		return apperror.NewValidation("era name is required")
	}
	if in.EndYear != nil && *in.EndYear < in.StartYear {
		return apperror.NewValidation(fmt.Sprintf("era %q: end year cannot be before start year", in.Name))
	}
	if in.Color == "" {
		in.Color = "#6366f1"
	}
	return nil
}

// CreateEra inserts a single era + publishes structure.updated.
func (s *calendarService) CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
	if err := validateEraInput(&input); err != nil {
		return nil, err
	}
	era, err := s.repo.CreateEra(ctx, calendarID, input)
	if err != nil {
		return nil, fmt.Errorf("create era: %w", err)
	}
	s.publishStructureUpdated(ctx, calendarID)
	return era, nil
}

// UpdateEra updates one era + publishes structure.updated. Looks up the
// existing row first so the WS publish has the calendar context and so
// "era not found" returns a clean NotFound.
func (s *calendarService) UpdateEra(ctx context.Context, eraID int, input EraInput) error {
	if err := validateEraInput(&input); err != nil {
		return err
	}
	existing, err := s.repo.GetEraByID(ctx, eraID)
	if err != nil {
		return fmt.Errorf("get era: %w", err)
	}
	if existing == nil {
		return apperror.NewNotFound("era not found")
	}
	if err := s.repo.UpdateEra(ctx, eraID, input); err != nil {
		return fmt.Errorf("update era: %w", err)
	}
	s.publishStructureUpdated(ctx, existing.CalendarID)
	return nil
}

// DeleteEra removes one era + publishes structure.updated for the calendar
// the era belonged to. Deleting a non-existent era is treated as a no-op
// (returns NotFound; consistent with existing Set* idempotency).
func (s *calendarService) DeleteEra(ctx context.Context, eraID int) error {
	calendarID, err := s.repo.DeleteEra(ctx, eraID)
	if err != nil {
		return fmt.Errorf("delete era: %w", err)
	}
	if calendarID == "" {
		return apperror.NewNotFound("era not found")
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
//
// Load-merge-write: any nil field on the input preserves the corresponding
// stored value rather than blanking it. Without this, a partial-save (the
// operator edits PresetID only via the settings tab; sync sends preset
// updates without re-asserting wind/precip) silently clears the unsent
// fields to NULL. Same class as C-PERMISSIONS-INLINE-COMPONENT's IsPrivate
// silent-flip; entities solved it with `*bool` at the input layer + a
// nil-guard at the service. Weather's input is already all-pointer, so the
// fix lives entirely at the service layer here. Audit at
// reports/chronicle/2026-05-19-c-cal-null-preserve-audit.md §2 + §3.
//
// One extra SELECT per write — fine for an Owner-only low-traffic edit.
// A future caller that wants explicit replace-all semantics should hit a
// DELETE endpoint first; "send all-nil" no longer round-trips as "blank
// everything" through this path.
func (s *calendarService) SetWeather(ctx context.Context, calendarID string, input WeatherInput) error {
	existing, err := s.repo.GetWeather(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("load weather for merge: %w", err)
	}
	merged := mergeWeatherInput(existing, input)
	if err := s.repo.SetWeather(ctx, calendarID, merged); err != nil {
		return fmt.Errorf("set weather: %w", err)
	}
	// Publish weather change event — payload reflects the merged state so
	// subscribers see the final value, not the sparse input.
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.weather.changed", cal.CampaignID, calendarID, merged)
	}
	return nil
}

// mergeWeatherInput overlays the non-nil fields of `in` on top of `existing`,
// returning the WeatherInput to persist. If `existing` is nil (no row yet),
// `in` is returned as-is (first write seeds the row directly).
//
// Pointer types throughout WeatherInput let us use nil as the "absent /
// preserve" signal. A future caller that wants to clear a single field
// would need a dedicated endpoint or explicit-null escape hatch; pointer
// types collapse "absent" and "explicit null" at the JSON-bind layer so the
// nil-preserve choice is the safe default. Same trade-off C-PERMISSIONS-
// INLINE-COMPONENT made for entities.IsPrivate.
func mergeWeatherInput(existing *Weather, in WeatherInput) WeatherInput {
	if existing == nil {
		return in
	}
	out := in
	if out.PresetID == nil {
		out.PresetID = existing.PresetID
	}
	if out.PresetLabel == nil {
		out.PresetLabel = existing.PresetLabel
	}
	if out.Icon == nil {
		out.Icon = existing.Icon
	}
	if out.Color == nil {
		out.Color = existing.Color
	}
	if out.TemperatureCelsius == nil {
		out.TemperatureCelsius = existing.TemperatureCelsius
	}
	if existing.Wind != nil {
		if out.WindSpeedKPH == nil {
			out.WindSpeedKPH = existing.Wind.SpeedKPH
		}
		if out.WindSpeedTier == nil {
			out.WindSpeedTier = existing.Wind.SpeedTier
		}
		if out.WindDirection == nil {
			out.WindDirection = existing.Wind.Direction
		}
		if out.WindDirectionDeg == nil {
			out.WindDirectionDeg = existing.Wind.DirectionDegrees
		}
	}
	if existing.Precipitation != nil {
		if out.PrecipitationType == nil {
			out.PrecipitationType = existing.Precipitation.Type
		}
		if out.PrecipitationIntensity == nil {
			out.PrecipitationIntensity = existing.Precipitation.Intensity
		}
	}
	if out.ZoneID == nil {
		out.ZoneID = existing.ZoneID
	}
	if out.ZoneName == nil {
		out.ZoneName = existing.ZoneName
	}
	if out.Description == nil {
		out.Description = existing.Description
	}
	return out
}

// --- Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES) ---

// weatherZoneIDPattern matches a slug-style zone id: lowercase alnum
// plus `_` and `-`, 1-50 chars. Mirrors the typical Chronicle slug shape
// (eras, event categories) without dragging in a separate slug helper.
var weatherZoneIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,50}$`)

// GetWeatherZones returns the catalog + the currently active zone
// reference (calendar_weather.zone_id) bundled into a single response.
// Either or both can be empty.
func (s *calendarService) GetWeatherZones(ctx context.Context, calendarID string) (*WeatherZonesState, error) {
	zones, err := s.repo.GetWeatherZones(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("load weather zones: %w", err)
	}
	active := ""
	if w, err := s.repo.GetWeather(ctx, calendarID); err == nil && w != nil && w.ZoneID != nil {
		active = *w.ZoneID
	}
	return &WeatherZonesState{ActiveZone: active, Zones: zones}, nil
}

// SetWeatherZones replaces the full zone catalog (REPLACE semantics
// matching SetMonths / SetSeasons / etc.). If ActiveZone is non-empty,
// the active-zone reference on calendar_weather is updated as part of
// the same operation; the reference must point at a zone in the new
// catalog. Publishes `calendar.weather.zones.changed` (catalog edit)
// and, when the active-zone changed, `calendar.weather.changed`
// (state edit, matching the existing SetWeather event).
func (s *calendarService) SetWeatherZones(ctx context.Context, calendarID string, state WeatherZonesState) error {
	// Validate every zone: id slug, non-empty name, payload shape.
	ids := make(map[string]struct{}, len(state.Zones))
	for i, z := range state.Zones {
		if !weatherZoneIDPattern.MatchString(z.ZoneID) {
			return apperror.NewValidation(fmt.Sprintf("zone %d: id %q must match [a-z0-9_-]{1,50}", i+1, z.ZoneID))
		}
		if _, dup := ids[z.ZoneID]; dup {
			return apperror.NewValidation(fmt.Sprintf("zone %d: id %q is duplicated", i+1, z.ZoneID))
		}
		ids[z.ZoneID] = struct{}{}
		if z.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("zone %q: name is required", z.ZoneID))
		}
		if err := validateWeatherZonePayload(z.Payload); err != nil {
			return apperror.NewValidation(fmt.Sprintf("zone %q: %s", z.ZoneID, err.Error()))
		}
	}
	if state.ActiveZone != "" {
		if _, ok := ids[state.ActiveZone]; !ok {
			return apperror.NewValidation(fmt.Sprintf("active_zone %q is not present in zones", state.ActiveZone))
		}
	}

	// Stamp calendar_id on each zone (handler may leave it blank since
	// the path already names the calendar) so the repo INSERTs the
	// correct value.
	zones := make([]WeatherZone, len(state.Zones))
	for i, z := range state.Zones {
		z.CalendarID = calendarID
		zones[i] = z
	}
	if err := s.repo.ApplyWeatherZones(ctx, calendarID, zones); err != nil {
		return fmt.Errorf("apply weather zones: %w", err)
	}

	// Active-zone reference: if the caller supplied an empty string,
	// we leave the existing reference alone (catalog-only edit). To
	// CLEAR the active zone the caller hits SetActiveWeatherZone("").
	activeChanged := false
	if state.ActiveZone != "" {
		// Lookup the zone name for the calendar_weather denormalized copy.
		var zoneName string
		for _, z := range zones {
			if z.ZoneID == state.ActiveZone {
				zoneName = z.Name
				break
			}
		}
		if err := s.repo.SetActiveWeatherZone(ctx, calendarID, state.ActiveZone, zoneName); err != nil {
			return fmt.Errorf("set active weather zone: %w", err)
		}
		activeChanged = true
	}

	cal, err := s.repo.GetByID(ctx, calendarID)
	if err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.weather.zones.changed", cal.CampaignID, calendarID, nil)
		if activeChanged {
			s.events.PublishCalendarEvent("calendar.weather.changed", cal.CampaignID, calendarID, nil)
		}
	}
	return nil
}

// SetActiveWeatherZone updates only the calendar_weather active-zone
// reference. Empty zoneID clears the active zone. The zoneID must
// already exist in the catalog (or be empty); we do not auto-create
// zones from this endpoint.
func (s *calendarService) SetActiveWeatherZone(ctx context.Context, calendarID, zoneID string) error {
	var zoneName string
	if zoneID != "" {
		zones, err := s.repo.GetWeatherZones(ctx, calendarID)
		if err != nil {
			return fmt.Errorf("load weather zones: %w", err)
		}
		found := false
		for _, z := range zones {
			if z.ZoneID == zoneID {
				zoneName = z.Name
				found = true
				break
			}
		}
		if !found {
			return apperror.NewValidation(fmt.Sprintf("zone %q not found in catalog", zoneID))
		}
	}
	if err := s.repo.SetActiveWeatherZone(ctx, calendarID, zoneID, zoneName); err != nil {
		return fmt.Errorf("set active weather zone: %w", err)
	}
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.weather.changed", cal.CampaignID, calendarID, nil)
	}
	return nil
}

// validateWeatherZonePayload sanity-checks the opaque zone payload.
// The full shape is owned by Calendaria/Foundry sync; we enforce the
// minimum that downstream code relies on:
//   - presets, if present, is an array and each entry has a non-empty
//     `label` (used in the picker) and a `temperature` number.
//   - season_overrides, if present, is an object (map).
//
// Everything else passes through verbatim — future schema growth on
// the Calendaria side doesn't need a Chronicle migration.
func validateWeatherZonePayload(payload map[string]any) error {
	if payload == nil {
		return nil
	}
	if raw, ok := payload["presets"]; ok && raw != nil {
		presets, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("payload.presets must be an array")
		}
		for i, p := range presets {
			m, ok := p.(map[string]any)
			if !ok {
				return fmt.Errorf("payload.presets[%d] must be an object", i)
			}
			label, _ := m["label"].(string)
			if label == "" {
				return fmt.Errorf("payload.presets[%d].label is required", i)
			}
			if _, ok := m["temperature"].(float64); !ok {
				return fmt.Errorf("payload.presets[%d].temperature must be a number", i)
			}
		}
	}
	if raw, ok := payload["season_overrides"]; ok && raw != nil {
		if _, ok := raw.(map[string]any); !ok {
			return fmt.Errorf("payload.season_overrides must be an object")
		}
	}
	return nil
}

// GetCycles returns all cycles for a calendar.
func (s *calendarService) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	return s.repo.GetCycles(ctx, calendarID)
}

// SetCycles replaces all cycles for a calendar.
//
// Emits two WS events per C-CAL-WS-DOTTED (2026-05-19): the umbrella
// calendar.structure.updated (for subscribers that just want "structure
// moved, refetch") plus the granular calendar.cycle.changed (for the
// Foundry-side editor's targeted refresh path).
func (s *calendarService) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	// Name validation added in C-CAL-WCF-UI to align with SetMonths /
	// SetWeekdays / SetMoons / SetSeasons / SetEras / SetEventCategories,
	// all of which already reject empty names with NewValidation. The
	// internal settings UI relies on this surface for inline error
	// rendering ("category: validation" → amber banner in the form).
	for i, c := range cycles {
		if c.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("cycle %d: name is required", i+1))
		}
	}
	if err := s.repo.SetCycles(ctx, calendarID, cycles); err != nil {
		return fmt.Errorf("set cycles: %w", err)
	}
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.structure.updated", cal.CampaignID, calendarID, nil)
		s.events.PublishCalendarEvent("calendar.cycle.changed", cal.CampaignID, calendarID, nil)
	}
	return nil
}

// GetFestivals returns all festivals for a calendar.
func (s *calendarService) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	return s.repo.GetFestivals(ctx, calendarID)
}

// SetFestivals replaces all festivals for a calendar.
//
// Mirrors SetCycles' double-emit: structure.updated for the broad
// listener + festival.changed for granular refresh. See C-CAL-WS-DOTTED.
func (s *calendarService) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	// Name + date validation added in C-CAL-WCF-UI. Mirrors the rest
	// of the Set* surface; the settings UI displays the resulting
	// "category: validation" payload inline so the operator doesn't
	// have to dig in server logs to find a typo.
	for i, f := range festivals {
		if f.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("festival %d: name is required", i+1))
		}
		// A festival is anchored either on a specific month+day or
		// between two months (after_month). Both empty leaves the
		// festival un-dated, which renders nowhere; reject it.
		if f.Month == nil && f.AfterMonth == nil {
			return apperror.NewValidation(fmt.Sprintf("festival %d: must have either month+day or after_month", i+1))
		}
	}
	if err := s.repo.SetFestivals(ctx, calendarID, festivals); err != nil {
		return fmt.Errorf("set festivals: %w", err)
	}
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.structure.updated", cal.CampaignID, calendarID, nil)
		s.events.PublishCalendarEvent("calendar.festival.changed", cal.CampaignID, calendarID, nil)
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
		Tier:                     input.Tier,
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

	// C-CAL-NULL-PRESERVE (chronicle PR for 2026-05-19 audit):
	// 18 pointer-typed input fields are nil-guarded so a partial-save
	// (e.g. title-only edit via the FM-CAL-EDITOR day inspector) doesn't
	// silently blank description, color, times, recurrence config, or
	// per-user visibility rules. Same pattern as
	// UpdateEntityInput.IsPrivate *bool from chronicle#318. Audit
	// context: reports/chronicle/2026-05-19-c-cal-null-preserve-audit.md
	// §2 risk matrix.
	//
	// Value-typed fields stay unguarded by design — they have no
	// "absent" semantic distinct from "false/default/empty":
	//   Name      — required; empty string would fail validation upstream.
	//   Year/Month/Day — required for an event edit.
	//   Visibility    — defaulted to evt.Visibility above when empty.
	//   IsRecurring   — bool: false IS the value, not "absent".
	//   AllDay        — same.
	evt.Name = input.Name
	if input.Description != nil {
		evt.Description = input.Description
	}
	// Sanitize rich text HTML if provided. nil input.DescriptionHTML
	// means "preserve current" (the nil-guard), not "clear". Empty-string
	// inside a non-nil pointer falls through to the else branch, which
	// preserves the original behavior of writing the (empty) value.
	if input.DescriptionHTML != nil {
		if *input.DescriptionHTML != "" {
			sanitized := sanitize.HTML(*input.DescriptionHTML)
			evt.DescriptionHTML = &sanitized
		} else {
			evt.DescriptionHTML = input.DescriptionHTML
		}
	}
	// EntityID intentionally NOT nil-guarded here pending C-ENTITY-LINK-
	// DESIGN (see cordinator/plans/BACKLOG.md). Today nil EntityID
	// continues to mean "clear the entity link" to preserve the existing
	// wire semantic until the entity-linking surface is reworked
	// holistically — including the deferred multi-entity N:M support
	// (C-CALENDAR-AUDIT Chunk 3) and cross-plugin link consistency. A
	// regression test (TestUpdateEvent_EntityIDStillClearsOnNil) pins
	// this deliberate non-fix so a future sweep can't accidentally
	// include it without revisiting the design dispatch. Audit context:
	// reports/chronicle/2026-05-19-c-cal-null-preserve-audit.md §5 risk #1.
	evt.EntityID = input.EntityID
	evt.Year = input.Year
	evt.Month = input.Month
	evt.Day = input.Day
	if input.StartHour != nil {
		evt.StartHour = input.StartHour
	}
	if input.StartMinute != nil {
		evt.StartMinute = input.StartMinute
	}
	if input.EndYear != nil {
		evt.EndYear = input.EndYear
	}
	if input.EndMonth != nil {
		evt.EndMonth = input.EndMonth
	}
	if input.EndDay != nil {
		evt.EndDay = input.EndDay
	}
	if input.EndHour != nil {
		evt.EndHour = input.EndHour
	}
	if input.EndMinute != nil {
		evt.EndMinute = input.EndMinute
	}
	evt.IsRecurring = input.IsRecurring
	if input.RecurrenceType != nil {
		evt.RecurrenceType = input.RecurrenceType
	}
	if input.RecurrenceInterval != nil {
		evt.RecurrenceInterval = input.RecurrenceInterval
	}
	if input.RecurrenceEndYear != nil {
		evt.RecurrenceEndYear = input.RecurrenceEndYear
	}
	if input.RecurrenceEndMonth != nil {
		evt.RecurrenceEndMonth = input.RecurrenceEndMonth
	}
	if input.RecurrenceEndDay != nil {
		evt.RecurrenceEndDay = input.RecurrenceEndDay
	}
	if input.RecurrenceMaxOccurrences != nil {
		evt.RecurrenceMaxOccurrences = input.RecurrenceMaxOccurrences
	}
	evt.Visibility = input.Visibility
	if input.VisibilityRules != nil {
		evt.VisibilityRules = input.VisibilityRules
	}
	if input.Category != nil {
		evt.Category = input.Category
	}
	// Tier nil-preserve (Wave 1.6 §D): nil leaves existing tier
	// untouched; explicit empty-string slug clears to platform default.
	if input.Tier != nil {
		evt.Tier = input.Tier
	}
	if input.Color != nil {
		evt.Color = input.Color
	}
	if input.Icon != nil {
		evt.Icon = input.Icon
	}
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

// ListAllEventsForCalendar returns every event for a calendar
// with no visibility filtering. Only intended for the public
// Foundry-facing API (C-CALENDAR-ENDPOINTS); UI paths should keep
// using the role-filtered variants.
func (s *calendarService) ListAllEventsForCalendar(ctx context.Context, calendarID string) ([]Event, error) {
	return s.repo.ListAllEvents(ctx, calendarID)
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

// UpdateCalendarVisibility validates + persists a calendar's visibility level
// and per-user rules (C-CAL-DASHBOARD-W5b). Mirrors UpdateEventVisibility: the
// base level must be everyone|dm_only, the rules must be valid JSON, and the
// write bulk-replaces visibility_rules. Per the W5a semantic, dm_only is a hard
// GM-gate (allow-list does not admit players); per-player access uses base
// 'everyone' + an allowed_users whitelist.
func (s *calendarService) UpdateCalendarVisibility(ctx context.Context, calendarID string, input UpdateCalendarVisibilityInput) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return err
	}
	return s.repo.UpdateCalendarVisibility(ctx, calendarID, input.Visibility, input.VisibilityRules)
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

// ApplyImport replaces a calendar's configuration with data from an
// ImportResult atomically. V1 ran 6 sequential service calls (Update +
// SetMonths + SetWeekdays + SetMoons + SetSeasons + SetEras), each with
// its own transaction; a partial failure (e.g., import file truncated
// after seasons) left the calendar with new months + new weekdays + new
// moons + new seasons + OLD eras — partial state operators couldn't
// recover from cleanly. V2 Wave 0 PR 2 wraps the entire workflow in a
// single repo-level transaction.
//
// Validation runs upfront — once the tx opens at the repo, we can't roll
// back cleanly for validation reasons (defeats the point). Per-resource
// validation mirrors the existing Set* methods exactly so AI Workspace
// Wave 3 + UI per-card-edit hit identical validation regardless of
// import shape.
//
// On success, a single structure.updated WS event publishes; consumers
// (UI / sync clients) refresh once rather than 6 times mid-import.
func (s *calendarService) ApplyImport(ctx context.Context, calendarID string, result *ImportResult) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	// Validate sub-resource inputs upfront. Reuse the same rules each
	// Set* method enforces; failing here keeps the tx unopened so the
	// calendar stays at its pre-import state.
	for i, m := range result.Months {
		if m.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("month %d: name is required", i+1))
		}
		if m.Days <= 0 {
			return apperror.NewValidation(fmt.Sprintf("month %q: days must be positive", m.Name))
		}
	}
	for i, w := range result.Weekdays {
		if w.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("weekday %d: name is required", i+1))
		}
	}
	for i, m := range result.Moons {
		if m.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("moon %d: name is required", i+1))
		}
		if m.CycleDays <= 0 {
			return apperror.NewValidation(fmt.Sprintf("moon %q: cycle_days must be positive", m.Name))
		}
	}
	for i, sn := range result.Seasons {
		if sn.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("season %d: name is required", i+1))
		}
	}
	for i, e := range result.Eras {
		if e.Name == "" {
			return apperror.NewValidation(fmt.Sprintf("era %d: name is required", i+1))
		}
		if e.EndYear != nil && *e.EndYear < e.StartYear {
			return apperror.NewValidation(fmt.Sprintf("era %q: end year cannot be before start year", e.Name))
		}
		if e.Color == "" {
			result.Eras[i].Color = "#6366f1"
		}
	}

	// Mutate cal to reflect import-side fields; repo.ApplyImport reads
	// these to UPDATE the calendars row within the tx.
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

	if err := s.repo.ApplyImport(ctx, cal, result); err != nil {
		return fmt.Errorf("apply import: %w", err)
	}

	s.publishStructureUpdated(ctx, calendarID)
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

// calendarVisibleTo reports whether a viewer may see a calendar
// (C-CAL-DASHBOARD-W5a). Owner/co-DM (CanSeeDmOnly) and the system context
// (empty userID) always pass; otherwise the calendar's own visibility + rules
// decide, via the SAME resolver events use (canUserView). Calendar visibility
// and event visibility compose: this gates the calendar; events inside a
// visible calendar are still filtered by filterEventsByUser.
func calendarVisibleTo(cal *Calendar, role int, userID string) bool {
	if cal == nil {
		return false
	}
	if permissions.CanSeeDmOnly(role) || userID == "" {
		return true
	}
	return canUserView(cal.Visibility, cal.VisibilityRules, role, userID)
}

// filterCalendarsByUser drops the calendars a viewer may not see, mirroring
// filterEventsByUser exactly (C-CAL-DASHBOARD-W5a). Owner/co-DM or the system
// context get the list unchanged.
func filterCalendarsByUser(cals []Calendar, role int, userID string) []Calendar {
	if permissions.CanSeeDmOnly(role) || userID == "" {
		return cals
	}
	filtered := cals[:0]
	for _, c := range cals {
		if canUserView(c.Visibility, c.VisibilityRules, role, userID) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

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

// publishStructureUpdated fires a calendar.structure.updated WS event.
func (s *calendarService) publishStructureUpdated(ctx context.Context, calendarID string) {
	if cal, err := s.repo.GetByID(ctx, calendarID); err == nil && cal != nil {
		s.events.PublishCalendarEvent("calendar.structure.updated", cal.CampaignID, calendarID, nil)
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
			s.events.PublishCalendarEvent("calendar.season.changed", cal.CampaignID, cal.ID, map[string]any{
				"id":    afterSeason.ID,
				"name":  afterSeason.Name,
				"color": afterSeason.Color,
			})
		}
	} else if beforeSeason != "" {
		// Left a season without entering a new one.
		s.events.PublishCalendarEvent("calendar.season.changed", cal.CampaignID, cal.ID, nil)
	}

	// Era change detection.
	if afterEra := cal.CurrentEra(); afterEra != nil {
		if afterEra.Name != beforeEra {
			s.events.PublishCalendarEvent("calendar.era.changed", cal.CampaignID, cal.ID, map[string]any{
				"id":    afterEra.ID,
				"name":  afterEra.Name,
				"color": afterEra.Color,
			})
		}
	}

	// Moon phase change detection.
	absDay := cal.CurrentAbsoluteDay()
	for i := range cal.Moons {
		moon := &cal.Moons[i]
		afterPhase := moon.MoonPhaseName(absDay)
		if afterPhase != beforeMoonPhases[moon.ID] {
			s.events.PublishCalendarEvent("calendar.moon.phase_changed", cal.CampaignID, cal.ID, map[string]any{
				"moon_id":        moon.ID,
				"moon_name":      moon.Name,
				"phase_name":     afterPhase,
				"phase_position": moon.MoonPhase(absDay),
			})
		}
	}
}
