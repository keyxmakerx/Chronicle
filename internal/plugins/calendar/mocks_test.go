// mocks_test.go: function-injection fakes for the four repository
// interfaces CalendarService sits over, shared by every *_test.go in this
// package. Mirrors internal/plugins/timeline/service_test.go's
// mockTimelineRepo shape: one struct per interface, one optional `xxxFn`
// field per method, a nil field falling back to a harmless zero-value
// default so a test only wires the calls it actually exercises.
package calendar

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- fakeCalendarRepo ---

type fakeCalendarRepo struct {
	createFn           func(ctx context.Context, cal *Calendar) error
	getByIDFn          func(ctx context.Context, id string) (*Calendar, error)
	getDefaultFn       func(ctx context.Context, campaignID string) (*Calendar, error)
	listByCampaignFn   func(ctx context.Context, campaignID string) ([]Calendar, error)
	setDefaultFn       func(ctx context.Context, campaignID, calendarID string) error
	updateFn           func(ctx context.Context, cal *Calendar) error
	deleteFn           func(ctx context.Context, id string) error
	updateVisibilityFn func(ctx context.Context, calendarID, visibility string, visRules *string) error
	setMonthsFn        func(ctx context.Context, calendarID string, months []MonthInput) error
	getMonthsFn        func(ctx context.Context, calendarID string) ([]Month, error)
	setWeekdaysFn      func(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	getWeekdaysFn      func(ctx context.Context, calendarID string) ([]Weekday, error)
	setMoonsFn         func(ctx context.Context, calendarID string, moons []MoonInput) error
	getMoonsFn         func(ctx context.Context, calendarID string) ([]Moon, error)
	setMoonHiddenFn    func(ctx context.Context, calendarID string, moonID int, hidden bool) error
	setSeasonsFn       func(ctx context.Context, calendarID string, seasons []Season) error
	getSeasonsFn       func(ctx context.Context, calendarID string) ([]Season, error)
	setErasFn          func(ctx context.Context, calendarID string, eras []EraInput) error
	createEraFn        func(ctx context.Context, calendarID string, input EraInput) (*Era, error)
	updateEraFn        func(ctx context.Context, calendarID string, eraID int, input EraInput) error
	deleteEraFn        func(ctx context.Context, calendarID string, eraID int) error
	getEraByIDFn       func(ctx context.Context, eraID int) (*Era, error)
	getErasFn          func(ctx context.Context, calendarID string) ([]Era, error)
	setCyclesFn        func(ctx context.Context, calendarID string, cycles []CycleInput) error
	getCyclesFn        func(ctx context.Context, calendarID string) ([]Cycle, error)
	setFestivalsFn     func(ctx context.Context, calendarID string, festivals []FestivalInput) error
	getFestivalsFn     func(ctx context.Context, calendarID string) ([]Festival, error)
	applyImportFn      func(ctx context.Context, cal *Calendar, result *ImportResult) error
}

func (m *fakeCalendarRepo) Create(ctx context.Context, cal *Calendar) error {
	if m.createFn != nil {
		return m.createFn(ctx, cal)
	}
	return nil
}
func (m *fakeCalendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("calendar not found")
}
func (m *fakeCalendarRepo) GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	if m.getDefaultFn != nil {
		return m.getDefaultFn(ctx, campaignID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetDefault(ctx context.Context, campaignID, calendarID string) error {
	if m.setDefaultFn != nil {
		return m.setDefaultFn(ctx, campaignID, calendarID)
	}
	return nil
}
func (m *fakeCalendarRepo) Update(ctx context.Context, cal *Calendar) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, cal)
	}
	return nil
}
func (m *fakeCalendarRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *fakeCalendarRepo) UpdateVisibility(ctx context.Context, calendarID, visibility string, visRules *string) error {
	if m.updateVisibilityFn != nil {
		return m.updateVisibilityFn(ctx, calendarID, visibility, visRules)
	}
	return nil
}
func (m *fakeCalendarRepo) SetMonths(ctx context.Context, calendarID string, months []MonthInput) error {
	if m.setMonthsFn != nil {
		return m.setMonthsFn(ctx, calendarID, months)
	}
	return nil
}
func (m *fakeCalendarRepo) GetMonths(ctx context.Context, calendarID string) ([]Month, error) {
	if m.getMonthsFn != nil {
		return m.getMonthsFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error {
	if m.setWeekdaysFn != nil {
		return m.setWeekdaysFn(ctx, calendarID, weekdays)
	}
	return nil
}
func (m *fakeCalendarRepo) GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error) {
	if m.getWeekdaysFn != nil {
		return m.getWeekdaysFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	if m.setMoonsFn != nil {
		return m.setMoonsFn(ctx, calendarID, moons)
	}
	return nil
}
func (m *fakeCalendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	if m.getMoonsFn != nil {
		return m.getMoonsFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetMoonHidden(ctx context.Context, calendarID string, moonID int, hidden bool) error {
	if m.setMoonHiddenFn != nil {
		return m.setMoonHiddenFn(ctx, calendarID, moonID, hidden)
	}
	return nil
}
func (m *fakeCalendarRepo) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	if m.setSeasonsFn != nil {
		return m.setSeasonsFn(ctx, calendarID, seasons)
	}
	return nil
}
func (m *fakeCalendarRepo) GetSeasons(ctx context.Context, calendarID string) ([]Season, error) {
	if m.getSeasonsFn != nil {
		return m.getSeasonsFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	if m.setErasFn != nil {
		return m.setErasFn(ctx, calendarID, eras)
	}
	return nil
}
func (m *fakeCalendarRepo) CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
	if m.createEraFn != nil {
		return m.createEraFn(ctx, calendarID, input)
	}
	return &Era{CalendarID: calendarID, Name: input.Name}, nil
}
func (m *fakeCalendarRepo) UpdateEra(ctx context.Context, calendarID string, eraID int, input EraInput) error {
	if m.updateEraFn != nil {
		return m.updateEraFn(ctx, calendarID, eraID, input)
	}
	return nil
}
func (m *fakeCalendarRepo) DeleteEra(ctx context.Context, calendarID string, eraID int) error {
	if m.deleteEraFn != nil {
		return m.deleteEraFn(ctx, calendarID, eraID)
	}
	return nil
}
func (m *fakeCalendarRepo) GetEraByID(ctx context.Context, eraID int) (*Era, error) {
	if m.getEraByIDFn != nil {
		return m.getEraByIDFn(ctx, eraID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) GetEras(ctx context.Context, calendarID string) ([]Era, error) {
	if m.getErasFn != nil {
		return m.getErasFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	if m.setCyclesFn != nil {
		return m.setCyclesFn(ctx, calendarID, cycles)
	}
	return nil
}
func (m *fakeCalendarRepo) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	if m.getCyclesFn != nil {
		return m.getCyclesFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	if m.setFestivalsFn != nil {
		return m.setFestivalsFn(ctx, calendarID, festivals)
	}
	return nil
}
func (m *fakeCalendarRepo) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	if m.getFestivalsFn != nil {
		return m.getFestivalsFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeCalendarRepo) ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error {
	if m.applyImportFn != nil {
		return m.applyImportFn(ctx, cal, result)
	}
	return nil
}

// --- fakeEventRepo ---

type fakeEventRepo struct {
	createEventFn         func(ctx context.Context, evt *Event) error
	getEventFn            func(ctx context.Context, id string) (*Event, error)
	getEventsByIDsFn      func(ctx context.Context, calendarID string, ids []string) ([]Event, error)
	updateEventFn         func(ctx context.Context, evt *Event) error
	deleteEventFn         func(ctx context.Context, id string) error
	listForMonthFn        func(ctx context.Context, calendarID string, year, month, role int) ([]Event, error)
	listForYearFn         func(ctx context.Context, calendarID string, year, role int) ([]Event, error)
	listForDateRangeFn    func(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay, role int) ([]Event, error)
	listForEntityFn       func(ctx context.Context, entityID string, role int) ([]Event, error)
	listUpcomingFn        func(ctx context.Context, calendarID string, year, month, day, role, limit int) ([]Event, error)
	searchFn              func(ctx context.Context, calendarID, query string, role int) ([]Event, error)
	listAllFn             func(ctx context.Context, calendarID string) ([]Event, error)
	strandedCountsFn      func(ctx context.Context, campaignID string) (map[string]int, error)
	eventDatesFn          func(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error)
	updateVisibilityFn    func(ctx context.Context, eventID, visibility string, visRules *string) error
	linkEntityEventFn     func(ctx context.Context, entityID, eventID, role string) error
	unlinkEntityEventFn   func(ctx context.Context, entityID, eventID string) error
	linkEntityEraFn       func(ctx context.Context, entityID string, eraID int, role *string) error
	unlinkEntityEraFn     func(ctx context.Context, entityID string, eraID int) error
	entitiesForEventFn    func(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error)
	entitiesForEraFn      func(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error)
	entitiesForCalendarFn func(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error)
	eventsForEntityFn     func(ctx context.Context, campaignID, entityID string, role int) ([]EntityEventTie, error)
	erasForEntityFn       func(ctx context.Context, campaignID, entityID string) ([]EntityEraTie, error)
}

func (m *fakeEventRepo) CreateEvent(ctx context.Context, evt *Event) error {
	if m.createEventFn != nil {
		return m.createEventFn(ctx, evt)
	}
	return nil
}
func (m *fakeEventRepo) GetEvent(ctx context.Context, id string) (*Event, error) {
	if m.getEventFn != nil {
		return m.getEventFn(ctx, id)
	}
	return nil, nil
}
func (m *fakeEventRepo) GetEventsByIDs(ctx context.Context, calendarID string, ids []string) ([]Event, error) {
	if m.getEventsByIDsFn != nil {
		return m.getEventsByIDsFn(ctx, calendarID, ids)
	}
	return nil, nil
}
func (m *fakeEventRepo) UpdateEvent(ctx context.Context, evt *Event) error {
	if m.updateEventFn != nil {
		return m.updateEventFn(ctx, evt)
	}
	return nil
}
func (m *fakeEventRepo) DeleteEvent(ctx context.Context, id string) error {
	if m.deleteEventFn != nil {
		return m.deleteEventFn(ctx, id)
	}
	return nil
}
func (m *fakeEventRepo) ListEventsForMonth(ctx context.Context, calendarID string, year, month, role int) ([]Event, error) {
	if m.listForMonthFn != nil {
		return m.listForMonthFn(ctx, calendarID, year, month, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ListEventsForYear(ctx context.Context, calendarID string, year, role int) ([]Event, error) {
	if m.listForYearFn != nil {
		return m.listForYearFn(ctx, calendarID, year, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay, role int) ([]Event, error) {
	if m.listForDateRangeFn != nil {
		return m.listForDateRangeFn(ctx, calendarID, year, startMonth, startDay, endMonth, endDay, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	if m.listForEntityFn != nil {
		return m.listForEntityFn(ctx, entityID, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day, role, limit int) ([]Event, error) {
	if m.listUpcomingFn != nil {
		return m.listUpcomingFn(ctx, calendarID, year, month, day, role, limit)
	}
	return nil, nil
}
func (m *fakeEventRepo) SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, calendarID, query, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ListAllEvents(ctx context.Context, calendarID string) ([]Event, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeEventRepo) StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error) {
	if m.strandedCountsFn != nil {
		return m.strandedCountsFn(ctx, campaignID)
	}
	return nil, nil
}
func (m *fakeEventRepo) EventDatesForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error) {
	if m.eventDatesFn != nil {
		return m.eventDatesFn(ctx, calIDs, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) UpdateEventVisibility(ctx context.Context, eventID, visibility string, visRules *string) error {
	if m.updateVisibilityFn != nil {
		return m.updateVisibilityFn(ctx, eventID, visibility, visRules)
	}
	return nil
}
func (m *fakeEventRepo) LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error {
	if m.linkEntityEventFn != nil {
		return m.linkEntityEventFn(ctx, entityID, eventID, role)
	}
	return nil
}
func (m *fakeEventRepo) UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error {
	if m.unlinkEntityEventFn != nil {
		return m.unlinkEntityEventFn(ctx, entityID, eventID)
	}
	return nil
}
func (m *fakeEventRepo) LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error {
	if m.linkEntityEraFn != nil {
		return m.linkEntityEraFn(ctx, entityID, eraID, role)
	}
	return nil
}
func (m *fakeEventRepo) UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error {
	if m.unlinkEntityEraFn != nil {
		return m.unlinkEntityEraFn(ctx, entityID, eraID)
	}
	return nil
}
func (m *fakeEventRepo) EntitiesForEvent(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error) {
	if m.entitiesForEventFn != nil {
		return m.entitiesForEventFn(ctx, eventID, role, userID)
	}
	return nil, nil
}
func (m *fakeEventRepo) EntitiesForEra(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error) {
	if m.entitiesForEraFn != nil {
		return m.entitiesForEraFn(ctx, eraID, role, userID)
	}
	return nil, nil
}
func (m *fakeEventRepo) EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
	if m.entitiesForCalendarFn != nil {
		return m.entitiesForCalendarFn(ctx, calendarID, role, userID)
	}
	return nil, nil
}
func (m *fakeEventRepo) EventsForEntity(ctx context.Context, campaignID, entityID string, role int) ([]EntityEventTie, error) {
	if m.eventsForEntityFn != nil {
		return m.eventsForEntityFn(ctx, campaignID, entityID, role)
	}
	return nil, nil
}
func (m *fakeEventRepo) ErasForEntity(ctx context.Context, campaignID, entityID string) ([]EntityEraTie, error) {
	if m.erasForEntityFn != nil {
		return m.erasForEntityFn(ctx, campaignID, entityID)
	}
	return nil, nil
}

// --- fakeEventKindRepo ---

type fakeEventKindRepo struct {
	createFn  func(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error)
	updateFn  func(ctx context.Context, id int, campaignID string, input EventKindInput) error
	deleteFn  func(ctx context.Context, id int, campaignID string) error
	getByIDFn func(ctx context.Context, id int, campaignID string) (*EventKind, error)
	listFn    func(ctx context.Context, campaignID string) ([]EventKind, error)
}

func (m *fakeEventKindRepo) Create(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error) {
	if m.createFn != nil {
		return m.createFn(ctx, campaignID, input)
	}
	return &EventKind{CampaignID: campaignID, Slug: input.Slug, Name: input.Name}, nil
}
func (m *fakeEventKindRepo) Update(ctx context.Context, id int, campaignID string, input EventKindInput) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, campaignID, input)
	}
	return nil
}
func (m *fakeEventKindRepo) Delete(ctx context.Context, id int, campaignID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, campaignID)
	}
	return nil
}
func (m *fakeEventKindRepo) GetByID(ctx context.Context, id int, campaignID string) (*EventKind, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id, campaignID)
	}
	return nil, nil
}
func (m *fakeEventKindRepo) List(ctx context.Context, campaignID string) ([]EventKind, error) {
	if m.listFn != nil {
		return m.listFn(ctx, campaignID)
	}
	return nil, nil
}

// --- fakeWeatherRepo ---

type fakeWeatherRepo struct {
	getFn func(ctx context.Context, calendarID string) (*Weather, error)
	setFn func(ctx context.Context, calendarID string, input WeatherInput) error

	listDaysFn  func(ctx context.Context, calendarID string, year, month int) ([]DayWeather, error)
	setDaysFn   func(ctx context.Context, calendarID string, days []DayWeatherInput) error
	clearDaysFn func(ctx context.Context, calendarID string, dates []DayDate) error
}

func (m *fakeWeatherRepo) Get(ctx context.Context, calendarID string) (*Weather, error) {
	if m.getFn != nil {
		return m.getFn(ctx, calendarID)
	}
	return nil, nil
}
func (m *fakeWeatherRepo) Set(ctx context.Context, calendarID string, input WeatherInput) error {
	if m.setFn != nil {
		return m.setFn(ctx, calendarID, input)
	}
	return nil
}

func (m *fakeWeatherRepo) ListDays(ctx context.Context, calendarID string, year, month int) ([]DayWeather, error) {
	if m.listDaysFn != nil {
		return m.listDaysFn(ctx, calendarID, year, month)
	}
	return nil, nil
}
func (m *fakeWeatherRepo) SetDays(ctx context.Context, calendarID string, days []DayWeatherInput) error {
	if m.setDaysFn != nil {
		return m.setDaysFn(ctx, calendarID, days)
	}
	return nil
}
func (m *fakeWeatherRepo) ClearDays(ctx context.Context, calendarID string, dates []DayDate) error {
	if m.clearDaysFn != nil {
		return m.clearDaysFn(ctx, calendarID, dates)
	}
	return nil
}

// newTestCalendarService builds a CalendarService over the four fakes, for
// tests that only need to override a subset of methods.
func newTestCalendarService(calRepo *fakeCalendarRepo, eventRepo *fakeEventRepo, kindRepo *fakeEventKindRepo, weatherRepo *fakeWeatherRepo) CalendarService {
	if calRepo == nil {
		calRepo = &fakeCalendarRepo{}
	}
	if eventRepo == nil {
		eventRepo = &fakeEventRepo{}
	}
	if kindRepo == nil {
		kindRepo = &fakeEventKindRepo{}
	}
	if weatherRepo == nil {
		weatherRepo = &fakeWeatherRepo{}
	}
	return NewCalendarService(calRepo, eventRepo, kindRepo, weatherRepo)
}
