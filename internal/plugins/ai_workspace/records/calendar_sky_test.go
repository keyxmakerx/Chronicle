package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestReadSun(t *testing.T) {
	red := "#ff6a3a"
	tests := []struct {
		name    string
		in      any
		base    calendar.SunLook
		want    calendar.SunLook
		wantErr string
	}{
		{name: "a kind", in: "Red", want: calendar.SunLook{Kind: calendar.SunKindRed}},
		{name: "a settings name", in: "blue-white", want: calendar.SunLook{Kind: calendar.SunKindBlue}},
		{name: "a map", in: map[string]any{"Kind": "orange", "color": red}, want: calendar.SunLook{Kind: calendar.SunKindOrange, Color: &red}},
		{name: "a colour keeps the kind", in: map[string]any{"color": red}, base: calendar.SunLook{Kind: calendar.SunKindWhite}, want: calendar.SunLook{Kind: calendar.SunKindWhite, Color: &red}},
		{name: "none clears the colour", in: map[string]any{"color": "none"}, base: calendar.SunLook{Kind: calendar.SunKindRed, Color: &red}, want: calendar.SunLook{Kind: calendar.SunKindRed}},
		{name: "a kind keeps the colour", in: "white", base: calendar.SunLook{Kind: calendar.SunKindRed, Color: &red}, want: calendar.SunLook{Kind: calendar.SunKindWhite, Color: &red}},
		{name: "an unknown kind", in: "green", wantErr: "not a kind of sun"},
		{name: "an empty map", in: map[string]any{}, wantErr: "needs a kind"},
		{name: "a list", in: []any{"red"}, wantErr: "map of kind and color"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := readSun(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := s.look(tt.base)
			if got.Kind != tt.want.Kind || (got.Color == nil) != (tt.want.Color == nil) || (got.Color != nil && *got.Color != *tt.want.Color) {
				t.Errorf("look = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCalendarKind_SunAndMoonLooks(t *testing.T) {
	tint := "0.4"
	f := &fakeStructure{cal: &calendar.Calendar{
		ID: "cal", Name: "Harptos",
		Months:   []calendar.Month{{Name: "Hammer", Days: 30}},
		Weekdays: []calendar.Weekday{{Name: "First"}},
		Moons: []calendar.Moon{
			{ID: 7, Name: "Selune", CycleDays: 30, Color: "#eeeeee", Tint: &tint},
			{ID: 8, Name: "Umbra", CycleDays: 9, Color: "#222222", HiddenFromPlayers: true},
		},
		Sun: calendar.SunLook{Kind: calendar.SunKindYellow},
	}}
	k := CalendarKind{Svc: f}
	r := rec(KindCalendar, ActionUpdate, "", map[string]any{
		"sun": "red",
		"moons": []any{
			map[string]any{"name": "Selune", "strength": 0.9},
			map[string]any{"name": "Umbra", "hidden": false, "strength": "default"},
			map[string]any{"name": "Tears", "cycle": 12, "strength": 1, "hidden": true},
		},
	}, "")
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" {
		t.Fatalf("plan: %+v", p)
	}
	for _, want := range []string{"colour strength of 2 moons", "shows 1 hidden moon to players", "gives it a red ember sun"} {
		if !strings.Contains(p.Summary, want) {
			t.Errorf("summary %q lacks %q", p.Summary, want)
		}
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if f.sun == nil || f.sun.Kind != calendar.SunKindRed {
		t.Errorf("sun = %+v, want red", f.sun)
	}
	if v := f.strengths[7]; v == nil || *v != 0.9 {
		t.Errorf("Selune strength = %v, want 0.9", v)
	}
	if v, ok := f.strengths[8]; !ok || v != nil {
		t.Errorf("Umbra strength = %v (set %v), want the default", v, ok)
	}
	if hidden, ok := f.hides[8]; !ok || hidden {
		t.Errorf("Umbra should be shown to players, hides = %v", f.hides)
	}
	// The new moon takes its look on insert, not through the setters.
	var tears *calendar.MoonInput
	for i := range f.edits[0].Moons {
		if f.edits[0].Moons[i].Name == "Tears" {
			tears = &f.edits[0].Moons[i]
		}
	}
	if tears == nil || tears.Tint == nil || *tears.Tint != "1" || !tears.HiddenFromPlayers || tears.ID != nil {
		t.Errorf("new moon Tears = %+v, want strength 1 and hidden on insert", tears)
	}
}

// A structure save keeps a kept moon's offset and colour as stored, so the
// block writes them after it; moons whose offset and colour stay are left.
func TestCalendarKind_MoonPhaseOffsets(t *testing.T) {
	cal := func() *calendar.Calendar {
		return &calendar.Calendar{
			ID: "cal", Name: "Harptos",
			Months:   []calendar.Month{{Name: "Hammer", Days: 30}},
			Weekdays: []calendar.Weekday{{Name: "First"}},
			Moons: []calendar.Moon{
				{ID: 7, Name: "Lacrimosa", CycleDays: 24, PhaseOffset: 1, Color: "#c0d0ff"},
				{ID: 8, Name: "Sanguin'mor", CycleDays: 24, PhaseOffset: 13, Color: "#aa2222"},
				{ID: 9, Name: "Quiet", CycleDays: 30, PhaseOffset: 4, Color: "#eeeeee"},
			},
		}
	}
	tests := []struct {
		name    string
		moons   []any
		summary string
		want    map[int]moonPhase
	}{
		{
			name: "offsets to zero and twelve, with a strength",
			moons: []any{
				map[string]any{"name": "Lacrimosa", "phase_offset": 0},
				map[string]any{"name": "Sanguin'mor", "phase_offset": 12, "strength": 0.8},
				map[string]any{"name": "Quiet"},
			},
			summary: "changes the phase offset or colour of 2 moons",
			want:    map[int]moonPhase{7: {0, "#c0d0ff"}, 8: {12, "#aa2222"}},
		},
		{
			name:    "a colour keeps the offset",
			moons:   []any{map[string]any{"name": "Quiet", "color": "#123456"}},
			summary: "changes the phase offset or colour of 1 moon",
			want:    map[int]moonPhase{9: {4, "#123456"}},
		},
		{
			name:  "the same offset and colour write nothing",
			moons: []any{map[string]any{"name": "Quiet", "phase_offset": 4, "color": "#EEEEEE"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStructure{cal: cal()}
			k := CalendarKind{Svc: f}
			r := rec(KindCalendar, ActionUpdate, "", map[string]any{"moons": tt.moons}, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if p.Error != "" {
				t.Fatalf("plan: %+v", p)
			}
			if tt.summary != "" && !strings.Contains(p.Summary, tt.summary) {
				t.Errorf("summary %q lacks %q", p.Summary, tt.summary)
			}
			if tt.summary == "" && strings.Contains(p.Summary, "phase offset") {
				t.Errorf("summary %q claims a phase change", p.Summary)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if len(f.phases) != len(tt.want) {
				t.Fatalf("phases = %+v, want %+v", f.phases, tt.want)
			}
			for id, w := range tt.want {
				if f.phases[id] != w {
					t.Errorf("moon %d = %+v, want %+v", id, f.phases[id], w)
				}
			}
		})
	}
}

func TestCalendarKind_CreateSun(t *testing.T) {
	f := &fakeStructure{}
	k := CalendarKind{Svc: f}
	fields := newCalendarFields()
	fields["sun"] = map[string]any{"kind": "white", "color": "#f0f4ff"}
	fields["moons"] = []any{map[string]any{"name": "Selune", "cycle": 30, "strength": 0.75}}
	r := rec(KindCalendar, ActionCreate, "Harptos", fields, "")
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || !strings.Contains(p.Summary, "a white sun, drawn #f0f4ff") {
		t.Fatalf("plan: %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if l := f.created.Settings.Sun; l == nil || l.Kind != calendar.SunKindWhite || l.Color == nil || *l.Color != "#f0f4ff" {
		t.Errorf("created sun = %+v", l)
	}
	if m := f.created.Moons[0]; m.Tint == nil || *m.Tint != "0.75" {
		t.Errorf("created moon = %+v, want strength 0.75", m)
	}
}

func TestCalendarKind_RefusesBadLooks(t *testing.T) {
	f := &fakeStructure{cal: &calendar.Calendar{ID: "cal", Name: "Harptos", Months: []calendar.Month{{Name: "Hammer", Days: 30}},
		Moons: []calendar.Moon{{ID: 7, Name: "Selune", CycleDays: 30}}}}
	k := CalendarKind{Svc: f}
	for _, fields := range []map[string]any{
		{"sun": "green"},
		{"moons": []any{map[string]any{"name": "Selune", "strength": 80}}},
		{"moons": []any{map[string]any{"name": "Selune", "strength": "bright"}}},
	} {
		if p := k.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "", fields, "")); p.Error == "" {
			t.Errorf("%v: plan passed, want a refusal", fields)
		}
	}
}

func TestCalendarKind_ExportSunAndMoonLooks(t *testing.T) {
	tint, red := "0.8", "#ff6a3a"
	f := &fakeStructure{cal: &calendar.Calendar{ID: "cal", Name: "Harptos",
		Months:   []calendar.Month{{Name: "Hammer", Days: 30}},
		Weekdays: []calendar.Weekday{{Name: "First"}},
		Moons:    []calendar.Moon{{ID: 7, Name: "Selune", CycleDays: 30, Color: "#eeeeee", Tint: &tint, HiddenFromPlayers: true}},
		Sun:      calendar.SunLook{Kind: calendar.SunKindRed, Color: &red},
	}}
	out, err := CalendarKind{Svc: f}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`sun: {kind: red, color: "#ff6a3a"}`, "strength: 0.8, hidden: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
}
