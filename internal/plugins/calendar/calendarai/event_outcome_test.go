package calendarai

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// TestEventKind_Outcome covers status and what_happened on an event row:
// what the owner's review says, what reaches the calendar, and what is
// refused before review.
func TestEventKind_Outcome(t *testing.T) {
	ctx := context.Background()
	weekly := calendar.RecurrenceWeekly
	stored := []calendar.Event{
		{ID: "e1", Name: "Arrival", Year: 1492, Month: 1, Day: 11},
		{ID: "e2", Name: "Market", Year: 1492, Month: 1, Day: 4, IsRecurring: true, RecurrenceType: &weekly},
	}
	tests := []struct {
		name, action, title string
		fields              map[string]any
		wantErr             string
		wantSummary         []string
		check               func(t *testing.T, c *fakeCal)
	}{
		{name: "happened with a line", action: ActionUpdate, title: "Arrival",
			fields:      map[string]any{"year": 1492, "month": 1, "day": 11, "status": "happened", "what_happened": "Reached the well at dusk"},
			wantSummary: []string{"marks it happened", `notes what happened: "Reached the well at dusk"`},
			check: func(t *testing.T, c *fakeCal) {
				in := c.updates[0]
				if s, _ := in.Status.Get(); s != calendar.EventStatusHappened {
					t.Errorf("status = %q", s)
				}
				if l, _ := in.Outcome.Get(); l != "Reached the well at dusk" {
					t.Errorf("line = %q", l)
				}
			}},
		{name: "a status written as a person would", action: ActionUpdate, title: "Arrival",
			fields:      map[string]any{"year": 1492, "month": 1, "day": 11, "status": "Called off"},
			wantSummary: []string{"calls it off"},
			check: func(t *testing.T, c *fakeCal) {
				if s, _ := c.updates[0].Status.Get(); s != calendar.EventStatusCalledOff {
					t.Errorf("status = %q", s)
				}
			}},
		{name: "none clears the line", action: ActionUpdate, title: "Arrival",
			fields:      map[string]any{"year": 1492, "month": 1, "day": 11, "what_happened": "none"},
			wantSummary: []string{"clears what happened"},
			check: func(t *testing.T, c *fakeCal) {
				if !c.updates[0].Outcome.IsNull() {
					t.Errorf("outcome = %+v, want null", c.updates[0].Outcome)
				}
			}},
		{name: "moved with its new day", action: ActionUpdate, title: "Arrival",
			fields:      map[string]any{"year": 1492, "month": 1, "day": 11, "status": "moved", "move_to_year": 1492, "move_to_month": 1, "move_to_day": 12},
			wantSummary: []string{"marks it moved"},
			check: func(t *testing.T, c *fakeCal) {
				in := c.updates[0]
				if s, _ := in.Status.Get(); s != calendar.EventStatusMoved {
					t.Errorf("status = %q", s)
				}
				if d, _ := in.Day.Get(); d != 12 {
					t.Errorf("day = %d", d)
				}
			}},
		{name: "a new event that already happened", action: ActionCreate, title: "Feast",
			fields: map[string]any{"year": 1492, "month": 1, "day": 2, "status": "happened", "what_happened": "Everyone came"},
			check: func(t *testing.T, c *fakeCal) {
				in := c.events[0]
				if in.Status != calendar.EventStatusHappened || in.Outcome == nil || *in.Outcome != "Everyone came" {
					t.Errorf("created %+v", in)
				}
			}},
		{name: "an unknown status", action: ActionUpdate, title: "Arrival",
			fields: map[string]any{"year": 1492, "month": 1, "day": 11, "status": "foiled"}, wantErr: "status must be"},
		{name: "moved without a day", action: ActionUpdate, title: "Arrival",
			fields: map[string]any{"year": 1492, "month": 1, "day": 11, "status": "moved"}, wantErr: "move_to_year"},
		{name: "moved on create", action: ActionCreate, title: "Feast",
			fields: map[string]any{"year": 1492, "month": 1, "day": 2, "status": "moved"}, wantErr: "can't be moved yet"},
		{name: "none on create", action: ActionCreate, title: "Feast",
			fields: map[string]any{"year": 1492, "month": 1, "day": 2, "what_happened": "none"}, wantErr: "only applies to action: update"},
		{name: "a line over the limit", action: ActionUpdate, title: "Arrival",
			fields: map[string]any{"year": 1492, "month": 1, "day": 11, "what_happened": strings.Repeat("x", calendar.MaxOutcomeChars+1)}, wantErr: "up to"},
		{name: "a repeating event takes no status", action: ActionUpdate, title: "Market",
			fields: map[string]any{"year": 1492, "month": 1, "day": 4, "status": "happened"}, wantErr: "one-off events"},
		{name: "nothing with delete", action: ActionDelete, title: "Arrival",
			fields: map[string]any{"year": 1492, "month": 1, "day": 11, "status": "happened"}, wantErr: "action: delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			c.stored = append([]calendar.Event(nil), stored...)
			k := EventKind{Svc: c}
			r := rec("event", tt.action, tt.title, tt.fields, "")
			p := k.Plan(ctx, camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error = %q, want it to say %q", p.Error, tt.wantErr)
				}
				if err := k.Apply(ctx, camp, owner, r); err == nil {
					t.Fatal("apply took a row the plan refused")
				}
				if len(c.updates)+len(c.events) != 0 {
					t.Fatal("a refused row reached the calendar")
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %+v", p)
			}
			for _, w := range tt.wantSummary {
				if !strings.Contains(p.Summary, w) {
					t.Errorf("summary %q does not say %q", p.Summary, w)
				}
			}
			if err := k.Apply(ctx, camp, owner, r); err != nil {
				t.Fatalf("apply: %v", err)
			}
			tt.check(t, c)
		})
	}
}

func TestOutcomeText(t *testing.T) {
	cal := &calendar.Calendar{Months: []calendar.Month{{Name: "Hammer", Days: 30}}}
	line, y, m, d := "Foiled by the party", 1492, 1, 10
	tests := []struct {
		name string
		e    calendar.Event
		want string
	}{
		{"planned says nothing", calendar.Event{Status: calendar.EventStatusPlanned}, ""},
		{"called off with its line", calendar.Event{Status: calendar.EventStatusCalledOff, Outcome: &line}, `, status called_off, what happened: "Foiled by the party"`},
		{"moved names its planned day", calendar.Event{Status: calendar.EventStatusMoved, MovedFromYear: &y, MovedFromMonth: &m, MovedFromDay: &d}, ", status moved (planned for "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outcomeText(cal, tt.e)
			if tt.want == "" && got != "" || !strings.HasPrefix(got, tt.want) {
				t.Errorf("outcomeText = %q, want it to start %q", got, tt.want)
			}
		})
	}
}
