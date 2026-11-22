package dmscreen

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// Meters, subtitle and conditions are built only from the sheet values the
// narrower leaves, so a value a reader may not see never reaches them.
func TestHeroViews_Narrowed(t *testing.T) {
	def := &systems.DMScreenDef{
		Party:          []systems.DMScreenMeter{{Label: "Stamina", Current: "stamina"}, {Label: "Doom", Current: "doom"}},
		HeroSubtitle:   "secret_class",
		HeroConditions: "curse",
	}
	heroes := []Hero{{ID: "h1", Name: "Ser Aldric", Fields: map[string]any{
		"stamina": float64(31), "doom": float64(3), "secret_class": "Lich in disguise", "curse": "Marked",
	}}}
	open := func(h Hero) map[string]any { return map[string]any{"stamina": h.Fields["stamina"]} }

	got := heroViews(def, heroes, nil, open)
	if len(got) != 1 {
		t.Fatalf("%d heroes", len(got))
	}
	h := got[0]
	if h.Subtitle != "" || len(h.Conditions) != 0 {
		t.Errorf("restricted subtitle or conditions shown: %+v", h)
	}
	for _, m := range h.Meters {
		if m.Label == "Doom" && m.Current != "" {
			t.Errorf("restricted meter shown: %+v", m)
		}
	}
	var stamina bool
	for _, m := range h.Meters {
		stamina = stamina || (m.Label == "Stamina" && m.Current == "31")
	}
	if !stamina {
		t.Errorf("open meter lost: %+v", h.Meters)
	}

	whole := heroViews(def, heroes, nil, nil)[0]
	if whole.Subtitle != "Lich in disguise" || len(whole.Conditions) != 1 {
		t.Errorf("unnarrowed view lost values: %+v", whole)
	}
}

// The level is read from the same values as the meters: shown when the sheet
// has one, empty when it has none or the narrower took it out.
func TestHeroViews_Level(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		narrow FieldNarrower
		want   string
	}{
		{"number", map[string]any{"level": float64(3)}, nil, "3"},
		{"text", map[string]any{"level": " 5 "}, nil, "5"},
		{"none", map[string]any{"class": "Fury"}, nil, ""},
		{"narrowed away", map[string]any{"level": float64(3)}, func(Hero) map[string]any { return map[string]any{} }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := heroViews(nil, []Hero{{ID: "h1", Fields: tc.fields}}, nil, tc.narrow)
			if got[0].Level != tc.want {
				t.Errorf("Level = %q, want %q", got[0].Level, tc.want)
			}
		})
	}
}
