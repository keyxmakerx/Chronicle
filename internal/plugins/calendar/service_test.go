package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockCalendarRepo struct {
	createFn             func(ctx context.Context, cal *Calendar) error
	getByCampaignIDFn    func(ctx context.Context, campaignID string) (*Calendar, error)
	getByIDFn            func(ctx context.Context, id string) (*Calendar, error)
	updateFn             func(ctx context.Context, cal *Calendar) error
	deleteFn             func(ctx context.Context, id string) error
	setMonthsFn          func(ctx context.Context, calendarID string, months []MonthInput) error
	getMonthsFn          func(ctx context.Context, calendarID string) ([]Month, error)
	setWeekdaysFn        func(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	getWeekdaysFn        func(ctx context.Context, calendarID string) ([]Weekday, error)
	setMoonsFn           func(ctx context.Context, calendarID string, moons []MoonInput) error
	getMoonsFn           func(ctx context.Context, calendarID string) ([]Moon, error)
	setSeasonsFn         func(ctx context.Context, calendarID string, seasons []Season) error
	getSeasonsFn         func(ctx context.Context, calendarID string) ([]Season, error)
	setErasFn            func(ctx context.Context, calendarID string, eras []EraInput) error
	getErasFn            func(ctx context.Context, calendarID string) ([]Era, error)
	setEventCategoriesFn func(ctx context.Context, calendarID string, cats []EventCategoryInput) error
	getEventCategoriesFn func(ctx context.Context, calendarID string) ([]EventCategory, error)
	createEventFn        func(ctx context.Context, evt *Event) error
	getEventFn           func(ctx context.Context, id string) (*Event, error)
	updateEventFn        func(ctx context.Context, evt *Event) error
	deleteEventFn        func(ctx context.Context, id string) error
	listEventsForMonthFn func(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	listEventsForYearFn  func(ctx context.Context, calendarID string, year int, role int) ([]Event, error)
	listEventsForDateRangeFn func(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error)
	listEventsForEntityFn func(ctx context.Context, entityID string, role int) ([]Event, error)
	listUpcomingEventsFn func(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error)
	searchEventsFn       func(ctx context.Context, calendarID, query string, role int) ([]Event, error)
	updateEventVisFn     func(ctx context.Context, eventID string, visibility string, visRules *string) error
	listByCampaignIDFn   func(ctx context.Context, campaignID string) ([]Calendar, error)
}

func (m *mockCalendarRepo) Create(ctx context.Context, cal *Calendar) error {
	if m.createFn != nil {
		return m.createFn(ctx, cal)
	}
	return nil
}

func (m *mockCalendarRepo) GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	if m.getByCampaignIDFn != nil {
		return m.getByCampaignIDFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	// Falls back to GetByCampaignID for test backward compat.
	if m.getByCampaignIDFn != nil {
		return m.getByCampaignIDFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error) {
	if m.listByCampaignIDFn != nil {
		return m.listByCampaignIDFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetDefault(ctx context.Context, campaignID, calendarID string) error {
	return nil
}

func (m *mockCalendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCalendarRepo) Update(ctx context.Context, cal *Calendar) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, cal)
	}
	return nil
}

func (m *mockCalendarRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockCalendarRepo) SetMonths(ctx context.Context, calendarID string, months []MonthInput) error {
	if m.setMonthsFn != nil {
		return m.setMonthsFn(ctx, calendarID, months)
	}
	return nil
}

func (m *mockCalendarRepo) GetMonths(ctx context.Context, calendarID string) ([]Month, error) {
	if m.getMonthsFn != nil {
		return m.getMonthsFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error {
	if m.setWeekdaysFn != nil {
		return m.setWeekdaysFn(ctx, calendarID, weekdays)
	}
	return nil
}

func (m *mockCalendarRepo) GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error) {
	if m.getWeekdaysFn != nil {
		return m.getWeekdaysFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	if m.setMoonsFn != nil {
		return m.setMoonsFn(ctx, calendarID, moons)
	}
	return nil
}

func (m *mockCalendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	if m.getMoonsFn != nil {
		return m.getMoonsFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	if m.setSeasonsFn != nil {
		return m.setSeasonsFn(ctx, calendarID, seasons)
	}
	return nil
}

func (m *mockCalendarRepo) GetSeasons(ctx context.Context, calendarID string) ([]Season, error) {
	if m.getSeasonsFn != nil {
		return m.getSeasonsFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	if m.setErasFn != nil {
		return m.setErasFn(ctx, calendarID, eras)
	}
	return nil
}

func (m *mockCalendarRepo) GetEras(ctx context.Context, calendarID string) ([]Era, error) {
	if m.getErasFn != nil {
		return m.getErasFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error {
	if m.setEventCategoriesFn != nil {
		return m.setEventCategoriesFn(ctx, calendarID, cats)
	}
	return nil
}

func (m *mockCalendarRepo) GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error) {
	if m.getEventCategoriesFn != nil {
		return m.getEventCategoriesFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) CreateEvent(ctx context.Context, evt *Event) error {
	if m.createEventFn != nil {
		return m.createEventFn(ctx, evt)
	}
	return nil
}

func (m *mockCalendarRepo) GetEvent(ctx context.Context, id string) (*Event, error) {
	if m.getEventFn != nil {
		return m.getEventFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCalendarRepo) UpdateEvent(ctx context.Context, evt *Event) error {
	if m.updateEventFn != nil {
		return m.updateEventFn(ctx, evt)
	}
	return nil
}

func (m *mockCalendarRepo) DeleteEvent(ctx context.Context, id string) error {
	if m.deleteEventFn != nil {
		return m.deleteEventFn(ctx, id)
	}
	return nil
}

func (m *mockCalendarRepo) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error) {
	if m.listEventsForMonthFn != nil {
		return m.listEventsForMonthFn(ctx, calendarID, year, month, role)
	}
	return nil, nil
}

func (m *mockCalendarRepo) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	if m.listEventsForYearFn != nil {
		return m.listEventsForYearFn(ctx, calendarID, year, role)
	}
	return nil, nil
}

func (m *mockCalendarRepo) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error) {
	if m.listEventsForDateRangeFn != nil {
		return m.listEventsForDateRangeFn(ctx, calendarID, year, startMonth, startDay, endMonth, endDay, role)
	}
	return nil, nil
}

func (m *mockCalendarRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	if m.listEventsForEntityFn != nil {
		return m.listEventsForEntityFn(ctx, entityID, role)
	}
	return nil, nil
}

func (m *mockCalendarRepo) ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error) {
	if m.listUpcomingEventsFn != nil {
		return m.listUpcomingEventsFn(ctx, calendarID, year, month, day, role, limit)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error) {
	if m.searchEventsFn != nil {
		return m.searchEventsFn(ctx, calendarID, query, role)
	}
	return nil, nil
}

func (m *mockCalendarRepo) UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error {
	if m.updateEventVisFn != nil {
		return m.updateEventVisFn(ctx, eventID, visibility, visRules)
	}
	return nil
}

func (m *mockCalendarRepo) GetWeather(ctx context.Context, calendarID string) (*Weather, error) {
	return nil, nil
}

func (m *mockCalendarRepo) SetWeather(ctx context.Context, calendarID string, input WeatherInput) error {
	return nil
}

func (m *mockCalendarRepo) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	return nil, nil
}

func (m *mockCalendarRepo) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	return nil
}

func (m *mockCalendarRepo) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	return nil, nil
}

func (m *mockCalendarRepo) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	return nil
}

// --- Test Helpers ---

func newTestCalendarService(repo *mockCalendarRepo) CalendarService {
	return NewCalendarService(repo)
}

func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status code %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- CreateCalendar Tests ---

func TestCreateCalendar_FantasySuccess(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	cal, err := svc.CreateCalendar(context.Background(), "camp-1", CreateCalendarInput{
		Mode: ModeFantasy,
		Name: "Arcane Calendar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal == nil {
		t.Fatal("expected calendar, got nil")
	}
	if cal.Name != "Arcane Calendar" {
		t.Errorf("expected name 'Arcane Calendar', got %q", cal.Name)
	}
	if cal.CampaignID != "camp-1" {
		t.Errorf("expected campaign_id 'camp-1', got %q", cal.CampaignID)
	}
	if cal.ID == "" {
		t.Error("expected a generated UUID, got empty string")
	}
	if cal.Mode != ModeFantasy {
		t.Errorf("expected mode 'fantasy', got %q", cal.Mode)
	}
	if cal.HoursPerDay != 24 {
		t.Errorf("expected default hours_per_day 24, got %d", cal.HoursPerDay)
	}
}

func TestCreateCalendar_DefaultName(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	cal, err := svc.CreateCalendar(context.Background(), "camp-1", CreateCalendarInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal.Name != "Campaign Calendar" {
		t.Errorf("expected default name 'Campaign Calendar', got %q", cal.Name)
	}
}

func TestCreateCalendar_RepoError(t *testing.T) {
	repo := &mockCalendarRepo{
		createFn: func(_ context.Context, _ *Calendar) error {
			return errors.New("db error")
		},
	}
	svc := newTestCalendarService(repo)

	_, err := svc.CreateCalendar(context.Background(), "camp-1", CreateCalendarInput{
		Name: "Calendar",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetCalendar Tests ---

func TestGetCalendar_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "World Calendar"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	cal, err := svc.GetCalendar(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal == nil {
		t.Fatal("expected calendar, got nil")
	}
	if cal.ID != "cal-1" {
		t.Errorf("expected ID 'cal-1', got %q", cal.ID)
	}
}

func TestGetCalendar_NotExists(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	cal, err := svc.GetCalendar(context.Background(), "camp-none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal != nil {
		t.Error("expected nil calendar for missing campaign")
	}
}

func TestGetCalendarByID_NotExists(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	cal, err := svc.GetCalendarByID(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal != nil {
		t.Error("expected nil calendar")
	}
}

// --- UpdateCalendar Tests ---

func TestUpdateCalendar_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", Name: "Old", CampaignID: "camp-1"}, nil
		},
		updateFn: func(_ context.Context, cal *Calendar) error {
			if cal.Name != "New Name" {
				t.Errorf("expected name 'New Name', got %q", cal.Name)
			}
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "New Name",
		CurrentYear:      1200,
		CurrentMonth:     3,
		CurrentDay:       15,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateCalendar_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "nonexistent", UpdateCalendarInput{
		Name:             "X",
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	assertAppError(t, err, 404)
}

func TestUpdateCalendar_InvalidHoursPerDay(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "X",
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      0, // invalid
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	assertAppError(t, err, 422)
}

func TestUpdateCalendar_InvalidCurrentMonth(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "X",
		CurrentMonth:     0, // invalid
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	assertAppError(t, err, 422)
}

// TestUpdateCalendar_AcceptsMode pins C-CAL1 Gap 1: a non-empty `mode` field
// on the input is validated and assigned to the calendar. Without this,
// Foundry's Calendaria bootstrap can't complete because the mode flip from
// fantasy → reallife (or vice versa) silently drops.
func TestUpdateCalendar_AcceptsMode(t *testing.T) {
	var captured *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", Mode: ModeFantasy, CampaignID: "camp-1"}, nil
		},
		updateFn: func(_ context.Context, cal *Calendar) error {
			captured = cal
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "Greg",
		Mode:             ModeRealLife,
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("update was not invoked")
	}
	if captured.Mode != ModeRealLife {
		t.Errorf("expected Mode=%q, got %q", ModeRealLife, captured.Mode)
	}
}

// TestUpdateCalendar_RejectsInvalidMode pins the validation guard. An
// unknown mode string must not silently drop or get assigned — return 422.
func TestUpdateCalendar_RejectsInvalidMode(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", Mode: ModeFantasy}, nil
		},
		updateFn: func(_ context.Context, _ *Calendar) error {
			t.Fatal("update should not be invoked when mode is invalid")
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "X",
		Mode:             "gibberish",
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	assertAppError(t, err, 422)
}

// TestUpdateCalendar_EmptyModeLeavesUnchanged pins the "empty means leave
// unchanged" contract — clients that don't send `mode` must not have the
// stored mode clobbered to the empty string.
func TestUpdateCalendar_EmptyModeLeavesUnchanged(t *testing.T) {
	var captured *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", Mode: ModeRealLife}, nil
		},
		updateFn: func(_ context.Context, cal *Calendar) error {
			captured = cal
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateCalendar(context.Background(), "cal-1", UpdateCalendarInput{
		Name:             "X",
		// Mode intentionally omitted.
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("update was not invoked")
	}
	if captured.Mode != ModeRealLife {
		t.Errorf("Mode was clobbered: expected %q (unchanged), got %q", ModeRealLife, captured.Mode)
	}
}

// --- DeleteCalendar Tests ---

func TestDeleteCalendar_Success(t *testing.T) {
	deleted := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1"}, nil
		},
		deleteFn: func(_ context.Context, id string) error {
			deleted = true
			if id != "cal-1" {
				t.Errorf("expected delete ID 'cal-1', got %q", id)
			}
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.DeleteCalendar(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected Delete to be called on repo")
	}
}

// --- SetMonths Tests ---

func TestSetMonths_Success(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.SetMonths(context.Background(), "cal-1", []MonthInput{
		{Name: "January", Days: 31, SortOrder: 0},
		{Name: "February", Days: 28, SortOrder: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetMonths_Empty(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMonths(context.Background(), "cal-1", []MonthInput{})
	assertAppError(t, err, 422)
}

func TestSetMonths_MissingName(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMonths(context.Background(), "cal-1", []MonthInput{
		{Name: "", Days: 30},
	})
	assertAppError(t, err, 422)
}

func TestSetMonths_InvalidDays(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMonths(context.Background(), "cal-1", []MonthInput{
		{Name: "Bad", Days: 0},
	})
	assertAppError(t, err, 422)
}

func TestSetMonths_NegativeLeapYearDays(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMonths(context.Background(), "cal-1", []MonthInput{
		{Name: "Bad", Days: 30, LeapYearDays: -1},
	})
	assertAppError(t, err, 422)
}

// --- SetWeekdays Tests ---

func TestSetWeekdays_Empty(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetWeekdays(context.Background(), "cal-1", []WeekdayInput{})
	assertAppError(t, err, 422)
}

func TestSetWeekdays_MissingName(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetWeekdays(context.Background(), "cal-1", []WeekdayInput{
		{Name: ""},
	})
	assertAppError(t, err, 422)
}

// --- SetMoons Tests ---

func TestSetMoons_InvalidCycleDays(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMoons(context.Background(), "cal-1", []MoonInput{
		{Name: "Luna", CycleDays: 0},
	})
	assertAppError(t, err, 422)
}

func TestSetMoons_MissingName(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetMoons(context.Background(), "cal-1", []MoonInput{
		{Name: "", CycleDays: 29},
	})
	assertAppError(t, err, 422)
}

// --- SetEras Tests ---

func TestSetEras_EndBeforeStart(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	endYear := 50
	err := svc.SetEras(context.Background(), "cal-1", []EraInput{
		{Name: "Bad Era", StartYear: 100, EndYear: &endYear},
	})
	assertAppError(t, err, 422)
}

func TestSetEras_DefaultColor(t *testing.T) {
	var savedEras []EraInput
	repo := &mockCalendarRepo{
		setErasFn: func(_ context.Context, _ string, eras []EraInput) error {
			savedEras = eras
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.SetEras(context.Background(), "cal-1", []EraInput{
		{Name: "First Age", StartYear: 1, Color: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if savedEras[0].Color != "#6366f1" {
		t.Errorf("expected default color '#6366f1', got %q", savedEras[0].Color)
	}
}

// --- SetEventCategories Tests ---

func TestSetEventCategories_MissingSlug(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetEventCategories(context.Background(), "cal-1", []EventCategoryInput{
		{Name: "Holiday", Slug: ""},
	})
	assertAppError(t, err, 422)
}

// --- CreateEvent Tests ---

func TestCreateEvent_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	evt, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:      "Battle of Dawn",
		Year:      1200,
		Month:     3,
		Day:       15,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt == nil {
		t.Fatal("expected event, got nil")
	}
	if evt.Name != "Battle of Dawn" {
		t.Errorf("expected name 'Battle of Dawn', got %q", evt.Name)
	}
	if evt.CalendarID != "cal-1" {
		t.Errorf("expected calendar_id 'cal-1', got %q", evt.CalendarID)
	}
	if evt.Visibility != "everyone" {
		t.Errorf("expected default visibility 'everyone', got %q", evt.Visibility)
	}
}

func TestCreateEvent_EmptyName(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:  "",
		Month: 1,
		Day:   1,
	})
	assertAppError(t, err, 422)
}

func TestCreateEvent_NameTooLong(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	longName := make([]byte, 256)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:  string(longName),
		Month: 1,
		Day:   1,
	})
	assertAppError(t, err, 422)
}

func TestCreateEvent_InvalidMonth(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:  "Event",
		Month: 0,
		Day:   1,
	})
	assertAppError(t, err, 422)
}

func TestCreateEvent_InvalidDay(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:  "Event",
		Month: 1,
		Day:   0,
	})
	assertAppError(t, err, 422)
}

func TestCreateEvent_InvalidVisibility(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:       "Event",
		Month:      1,
		Day:        1,
		Visibility: "invalid",
	})
	assertAppError(t, err, 422)
}

func TestCreateEvent_RepoError(t *testing.T) {
	repo := &mockCalendarRepo{
		createEventFn: func(_ context.Context, _ *Event) error {
			return errors.New("db error")
		},
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name:  "Event",
		Month: 1,
		Day:   1,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetEvent Tests ---

func TestGetEvent_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			return &Event{ID: id, Name: "Battle"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	evt, err := svc.GetEvent(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.ID != "evt-1" {
		t.Errorf("expected ID 'evt-1', got %q", evt.ID)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	_, err := svc.GetEvent(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- UpdateEvent Tests ---

func TestUpdateEvent_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Old", Visibility: "everyone"}, nil
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			if evt.Name != "New Name" {
				t.Errorf("expected name 'New Name', got %q", evt.Name)
			}
			return nil
		},
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateEvent(context.Background(), "evt-1", UpdateEventInput{
		Name:       "New Name",
		Month:      5,
		Day:        10,
		Visibility: "everyone",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateEvent_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.UpdateEvent(context.Background(), "nonexistent", UpdateEventInput{
		Name:       "X",
		Visibility: "everyone",
	})
	assertAppError(t, err, 404)
}

func TestUpdateEvent_InvalidVisibility(t *testing.T) {
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "evt-1", Visibility: "everyone"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateEvent(context.Background(), "evt-1", UpdateEventInput{
		Name:       "X",
		Visibility: "invalid",
	})
	assertAppError(t, err, 422)
}

// --- DeleteEvent Tests ---

func TestDeleteEvent_Success(t *testing.T) {
	deleted := false
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "evt-1", CalendarID: "cal-1"}, nil
		},
		deleteEventFn: func(_ context.Context, id string) error {
			deleted = true
			return nil
		},
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.DeleteEvent(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteEvent to be called on repo")
	}
}

// --- UpdateEventVisibility Tests ---

func TestUpdateEventVisibility_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.UpdateEventVisibility(context.Background(), "nonexistent", UpdateEventVisibilityInput{
		Visibility: "everyone",
	})
	assertAppError(t, err, 404)
}

func TestUpdateEventVisibility_InvalidVisibility(t *testing.T) {
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "evt-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.UpdateEventVisibility(context.Background(), "evt-1", UpdateEventVisibilityInput{
		Visibility: "invalid",
	})
	assertAppError(t, err, 422)
}

// --- SetDate Tests ---

func TestSetDate_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1", HoursPerDay: 24, MinutesPerHour: 60}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.SetDate(context.Background(), "cal-1", 1200, 6, 15, 10, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetDate_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.SetDate(context.Background(), "nonexistent", 1200, 6, 15, 10, 30)
	assertAppError(t, err, 404)
}

func TestSetDate_InvalidMonth(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", HoursPerDay: 24, MinutesPerHour: 60}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.SetDate(context.Background(), "cal-1", 1200, 0, 15, 10, 30)
	assertAppError(t, err, 422)
}

func TestSetDate_InvalidHour(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", HoursPerDay: 24, MinutesPerHour: 60}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.SetDate(context.Background(), "cal-1", 1200, 6, 15, 25, 30)
	assertAppError(t, err, 422)
}

// --- AdvanceDate Tests ---

func TestAdvanceDate_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.AdvanceDate(context.Background(), "nonexistent", 5)
	assertAppError(t, err, 404)
}

func TestAdvanceDate_NoMonths(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) {
			return []Month{}, nil
		},
	}
	svc := newTestCalendarService(repo)

	err := svc.AdvanceDate(context.Background(), "cal-1", 5)
	assertAppError(t, err, 422)
}

// --- AdvanceTime Tests ---

func TestAdvanceTime_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.AdvanceTime(context.Background(), "nonexistent", 2, 30)
	assertAppError(t, err, 404)
}

// --- SearchCalendarEvents Tests ---

func TestSearchCalendarEvents_NoCal(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	results, err := svc.SearchCalendarEvents(context.Background(), "camp-1", "battle", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results when no calendar exists, got %v", results)
	}
}

func TestSearchCalendarEvents_Success(t *testing.T) {
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return []Calendar{{ID: "cal-1", CampaignID: "camp-1", Name: "Main"}}, nil
		},
		searchEventsFn: func(_ context.Context, _ string, _ string, _ int) ([]Event, error) {
			return []Event{
				{ID: "evt-1", Name: "Battle of Dawn"},
			}, nil
		},
	}
	svc := newTestCalendarService(repo)

	results, err := svc.SearchCalendarEvents(context.Background(), "camp-1", "battle", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "Battle of Dawn" {
		t.Errorf("expected name 'Battle of Dawn', got %q", results[0]["name"])
	}
	if results[0]["type_name"] != "Event" {
		t.Errorf("expected type_name 'Event', got %q", results[0]["type_name"])
	}
}

// --- SetSeasons Tests ---

func TestSetSeasons_MissingName(t *testing.T) {
	svc := newTestCalendarService(&mockCalendarRepo{})
	err := svc.SetSeasons(context.Background(), "cal-1", []Season{
		{Name: ""},
	})
	assertAppError(t, err, 422)
}

func TestSetSeasons_Success(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := newTestCalendarService(repo)

	err := svc.SetSeasons(context.Background(), "cal-1", []Season{
		{Name: "Spring", StartMonth: 3, StartDay: 1, EndMonth: 5, EndDay: 31},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ListUpcomingEvents Tests ---

func TestListUpcomingEvents_LimitClamped(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CurrentYear: 1200, CurrentMonth: 3, CurrentDay: 10}, nil
		},
		listUpcomingEventsFn: func(_ context.Context, _ string, _, _, _, _ int, limit int) ([]Event, error) {
			if limit > 20 {
				t.Errorf("expected limit clamped to 20, got %d", limit)
			}
			return nil, nil
		},
	}
	svc := newTestCalendarService(repo)

	// Limit over 20 should be clamped.
	_, err := svc.ListUpcomingEvents(context.Background(), "cal-1", 100, 3, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
