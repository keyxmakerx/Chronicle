// recurrence_budget_test.go pins how far a month read may walk: a counted
// rule whose start is years back still has its dates, a read that cannot
// finish keeps the event and says its dates are unknown, and many rules on
// one anchor walk that anchor once, all inside one request's budget.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// budgetCal is a plain 12 x 30-day calendar with a 7-day week and one moon.
func budgetCal() *Calendar {
	months := make([]Month, 12)
	for i := range months {
		months[i] = Month{Name: fmt.Sprintf("M%d", i+1), Days: 30}
	}
	return &Calendar{
		ID: "cal-b", CampaignID: testCampaignA, Visibility: "everyone",
		Months: months, Weekdays: make([]Weekday, 7),
		Moons: []Moon{{ID: 1, Name: "Luna", CycleDays: 29.5}},
	}
}

// budgetService is a service whose repositories answer only what
// expandMonth reads: moons, seasons, anchors and overrides.
func budgetService(cal *Calendar, anchors []Event) *calendarService {
	return &calendarService{
		calRepo: &fakeCalendarRepo{
			getMoonsFn:   func(context.Context, string) ([]Moon, error) { return append([]Moon(nil), cal.Moons...), nil },
			getSeasonsFn: func(context.Context, string) ([]Season, error) { return nil, nil },
		},
		eventRepo: &fakeEventRepo{
			getEventsByIDsFn: func(context.Context, string, []string) ([]Event, error) { return anchors, nil },
			listOverridesFn: func(context.Context, []string) (map[string][]OccurrenceOverride, error) {
				return map[string][]OccurrenceOverride{}, nil
			},
		},
	}
}

func TestExpandMonth_CountedRuleFarFromItsStart(t *testing.T) {
	cal := budgetCal()
	maxOcc := 200
	fullMoons := ruleEvent("fm", 1, 1, 1, mustRule(t, `{"match":[{"kind":"moon_phase","moon_id":1,"phase":"full"}]}`))
	fullMoons.RecurrenceMaxOccurrences = &maxOcc
	farAway := ruleEvent("far", 1, 1, 1, mustRule(t, `{"match":[{"kind":"day_of_month","day":1}],"every":2}`))
	s := budgetService(cal, nil)

	tests := []struct {
		name          string
		event         Event
		year          int
		wantDates     bool
		wantTruncated bool
	}{
		// 200 full moons run about 16 years, so year 8 is well inside them,
		// though more than five years past the start.
		{"max 200 full moons, year 8", fullMoons, 8, true, false},
		// Past the count bound: the event stays, with no dates and the flag.
		{"start beyond the count bound", farAway, 1 + 2*ruleCountYears, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range []struct {
				name   string
				viewer func() []Event
			}{
				{"owner", func() []Event {
					out, err := s.expandMonth(context.Background(), cal, []Event{tt.event}, tt.year, 1, ownerViewer("u"))
					if err != nil {
						t.Fatal(err)
					}
					return out
				}},
				{"player", func() []Event {
					out, err := s.expandMonth(context.Background(), cal, []Event{tt.event}, tt.year, 1, playerViewer("p"))
					if err != nil {
						t.Fatal(err)
					}
					return out
				}},
			} {
				got := v.viewer()
				if len(got) != 1 {
					t.Fatalf("%s: event dropped from the month (got %d events)", v.name, len(got))
				}
				e := got[0]
				if (len(e.Occurrences) > 0) != tt.wantDates || e.OccurrencesTruncated != tt.wantTruncated {
					t.Fatalf("%s: occurrences=%v truncated=%v", v.name, e.Occurrences, e.OccurrencesTruncated)
				}
				if tt.wantTruncated {
					raw, _ := json.Marshal(e)
					if !strings.Contains(string(raw), `"occurrences":[]`) || !strings.Contains(string(raw), `"occurrences_truncated":true`) {
						t.Fatalf("%s: truncated event must send an empty list and the flag: %s", v.name, raw)
					}
				}
			}
		})
	}

	// An override write checks the date the same way, so year 8 is a date
	// the event really falls on.
	out, _ := s.expandMonth(context.Background(), cal, []Event{fullMoons}, 8, 1, ownerViewer("u"))
	o := out[0].Occurrences[0]
	x := newExpander(cal, nil, nil)
	if nat, tr := x.natural(&fullMoons, DayDate{o.Year, o.Month, o.Day}, DayDate{o.Year, o.Month, o.Day}, 1, 0); len(nat) != 1 || tr {
		t.Fatalf("an override on %d-%d-%d would be refused: %v truncated=%v", o.Year, o.Month, o.Day, nat, tr)
	}
}

// manyDependents is n rule events, each with six after_event conditions
// (one to six days after) on the same anchor, ANDed with any weekday so
// every condition is actually tested.
func manyDependents(t *testing.T, n int) []Event {
	var conds []string
	for d := 1; d <= 6; d++ {
		conds = append(conds, fmt.Sprintf(`{"kind":"after_event","event_id":"daily","days":%d}`, d))
	}
	rule := `{"match":[` + strings.Join(conds, ",") + `]}`
	out := make([]Event, n)
	for i := range out {
		out[i] = ruleEvent(fmt.Sprintf("dep-%d", i), 1, 1, 1, mustRule(t, rule))
	}
	return out
}

// countedDailyAnchor repeats every 99th day from year 1, so each walk of it
// counts from its start.
func countedDailyAnchor(t *testing.T) Event {
	return ruleEvent("daily", 1, 1, 1, mustRule(t, `{"match":[{"kind":"weekday","weekdays":[0,1,2,3,4,5,6]}],"every":99}`))
}

func TestExpander_AnchorWalkedOncePerRequest(t *testing.T) {
	cal := budgetCal()
	anchor := countedDailyAnchor(t)
	deps := manyDependents(t, 100)
	x := newExpander(cal, map[string]*Event{"daily": &anchor}, nil)

	from, to := DayDate{8, 1, 1}, DayDate{8, 1, 30}
	for i := range deps {
		if _, truncated := x.occurrences(&deps[i], from, to, 0); truncated {
			t.Fatalf("dependent %d truncated", i)
		}
	}
	// One anchor walk from its start (8 years, plus the memo's padding)
	// and 100 month-long walks. Without the memo it is 600 anchor walks,
	// well over a million days.
	used := expandBudgetDays - x.budget
	if limit := 8*360 + 2*anchorWindowPad + 100*30 + 1000; used > limit {
		t.Fatalf("one month read tested %d days, want at most %d", used, limit)
	}
}

func TestExpander_BudgetDegradesToTruncated(t *testing.T) {
	cal := budgetCal()
	anchor := countedDailyAnchor(t)
	deps := manyDependents(t, 50)
	// Every dependent counts from its start, so each walks years of days.
	for i := range deps {
		deps[i].RecurrenceRule.Every = 2
	}
	x := newExpander(cal, map[string]*Event{"daily": &anchor}, nil)
	x.budget = 20000

	from, to := DayDate{8, 1, 1}, DayDate{8, 1, 30}
	truncatedFrom := -1
	for i := range deps {
		_, truncated := x.occurrences(&deps[i], from, to, 0)
		if truncated && truncatedFrom < 0 {
			truncatedFrom = i
		}
		if !truncated && truncatedFrom >= 0 {
			t.Fatalf("dependent %d expanded after the budget ran out at %d", i, truncatedFrom)
		}
	}
	if truncatedFrom < 0 {
		t.Fatal("a 20000-day budget expanded 50 eight-year counted walks without truncating")
	}
	if x.budget != 0 {
		t.Fatalf("budget left %d, want it spent", x.budget)
	}
}
