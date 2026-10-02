// recurrence_service_test.go pins the service side of repeat rules and
// "this one only": who sees skipped dates and rule references, the checks a
// saved rule must pass (including the anchor-cycle refusal), the
// partial-update contract for recurrence_rule, the override writes and the
// preview.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// ruleWorld is an in-memory calendar with events and overrides behind the
// repository fakes, so the real service runs end to end.
type ruleWorld struct {
	cal       Calendar
	events    map[string]*Event
	overrides map[string][]OccurrenceOverride
	updated   *Event
	created   *Event
	setOv     *OccurrenceOverride
}

func newRuleWorld() *ruleWorld {
	cal := *ruleCal()
	cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 50, 1, 1
	cal.Moons = append(cal.Moons, Moon{ID: 6, Name: "Secret", CycleDays: 10, HiddenFromPlayers: true})
	yearly, weekly := RecurrenceYearly, RecurrenceWeekly
	w := &ruleWorld{cal: cal, overrides: map[string][]OccurrenceOverride{}, events: map[string]*Event{
		"masks":  {ID: "masks", CalendarID: "cal-1", Name: "Festival of Masks", Year: 1, Month: 1, Day: 30, IsRecurring: true, RecurrenceType: &yearly, Visibility: "everyone"},
		"secret": {ID: "secret", CalendarID: "cal-1", Name: "Secret Council", Year: 1, Month: 1, Day: 10, IsRecurring: true, RecurrenceType: &yearly, Visibility: "dm_only"},
		"weekly": {ID: "weekly", CalendarID: "cal-1", Name: "Market", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &weekly, Visibility: "everyone"},
		"once":   {ID: "once", CalendarID: "cal-1", Name: "Coronation", Year: 1, Month: 1, Day: 5, Visibility: "everyone"},
	}}
	return w
}

func (w *ruleWorld) add(e Event) {
	e.CalendarID = "cal-1"
	if e.Visibility == "" {
		e.Visibility = "everyone"
	}
	w.events[e.ID] = &e
}

func (w *ruleWorld) svc() CalendarService {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != w.cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			c := w.cal
			c.Months, c.Weekdays, c.Moons, c.Seasons = nil, nil, nil, nil
			return &c, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) { return append([]Month(nil), w.cal.Months...), nil },
		getWeekdaysFn: func(context.Context, string) ([]Weekday, error) {
			return append([]Weekday(nil), w.cal.Weekdays...), nil
		},
		getMoonsFn:   func(context.Context, string) ([]Moon, error) { return append([]Moon(nil), w.cal.Moons...), nil },
		getSeasonsFn: func(context.Context, string) ([]Season, error) { return append([]Season(nil), w.cal.Seasons...), nil },
	}
	all := func(role int) []Event {
		var out []Event
		for _, id := range []string{"masks", "secret", "weekly", "once", "rite", "dep", "fair", "hidden-moon", "dep-secret"} {
			if e, ok := w.events[id]; ok && (e.Visibility == "everyone" || permissions.CanSeeDmOnly(role)) {
				out = append(out, *e)
			}
		}
		return out
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if e, ok := w.events[id]; ok {
				c := *e
				return &c, nil
			}
			return nil, nil
		},
		getEventsByIDsFn: func(_ context.Context, _ string, ids []string) ([]Event, error) {
			var out []Event
			for _, id := range ids {
				if e, ok := w.events[id]; ok {
					out = append(out, *e)
				}
			}
			return out, nil
		},
		listForMonthFn: func(_ context.Context, _ string, _, _ int, role int) ([]Event, error) { return all(role), nil },
		listRuleEventsFn: func(context.Context, string) ([]Event, error) {
			var out []Event
			for _, e := range all(permissions.RoleOwner) {
				if e.RecurrenceType != nil && *e.RecurrenceType == RecurrenceByRule {
					out = append(out, e)
				}
			}
			return out, nil
		},
		listOverridesFn: func(_ context.Context, ids []string) (map[string][]OccurrenceOverride, error) {
			out := map[string][]OccurrenceOverride{}
			for _, id := range ids {
				if o := w.overrides[id]; len(o) > 0 {
					out[id] = o
				}
			}
			return out, nil
		},
		setOverrideFn: func(_ context.Context, o OccurrenceOverride) error { w.setOv = &o; return nil },
		updateEventFn: func(_ context.Context, e *Event) error { c := *e; w.updated = &c; return nil },
		createEventFn: func(_ context.Context, e *Event) error { c := *e; w.created = &c; return nil },
	}
	return NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
}

func findEvent(events []Event, id string) *Event {
	for i := range events {
		if events[i].ID == id {
			return &events[i]
		}
	}
	return nil
}

func wantStatus(t *testing.T, err error, status int, msg string) {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != status {
		t.Fatalf("err = %v, want HTTP %d", err, status)
	}
	if msg != "" && !strings.Contains(err.Error(), msg) {
		t.Fatalf("err = %v, want it to mention %q", err, msg)
	}
}

func TestListEventsForMonth_SkippedDatesOnlyForEditors(t *testing.T) {
	w := newRuleWorld()
	y, m, d := 1, 3, 2
	w.overrides["weekly"] = []OccurrenceOverride{
		{EventID: "weekly", Year: 1, Month: 1, Day: 8, Action: OverrideSkip},
		{EventID: "weekly", Year: 1, Month: 1, Day: 15, Action: OverrideMove, NewYear: &y, NewMonth: &m, NewDay: &d},
	}
	tests := []struct {
		name    string
		v       permissions.Viewer
		want    string
		skipped bool
	}{
		{"owner", ownerViewer("o"), "1,8s,22,29", true},
		{"scribe", scribeViewer("s"), "1,8s,22,29", true},
		{"player", playerViewer("p"), "1,22,29", false},
		{"public", publicViewer(), "1,22,29", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := w.svc().ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 1, 1, tt.v)
			if err != nil {
				t.Fatal(err)
			}
			e := findEvent(events, "weekly")
			if e == nil {
				t.Fatal("weekly event missing")
			}
			var parts []string
			for _, o := range e.Occurrences {
				s := mustJSON(t, o.Day)
				if o.Skipped {
					s += "s"
				}
				parts = append(parts, s)
			}
			if got := strings.Join(parts, ","); got != tt.want {
				t.Fatalf("occurrences %s, want %s", got, tt.want)
			}
			// The moved one shows in Beta, with its origin only for editors.
			beta, err := w.svc().ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 1, 3, tt.v)
			if err != nil {
				t.Fatal(err)
			}
			b := findEvent(beta, "weekly")
			var moved *Occurrence
			for i := range b.Occurrences {
				if b.Occurrences[i].Day == 2 {
					moved = &b.Occurrences[i]
				}
			}
			if moved == nil {
				t.Fatal("moved occurrence missing from Beta")
			}
			if (moved.MovedFrom != nil) != tt.skipped {
				t.Errorf("moved_from shown=%v, want %v", moved.MovedFrom != nil, tt.skipped)
			}
		})
	}
}

func TestListEventsForMonth_AllSkippedDropsEventForPlayers(t *testing.T) {
	w := newRuleWorld()
	for _, d := range []int{1, 8, 15, 22, 29} {
		w.overrides["weekly"] = append(w.overrides["weekly"], OccurrenceOverride{EventID: "weekly", Year: 1, Month: 1, Day: d, Action: OverrideSkip})
	}
	for _, tc := range []struct {
		v    permissions.Viewer
		want bool
	}{{ownerViewer("o"), true}, {playerViewer("p"), false}} {
		events, err := w.svc().ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 1, 1, tc.v)
		if err != nil {
			t.Fatal(err)
		}
		if got := findEvent(events, "weekly") != nil; got != tc.want {
			t.Errorf("role %d: weekly listed=%v, want %v", tc.v.Role(), got, tc.want)
		}
	}
}

// TestListEventsForMonth_RuleEventsAndRedaction: rule events land on their
// rule's dates; a player sees those dates but never the rule when it names
// a hidden moon or an anchor the player cannot see.
func TestListEventsForMonth_RuleEventsAndRedaction(t *testing.T) {
	w := newRuleWorld()
	byRule := RecurrenceByRule
	two := 2
	w.add(Event{ID: "dep", Name: "Masks Aftermath", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
		RecurrenceRule: &RecurrenceRule{Match: []RuleCondition{{Kind: RuleRelativeToEvent, EventID: "masks"}}, OffsetDays: -3}})
	w.add(Event{ID: "dep-secret", Name: "After the council", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
		RecurrenceRule: &RecurrenceRule{Match: []RuleCondition{{Kind: RuleAfterEvent, EventID: "secret", Days: &two}}}})
	w.add(Event{ID: "hidden-moon", Name: "Secret rite", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
		RecurrenceRule: &RecurrenceRule{Match: []RuleCondition{{Kind: RuleMoonPhase, MoonID: 6, Phase: "full"}}}})

	for _, tc := range []struct {
		name         string
		v            permissions.Viewer
		ruleRedacted bool
	}{{"owner", ownerViewer("o"), false}, {"player", playerViewer("p"), true}} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := w.svc().ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 1, 1, tc.v)
			if err != nil {
				t.Fatal(err)
			}
			dep := findEvent(events, "dep")
			if dep == nil || dep.RecurrenceRule == nil || len(dep.Occurrences) != 1 || dep.Occurrences[0].Day != 27 {
				t.Fatalf("aftermath (anchor visible to all): %+v", dep)
			}
			ds := findEvent(events, "dep-secret")
			if ds == nil || len(ds.Occurrences) != 1 || ds.Occurrences[0].Day != 12 {
				t.Fatalf("event after a hidden anchor must still show its dates: %+v", ds)
			}
			if (ds.RecurrenceRule == nil) != tc.ruleRedacted {
				t.Errorf("rule naming a hidden anchor redacted=%v, want %v", ds.RecurrenceRule == nil, tc.ruleRedacted)
			}
			hm := findEvent(events, "hidden-moon")
			if hm == nil || len(hm.Occurrences) == 0 {
				t.Fatalf("hidden-moon rule event missing its dates: %+v", hm)
			}
			if (hm.RecurrenceRule == nil) != tc.ruleRedacted {
				t.Errorf("rule naming a hidden moon redacted=%v, want %v", hm.RecurrenceRule == nil, tc.ruleRedacted)
			}
		})
	}
}

func TestCreateEvent_RuleChecks(t *testing.T) {
	rule := func(js string) json.RawMessage { return json.RawMessage(js) }
	byRule := RecurrenceByRule
	tests := []struct {
		name   string
		author permissions.Viewer
		rule   json.RawMessage
		status int
		msg    string
	}{
		{"rule type without a rule", ownerViewer("o"), nil, http.StatusUnprocessableEntity, "needs a recurrence_rule"},
		{"good weekday rule", scribeViewer("s"), rule(`{"match":[{"kind":"weekday","weekday":2}]}`), 0, ""},
		{"moon of another calendar", ownerViewer("o"), rule(`{"match":[{"kind":"moon_phase","moon_id":77,"phase":"full"}]}`), http.StatusUnprocessableEntity, "moon_id"},
		{"hidden moon for a scribe", scribeViewer("s"), rule(`{"match":[{"kind":"moon_phase","moon_id":6,"phase":"full"}]}`), http.StatusUnprocessableEntity, "moon_id"},
		{"hidden moon for the owner", ownerViewer("o"), rule(`{"match":[{"kind":"moon_phase","moon_id":6,"phase":"full"}]}`), 0, ""},
		{"dm_only anchor for a scribe answers as unknown", scribeViewer("s"), rule(`{"match":[{"kind":"relative_to_event","event_id":"secret"}]}`), http.StatusUnprocessableEntity, "not an event of this calendar"},
		{"dm_only anchor for the owner", ownerViewer("o"), rule(`{"match":[{"kind":"relative_to_event","event_id":"secret"}]}`), 0, ""},
		{"anchor that follows another event", ownerViewer("o"), rule(`{"match":[{"kind":"after_event","event_id":"dep","days":1}]}`), http.StatusUnprocessableEntity, "itself repeats relative"},
		{"unknown anchor", ownerViewer("o"), rule(`{"match":[{"kind":"after_event","event_id":"nope","days":1}]}`), http.StatusUnprocessableEntity, "not an event"},
		{"bad shape", ownerViewer("o"), rule(`{"match":[{"kind":"zodiac"}]}`), http.StatusUnprocessableEntity, "unknown condition kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			w.add(Event{ID: "dep", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: &RecurrenceRule{Match: []RuleCondition{{Kind: RuleRelativeToEvent, EventID: "masks"}}}})
			_, err := w.svc().CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{
				Name: "New", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: tt.rule, Author: tt.author, CanAuthorDmOnly: tt.author.SkipsPerUserRules(),
			})
			if tt.status == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if w.created == nil || w.created.RecurrenceRule == nil {
					t.Fatal("rule not stored")
				}
				return
			}
			wantStatus(t, err, tt.status, tt.msg)
		})
	}
}

// TestCreateEvent_RuleOnlyOnRuleType: a rule sent with any other repeat
// type is neither checked nor stored, so it can never lie dormant.
func TestCreateEvent_RuleOnlyOnRuleType(t *testing.T) {
	weekly := RecurrenceWeekly
	tests := []struct {
		name string
		rule json.RawMessage
	}{
		{"a valid rule", json.RawMessage(`{"match":[{"kind":"weekday","weekday":2}]}`)},
		{"a rule naming a missing anchor", json.RawMessage(`{"match":[{"kind":"relative_to_event","event_id":"gone"}]}`)},
		{"a malformed rule", json.RawMessage(`{"match":[]}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			_, err := w.svc().CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{
				Name: "Weekly", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &weekly,
				RecurrenceRule: tt.rule, Author: ownerViewer("o"), CanAuthorDmOnly: true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w.created == nil || w.created.RecurrenceRule != nil {
				t.Fatalf("a weekly event stored a rule: %+v", w.created)
			}
		})
	}
}

func TestUpdateEvent_RecurrenceRulePartialContract(t *testing.T) {
	byRule, weekly := RecurrenceByRule, RecurrenceWeekly
	stored := &RecurrenceRule{Match: []RuleCondition{{Kind: RuleDayOfMonth, Day: 3}}}
	tests := []struct {
		name   string
		input  UpdateEventInput
		status int
		want   string // the stored rule's day after the update, "" for none
	}{
		{"absent preserves (a rename)", UpdateEventInput{Name: patch.Of("Renamed")}, 0, "3"},
		{"present replaces", UpdateEventInput{RecurrenceRule: patch.Of(json.RawMessage(`{"match":[{"kind":"day_of_month","day":9}]}`))}, 0, "9"},
		{"null clears once the type moves off rule", UpdateEventInput{RecurrenceType: patch.Of(weekly), RecurrenceRule: patch.Null[json.RawMessage]()}, 0, ""},
		{"null while still a rule event is refused", UpdateEventInput{RecurrenceRule: patch.Null[json.RawMessage]()}, http.StatusUnprocessableEntity, ""},
		{"a bad rule is refused", UpdateEventInput{RecurrenceRule: patch.Of(json.RawMessage(`{"match":[]}`))}, http.StatusUnprocessableEntity, ""},
		{"moving off rule drops the stored rule", UpdateEventInput{RecurrenceType: patch.Of(weekly)}, 0, ""},
		{"a rule sent with another type is not stored", UpdateEventInput{RecurrenceType: patch.Of(weekly), RecurrenceRule: patch.Of(json.RawMessage(`{"match":[{"kind":"day_of_month","day":9}]}`))}, 0, ""},
		{"a rule sent with another type is not judged", UpdateEventInput{RecurrenceType: patch.Of(weekly), RecurrenceRule: patch.Of(json.RawMessage(`{"match":[]}`))}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			w.add(Event{ID: "fair", Name: "Fair", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule, RecurrenceRule: stored})
			err := w.svc().UpdateEvent(context.Background(), "fair", "cal-1", testCampaignA, tt.input, ownerViewer("o"))
			if tt.status != 0 {
				wantStatus(t, err, tt.status, "")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if w.updated.RecurrenceRule != nil {
				got = mustJSON(t, w.updated.RecurrenceRule.Match[0].Day)
			}
			if got != tt.want {
				t.Fatalf("stored rule day %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUpdateEvent_AnchorCycleRefused: an event other rules repeat relative
// to may not itself start repeating relative to another event, and no event
// may repeat relative to itself.
func TestUpdateEvent_AnchorCycleRefused(t *testing.T) {
	byRule := RecurrenceByRule
	tests := []struct {
		name, id, rule, msg string
	}{
		{"anchor gains after_event", "masks", `{"match":[{"kind":"after_event","event_id":"once","days":1}]}`, "other events repeat relative to this one"},
		{"self anchor", "dep", `{"match":[{"kind":"after_event","event_id":"dep","days":1}]}`, "relative to itself"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			w.add(Event{ID: "dep", Name: "Aftermath", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: &RecurrenceRule{Match: []RuleCondition{{Kind: RuleRelativeToEvent, EventID: "masks"}}}})
			err := w.svc().UpdateEvent(context.Background(), tt.id, "cal-1", testCampaignA, UpdateEventInput{
				RecurrenceType: patch.Of(byRule), RecurrenceRule: patch.Of(json.RawMessage(tt.rule)),
			}, ownerViewer("o"))
			wantStatus(t, err, http.StatusUnprocessableEntity, tt.msg)
		})
	}
}

func TestSetOccurrenceOverride(t *testing.T) {
	tests := []struct {
		name   string
		event  string
		occ    DayDate
		input  OccurrenceOverrideInput
		v      permissions.Viewer
		status int
	}{
		{"skip an occurrence", "weekly", DayDate{1, 1, 8}, OccurrenceOverrideInput{Action: OverrideSkip}, scribeViewer("s"), 0},
		{"move an occurrence", "weekly", DayDate{1, 1, 8}, OccurrenceOverrideInput{Action: OverrideMove, Year: 1, Month: 1, Day: 10}, ownerViewer("o"), 0},
		{"not an occurrence", "weekly", DayDate{1, 1, 9}, OccurrenceOverrideInput{Action: OverrideSkip}, ownerViewer("o"), http.StatusUnprocessableEntity},
		{"not a date of the calendar", "weekly", DayDate{1, 1, 31}, OccurrenceOverrideInput{Action: OverrideSkip}, ownerViewer("o"), http.StatusUnprocessableEntity},
		{"one-off event", "once", DayDate{1, 1, 5}, OccurrenceOverrideInput{Action: OverrideSkip}, ownerViewer("o"), http.StatusUnprocessableEntity},
		{"move onto another occurrence", "weekly", DayDate{1, 1, 8}, OccurrenceOverrideInput{Action: OverrideMove, Year: 1, Month: 1, Day: 15}, ownerViewer("o"), http.StatusConflict},
		{"move onto itself", "weekly", DayDate{1, 1, 8}, OccurrenceOverrideInput{Action: OverrideMove, Year: 1, Month: 1, Day: 8}, ownerViewer("o"), http.StatusUnprocessableEntity},
		{"unknown action", "weekly", DayDate{1, 1, 8}, OccurrenceOverrideInput{Action: "cancel"}, ownerViewer("o"), http.StatusUnprocessableEntity},
		{"dm_only event, scribe cannot see it", "secret", DayDate{1, 1, 10}, OccurrenceOverrideInput{Action: OverrideSkip}, scribeViewer("s"), http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			o, err := w.svc().SetOccurrenceOverride(context.Background(), tt.event, "cal-1", testCampaignA, tt.occ, tt.input, tt.v)
			if tt.status != 0 {
				wantStatus(t, err, tt.status, "")
				if w.setOv != nil {
					t.Fatal("a refused override was stored")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if w.setOv == nil || w.setOv.Action != tt.input.Action || o.Day != tt.occ.Day {
				t.Fatalf("stored %+v", w.setOv)
			}
		})
	}
}

func TestPreviewRecurrence(t *testing.T) {
	tests := []struct {
		name   string
		v      permissions.Viewer
		rule   string
		count  int
		want   int
		status int
	}{
		{"next five", playerViewer("p"), `{"match":[{"kind":"weekday","weekday":2}]}`, 0, 5, 0},
		{"count capped at ten", playerViewer("p"), `{"match":[{"kind":"weekday","weekday":2}]}`, 50, 10, 0},
		{"hidden anchor answers as unknown to a player", playerViewer("p"), `{"match":[{"kind":"relative_to_event","event_id":"secret"}]}`, 3, 0, http.StatusUnprocessableEntity},
		{"hidden anchor previews for the owner", ownerViewer("o"), `{"match":[{"kind":"relative_to_event","event_id":"secret"}]}`, 3, 3, 0},
		{"never matches: empty, flagged", ownerViewer("o"), `{"match":[{"kind":"month","month":1},{"kind":"day_of_month","day":31}]}`, 3, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newRuleWorld()
			out, err := w.svc().PreviewRecurrence(context.Background(), "cal-1", testCampaignA, RecurrencePreviewInput{
				Rule: json.RawMessage(tt.rule), Start: DayDate{1, 1, 1}, Count: tt.count,
			}, tt.v)
			if tt.status != 0 {
				wantStatus(t, err, tt.status, "")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Dates) != tt.want {
				t.Fatalf("got %d dates (%s), want %d", len(out.Dates), dates(out.Dates), tt.want)
			}
			if tt.want == 0 && !out.Truncated {
				t.Error("a preview that found nothing must say it stopped looking")
			}
		})
	}
}
