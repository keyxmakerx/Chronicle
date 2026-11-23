// event_outcome_test.go: table-driven tests for what became of an event:
// the status rules on update and import, and the list the calendar asks
// about once today moves past events.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestMergeEventStatus(t *testing.T) {
	weekly := RecurrenceWeekly
	day := func(y, m, d int) DayDate { return DayDate{Year: y, Month: m, Day: d} }
	tests := []struct {
		name      string
		evt       Event   // the event after the update's date fields merged
		was       DayDate // the date stored before
		in        UpdateEventInput
		wantErr   bool
		status    string
		movedFrom *DayDate
		outcome   *string
	}{
		{name: "nothing sent keeps planned", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10), status: EventStatusPlanned},
		{name: "an older row with no status reads planned", evt: Event{Year: 5, Month: 1, Day: 10, Status: ""}, was: day(5, 1, 10), status: EventStatusPlanned},
		{name: "happened with a line", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of(EventStatusHappened), Outcome: patch.Of("  Reached the well at dusk ")}, status: EventStatusHappened, outcome: ptr("Reached the well at dusk")},
		{name: "called off", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of(EventStatusCalledOff)}, status: EventStatusCalledOff},
		{name: "an unknown word is refused", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of("foiled")}, wantErr: true},
		{name: "moved without a new day is refused", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of(EventStatusMoved)}, wantErr: true},
		{name: "first move keeps the planned day", evt: Event{Year: 5, Month: 1, Day: 12}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of(EventStatusMoved)}, status: EventStatusMoved, movedFrom: ptr(day(5, 1, 10))},
		{name: "a second move keeps the first planned day",
			evt: Event{Year: 5, Month: 1, Day: 14, Status: EventStatusMoved, MovedFromYear: ptr(5), MovedFromMonth: ptr(1), MovedFromDay: ptr(10)},
			was: day(5, 1, 12), status: EventStatusMoved, movedFrom: ptr(day(5, 1, 10))},
		{name: "moved back onto its planned day is planned again",
			evt: Event{Year: 5, Month: 1, Day: 10, Status: EventStatusMoved, MovedFromYear: ptr(5), MovedFromMonth: ptr(1), MovedFromDay: ptr(10)},
			was: day(5, 1, 12), status: EventStatusPlanned},
		{name: "planned again leaves a moved event where it is",
			evt: Event{Year: 5, Month: 1, Day: 12, Status: EventStatusMoved, MovedFromYear: ptr(5), MovedFromMonth: ptr(1), MovedFromDay: ptr(10)},
			was: day(5, 1, 12), in: UpdateEventInput{Status: patch.Of(EventStatusPlanned)}, status: EventStatusPlanned},
		{name: "an empty line clears it", evt: Event{Year: 5, Month: 1, Day: 10, Status: EventStatusHappened, Outcome: ptr("old")}, was: day(5, 1, 10),
			in: UpdateEventInput{Outcome: patch.Of("   ")}, status: EventStatusHappened},
		{name: "null clears the line", evt: Event{Year: 5, Month: 1, Day: 10, Outcome: ptr("old")}, was: day(5, 1, 10),
			in: UpdateEventInput{Outcome: patch.Null[string]()}, status: EventStatusPlanned},
		{name: "a line over the limit is refused", evt: Event{Year: 5, Month: 1, Day: 10}, was: day(5, 1, 10),
			in: UpdateEventInput{Outcome: patch.Of(string(make([]rune, MaxOutcomeChars+1)) + "x")}, wantErr: true},
		{name: "a repeating event refuses a status", evt: Event{Year: 5, Month: 1, Day: 10, IsRecurring: true, RecurrenceType: &weekly}, was: day(5, 1, 10),
			in: UpdateEventInput{Status: patch.Of(EventStatusHappened)}, wantErr: true},
		{name: "a repeating event refuses a line", evt: Event{Year: 5, Month: 1, Day: 10, IsRecurring: true, RecurrenceType: &weekly}, was: day(5, 1, 10),
			in: UpdateEventInput{Outcome: patch.Of("it rained")}, wantErr: true},
		{name: "an event made to repeat goes back to planned",
			evt: Event{Year: 5, Month: 1, Day: 12, IsRecurring: true, RecurrenceType: &weekly, Status: EventStatusMoved, MovedFromYear: ptr(5), MovedFromMonth: ptr(1), MovedFromDay: ptr(10)},
			was: day(5, 1, 12), status: EventStatusPlanned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := tt.evt
			err := mergeEventStatus(&evt, tt.was, tt.in)
			if tt.wantErr {
				wantValidationErr(t, err, tt.name)
				return
			}
			if err != nil {
				t.Fatalf("mergeEventStatus: %v", err)
			}
			if evt.Status != tt.status {
				t.Errorf("status = %q, want %q", evt.Status, tt.status)
			}
			if got := evt.movedFrom(); (got == nil) != (tt.movedFrom == nil) || (got != nil && *got != *tt.movedFrom) {
				t.Errorf("moved from = %v, want %v", got, tt.movedFrom)
			}
			if (evt.Outcome == nil) != (tt.outcome == nil) || (evt.Outcome != nil && *evt.Outcome != *tt.outcome) {
				t.Errorf("outcome = %v, want %v", evt.Outcome, tt.outcome)
			}
		})
	}
}

func TestBuildEventStatus(t *testing.T) {
	weekly := RecurrenceWeekly
	tests := []struct {
		name      string
		evt       Event
		in        CreateEventInput
		status    string
		movedFrom bool
	}{
		{name: "none given is planned", evt: Event{Year: 5, Month: 1, Day: 10}, status: EventStatusPlanned},
		{name: "a restored happened", evt: Event{Year: 5, Month: 1, Day: 10}, in: CreateEventInput{Status: EventStatusHappened}, status: EventStatusHappened},
		{name: "a word it cannot use stays planned", evt: Event{Year: 5, Month: 1, Day: 10}, in: CreateEventInput{Status: "rumoured"}, status: EventStatusPlanned},
		{name: "moved with its planned day", evt: Event{Year: 5, Month: 1, Day: 12},
			in: CreateEventInput{Status: EventStatusMoved, MovedFrom: &DayDate{Year: 5, Month: 1, Day: 10}}, status: EventStatusMoved, movedFrom: true},
		{name: "moved without a planned day stays planned", evt: Event{Year: 5, Month: 1, Day: 12}, in: CreateEventInput{Status: EventStatusMoved}, status: EventStatusPlanned},
		{name: "moved from its own day stays planned", evt: Event{Year: 5, Month: 1, Day: 12},
			in: CreateEventInput{Status: EventStatusMoved, MovedFrom: &DayDate{Year: 5, Month: 1, Day: 12}}, status: EventStatusPlanned},
		{name: "a repeating event stays planned", evt: Event{Year: 5, Month: 1, Day: 12, IsRecurring: true, RecurrenceType: &weekly},
			in: CreateEventInput{Status: EventStatusCalledOff}, status: EventStatusPlanned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := tt.evt
			if err := buildEventStatus(&evt, tt.in); err != nil {
				t.Fatalf("buildEventStatus: %v", err)
			}
			if evt.Status != tt.status || (evt.movedFrom() != nil) != tt.movedFrom {
				t.Errorf("got status %q moved from %v; want %q, moved from set %v", evt.Status, evt.movedFrom(), tt.status, tt.movedFrom)
			}
		})
	}
}

func TestAppendPassed(t *testing.T) {
	weekly := RecurrenceWeekly
	from, to := DayDate{Year: 5, Month: 1, Day: 9}, DayDate{Year: 5, Month: 1, Day: 12}
	tests := []struct {
		name string
		evt  Event
		want []DayDate // the dates asked about
	}{
		{name: "a planned one-off on a passed day", evt: Event{ID: "a", Year: 5, Month: 1, Day: 10}, want: []DayDate{{5, 1, 10}}},
		{name: "the first passed day counts", evt: Event{ID: "a", Year: 5, Month: 1, Day: 9}, want: []DayDate{{5, 1, 9}}},
		{name: "the new today has not passed", evt: Event{ID: "a", Year: 5, Month: 1, Day: 12}},
		{name: "before the jump is not asked again", evt: Event{ID: "a", Year: 5, Month: 1, Day: 8}},
		{name: "already answered", evt: Event{ID: "a", Year: 5, Month: 1, Day: 10, Status: EventStatusHappened}},
		{name: "a span that ends on a passed day", evt: Event{ID: "a", Year: 5, Month: 1, Day: 7, EndYear: ptr(5), EndMonth: ptr(1), EndDay: ptr(10)}, want: []DayDate{{5, 1, 7}}},
		{name: "a span still running is not asked", evt: Event{ID: "a", Year: 5, Month: 1, Day: 10, EndYear: ptr(5), EndMonth: ptr(1), EndDay: ptr(14)}},
		{name: "each date of a repeating event, skipped ones left out",
			evt: Event{ID: "r", Year: 5, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &weekly,
				Occurrences: []Occurrence{{Year: 5, Month: 1, Day: 8}, {Year: 5, Month: 1, Day: 9}, {Year: 5, Month: 1, Day: 10, Skipped: true}, {Year: 5, Month: 1, Day: 11}}},
			want: []DayDate{{5, 1, 9}, {5, 1, 11}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := tt.evt
			seen := map[string]bool{}
			got := appendPassed(nil, &evt, from, to, seen)
			got = appendPassed(got, &evt, from, to, seen) // a second month read adds nothing
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows %+v, want %v", len(got), got, tt.want)
			}
			for i, p := range got {
				if (DayDate{p.Year, p.Month, p.Day}) != tt.want[i] {
					t.Errorf("row %d = %d-%d-%d, want %v", i, p.Year, p.Month, p.Day, tt.want[i])
				}
				if p.Repeating != evt.isRepeating() {
					t.Errorf("row %d repeating = %v", i, p.Repeating)
				}
			}
		})
	}
}

// passedFixture is setDateFixture's calendar (months of 30 and 20 days)
// whose event repo answers each month from byMonth.
func passedFixture(t *testing.T, byMonth map[[2]int][]Event) CalendarService {
	t.Helper()
	svc, _ := setDateFixture(t, nil)
	svc.(*calendarService).eventRepo = &fakeEventRepo{
		listForMonthFn: func(_ context.Context, _ string, y, m, _ int) ([]Event, error) {
			var out []Event
			for _, e := range byMonth[[2]int{y, m}] {
				e.CalendarID = "cal-1"
				out = append(out, e)
			}
			return out, nil
		},
	}
	return svc
}

func TestListPassedEvents(t *testing.T) {
	months := map[[2]int][]Event{
		{1000, 1}: {
			{ID: "b", Name: "Ambush", Year: 1000, Month: 1, Day: 29, Visibility: "dm_only"},
			{ID: "a", Name: "Arrival", Year: 1000, Month: 1, Day: 29, Visibility: "everyone", Outcome: ptr("a note")},
			{ID: "done", Name: "Done", Year: 1000, Month: 1, Day: 29, Status: EventStatusCalledOff},
			{ID: "span", Name: "Journey", Year: 1000, Month: 1, Day: 30, EndYear: ptr(1000), EndMonth: ptr(2), EndDay: ptr(2)},
		},
		{1000, 2}: {
			{ID: "span", Name: "Journey", Year: 1000, Month: 1, Day: 30, EndYear: ptr(1000), EndMonth: ptr(2), EndDay: ptr(2)},
			{ID: "today", Name: "Today", Year: 1000, Month: 2, Day: 5},
		},
	}
	tests := []struct {
		name      string
		from, to  DayDate
		wantIDs   []string
		wantErr   bool
		truncated bool
	}{
		{name: "across a month end, by day then name", from: DayDate{1000, 1, 28}, to: DayDate{1000, 2, 5}, wantIDs: []string{"b", "a", "span"}},
		{name: "a step back asks nothing", from: DayDate{1000, 2, 5}, to: DayDate{1000, 1, 28}},
		{name: "not a day of this calendar", from: DayDate{1000, 2, 25}, to: DayDate{1000, 2, 26}, wantErr: true},
		{name: "a jump of years stops at its first stretch", from: DayDate{1000, 1, 1}, to: DayDate{1100, 1, 1}, wantIDs: []string{"b", "a", "span", "today"}, truncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := passedFixture(t, months)
			out, err := svc.ListPassedEvents(context.Background(), "cal-1", testCampaignA, tt.from, tt.to, ownerViewer("u-owner"))
			if tt.wantErr {
				wantValidationErr(t, err, tt.name)
				return
			}
			if err != nil {
				t.Fatalf("ListPassedEvents: %v", err)
			}
			var ids []string
			for _, e := range out.Events {
				ids = append(ids, e.ID)
			}
			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
				}
			}
			if out.Truncated != tt.truncated {
				t.Errorf("truncated = %v, want %v", out.Truncated, tt.truncated)
			}
		})
	}
}

func TestListPassedEventsCapsTheSheet(t *testing.T) {
	var many []Event
	for i := 0; i < maxPassedEvents+5; i++ {
		many = append(many, Event{ID: "e" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Name: "E", Year: 1000, Month: 1, Day: 3})
	}
	svc := passedFixture(t, map[[2]int][]Event{{1000, 1}: many})
	out, err := svc.ListPassedEvents(context.Background(), "cal-1", testCampaignA, DayDate{1000, 1, 1}, DayDate{1000, 1, 5}, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("ListPassedEvents: %v", err)
	}
	if len(out.Events) != maxPassedEvents || !out.Truncated {
		t.Errorf("got %d events, truncated %v; want %d and truncated", len(out.Events), out.Truncated, maxPassedEvents)
	}
}

func TestUpdateEventStoresStatus(t *testing.T) {
	tests := []struct {
		name      string
		in        UpdateEventInput
		status    string
		movedFrom *DayDate
		wantErr   bool
	}{
		{name: "happened", in: UpdateEventInput{Status: patch.Of(EventStatusHappened), Outcome: patch.Of("Fine")}, status: EventStatusHappened},
		{name: "moved two days on", in: UpdateEventInput{Status: patch.Of(EventStatusMoved), Day: patch.Of(12)}, status: EventStatusMoved, movedFrom: &DayDate{1000, 1, 10}},
		{name: "moved nowhere", in: UpdateEventInput{Status: patch.Of(EventStatusMoved)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := eventDatesFixture(t)
			var saved *Event
			repo := svc.(*calendarService).eventRepo.(*fakeEventRepo)
			repo.updateEventFn = func(_ context.Context, e *Event) error { c := *e; saved = &c; return nil }
			err := svc.UpdateEvent(context.Background(), "evt-1", "cal-1", testCampaignA, tt.in, ownerViewer("u-owner"))
			if tt.wantErr {
				wantValidationErr(t, err, tt.name)
				if saved != nil {
					t.Error("a refused update must not be stored")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateEvent: %v", err)
			}
			if saved == nil || saved.Status != tt.status {
				t.Fatalf("stored %+v, want status %q", saved, tt.status)
			}
			if got := saved.movedFrom(); (got == nil) != (tt.movedFrom == nil) || (got != nil && *got != *tt.movedFrom) {
				t.Errorf("moved from = %v, want %v", got, tt.movedFrom)
			}
		})
	}
}

func TestSetAskWhenDayPasses(t *testing.T) {
	for _, ask := range []bool{true, false} {
		svc, _ := setDateFixture(t, nil)
		var got *bool
		svc.(*calendarService).calRepo.(*fakeCalendarRepo).setAskFn = func(_ context.Context, id string, a bool) error {
			if id != "cal-1" {
				t.Errorf("calendar id = %q", id)
			}
			got = &a
			return nil
		}
		pub := &recordingPublisher{}
		svc.(*calendarService).SetEventPublisher(pub)
		if err := svc.SetAskWhenDayPasses(context.Background(), "cal-1", testCampaignA, ask); err != nil {
			t.Fatalf("SetAskWhenDayPasses: %v", err)
		}
		if got == nil || *got != ask {
			t.Errorf("stored %v, want %v", got, ask)
		}
		if len(pub.got) != 1 || pub.got[0].eventType != PubStructureUpdated {
			t.Errorf("published %+v, want one %s", pub.got, PubStructureUpdated)
		}
	}
	svc, _ := setDateFixture(t, nil)
	if err := svc.SetAskWhenDayPasses(context.Background(), "cal-1", "another-campaign", false); err == nil {
		t.Error("a calendar of another campaign must be refused")
	}
}

func TestParseDayParam(t *testing.T) {
	tests := []struct {
		raw     string
		want    DayDate
		wantErr bool
	}{
		{raw: "1000-2-5", want: DayDate{1000, 2, 5}},
		{raw: "-12-3-4", want: DayDate{-12, 3, 4}},
		{raw: "1000-2", wantErr: true},
		{raw: "1000-x-5", wantErr: true},
		{raw: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseDayParam(tt.raw)
			if (err != nil) != tt.wantErr || (!tt.wantErr && got != tt.want) {
				t.Errorf("parseDayParam(%q) = %v, %v", tt.raw, got, err)
			}
		})
	}
}
