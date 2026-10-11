package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// fakeStructure is the calendar service as the calendar kind sees it.
// cal nil means the campaign has no main calendar; others are the
// campaign's calendars that are not the main one.
type fakeStructure struct {
	cal     *calendar.Calendar
	others  []calendar.Calendar
	created *calendar.ImportResult
	dflt    string
	edits   []calendar.StructureEdit
	applied int
	updates []calendar.UpdateCalendarInput
	today   []int
	eras    []calendar.EraInput
	eraUps  []calendar.UpdateEraInput
	preview calendar.StructurePreview
}

func (f *fakeStructure) GetDefaultCalendarForViewer(context.Context, string, permissions.Viewer) (*calendar.Calendar, error) {
	if f.cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return f.cal, nil
}
func (f *fakeStructure) ListCalendars(context.Context, string, permissions.Viewer) ([]calendar.Calendar, error) {
	out := append([]calendar.Calendar{}, f.others...)
	if f.cal != nil {
		out = append(out, *f.cal)
	}
	return out, nil
}
func (f *fakeStructure) CreateCalendarFromImport(_ context.Context, _ string, ir *calendar.ImportResult, _ calendar.CreateCalendarFromImportOptions) (*calendar.Calendar, error) {
	f.created = ir
	return &calendar.Calendar{ID: "new"}, nil
}
func (f *fakeStructure) SetDefaultCalendar(_ context.Context, _, id string) error {
	f.dflt = id
	return nil
}
func (f *fakeStructure) UpdateCalendar(_ context.Context, _, _ string, in calendar.UpdateCalendarInput) error {
	f.updates = append(f.updates, in)
	return nil
}
func (f *fakeStructure) SetCurrentDate(_ context.Context, _, _ string, y, m, d, _, _ int) error {
	f.today = []int{y, m, d}
	return nil
}
func (f *fakeStructure) PreviewStructureEdit(_ context.Context, _, _ string, e calendar.StructureEdit) (*calendar.StructurePreview, error) {
	f.edits = append(f.edits, e)
	p := f.preview
	p.Fingerprint = "fp"
	return &p, nil
}
func (f *fakeStructure) ApplyStructureEdit(_ context.Context, _, _, fp string, _ calendar.StructureEdit) (*calendar.StructurePreview, error) {
	if fp != "fp" {
		return nil, apperror.NewConflict("stale")
	}
	f.applied++
	return &f.preview, nil
}
func (f *fakeStructure) CreateEra(_ context.Context, _, _ string, in calendar.EraInput) (*calendar.Era, error) {
	f.eras = append(f.eras, in)
	return &calendar.Era{}, nil
}
func (f *fakeStructure) UpdateEra(_ context.Context, _ int, _, _ string, in calendar.UpdateEraInput) error {
	f.eraUps = append(f.eraUps, in)
	return nil
}

// fakeStructureCal also answers the event kind, so one fake serves a
// calendar block and the events after it.
type fakeStructureCal struct {
	*fakeStructure
	*fakeCal
}

func (f fakeStructureCal) GetDefaultCalendarForViewer(ctx context.Context, c string, v permissions.Viewer) (*calendar.Calendar, error) {
	return f.fakeStructure.GetDefaultCalendarForViewer(ctx, c, v)
}

func newCalendarFields() map[string]any {
	return map[string]any{
		"year_label":    "DR",
		"current_year":  1492,
		"current_month": "Alturiak",
		"current_day":   5,
		"months": []any{
			map[string]any{"name": "Hammer", "days": 30, "season": "Winter"},
			map[string]any{"name": "Alturiak", "days": 30, "season": "Spring"},
			map[string]any{"Name": "Ches", "Days": 30, "Season": "Winter"},
		},
		"weekdays": []any{"First", map[string]any{"name": "Rest", "rest_day": true}},
		"moons":    []any{map[string]any{"name": "Selune", "cycle": 30.4375, "phase_offset": 2, "color": "#eeeeee"}},
		"eras":     []any{map[string]any{"name": "Dalereckoning", "start_year": 1}},
	}
}

func TestCalendarKind_Create(t *testing.T) {
	f := &fakeStructure{}
	k := CalendarKind{Svc: f}
	r := rec(KindCalendar, ActionCreate, "Harptos", newCalendarFields(), "The Forgotten Realms year.")
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" {
		t.Fatalf("plan: %+v", p)
	}
	for _, want := range []string{"3 months", "2-day week", "1 moon", "1 era", "Alturiak 5, 1492 DR"} {
		if !strings.Contains(p.Summary, want) {
			t.Errorf("summary %q lacks %q", p.Summary, want)
		}
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	ir := f.created
	if ir == nil || f.dflt != "new" {
		t.Fatalf("created %v default %q: the new calendar must become the main one", ir, f.dflt)
	}
	if ir.CalendarName != "Harptos" || *ir.Settings.EpochName != "DR" || *ir.Settings.Description != "The Forgotten Realms year." {
		t.Errorf("settings %+v", ir.Settings)
	}
	if *ir.Today.Month != 2 || *ir.Today.Day != 5 || ir.Today.Year != 1492 {
		t.Errorf("today %+v", ir.Today)
	}
	if !ir.Weekdays[1].IsRestDay || ir.Moons[0].CycleDays != 30.4375 || ir.Moons[0].PhaseOffset != 2 {
		t.Errorf("weekdays %+v moons %+v", ir.Weekdays, ir.Moons)
	}
	// Winter ends the year and starts it, so it is one season that wraps.
	if len(ir.Seasons) != 2 {
		t.Fatalf("seasons %+v", ir.Seasons)
	}
	w := ir.Seasons[0]
	if w.Name != "Winter" || w.StartMonth != 3 || w.EndMonth != 1 || w.EndDay != 30 {
		t.Errorf("winter %+v", w)
	}
	if ir.Eras[0].StartMonth != 1 || ir.Eras[0].StartDay != 1 {
		t.Errorf("era %+v", ir.Eras[0])
	}
}

func TestCalendarKind_Refusals(t *testing.T) {
	existing := &calendar.Calendar{ID: "cal", Name: "Harptos", Months: []calendar.Month{{Name: "Hammer", Days: 30}}}
	full := newCalendarFields()
	noDate := newCalendarFields()
	delete(noDate, "current_day")
	badSeason := newCalendarFields()
	badSeason["seasons"] = []any{map[string]any{"name": "Wet", "start_month": "Nowhere", "end_month": 1}}
	tests := []struct {
		name   string
		cal    *calendar.Calendar
		a      Actor
		action string
		fields map[string]any
		want   string
	}{
		{"player", nil, player, ActionCreate, full, "only the campaign owner"},
		{"already has one", existing, owner, ActionCreate, full, "already has the calendar"},
		{"remove", existing, owner, ActionDelete, full, "Calendars page"},
		{"update with none", nil, owner, ActionUpdate, full, "use action: create"},
		{"no months", nil, owner, ActionCreate, map[string]any{"weekdays": []any{"a"}}, "needs months"},
		{"no date", nil, owner, ActionCreate, noDate, "current_day"},
		{"unknown season month", nil, owner, ActionCreate, badSeason, "not one of the months"},
		{"nothing to change", existing, owner, ActionUpdate, map[string]any{}, "changes nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := CalendarKind{Svc: &fakeStructure{cal: tt.cal}}
			p := k.Plan(context.Background(), camp, tt.a, rec(KindCalendar, tt.action, "Harptos", tt.fields, ""))
			if !strings.Contains(p.Error, tt.want) {
				t.Fatalf("error %q, want %q", p.Error, tt.want)
			}
		})
	}
}

func TestCalendarKind_Update(t *testing.T) {
	f := &fakeStructure{
		cal: &calendar.Calendar{
			ID: "cal", Name: "Harptos", CurrentHour: 9,
			Months:   []calendar.Month{{Name: "Hammer", Days: 30}, {Name: "Alturiak", Days: 30}},
			Weekdays: []calendar.Weekday{{Name: "First"}},
			Moons:    []calendar.Moon{{ID: 7, Name: "Selune", CycleDays: 30}},
			Seasons:  []calendar.Season{{ID: 4, Name: "Winter", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30, Color: "#0000ff"}},
			Eras:     []calendar.Era{{ID: 2, Name: "Dalereckoning", StartYear: 1}},
		},
		preview: calendar.StructurePreview{MovedEvents: 2},
	}
	k := CalendarKind{Svc: f}
	fields := map[string]any{
		"rename_to":     "Harptos Reckoning",
		"current_year":  1493,
		"current_month": "Ches",
		"current_day":   2,
		"months": []any{
			map[string]any{"name": "Hammer", "days": 30},
			map[string]any{"name": "Alturiak", "days": 30},
			map[string]any{"name": "Ches", "days": 30},
		},
		"moons": []any{map[string]any{"name": "selune", "cycle": 30.4}, map[string]any{"name": "Tears", "cycle": 12}},
		"eras": []any{
			map[string]any{"name": "Dalereckoning", "color": "#ff0000"},
			map[string]any{"name": "Age of Humanity", "start_year": -2000, "end_year": 0},
		},
	}
	r := rec(KindCalendar, ActionUpdate, "", fields, "")
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" {
		t.Fatalf("plan: %+v", p)
	}
	// Ches only exists after the month change; today must be read against it.
	if !strings.Contains(p.Summary, "sets today to Ches 2, 1493") || !strings.Contains(strings.Join(p.Warnings, "|"), "2 events follow") {
		t.Errorf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if f.applied != 1 {
		t.Fatal("structure not saved")
	}
	e := f.edits[len(f.edits)-1]
	if len(e.Months) != 3 || len(e.Weekdays) != 1 {
		t.Errorf("edit months %d weekdays %d: a left-out list must be kept", len(e.Months), len(e.Weekdays))
	}
	if e.Moons[0].ID == nil || *e.Moons[0].ID != 7 || e.Moons[1].ID != nil {
		t.Errorf("moons %+v: a moon matched by name keeps its id", e.Moons)
	}
	if len(e.Seasons) != 1 || e.Seasons[0].ID != 4 {
		t.Errorf("seasons %+v: left-out seasons must be kept", e.Seasons)
	}
	if len(f.updates) != 1 || f.updates[0].Name != "Harptos Reckoning" {
		t.Errorf("updates %+v", f.updates)
	}
	if f.today == nil || f.today[1] != 3 {
		t.Errorf("today %v", f.today)
	}
	if len(f.eras) != 1 || f.eras[0].Name != "Age of Humanity" || len(f.eraUps) != 1 {
		t.Fatalf("eras created %+v updated %+v", f.eras, f.eraUps)
	}
	if f.eraUps[0].StartYear.Present() || f.eraUps[0].Color.Val("") != "#ff0000" {
		t.Errorf("era update %+v: only written keys change", f.eraUps[0])
	}
}

func TestCalendarKind_RealTimeRefused(t *testing.T) {
	f := &fakeStructure{cal: &calendar.Calendar{ID: "cal", Name: "Earth", Mode: calendar.ModeRealLife, TracksRealTime: true,
		Months: []calendar.Month{{Name: "January", Days: 31}}}}
	k := CalendarKind{Svc: f}
	p := k.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "", map[string]any{"weekdays": []any{"a"}}, ""))
	if !strings.Contains(p.Error, "real-world") {
		t.Fatalf("plan %+v", p)
	}
}

func TestEventKind_NeedsCalendarFirst(t *testing.T) {
	tests := []struct {
		name   string
		others []calendar.Calendar
		want   string
	}{
		{"none at all", nil, "has no calendar yet"},
		{"none is the main one", []calendar.Calendar{{ID: "x"}}, "set as the main one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := fakeStructureCal{&fakeStructure{others: tt.others}, newCal()}
			k := EventKind{Svc: svc}
			p := k.Plan(context.Background(), camp, owner, rec("event", ActionCreate, "Feast", map[string]any{"year": 1, "month": 1, "day": 1}, ""))
			if !strings.HasPrefix(p.Error, needsCalendarPrefix) || !strings.Contains(p.Error, tt.want) {
				t.Fatalf("error %q", p.Error)
			}
		})
	}
}

// A calendar block lets the events after it be checked against the
// calendar it will make; events before it still need one.
func TestPlanAll_EventsAfterNewCalendar(t *testing.T) {
	svc := fakeStructureCal{&fakeStructure{}, newCal()}
	reg := NewRegistry(CalendarKind{Svc: svc.fakeStructure}, EventKind{Svc: svc})
	feast := rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": "Ches", "day": 3}, "")
	cal := rec(KindCalendar, ActionCreate, "Harptos", newCalendarFields(), "")
	_, plans := reg.PlanAll(context.Background(), camp, owner, []Record{feast, cal, feast})
	if !strings.HasPrefix(plans[0].Error, needsCalendarPrefix) {
		t.Errorf("event before the calendar: %+v", plans[0])
	}
	if plans[1].Error != "" {
		t.Fatalf("calendar: %+v", plans[1])
	}
	if plans[2].Error != "" || !strings.Contains(plans[2].Summary, "Ches 3, 1492 on Harptos") {
		t.Errorf("event after the calendar: %+v", plans[2])
	}
}

// Review findings on the update path: what a block leaves out is kept.
func TestCalendarKind_UpdateKeeps(t *testing.T) {
	base := func() *calendar.Calendar {
		return &calendar.Calendar{
			ID: "cal", Name: "Harptos",
			Months:   []calendar.Month{{Name: "Hammer", Days: 30}, {Name: "Alturiak", Days: 30}},
			Weekdays: []calendar.Weekday{{Name: "First"}},
			Moons: []calendar.Moon{
				{ID: 7, Name: "Selune", CycleDays: 30, PhaseOffset: 3, Color: "#eeeeee"},
				{ID: 8, Name: "Shadow", CycleDays: 9, HiddenFromPlayers: true},
			},
			Seasons: []calendar.Season{{ID: 4, Name: "Winter", StartMonth: 2, StartDay: 1, EndMonth: 2, EndDay: 30}},
			Eras:    []calendar.Era{{ID: 2, Name: "Dalereckoning", StartYear: 1}},
		}
	}
	t.Run("moon keys left out and hidden moons", func(t *testing.T) {
		f := &fakeStructure{cal: base()}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": []any{map[string]any{"name": "Selune"}}}, "")
		if err := (CalendarKind{Svc: f}).Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		ms := f.edits[0].Moons
		if len(ms) != 2 || ms[0].CycleDays != 30 || ms[0].PhaseOffset != 3 || ms[0].Color != "#eeeeee" {
			t.Fatalf("moons %+v", ms)
		}
		if ms[1].ID == nil || *ms[1].ID != 8 {
			t.Fatalf("hidden moon dropped: %+v", ms)
		}
	})
	t.Run("seasons follow their months", func(t *testing.T) {
		f := &fakeStructure{cal: base()}
		months := []any{map[string]any{"name": "New", "days": 10}, map[string]any{"name": "Hammer", "days": 30}, map[string]any{"name": "Alturiak", "days": 30}}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"months": months}, "")
		if err := (CalendarKind{Svc: f}).Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if s := f.edits[0].Seasons[0]; s.StartMonth != 3 || s.EndMonth != 3 {
			t.Fatalf("season %+v", s)
		}
		gone := rec(KindCalendar, ActionUpdate, "", map[string]any{"months": months[:2]}, "")
		if p := (CalendarKind{Svc: f}).Plan(context.Background(), camp, owner, gone); !strings.Contains(p.Error, "give seasons too") {
			t.Fatalf("plan %+v", p)
		}
	})
	t.Run("era end with only a year", func(t *testing.T) {
		f := &fakeStructure{cal: base()}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"eras": []any{
			map[string]any{"name": "Dalereckoning", "end_year": 1500},
			map[string]any{"name": "Short", "start_year": 1500, "end_year": 1500},
		}}, "")
		if err := (CalendarKind{Svc: f}).Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if !f.eraUps[0].EndMonth.IsNull() || f.eras[0].EndMonth != nil {
			t.Fatalf("an end year alone must run to the year's end: %+v %+v", f.eraUps[0], f.eras[0])
		}
	})
	t.Run("another calendar's name", func(t *testing.T) {
		f := &fakeStructure{cal: base()}
		p := (CalendarKind{Svc: f}).Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "Greyhawk", map[string]any{"year_label": "CY"}, ""))
		if !strings.Contains(p.Error, "rename_to") {
			t.Fatalf("plan %+v", p)
		}
	})
}
