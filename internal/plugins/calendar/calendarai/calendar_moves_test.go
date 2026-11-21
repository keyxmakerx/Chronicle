package calendarai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// movesCalendar is a main calendar with one moon on its cycle (Selune) and one
// already set by hand (Umbra, with a word of its own).
func movesCalendar() *calendar.Calendar {
	return &calendar.Calendar{
		ID: "cal", Name: "Harptos",
		Months:   []calendar.Month{{Name: "Hammer", Days: 30}},
		Weekdays: []calendar.Weekday{{Name: "First"}},
		Moons: []calendar.Moon{
			{ID: 7, Name: "Selune", CycleDays: 30.4, PhaseMode: calendar.MoonCycle},
			{ID: 8, Name: "Umbra", CycleDays: 21, PhaseMode: calendar.MoonManual, StateNames: map[string]string{"vanished": "Gone below"}},
		},
	}
}

// A new moon that moves by hand may leave out its cycle, which is stored as an
// ordinary 30 days; one that moves any other way still needs it.
func TestCalendarKind_NewMoonByHandNeedsNoCycle(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		base      *calendar.Calendar
		moons     []any
		wantErr   string
		wantCycle float64
	}{
		{name: "create by hand, no cycle", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "by_hand"}}, wantCycle: 30},
		{name: "create by hand, spelled differently", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "By Hand"}}, wantCycle: 30},
		{name: "create by hand keeps a cycle it was given", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "by_hand", "cycle": 12}}, wantCycle: 12},
		{name: "create with no moves and no cycle", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra"}}, wantErr: "needs a cycle in days"},
		{name: "create by cycle and no cycle", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "cycle"}}, wantErr: "needs a cycle in days"},
		{name: "a cycle of zero is still refused by hand", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "by_hand", "cycle": 0}}, wantErr: "needs a cycle in days"},
		{name: "bad moves", action: ActionCreate,
			moons: []any{map[string]any{"name": "Umbra", "moves": "sideways"}}, wantErr: "`by_hand` or `cycle`"},
		{name: "update adds a moon by hand", action: ActionUpdate, base: movesCalendar(),
			moons: []any{map[string]any{"name": "Selune"}, map[string]any{"name": "Tears", "moves": "by_hand"}}, wantCycle: 30},
		{name: "update adds a moon with no cycle and no moves", action: ActionUpdate, base: movesCalendar(),
			moons: []any{map[string]any{"name": "Tears"}}, wantErr: "needs a cycle in days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStructure{cal: tt.base}
			k := CalendarKind{Svc: f, Marks: &fakeMarks{}}
			fields := newCalendarFields()
			fields["moons"] = tt.moons
			if tt.action == ActionUpdate {
				fields = map[string]any{"moons": tt.moons}
			}
			r := rec(KindCalendar, tt.action, "Harptos", fields, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %s", p.Error)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			var got calendar.MoonInput
			if tt.action == ActionCreate {
				got = f.created.Moons[0]
			} else {
				ms := f.edits[len(f.edits)-1].Moons
				got = ms[len(ms)-1]
			}
			if got.CycleDays != tt.wantCycle {
				t.Errorf("stored cycle %v, want %v", got.CycleDays, tt.wantCycle)
			}
		})
	}
}

// `moves` in the calendar block switches the moon after the structure is
// saved, using the moon's id as it is then, and keeps its own words.
func TestCalendarKind_MovesSwitchesMoons(t *testing.T) {
	t.Run("a new moon on create", func(t *testing.T) {
		f := &fakeStructure{}
		marks := &fakeMarks{}
		k := CalendarKind{Svc: f, Marks: marks}
		fields := newCalendarFields()
		fields["moons"] = []any{
			map[string]any{"name": "Selune", "cycle": 30.4375},
			map[string]any{"name": "Umbra", "moves": "by_hand"},
		}
		r := rec(KindCalendar, ActionCreate, "Harptos", fields, "")
		p := k.Plan(context.Background(), camp, owner, r)
		if p.Error != "" || !strings.Contains(p.Summary, "2 moons (1 set by hand)") {
			t.Fatalf("plan %+v", p)
		}
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(marks.modes) != 1 || marks.modes[0] != calendar.MoonManual {
			t.Errorf("modes %v: only the moon that moves by hand is switched", marks.modes)
		}
	})

	t.Run("update switches existing and new moons", func(t *testing.T) {
		f := &fakeStructure{cal: movesCalendar()}
		after := movesCalendar()
		after.Moons = append(after.Moons, calendar.Moon{ID: 9, Name: "Tears", CycleDays: 30, PhaseMode: calendar.MoonCycle})
		f.afterApply = after
		marks := &fakeMarks{}
		k := CalendarKind{Svc: f, Marks: marks}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": []any{
			map[string]any{"name": "Selune", "moves": "by_hand"},
			map[string]any{"name": "Umbra", "moves": "cycle"},
			map[string]any{"name": "Tears", "moves": "by_hand"},
		}}, "")
		p := k.Plan(context.Background(), camp, owner, r)
		if p.Error != "" {
			t.Fatalf("plan %s", p.Error)
		}
		for _, want := range []string{"sets 2 moons by hand", "has 1 moon follow their cycle"} {
			if !strings.Contains(p.Summary, want) {
				t.Errorf("summary %q lacks %q", p.Summary, want)
			}
		}
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if strings.Join(marks.modes, ",") != "manual,cycle,manual" {
			t.Fatalf("modes %v", marks.modes)
		}
		// Umbra goes back to its cycle with the words it had.
		if marks.names[1]["vanished"] != "Gone below" {
			t.Errorf("names %v: a mode change must not drop the moon's own words", marks.names)
		}
	})

	t.Run("a moon already moving that way is left alone", func(t *testing.T) {
		f := &fakeStructure{cal: movesCalendar()}
		marks := &fakeMarks{}
		k := CalendarKind{Svc: f, Marks: marks}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": []any{
			map[string]any{"name": "Selune", "moves": "cycle", "phase_offset": 3},
			map[string]any{"name": "Umbra", "moves": "by_hand"},
		}}, "")
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(marks.modes) != 0 {
			t.Errorf("modes %v", marks.modes)
		}
		if p := k.Plan(context.Background(), camp, owner, r); strings.Contains(p.Summary, "by hand") || strings.Contains(p.Summary, "follow their cycle") {
			t.Errorf("summary %q names a switch nothing makes", p.Summary)
		}
	})

	t.Run("a moon with no moves keeps how it moves", func(t *testing.T) {
		f := &fakeStructure{cal: movesCalendar()}
		marks := &fakeMarks{}
		k := CalendarKind{Svc: f, Marks: marks}
		r := rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": []any{
			map[string]any{"name": "Umbra", "phase_offset": 4},
		}}, "")
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(marks.modes) != 0 {
			t.Errorf("modes %v", marks.modes)
		}
	})
}

// A real-world calendar's moon is the real one, and a server without the moon
// mark service can't switch a moon, so both refuse at review.
func TestCalendarKind_MovesRefusals(t *testing.T) {
	real := &calendar.Calendar{ID: "cal", Name: "Earth", Mode: calendar.ModeRealLife,
		Months: []calendar.Month{{Name: "January", Days: 31}}, Moons: []calendar.Moon{{ID: 1, Name: "Moon", CycleDays: 29.5}}}
	tests := []struct {
		name   string
		k      CalendarKind
		action string
		fields map[string]any
		want   string
	}{
		{"real-world calendar", CalendarKind{Svc: &fakeStructure{cal: real}, Marks: &fakeMarks{}}, ActionUpdate,
			map[string]any{"moons": []any{map[string]any{"name": "Moon", "moves": "by_hand"}}}, "follows the real Moon"},
		{"no moon mark service, update", CalendarKind{Svc: &fakeStructure{cal: movesCalendar()}}, ActionUpdate,
			map[string]any{"moons": []any{map[string]any{"name": "Selune", "moves": "by_hand"}}}, "aren't available on this server"},
		{"no moon mark service, create", CalendarKind{Svc: &fakeStructure{}}, ActionCreate,
			func() map[string]any {
				f := newCalendarFields()
				f["moons"] = []any{map[string]any{"name": "Umbra", "moves": "by_hand"}}
				return f
			}(), "aren't available on this server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "Harptos"
			if tt.action == ActionUpdate {
				name = "" // an update names no calendar but the main one
			}
			p := tt.k.Plan(context.Background(), camp, owner, rec(KindCalendar, tt.action, name, tt.fields, ""))
			if !strings.Contains(p.Error, tt.want) {
				t.Fatalf("error %q, want %q", p.Error, tt.want)
			}
		})
	}
	// Without `moves` the block never needs the service.
	k := CalendarKind{Svc: &fakeStructure{cal: movesCalendar()}}
	if p := k.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": []any{map[string]any{"name": "Selune", "cycle": 30}}}, "")); p.Error != "" {
		t.Errorf("plan %q", p.Error)
	}
}

func TestCalendarKind_DocDescribesMoves(t *testing.T) {
	d := CalendarKind{}.Doc()
	for _, want := range []string{"`moves`", "may leave out `cycle`", "not for a real-world calendar"} {
		if !strings.Contains(d, want) {
			t.Errorf("the calendar doc lacks %q", want)
		}
	}
}

// A create whose moon switch fails leaves nothing behind: the calendar is
// taken back and never made the main one, so players never see a by-hand moon
// on an invented 30-day cycle. If it cannot be taken back, the error says where
// it is.
func TestCalendarKind_CreateIsAllOrNothing(t *testing.T) {
	moons := []any{
		map[string]any{"name": "Selune", "cycle": 30.4375},
		map[string]any{"name": "Umbra", "moves": "by_hand"},
	}
	apply := func(f *fakeStructure, k CalendarKind) error {
		fields := newCalendarFields()
		fields["moons"] = moons
		return k.Apply(context.Background(), camp, owner, rec(KindCalendar, ActionCreate, "Harptos", fields, ""))
	}
	t.Run("a failed switch rolls the calendar back and it never becomes main", func(t *testing.T) {
		f := &fakeStructure{}
		err := apply(f, CalendarKind{Svc: f, Marks: &fakeMarks{modeErr: errors.New("db down")}})
		if err == nil || !strings.Contains(err.Error(), "db down") {
			t.Fatalf("err = %v", err)
		}
		if f.dflt != "" {
			t.Errorf("the calendar was made main (%q) before its moons were set", f.dflt)
		}
		if len(f.deleted) != 1 || f.deleted[0] != "new" {
			t.Errorf("deleted = %v, want the new calendar taken back", f.deleted)
		}
	})
	t.Run("a rollback that fails says where the calendar is", func(t *testing.T) {
		f := &fakeStructure{deleteErr: errors.New("locked")}
		err := apply(f, CalendarKind{Svc: f, Marks: &fakeMarks{modeErr: errors.New("db down")}})
		if err == nil || !strings.Contains(err.Error(), "db down") || !strings.Contains(err.Error(), "not the main calendar") {
			t.Fatalf("err = %v", err)
		}
		if f.dflt != "" {
			t.Errorf("the calendar was made main (%q)", f.dflt)
		}
	})
	t.Run("no Marks service is refused before anything is created", func(t *testing.T) {
		f := &fakeStructure{}
		if err := apply(f, CalendarKind{Svc: f}); err == nil {
			t.Fatal("want a refusal")
		}
		if f.created != nil || f.dflt != "" {
			t.Errorf("created %v, main %q: nothing may be written", f.created, f.dflt)
		}
	})
	t.Run("success switches the moon, then makes it main", func(t *testing.T) {
		f := &fakeStructure{}
		marks := &fakeMarks{}
		if err := apply(f, CalendarKind{Svc: f, Marks: marks}); err != nil {
			t.Fatal(err)
		}
		if f.dflt != "new" || len(marks.modes) != 1 || len(f.deleted) != 0 {
			t.Errorf("main %q, modes %v, deleted %v", f.dflt, marks.modes, f.deleted)
		}
	})
}
