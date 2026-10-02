package calendar

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// structureFixture is a three-month calendar (Alpha 30, Beta 30, Gamma 30)
// with a seven-day week, a leap rule of every 4 years adding a day to Beta,
// one moon and one season; today is Gamma 5, year 4.
func structureFixture() *Calendar {
	return &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Mode: ModeFantasy, Name: "Test",
		CurrentYear: 4, CurrentMonth: 3, CurrentDay: 5,
		LeapYearEvery: 4,
		Months: []Month{
			{Name: "Alpha", Days: 30},
			{Name: "Beta", Days: 30, LeapYearDays: 1},
			{Name: "Gamma", Days: 30},
		},
		Weekdays: []Weekday{{Name: "D1"}, {Name: "D2"}, {Name: "D3"}, {Name: "D4"}, {Name: "D5"}, {Name: "D6"}, {Name: "D7"}},
		Moons:    []Moon{{ID: 7, Name: "Luna", CycleDays: 28}},
		Seasons:  []Season{{ID: 3, Name: "Warm", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30}},
	}
}

// editFrom is the fixture's own structure as the editor would send it
// back unchanged; a test case then changes what it needs.
func editFrom(cal *Calendar) StructureEdit {
	e := StructureEdit{LeapYearEvery: cal.LeapYearEvery}
	for _, m := range cal.Months {
		e.Months = append(e.Months, MonthInput{Name: m.Name, Days: m.Days, LeapYearDays: m.LeapYearDays})
	}
	for _, w := range cal.Weekdays {
		e.Weekdays = append(e.Weekdays, WeekdayInput{Name: w.Name})
	}
	for _, m := range cal.Moons {
		id := m.ID
		e.Moons = append(e.Moons, MoonInput{ID: &id, Name: m.Name, CycleDays: m.CycleDays})
	}
	e.Seasons = append(e.Seasons, cal.Seasons...)
	return e
}

func ev(id string, year, month, day int) Event {
	return Event{ID: id, Name: "Event " + id, Year: year, Month: month, Day: day}
}

func eventIDs(changes []StructureEventChange) []string {
	ids := []string{}
	for _, c := range changes {
		ids = append(ids, c.EventID)
	}
	return ids
}

func TestPlanStructureEdit(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		change func(e *StructureEdit)
		// wants
		remap        map[int]int
		moved        int
		redated      []string
		stranded     []string
		currentMonth int
		currentDay   int
		clamped      bool
		noteHas      string
	}{
		{
			name:         "no change touches nothing",
			events:       []Event{ev("a", 4, 1, 3), ev("g", 4, 3, 30)},
			change:       func(e *StructureEdit) {},
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 3, currentDay: 5,
		},
		{
			name:   "last month removed strands its events and clamps today",
			events: []Event{ev("a", 4, 1, 3), ev("g", 4, 3, 10)},
			change: func(e *StructureEdit) { e.Months = e.Months[:2] },
			remap:  map[int]int{},
			// Gamma's position (3) no longer exists at all.
			redated:      []string{},
			stranded:     []string{"g"},
			currentMonth: 2, currentDay: 5, clamped: true,
			noteHas: "Gamma is removed.",
		},
		{
			name:   "middle month removed: later months follow by name, its own events land in the next month",
			events: []Event{ev("b", 4, 2, 3), ev("g", 4, 3, 10)},
			change: func(e *StructureEdit) { e.Months = []MonthInput{e.Months[0], e.Months[2]} },
			remap:  map[int]int{3: 2},
			moved:  1,
			// Beta's event stays at position 2, which is now Gamma.
			redated:      []string{"b"},
			stranded:     []string{},
			currentMonth: 2, currentDay: 5,
			noteHas: "Gamma moves from 3rd to 2nd.",
		},
		{
			name:         "month shortened strands only the days that are gone",
			events:       []Event{ev("a10", 4, 1, 10), ev("a25", 4, 1, 25)},
			change:       func(e *StructureEdit) { e.Months[0].Days = 20 },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{"a25"},
			currentMonth: 3, currentDay: 5,
			noteHas: "Alpha goes from 30 to 20 days.",
		},
		{
			name:         "shortening today's month clamps today to its last day",
			events:       nil,
			change:       func(e *StructureEdit) { e.Months[2].Days = 3 },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 3, currentDay: 3, clamped: true,
		},
		{
			name:         "month renamed in place keeps its events",
			events:       []Event{ev("a", 4, 1, 3)},
			change:       func(e *StructureEdit) { e.Months[0].Name = "First" },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 3, currentDay: 5,
			noteHas: "Alpha is renamed First.",
		},
		{
			name:   "months reordered by name: events and today follow their month",
			events: []Event{ev("a", 4, 1, 3), ev("b", 4, 2, 3), ev("g", 4, 3, 3)},
			change: func(e *StructureEdit) {
				e.Months = []MonthInput{e.Months[2], e.Months[0], e.Months[1]}
			},
			remap:        map[int]int{1: 2, 2: 3, 3: 1},
			moved:        3,
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 1, currentDay: 5,
			noteHas: "Gamma moves from 3rd to 1st.",
		},
		{
			name:         "a month added in front pushes every month along by name",
			events:       []Event{ev("a", 4, 1, 3)},
			change:       func(e *StructureEdit) { e.Months = append([]MonthInput{{Name: "New", Days: 10}}, e.Months...) },
			remap:        map[int]int{1: 2, 2: 3, 3: 4},
			moved:        1,
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 4, currentDay: 5,
			noteHas: "New is new, 1st in the year.",
		},
		{
			name:         "turning leap years off strands an event on a leap day",
			events:       []Event{ev("leapday", 4, 2, 31), ev("ordinary", 4, 2, 30), ev("nonleap", 5, 2, 30)},
			change:       func(e *StructureEdit) { e.LeapYearEvery = 0 },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{"leapday"},
			currentMonth: 3, currentDay: 5,
		},
		{
			name:         "removing a month's leap days strands its leap day too",
			events:       []Event{ev("leapday", 8, 2, 31)},
			change:       func(e *StructureEdit) { e.Months[1].LeapYearDays = 0 },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{"leapday"},
			currentMonth: 3, currentDay: 5,
		},
		{
			name: "an event already off the calendar is not reported as this save's doing",
			events: []Event{
				ev("before", 4, 9, 1), // month 9 never existed
			},
			change:       func(e *StructureEdit) { e.Months = e.Months[:2] },
			remap:        map[int]int{},
			redated:      []string{},
			stranded:     []string{},
			currentMonth: 2, currentDay: 5, clamped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := structureFixture()
			edit := editFrom(cal)
			tt.change(&edit)
			plan := planStructureEdit(cal, tt.events, edit)
			p := plan.preview

			if !reflect.DeepEqual(plan.remap, tt.remap) {
				t.Errorf("remap = %v, want %v", plan.remap, tt.remap)
			}
			if p.MovedEvents != tt.moved {
				t.Errorf("moved = %d, want %d", p.MovedEvents, tt.moved)
			}
			if got := eventIDs(p.Redated); !reflect.DeepEqual(got, tt.redated) {
				t.Errorf("redated = %v, want %v", got, tt.redated)
			}
			if got := eventIDs(p.Stranded); !reflect.DeepEqual(got, tt.stranded) {
				t.Errorf("stranded = %v, want %v", got, tt.stranded)
			}
			if p.RedatedTotal != len(tt.redated) || p.StrandedTotal != len(tt.stranded) {
				t.Errorf("totals = %d redated, %d stranded; want %d, %d", p.RedatedTotal, p.StrandedTotal, len(tt.redated), len(tt.stranded))
			}
			if plan.currentMonth != tt.currentMonth || plan.currentDay != tt.currentDay {
				t.Errorf("current date = month %d day %d, want month %d day %d", plan.currentMonth, plan.currentDay, tt.currentMonth, tt.currentDay)
			}
			if p.CurrentDateClamped != tt.clamped {
				t.Errorf("clamped = %v, want %v", p.CurrentDateClamped, tt.clamped)
			}
			if tt.noteHas != "" && !strings.Contains(strings.Join(p.MonthNotes, "\n"), tt.noteHas) {
				t.Errorf("month notes %q missing %q", p.MonthNotes, tt.noteHas)
			}
		})
	}
}

func TestReconcileMonths(t *testing.T) {
	old := []Month{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	tests := []struct {
		name string
		next []string
		want []int
	}{
		{"unchanged", []string{"A", "B", "C"}, []int{1, 2, 3}},
		{"renamed in place", []string{"A", "Bee", "C"}, []int{1, 2, 3}},
		{"reordered", []string{"C", "A", "B"}, []int{2, 3, 1}},
		{"removed", []string{"A", "C"}, []int{1, 0, 2}},
		{"a name moved onto a removed month's place is not a rename", []string{"A", "C", "D"}, []int{1, 0, 2}},
		{"duplicate names fall back to position", []string{"A", "A", "C"}, []int{1, 0, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := make([]MonthInput, len(tt.next))
			for i, n := range tt.next {
				next[i] = MonthInput{Name: n}
			}
			if got := reconcileMonths(old, next); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reconcileMonths = %v, want %v", got, tt.want)
			}
		})
	}
}

// structureService wires a calendarService over fakes holding cal and
// events, capturing what ApplyStructure is asked to write.
func structureService(cal *Calendar, events []Event, wrote *StructureWrite, calls *int) *calendarService {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			c := *cal
			return &c, nil
		},
		getMonthsFn:   func(context.Context, string) ([]Month, error) { return cal.Months, nil },
		getWeekdaysFn: func(context.Context, string) ([]Weekday, error) { return cal.Weekdays, nil },
		getMoonsFn:    func(context.Context, string) ([]Moon, error) { return cal.Moons, nil },
		getSeasonsFn:  func(context.Context, string) ([]Season, error) { return cal.Seasons, nil },
		applyStructureFn: func(_ context.Context, _ string, w StructureWrite) error {
			*calls++
			*wrote = w
			return nil
		},
	}
	eventRepo := &fakeEventRepo{listAllFn: func(context.Context, string) ([]Event, error) { return events, nil }}
	return NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{}).(*calendarService)
}

func TestApplyStructureEdit(t *testing.T) {
	ctx := context.Background()

	t.Run("writes the planned remap, clamped date and the moons and seasons by id", func(t *testing.T) {
		cal := structureFixture()
		events := []Event{ev("g", 4, 3, 10)}
		var wrote StructureWrite
		calls := 0
		svc := structureService(cal, events, &wrote, &calls)

		edit := editFrom(cal)
		edit.Months = []MonthInput{edit.Months[2], edit.Months[0]} // Gamma first, Beta gone
		edit.Moons[0].Name = "Selene"
		edit.Moons = append(edit.Moons, MoonInput{Name: "Second", CycleDays: 10})
		edit.Seasons[0].Name = "Summer"

		preview, err := svc.PreviewStructureEdit(ctx, cal.ID, cal.CampaignID, edit)
		if err != nil {
			t.Fatalf("PreviewStructureEdit: %v", err)
		}
		if _, err := svc.ApplyStructureEdit(ctx, cal.ID, cal.CampaignID, preview.Fingerprint, edit); err != nil {
			t.Fatalf("ApplyStructureEdit: %v", err)
		}
		if calls != 1 {
			t.Fatalf("ApplyStructure called %d times, want 1", calls)
		}
		if want := map[int]int{1: 2, 3: 1}; !reflect.DeepEqual(wrote.MonthRemap, want) {
			t.Errorf("remap = %v, want %v", wrote.MonthRemap, want)
		}
		if wrote.CurrentMonth != 1 || wrote.CurrentDay != 5 {
			t.Errorf("today = %d/%d, want Gamma (now 1st) day 5, unchanged", wrote.CurrentMonth, wrote.CurrentDay)
		}
		if len(wrote.Moons) != 2 || wrote.Moons[0].ID == nil || *wrote.Moons[0].ID != 7 || wrote.Moons[1].ID != nil {
			t.Errorf("moons = %+v, want Luna's id 7 kept and the new moon with no id", wrote.Moons)
		}
		if len(wrote.Seasons) != 1 || wrote.Seasons[0].ID != 3 || wrote.Seasons[0].Name != "Summer" {
			t.Errorf("seasons = %+v, want season 3 renamed in place", wrote.Seasons)
		}
		for i, m := range wrote.Months {
			if m.SortOrder != i {
				t.Errorf("month %d sort order = %d, want its position", i, m.SortOrder)
			}
		}
	})

	t.Run("a stale fingerprint writes nothing and returns the fresh preview", func(t *testing.T) {
		cal := structureFixture()
		var wrote StructureWrite
		calls := 0
		svc := structureService(cal, nil, &wrote, &calls)
		p, err := svc.ApplyStructureEdit(ctx, cal.ID, cal.CampaignID, "stale", editFrom(cal))
		if apperror.SafeCode(err) != http.StatusConflict {
			t.Fatalf("err = %v, want a Conflict", err)
		}
		if p == nil || p.Fingerprint == "" {
			t.Errorf("a conflict must carry the fresh preview, got %+v", p)
		}
		if calls != 0 {
			t.Errorf("ApplyStructure called %d times on a stale preview, want 0", calls)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(cal *Calendar, e *StructureEdit)
			code   int
		}{
			{"no months", func(_ *Calendar, e *StructureEdit) { e.Months = nil }, http.StatusBadRequest},
			{"no weekdays", func(_ *Calendar, e *StructureEdit) { e.Weekdays = nil }, http.StatusBadRequest},
			{"season outside the months", func(_ *Calendar, e *StructureEdit) { e.Seasons[0].EndMonth = 9 }, http.StatusBadRequest},
			{"a real-time calendar", func(cal *Calendar, _ *StructureEdit) {
				cal.Mode = ModeRealLife
				cal.TracksRealTime = true
			}, http.StatusUnprocessableEntity},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cal := structureFixture()
				edit := editFrom(cal)
				tt.mutate(cal, &edit)
				var wrote StructureWrite
				calls := 0
				svc := structureService(cal, nil, &wrote, &calls)
				if _, err := svc.PreviewStructureEdit(ctx, cal.ID, cal.CampaignID, edit); apperror.SafeCode(err) != tt.code {
					t.Errorf("preview err = %v, want %d", err, tt.code)
				}
				if _, err := svc.ApplyStructureEdit(ctx, cal.ID, cal.CampaignID, "x", edit); apperror.SafeCode(err) != tt.code {
					t.Errorf("apply err = %v, want %d", err, tt.code)
				}
				if calls != 0 {
					t.Errorf("a refused save wrote %d times", calls)
				}
			})
		}
	})

	t.Run("a calendar in another campaign is not found", func(t *testing.T) {
		cal := structureFixture()
		var wrote StructureWrite
		calls := 0
		svc := structureService(cal, nil, &wrote, &calls)
		if _, err := svc.PreviewStructureEdit(ctx, cal.ID, "other-campaign", editFrom(cal)); apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("err = %v, want NotFound", err)
		}
	})
}
