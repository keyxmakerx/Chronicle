package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func harptosCal(id string) calendar.Calendar {
	return calendar.Calendar{ID: id, Name: "Harptos", CurrentYear: 1492, CurrentMonth: 1, CurrentDay: 1,
		Months: []calendar.Month{{Name: "Hammer", Days: 30}, {Name: "Ches", Days: 30}}}
}

// `main: true` makes a calendar the main one, alone or with other changes,
// whether none was main or another one was.
func TestCalendarKind_MakeMain(t *testing.T) {
	tests := []struct {
		name     string
		main     *calendar.Calendar
		fields   map[string]any
		summary  []string
		warn     string
		wantDate bool
	}{
		{"none was main", nil, map[string]any{"main": true}, []string{"makes it the main calendar on Harptos"}, "", false},
		{"switches from another", &calendar.Calendar{ID: "old", Name: "Old Reckoning"}, map[string]any{"main": "yes"},
			[]string{"makes it the main calendar"}, `"Old Reckoning" stops being the main calendar`, false},
		{"and changes it", nil, map[string]any{"main": true, "current_month": "Ches", "current_day": 3, "current_year": 1492},
			[]string{"makes it the main calendar", "sets today to"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStructure{cal: tt.main, others: []calendar.Calendar{harptosCal("h")}}
			k := CalendarKind{Svc: f}
			r := rec(KindCalendar, ActionUpdate, "harptos", tt.fields, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if p.Error != "" {
				t.Fatalf("plan: %+v", p)
			}
			for _, want := range tt.summary {
				if !strings.Contains(p.Summary, want) {
					t.Errorf("summary %q lacks %q", p.Summary, want)
				}
			}
			if tt.warn != "" && !strings.Contains(strings.Join(p.Warnings, "|"), tt.warn) {
				t.Errorf("warnings %v lack %q", p.Warnings, tt.warn)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if f.dflt != "h" {
				t.Errorf("main calendar set to %q, want h", f.dflt)
			}
			if tt.wantDate && (len(f.today) != 3 || f.today[1] != 2 || f.today[2] != 3) {
				t.Errorf("today %v", f.today)
			}
		})
	}
}

func TestCalendarKind_MakeMainRefusals(t *testing.T) {
	main := harptosCal("h")
	tests := []struct {
		name   string
		main   *calendar.Calendar
		others []calendar.Calendar
		rname  string
		fields map[string]any
		want   string
	}{
		{"no main, no main key", nil, []calendar.Calendar{harptosCal("h")}, "Harptos", map[string]any{"current_day": 2}, "add main: true"},
		{"no calendars at all", nil, nil, "Harptos", map[string]any{"main": true}, "there is no calendar called"},
		{"unknown name", nil, []calendar.Calendar{harptosCal("h")}, "Nowhere", map[string]any{"main": true}, `no calendar called "Nowhere"`},
		{"needs a name", nil, []calendar.Calendar{harptosCal("h")}, "", map[string]any{"main": true}, "needs the name"},
		{"main false", &main, nil, "Harptos", map[string]any{"main": false}, "can't unset"},
		{"other name without main", &main, []calendar.Calendar{{ID: "o", Name: "Other"}}, "Other", map[string]any{"current_day": 2}, "add main: true to switch"},
		{"already main and nothing else", &main, nil, "Harptos", map[string]any{"main": true}, "changes nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStructure{cal: tt.main, others: tt.others}
			p := CalendarKind{Svc: f}.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, tt.rname, tt.fields, ""))
			if !strings.Contains(p.Error, tt.want) {
				t.Fatalf("error %q, want %q", p.Error, tt.want)
			}
		})
	}
}

// Events after a block that makes a calendar the main one are checked
// against that calendar, not the old main or none.
func TestPlanAll_EventsAfterMakeMain(t *testing.T) {
	svc := fakeStructureCal{&fakeStructure{others: []calendar.Calendar{harptosCal("h")}}, newCal()}
	reg := NewRegistry(CalendarKind{Svc: svc.fakeStructure}, EventKind{Svc: svc})
	feast := rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": "Ches", "day": 3}, "")
	main := rec(KindCalendar, ActionUpdate, "Harptos", map[string]any{"main": true}, "")
	_, plans := reg.PlanAll(context.Background(), camp, owner, []Record{feast, main, feast})
	if !strings.HasPrefix(plans[0].Error, needsCalendarPrefix) || !strings.Contains(plans[0].Error, "main: true") {
		t.Errorf("event before: %+v", plans[0])
	}
	if plans[1].Error != "" {
		t.Fatalf("calendar: %+v", plans[1])
	}
	if plans[2].Error != "" || !strings.Contains(plans[2].Summary, "on Harptos") {
		t.Errorf("event after: %+v", plans[2])
	}
}

// With no main calendar, the calendar lookups answer with how to set one
// up instead of "Not answered".
func TestLookups_NoMainCalendar(t *testing.T) {
	tests := []struct {
		name   string
		others []calendar.Calendar
		what   string
		want   []string
	}{
		{"calendars exist", []calendar.Calendar{harptosCal("h"), {ID: "o", Name: "Old"}}, lookupWhatCalendar,
			[]string{`2 calendars ("Harptos", "Old")`, "kind: calendar\naction: update\nname: \"Harptos\"\nmain: true"}},
		{"no calendar", nil, lookupWhatCalendar, []string{"no calendar yet", "`action: create`", "`main: true` (update only)"}},
		{"events point at the calendar lookup", nil, "events", []string{"no main calendar yet", "`what: calendar`"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lookups{Cal: fakeStructureCal{&fakeStructure{others: tt.others}, newCal()}}
			fields := map[string]any{}
			if tt.what == "events" {
				fields = map[string]any{"year": 1492, "month": 1}
			}
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup(tt.what, fields)})
			if ans[0].Error != "" {
				t.Fatalf("not answered: %s", ans[0].Error)
			}
			md := Markdown(ans)
			for _, w := range tt.want {
				if !strings.Contains(md, w) {
					t.Errorf("answer lacks %q:\n%s", w, md)
				}
			}
			if strings.Contains(md, "Not answered") {
				t.Errorf("still a dead end:\n%s", md)
			}
		})
	}
}

func TestLookups_Format(t *testing.T) {
	l := &Lookups{Kinds: NewRegistry(CalendarKind{}, EventKind{})}
	tests := []struct {
		name, block, want, err string
	}{
		{"a kind's doc", KindCalendar, "`main: true` (update only)", ""},
		{"normalised", " Event ", "## Format: kind: event", ""},
		{"unknown lists the kinds", "dragon", "", "the kinds are calendar, event"},
		{"missing", "", "", "it needs `block:`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatFormat, map[string]any{"block": tt.block})})[0]
			if tt.err != "" {
				if !strings.Contains(ans.Error, tt.err) {
					t.Fatalf("error %q, want %q", ans.Error, tt.err)
				}
				return
			}
			if ans.Error != "" || !strings.Contains(ans.Text, tt.want) {
				t.Fatalf("answer %+v lacks %q", ans, tt.want)
			}
		})
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatFormat, map[string]any{"block": KindCalendar})})[0]; got.Error == "" {
		t.Error("a server without the registry must say formats can't be looked up")
	}
}

// Not answered lines never end in a doubled full stop.
func TestMarkdown_NoDoubleStop(t *testing.T) {
	md := Markdown([]LookupAnswer{{Chip: "X", Error: "choose one on the Calendars page."}})
	if strings.Contains(md, "..") {
		t.Errorf("%q", md)
	}
}

func TestRegistry_DocsFor(t *testing.T) {
	reg := NewRegistry(EventKind{}, CalendarKind{}, TableKind{})
	got := reg.DocsFor(CalendarKinds...)
	if !strings.Contains(got, "### kind: calendar") || !strings.Contains(got, "### kind: event") || strings.Contains(got, "kind: table") {
		t.Errorf("docs: %s", got)
	}
	if strings.Index(got, "kind: calendar") > strings.Index(got, "kind: event") {
		t.Error("docs must follow the order asked for")
	}
}
