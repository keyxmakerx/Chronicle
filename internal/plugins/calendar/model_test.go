// model_test.go: table-driven unit tests for the domain-layer date logic
// that the repository and service layers both depend on (no database
// needed). ContainsDate/EraForDate/CurrentEra decide which era a date falls
// in; EffectiveAnnounced decides an event's announce-ahead-or-on-day rule.
// Both have caused real regressions when their precedence was misread, so
// every branch gets its own case here rather than relying on integration
// tests to exercise them incidentally.
package calendar

import "testing"

func TestEraContainsDate(t *testing.T) {
	endYear, endMonth, endDay := 10, 6, 15

	tests := []struct {
		name             string
		era              Era
		year, month, day int
		want             bool
	}{
		{
			name: "before the start date, same year",
			era:  Era{StartYear: 5, StartMonth: 6, StartDay: 15},
			year: 5, month: 6, day: 14,
			want: false,
		},
		{
			name: "exactly on the start date",
			era:  Era{StartYear: 5, StartMonth: 6, StartDay: 15},
			year: 5, month: 6, day: 15,
			want: true,
		},
		{
			name: "ongoing era (nil EndYear) covers a date decades later",
			era:  Era{StartYear: 5, StartMonth: 1, StartDay: 1},
			year: 500, month: 12, day: 31,
			want: true,
		},
		{
			name: "year-only end (nil EndMonth/EndDay): the whole end year counts",
			era:  Era{StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &endYear},
			year: 10, month: 12, day: 31,
			want: true,
		},
		{
			name: "year-only end: the year after the end year does not count",
			era:  Era{StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &endYear},
			year: 11, month: 1, day: 1,
			want: false,
		},
		{
			name: "day-granular end: on the end day counts",
			era:  Era{StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &endYear, EndMonth: &endMonth, EndDay: &endDay},
			year: 10, month: 6, day: 15,
			want: true,
		},
		{
			name: "day-granular end: the day after the end day does not count",
			era:  Era{StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &endYear, EndMonth: &endMonth, EndDay: &endDay},
			year: 10, month: 6, day: 16,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.era.ContainsDate(tt.year, tt.month, tt.day); got != tt.want {
				t.Errorf("ContainsDate(%d,%d,%d) = %v, want %v", tt.year, tt.month, tt.day, got, tt.want)
			}
		})
	}
}

func TestCalendarEraForDateAndCurrentEra(t *testing.T) {
	age2End := 100
	cal := &Calendar{
		Eras: []Era{
			{ID: 1, Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &age2End},
			{ID: 2, Name: "Second Age", StartYear: 101, StartMonth: 1, StartDay: 1},
		},
	}

	t.Run("EraForDate picks the era whose range contains the date", func(t *testing.T) {
		got := cal.EraForDate(50, 6, 1)
		if got == nil || got.ID != 1 {
			t.Errorf("EraForDate(50,6,1) = %+v, want First Age", got)
		}
		got = cal.EraForDate(200, 1, 1)
		if got == nil || got.ID != 2 {
			t.Errorf("EraForDate(200,1,1) = %+v, want Second Age", got)
		}
	})

	t.Run("EraForDate returns nil before any era starts", func(t *testing.T) {
		if got := cal.EraForDate(0, 1, 1); got != nil {
			t.Errorf("EraForDate(0,1,1) = %+v, want nil", got)
		}
	})

	t.Run("CurrentEra reads the calendar's own current date, day-granular", func(t *testing.T) {
		// Regression: an imported era with a year-only end (nil
		// EndMonth/EndDay) must still cover every day of that end year,
		// not just year comparisons that ignore month/day.
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 100, 12, 31
		got := cal.CurrentEra()
		if got == nil || got.ID != 1 {
			t.Errorf("CurrentEra() at (100,12,31) = %+v, want First Age (its end year, no end day, must be fully covered)", got)
		}

		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 101, 1, 1
		got = cal.CurrentEra()
		if got == nil || got.ID != 2 {
			t.Errorf("CurrentEra() at (101,1,1) = %+v, want Second Age", got)
		}
	})
}

func TestEventEffectiveAnnounced(t *testing.T) {
	yearly := RecurrenceYearly
	weekly := RecurrenceWeekly
	onDay := AnnouncedOnDay
	empty := ""

	matchingKind := &EventKind{ID: 7, DefaultAnnounced: AnnouncedAhead}
	otherKind := &EventKind{ID: 9, DefaultAnnounced: AnnouncedAhead}
	matchingKindID := 7

	tests := []struct {
		name string
		evt  Event
		kind *EventKind
		want string
	}{
		{
			name: "the event's own setting wins over everything else",
			evt:  Event{Announced: &onDay, IsRecurring: true, RecurrenceType: &yearly, KindID: &matchingKindID},
			kind: matchingKind,
			want: AnnouncedOnDay,
		},
		{
			name: "an empty-string Announced does not count as a setting",
			evt:  Event{Announced: &empty},
			kind: nil,
			want: AnnouncedOnDay,
		},
		{
			name: "a yearly-recurring event defaults to ahead with no kind at all",
			evt:  Event{IsRecurring: true, RecurrenceType: &yearly},
			kind: nil,
			want: AnnouncedAhead,
		},
		{
			name: "a non-yearly recurrence does not get the yearly rule",
			evt:  Event{IsRecurring: true, RecurrenceType: &weekly},
			kind: nil,
			want: AnnouncedOnDay,
		},
		{
			name: "the kind's default applies only when the event's own KindID matches it",
			evt:  Event{KindID: &matchingKindID},
			kind: matchingKind,
			want: AnnouncedAhead,
		},
		{
			name: "a kind passed in that the event isn't actually assigned must never apply",
			evt:  Event{KindID: nil},
			kind: otherKind,
			want: AnnouncedOnDay,
		},
		{
			name: "a kind passed in with a different id than the event's own KindID must never apply",
			evt:  Event{KindID: &matchingKindID},
			kind: otherKind,
			want: AnnouncedOnDay,
		},
		{
			name: "no announced, no recurrence, no kind: the final fallback",
			evt:  Event{},
			kind: nil,
			want: AnnouncedOnDay,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.evt.EffectiveAnnounced(tt.kind); got != tt.want {
				t.Errorf("EffectiveAnnounced() = %q, want %q", got, tt.want)
			}
		})
	}
}
