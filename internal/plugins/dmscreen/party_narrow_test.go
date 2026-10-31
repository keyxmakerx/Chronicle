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
