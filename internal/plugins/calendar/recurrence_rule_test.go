// recurrence_rule_test.go pins rule parsing and the occurrence expander:
// each condition kind, combinations, every/offset, leap years, intercalary
// days, Each-month-starts-a-new-week, moon-phase agreement with the moon
// view's own naming, after_event, overrides and the scan bound.
package calendar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ruleCal is a small calendar with the awkward parts: a one-day intercalary
// festival month between two ordinary ones, a leap day on the last month
// every 4 years, a 7-day week, one moon and two seasons.
func ruleCal() *Calendar {
	return &Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Visibility: "everyone",
		Months: []Month{
			{Name: "Alpha", Days: 30},
			{Name: "Midfest", Days: 1, IsIntercalary: true},
			{Name: "Beta", Days: 30, LeapYearDays: 1},
		},
		Weekdays:      make([]Weekday, 7),
		LeapYearEvery: 4,
		Moons:         []Moon{{ID: 5, Name: "Luna", CycleDays: 29.5, PhaseOffset: 3.2}},
		Seasons: []Season{
			{ID: 8, Name: "Bloom", StartMonth: 1, StartDay: 10, EndMonth: 1, EndDay: 20},
			{ID: 9, Name: "Long", StartMonth: 3, StartDay: 25, EndMonth: 1, EndDay: 5},
		},
	}
}

// ruleEvent is a rule event starting on (y,m,d).
func ruleEvent(id string, y, m, d int, rule *RecurrenceRule) Event {
	t := RecurrenceByRule
	return Event{ID: id, Year: y, Month: m, Day: d, IsRecurring: true, RecurrenceType: &t, RecurrenceRule: rule, Visibility: "everyone"}
}

func mustRule(t *testing.T, js string) *RecurrenceRule {
	t.Helper()
	r, err := ParseRecurrenceRule([]byte(js))
	if err != nil {
		t.Fatalf("parse %s: %v", js, err)
	}
	return r
}

func dates(ds []DayDate) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = fmt.Sprintf("%d-%d-%d", d.Year, d.Month, d.Day)
	}
	return strings.Join(parts, " ")
}

// eachDay calls fn for every day of years [y1, y2] the calendar has, by
// nested loops over MonthDays: the reference the cursor is checked against.
func eachDay(cal *Calendar, y1, y2 int, fn func(y, m, d int)) {
	for y := y1; y <= y2; y++ {
		for m := 1; m <= len(cal.Months); m++ {
			for d := 1; d <= cal.MonthDays(m-1, y); d++ {
				fn(y, m, d)
			}
		}
	}
}

func TestParseRecurrenceRule_Rejects(t *testing.T) {
	nine := `{"kind":"weekday","weekdays":[0]}`
	tests := []struct {
		name, js, want string
	}{
		{"no conditions", `{"match":[]}`, "at least one"},
		{"too many conditions", `{"match":[` + strings.Repeat(nine+",", 6) + nine + `]}`, "at most 6"},
		{"unknown top-level key", `{"match":[` + nine + `],"evry":2}`, "not a valid rule"},
		{"unknown kind", `{"match":[{"kind":"full_moonish"}]}`, "unknown condition kind"},
		{"unknown condition key", `{"match":[{"kind":"weekday","weekdays":[0],"colour":"red"}]}`, "not a valid rule"},
		{"field from another kind", `{"match":[{"kind":"weekday","weekdays":[0],"moon_id":3}]}`, "does not take moon_id"},
		{"every too big", `{"match":[` + nine + `],"every":100}`, "every"},
		{"every negative", `{"match":[` + nine + `],"every":-1}`, "every"},
		{"offset too big", `{"match":[` + nine + `],"offset_days":366}`, "offset_days"},
		{"bad phase", `{"match":[{"kind":"moon_phase","moon_id":1,"phase":"gibbous"}]}`, "phase"},
		{"moon without id", `{"match":[{"kind":"moon_phase","phase":"full"}]}`, "moon_id"},
		{"empty weekdays", `{"match":[{"kind":"weekday","weekdays":[]}]}`, "at least one"},
		{"weekday and weekdays", `{"match":[{"kind":"weekday","weekday":1,"weekdays":[2]}]}`, "not both"},
		{"duplicate weekday", `{"match":[{"kind":"weekday","weekdays":[1,1]}]}`, "twice"},
		{"nth zero", `{"match":[{"kind":"nth_weekday","n":0,"weekday":1}]}`, "n must"},
		{"nth six", `{"match":[{"kind":"nth_weekday","n":6,"weekday":1}]}`, "n must"},
		{"nth no weekday", `{"match":[{"kind":"nth_weekday","n":2}]}`, "weekday"},
		{"day zero", `{"match":[{"kind":"day_of_month","day":0}]}`, "day must"},
		{"day -2", `{"match":[{"kind":"day_of_month","day":-2}]}`, "day must"},
		{"month zero", `{"match":[{"kind":"months","months":[0]}]}`, "out-of-range"},
		{"single month missing", `{"match":[{"kind":"month"}]}`, "month"},
		{"season no id", `{"match":[{"kind":"season_start"}]}`, "season_id"},
		{"after_event no days", `{"match":[{"kind":"after_event","event_id":"x"}]}`, "days"},
		{"after_event days too far", `{"match":[{"kind":"after_event","event_id":"x","days":400}]}`, "days must"},
		{"relative_to_event takes no days", `{"match":[{"kind":"relative_to_event","event_id":"x","days":1}]}`, "does not take days"},
		{"trailing data", `{"match":[` + nine + `]} {}`, "trailing"},
		{"not an object", `[1,2]`, "not a valid rule"},
		{"too large", `{"match":[` + nine + `],"x":"` + strings.Repeat("a", maxRuleJSONBytes) + `"}`, "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRecurrenceRule([]byte(tt.js))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

func TestParseRecurrenceRule_NullIsNoRule(t *testing.T) {
	for _, in := range []string{"", "null", "  null "} {
		if r, err := ParseRecurrenceRule([]byte(in)); r != nil || err != nil {
			t.Errorf("%q: got %v, %v; want no rule", in, r, err)
		}
	}
}

func TestRecurrenceRule_ValidateAgainstCalendar(t *testing.T) {
	cal := ruleCal()
	cal.Moons = append(cal.Moons, Moon{ID: 6, Name: "Secret", CycleDays: 10, HiddenFromPlayers: true})
	player := func(m Moon) bool { return !m.HiddenFromPlayers }
	tests := []struct {
		name, js string
		ok       bool
	}{
		{"weekday in week", `{"match":[{"kind":"weekday","weekday":6}]}`, true},
		{"weekday past week", `{"match":[{"kind":"weekday","weekday":7}]}`, false},
		{"weekdays past week", `{"match":[{"kind":"weekday","weekdays":[1,9]}]}`, false},
		{"nth weekday past week", `{"match":[{"kind":"nth_weekday","n":1,"weekday":7}]}`, false},
		{"month exists", `{"match":[{"kind":"month","month":3}]}`, true},
		{"month missing", `{"match":[{"kind":"month","month":4}]}`, false},
		{"months missing", `{"match":[{"kind":"months","months":[1,4]}]}`, false},
		{"moon of calendar", `{"match":[{"kind":"moon_phase","moon_id":5,"phase":"new"}]}`, true},
		{"moon of another calendar", `{"match":[{"kind":"moon_phase","moon_id":99,"phase":"new"}]}`, false},
		{"hidden moon for a player", `{"match":[{"kind":"moon_phase","moon_id":6,"phase":"new"}]}`, false},
		{"season of calendar", `{"match":[{"kind":"season","season_id":8}]}`, true},
		{"season elsewhere", `{"match":[{"kind":"season_start","season_id":99}]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mustRule(t, tt.js).validateAgainstCalendar(cal, player)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

// TestRuleExpansion_MatchesReference checks every single-condition kind and
// a few combinations against a brute-force evaluation over five years built
// only from the calendar's public day math (MonthDays, WeekdayIndex,
// SeasonForDate), so the walk can never drift from what the grid computes.
func TestRuleExpansion_MatchesReference(t *testing.T) {
	cal := ruleCal()
	wl := cal.WeekLength()
	nth := func(m, d int) int { return (d-1)/wl + 1 }
	tests := []struct {
		name string
		js   string
		ref  func(y, m, d int) bool
	}{
		{"weekday", `{"match":[{"kind":"weekday","weekday":3}]}`,
			func(y, m, d int) bool { return cal.WeekdayIndex(y, m, d) == 3 }},
		{"weekdays", `{"match":[{"kind":"weekday","weekdays":[0,6]}]}`,
			func(y, m, d int) bool { w := cal.WeekdayIndex(y, m, d); return w == 0 || w == 6 }},
		{"2nd weekday 4", `{"match":[{"kind":"nth_weekday","n":2,"weekday":4}]}`,
			func(y, m, d int) bool { return cal.WeekdayIndex(y, m, d) == 4 && nth(m, d) == 2 }},
		{"last weekday 1", `{"match":[{"kind":"nth_weekday","n":-1,"weekday":1}]}`,
			func(y, m, d int) bool {
				return cal.WeekdayIndex(y, m, d) == 1 && d+wl > cal.MonthDays(m-1, y)
			}},
		{"day 15", `{"match":[{"kind":"day_of_month","day":15}]}`,
			func(y, m, d int) bool { return d == 15 }},
		{"last day of month", `{"match":[{"kind":"day_of_month","day":-1}]}`,
			func(y, m, d int) bool { return d == cal.MonthDays(m-1, y) }},
		{"months", `{"match":[{"kind":"months","months":[2,3]}]}`,
			func(y, m, d int) bool { return m == 2 || m == 3 }},
		{"month", `{"match":[{"kind":"month","month":1}]}`,
			func(y, m, d int) bool { return m == 1 }},
		{"season", `{"match":[{"kind":"season","season_id":9}]}`,
			func(y, m, d int) bool { s := cal.SeasonForDate(m, d); return s != nil && s.ID == 9 }},
		{"season start", `{"match":[{"kind":"season_start","season_id":8}]}`,
			func(y, m, d int) bool { return m == 1 && d == 10 }},
		{"custom: last day of the year", `{"match":[{"kind":"month","month":3},{"kind":"day_of_month","day":-1}]}`,
			func(y, m, d int) bool { return m == 3 && d == cal.MonthDays(2, y) }},
		{"custom: weekday 2 inside a season", `{"match":[{"kind":"weekday","weekday":2},{"kind":"season","season_id":8}]}`,
			func(y, m, d int) bool { return cal.WeekdayIndex(y, m, d) == 2 && m == 1 && d >= 10 && d <= 20 }},
		{"never: day 31 of Alpha", `{"match":[{"kind":"month","month":1},{"kind":"day_of_month","day":31}]}`,
			func(y, m, d int) bool { return false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ruleEvent("e", 1, 1, 1, mustRule(t, tt.js))
			x := newExpander(cal, nil, nil)
			got, truncated := x.natural(&e, DayDate{1, 1, 1}, DayDate{5, 3, 31}, 0, 0)
			if truncated {
				t.Fatal("five years must not hit the scan bound")
			}
			var want []DayDate
			eachDay(cal, 1, 5, func(y, m, d int) {
				if tt.ref(y, m, d) {
					want = append(want, DayDate{y, m, d})
				}
			})
			if dates(got) != dates(want) {
				t.Fatalf("got  %s\nwant %s", dates(got), dates(want))
			}
		})
	}
}

// TestRuleExpansion_LeapAndIntercalary pins the hand-checkable cases: the
// leap day exists only in leap years, and the one-day festival month is a
// real day the walk steps through.
func TestRuleExpansion_LeapAndIntercalary(t *testing.T) {
	cal := ruleCal()
	tests := []struct {
		name     string
		js       string
		from, to DayDate
		want     string
	}{
		{"last day of Beta, leap and common", `{"match":[{"kind":"month","month":3},{"kind":"day_of_month","day":-1}]}`,
			DayDate{3, 1, 1}, DayDate{5, 3, 31}, "3-3-30 4-3-31 5-3-30"},
		{"day 31 only in leap years", `{"match":[{"kind":"day_of_month","day":31}]}`,
			DayDate{4, 1, 1}, DayDate{8, 3, 31}, "4-3-31 8-3-31"},
		{"festival day is day 1 and the last day of its month", `{"match":[{"kind":"month","month":2},{"kind":"day_of_month","day":-1}]}`,
			DayDate{1, 1, 1}, DayDate{2, 3, 30}, "1-2-1 2-2-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ruleEvent("e", 1, 1, 1, mustRule(t, tt.js))
			got, _ := newExpander(cal, nil, nil).natural(&e, tt.from, tt.to, 0, 0)
			if dates(got) != tt.want {
				t.Fatalf("got %s, want %s", dates(got), tt.want)
			}
		})
	}
}

// TestRuleExpansion_MonthStartsNewWeek: with the switch on, every month
// starts on weekday 0 and the festival day belongs to no week at all.
func TestRuleExpansion_MonthStartsNewWeek(t *testing.T) {
	cal := ruleCal()
	cal.MonthStartsNewWeek = true
	tests := []struct {
		name, js, want string
	}{
		{"weekday 0 is days 1, 8, 15, 22, 29, never the festival",
			`{"match":[{"kind":"weekday","weekday":0}]}`,
			"1-1-1 1-1-8 1-1-15 1-1-22 1-1-29 1-3-1 1-3-8 1-3-15 1-3-22 1-3-29"},
		{"first weekday 0 of each month is day 1",
			`{"match":[{"kind":"nth_weekday","n":1,"weekday":0}]}`, "1-1-1 1-3-1"},
		{"last weekday 1 of each month",
			`{"match":[{"kind":"nth_weekday","n":-1,"weekday":1}]}`, "1-1-30 1-3-30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ruleEvent("e", 1, 1, 1, mustRule(t, tt.js))
			got, _ := newExpander(cal, nil, nil).natural(&e, DayDate{1, 1, 1}, DayDate{1, 3, 30}, 0, 0)
			if dates(got) != tt.want {
				t.Fatalf("got %s, want %s", dates(got), tt.want)
			}
		})
	}
}

// TestRuleExpansion_MoonPhaseAgreesWithMoonView: every day a phase rule
// picks is one the moon view names with that phase (Moon.MoonPhaseName on
// the same day counter), once per cycle, for a fantasy and a real-world
// calendar.
func TestRuleExpansion_MoonPhaseAgreesWithMoonView(t *testing.T) {
	names := map[string]string{"new": "New Moon", "first_quarter": "First Quarter", "full": "Full Moon", "last_quarter": "Last Quarter"}
	real := &Calendar{
		ID: "cal-r", Mode: ModeRealLife, TracksRealTime: true,
		Months:   make([]Month, 12),
		Weekdays: make([]Weekday, 7),
		Moons:    []Moon{{ID: 1, Name: "Moon", CycleDays: 29.530588, PhaseOffset: 11.4}},
	}
	for _, cal := range []*Calendar{ruleCal(), real} {
		moon := cal.Moons[0]
		for phase, name := range names {
			t.Run(cal.ID+"/"+phase, func(t *testing.T) {
				js := fmt.Sprintf(`{"match":[{"kind":"moon_phase","moon_id":%d,"phase":%q}]}`, moon.ID, phase)
				e := ruleEvent("e", 2000, 1, 1, mustRule(t, js))
				x := newExpander(cal, nil, nil)
				got, _ := x.natural(&e, DayDate{2000, 1, 1}, DayDate{2003, 12, 1}, 0, 0)
				span := float64(cal.absDayIndex(2003, 12, 1) - cal.absDayIndex(2000, 1, 1))
				if min := int(span/moon.CycleDays) - 1; len(got) < min {
					t.Fatalf("only %d %s days, want at least %d", len(got), phase, min)
				}
				prev := -1
				for _, d := range got {
					abs := cal.absDayIndex(d.Year, d.Month, d.Day)
					if n := moon.MoonPhaseName(abs); n != name {
						t.Errorf("%s: rule picked a day the moon view calls %q", dates([]DayDate{d}), n)
					}
					if prev >= 0 {
						if gap := abs - prev; gap < 29 || gap > 30 {
							t.Errorf("gap of %d days between %s days", gap, phase)
						}
					}
					prev = abs
				}
			})
		}
	}
}

func TestRuleExpansion_EveryOffsetAndLimits(t *testing.T) {
	cal := ruleCal()
	from, to := DayDate{1, 1, 1}, DayDate{1, 1, 30}
	// Weekday-of-(1,1,d) in ruleCal is fixed; take the reference list.
	var all []DayDate
	eachDay(cal, 1, 1, func(y, m, d int) {
		if m == 1 && cal.WeekdayIndex(y, m, d) == 2 {
			all = append(all, DayDate{y, m, d})
		}
	})
	shift := func(ds []DayDate, n int) []DayDate {
		out := make([]DayDate, len(ds))
		for i, d := range ds {
			out[i], _ = shiftDate(cal, d, n)
		}
		return out
	}
	tests := []struct {
		name string
		js   string
		max  *int
		end  *DayDate
		want []DayDate
	}{
		{name: "every 2nd", js: `{"match":[{"kind":"weekday","weekday":2}],"every":2}`, want: []DayDate{all[0], all[2]}},
		{name: "offset +1", js: `{"match":[{"kind":"weekday","weekday":2}],"offset_days":1}`, want: shift(all, 1)},
		// The match on the festival day (the year's 31st day) lands on Alpha
		// 29 once moved two days back.
		{name: "offset -2", js: `{"match":[{"kind":"weekday","weekday":2}],"offset_days":-2}`,
			want: append(shift(all, -2), DayDate{1, 1, 29})},
		{name: "max 2", js: `{"match":[{"kind":"weekday","weekday":2}]}`, max: ptr(2), want: all[:2]},
		{name: "every 2 with max 1", js: `{"match":[{"kind":"weekday","weekday":2}],"every":2}`, max: ptr(1), want: all[:1]},
		{name: "end date", js: `{"match":[{"kind":"weekday","weekday":2}]}`, end: &all[1], want: all[:2]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ruleEvent("e", 1, 1, 1, mustRule(t, tt.js))
			e.RecurrenceMaxOccurrences = tt.max
			if tt.end != nil {
				e.RecurrenceEndYear, e.RecurrenceEndMonth, e.RecurrenceEndDay = &tt.end.Year, &tt.end.Month, &tt.end.Day
			}
			got, _ := newExpander(cal, nil, nil).natural(&e, from, to, 0, 0)
			if dates(got) != dates(tt.want) {
				t.Fatalf("got %s, want %s", dates(got), dates(tt.want))
			}
		})
	}
}

// TestRuleExpansion_EveryCountsFromStart: with every > 1 the count runs
// from the event's start, so a month read far later still lands on the
// same alternation the first months had.
func TestRuleExpansion_EveryCountsFromStart(t *testing.T) {
	cal := ruleCal()
	e := ruleEvent("e", 1, 1, 1, mustRule(t, `{"match":[{"kind":"day_of_month","day":1}],"every":2}`))
	x := newExpander(cal, nil, nil)
	// Day 1 of each month: (1,1),(1,2),(1,3),(2,1),(2,2),(2,3)… every 2nd.
	got, _ := x.natural(&e, DayDate{2, 1, 1}, DayDate{2, 3, 30}, 0, 0)
	if want := "2-2-1"; dates(got) != want {
		t.Fatalf("got %s, want %s", dates(got), want)
	}
}

func TestRuleExpansion_ScanBound(t *testing.T) {
	cal := ruleCal()
	cap := ruleScanCapDays(cal)
	if cap < ruleScanMinDays || cap > ruleScanMaxDays {
		t.Fatalf("cap %d outside its bounds", cap)
	}
	// every 2 counts from the start: a start 100 years back cannot be
	// counted within the bound, so the read is empty and says so.
	e := ruleEvent("e", 1, 1, 1, mustRule(t, `{"match":[{"kind":"day_of_month","day":1}],"every":2}`))
	got, truncated := newExpander(cal, nil, nil).natural(&e, DayDate{100, 1, 1}, DayDate{100, 3, 30}, 0, 0)
	if len(got) != 0 || !truncated {
		t.Fatalf("got %s truncated=%v, want nothing and truncated", dates(got), truncated)
	}
	// every 1 needs no count: the same far-off month expands fine.
	e = ruleEvent("e", 1, 1, 1, mustRule(t, `{"match":[{"kind":"day_of_month","day":1}]}`))
	got, truncated = newExpander(cal, nil, nil).natural(&e, DayDate{100, 1, 1}, DayDate{100, 3, 30}, 0, 0)
	if dates(got) != "100-1-1 100-2-1 100-3-1" || truncated {
		t.Fatalf("got %s truncated=%v", dates(got), truncated)
	}
}

func TestRuleExpansion_AfterEvent(t *testing.T) {
	cal := ruleCal()
	yearly := RecurrenceYearly
	anchor := Event{ID: "masks", Year: 1, Month: 1, Day: 30, IsRecurring: true, RecurrenceType: &yearly}
	anchors := map[string]*Event{"masks": &anchor}
	tests := []struct {
		name, js, want string
	}{
		// Alpha 30 + 1 is the festival day, + 2 is Beta 1.
		{"2 days after crosses the festival", `{"match":[{"kind":"after_event","event_id":"masks","days":2}]}`, "1-3-1 2-3-1"},
		{"1 day after is the festival", `{"match":[{"kind":"after_event","event_id":"masks","days":1}]}`, "1-2-1 2-2-1"},
		{"3 days before", `{"match":[{"kind":"after_event","event_id":"masks","days":-3}]}`, "1-1-27 2-1-27"},
		{"relative_to_event plus offset", `{"match":[{"kind":"relative_to_event","event_id":"masks"}],"offset_days":2}`, "1-3-1 2-3-1"},
		{"missing anchor matches nothing", `{"match":[{"kind":"after_event","event_id":"gone","days":2}]}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ruleEvent("dep", 1, 1, 1, mustRule(t, tt.js))
			got, _ := newExpander(cal, anchors, nil).natural(&e, DayDate{1, 1, 1}, DayDate{2, 3, 30}, 0, 0)
			if dates(got) != tt.want {
				t.Fatalf("got %q, want %q", dates(got), tt.want)
			}
		})
	}

	t.Run("anchor's skip and move carry over", func(t *testing.T) {
		y, m, d := 2, 1, 10
		overrides := map[string][]OccurrenceOverride{"masks": {
			{EventID: "masks", Year: 1, Month: 1, Day: 30, Action: OverrideSkip},
			{EventID: "masks", Year: 2, Month: 1, Day: 30, Action: OverrideMove, NewYear: &y, NewMonth: &m, NewDay: &d},
		}}
		e := ruleEvent("dep", 1, 1, 1, mustRule(t, `{"match":[{"kind":"after_event","event_id":"masks","days":2}]}`))
		got, _ := newExpander(cal, anchors, overrides).natural(&e, DayDate{1, 1, 1}, DayDate{2, 3, 30}, 0, 0)
		if want := "2-1-12"; dates(got) != want {
			t.Fatalf("got %s, want %s", dates(got), want)
		}
	})

	t.Run("an anchor that itself follows another event never chains", func(t *testing.T) {
		mid := ruleEvent("mid", 1, 1, 1, mustRule(t, `{"match":[{"kind":"after_event","event_id":"masks","days":1}]}`))
		chained := map[string]*Event{"masks": &anchor, "mid": &mid}
		e := ruleEvent("dep", 1, 1, 1, mustRule(t, `{"match":[{"kind":"after_event","event_id":"mid","days":1}]}`))
		got, _ := newExpander(cal, chained, nil).natural(&e, DayDate{1, 1, 1}, DayDate{2, 3, 30}, 0, 0)
		if len(got) != 0 {
			t.Fatalf("got %s, want nothing", dates(got))
		}
	})
}

func TestOccurrences_Overrides(t *testing.T) {
	cal := ruleCal()
	weekly := RecurrenceWeekly
	e := Event{ID: "w", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &weekly}
	ny, nm, nd := 1, 3, 2
	farY, farM, farD := 1, 1, 3
	overrides := map[string][]OccurrenceOverride{"w": {
		{EventID: "w", Year: 1, Month: 1, Day: 8, Action: OverrideSkip},
		{EventID: "w", Year: 1, Month: 1, Day: 15, Action: OverrideMove, NewYear: &ny, NewMonth: &nm, NewDay: &nd},
		// Stale: day 2 is not an occurrence (the event changed since), so
		// this move is ignored.
		{EventID: "w", Year: 1, Month: 1, Day: 2, Action: OverrideMove, NewYear: &farY, NewMonth: &farM, NewDay: &farD},
	}}
	x := newExpander(cal, nil, overrides)

	got, _ := x.occurrences(&e, DayDate{1, 1, 1}, DayDate{1, 1, 30}, 0)
	want := []Occurrence{{Year: 1, Month: 1, Day: 1}, {Year: 1, Month: 1, Day: 8, Skipped: true}, {Year: 1, Month: 1, Day: 22}, {Year: 1, Month: 1, Day: 29}}
	if a, b := mustJSON(t, got), mustJSON(t, want); a != b {
		t.Fatalf("Alpha:\n got  %s\n want %s", a, b)
	}
	// The moved occurrence shows in the month it moved into, with its origin.
	got, _ = x.occurrences(&e, DayDate{1, 3, 1}, DayDate{1, 3, 3}, 0)
	if len(got) != 1 || got[0].Day != 2 || got[0].MovedFrom == nil || got[0].MovedFrom.Day != 15 {
		t.Fatalf("Beta: got %s", mustJSON(t, got))
	}
	if players := forPlayers(got); players[0].MovedFrom != nil {
		t.Error("a player must not see where a moved occurrence came from")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDayCursor_WalksBothWays(t *testing.T) {
	cal := ruleCal()
	var all []DayDate
	eachDay(cal, 3, 5, func(y, m, d int) { all = append(all, DayDate{y, m, d}) })
	c := newDayCursor(cal, all[0])
	for i, want := range all {
		if c.date() != want || c.abs != cal.absDayIndex(want.Year, want.Month, want.Day) {
			t.Fatalf("step %d: at %v abs %d, want %v abs %d", i, c.date(), c.abs, want, cal.absDayIndex(want.Year, want.Month, want.Day))
		}
		c.next()
	}
	c.prev()
	for i := len(all) - 1; i >= 0; i-- {
		if c.date() != all[i] {
			t.Fatalf("back step %d: at %v want %v", i, c.date(), all[i])
		}
		c.prev()
	}
	// A leap day in a common year starts on the next real day.
	leap := newDayCursor(cal, DayDate{5, 3, 31})
	if got := leap.date(); got != (DayDate{6, 1, 1}) {
		t.Errorf("leap day in a common year started at %v", got)
	}
}
