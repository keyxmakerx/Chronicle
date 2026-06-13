package calendar

import "testing"

// calendariaWeatherPresets is the 42-id Calendaria weather vocabulary — the
// sync contract (C-CAL-PARITY-W0W1), verbatim from upstream
// Sayshal/Calendaria scripts/weather/data/weather-presets.mjs and the wiki
// mirror references/calendaria-wiki/Weather-System.md. This list is the
// authority: gmWeatherTypes() MUST contain every id here, in the matching
// Calendaria category. Verified upstream == this list exactly (no presets
// added since) on 2026-06-13.
var calendariaWeatherPresets = map[string]string{
	// Standard (13)
	"clear": "Standard", "partly-cloudy": "Standard", "cloudy": "Standard",
	"overcast": "Standard", "drizzle": "Standard", "rain": "Standard",
	"fog": "Standard", "mist": "Standard", "windy": "Standard",
	"sunshower": "Standard", "snow": "Standard", "sleet": "Standard",
	"heat-wave": "Standard",
	// Severe (7)
	"thunderstorm": "Severe", "blizzard": "Severe", "hail": "Severe",
	"tornado": "Severe", "hurricane": "Severe", "ice-storm": "Severe",
	"monsoon": "Severe",
	// Environmental (8)
	"ashfall": "Environmental", "sandstorm": "Environmental",
	"luminous-sky": "Environmental", "sakura-bloom": "Environmental",
	"autumn-leaves": "Environmental", "rolling-fog": "Environmental",
	"wildfire-smoke": "Environmental", "dust-devil": "Environmental",
	// Fantasy (14)
	"black-sun": "Fantasy", "ley-surge": "Fantasy", "aether-haze": "Fantasy",
	"nullfront": "Fantasy", "permafrost-surge": "Fantasy", "gravewind": "Fantasy",
	"veilfall": "Fantasy", "arcane-winds": "Fantasy", "acid-rain": "Fantasy",
	"blood-rain": "Fantasy", "meteor-shower": "Fantasy", "spore-cloud": "Fantasy",
	"divine-light": "Fantasy", "plague-miasma": "Fantasy",
}

// chronicleWeatherExtras are Chronicle-native weather ids Calendaria lacks.
// Kept forever (stored day-weather rows must keep rendering) and grouped under
// the "Chronicle" category.
var chronicleWeatherExtras = []string{
	"heavy-rain", "snow-flurries", "ember-rain", "falling-leaves",
	"pollen-drift", "fireflies", "miasma",
}

// TestWeatherCatalogContract pins the Go half of the 3-way weather catalog sync
// (C-CAL-PARITY-W0W1). The JS half (SKY_FX ↔ SKY_FX_META) is pinned by
// test/js/weather_catalog_completeness.test.mjs against the SAME id set, so a
// new weather type can't half-land. If you add a preset, update both.
func TestWeatherCatalogContract(t *testing.T) {
	got := gmWeatherTypes()
	byID := make(map[string]gmWeatherType, len(got))
	for _, w := range got {
		if _, dup := byID[w.ID]; dup {
			t.Fatalf("duplicate weather id %q in gmWeatherTypes()", w.ID)
		}
		byID[w.ID] = w
	}

	// 1. Every Calendaria preset present, in its Calendaria category (the
	//    contract). hail→Severe and sandstorm→Environmental are exercised here.
	for id, cat := range calendariaWeatherPresets {
		w, ok := byID[id]
		if !ok {
			t.Errorf("missing Calendaria preset %q (sync contract)", id)
			continue
		}
		if w.Category != cat {
			t.Errorf("preset %q category = %q, want Calendaria %q", id, w.Category, cat)
		}
	}

	// 2. Every Chronicle extra present, in the "Chronicle" category.
	for _, id := range chronicleWeatherExtras {
		w, ok := byID[id]
		if !ok {
			t.Errorf("missing Chronicle extra %q (must never be deleted)", id)
			continue
		}
		if w.Category != "Chronicle" {
			t.Errorf("extra %q category = %q, want Chronicle", id, w.Category)
		}
	}

	// 3. No stray ids beyond contract + extras, and every entry has a
	//    non-empty label + glyph (the console tile needs both).
	want := len(calendariaWeatherPresets) + len(chronicleWeatherExtras)
	if len(got) != want {
		t.Errorf("gmWeatherTypes() has %d entries, want %d (42 Calendaria + %d extras)", len(got), want, len(chronicleWeatherExtras))
	}
	allowedCats := map[string]bool{"Standard": true, "Severe": true, "Environmental": true, "Fantasy": true, "Chronicle": true}
	for _, w := range got {
		if !allowedCats[w.Category] {
			t.Errorf("weather id %q has unknown category %q", w.ID, w.Category)
		}
		if w.Label == "" || w.Glyph == "" {
			t.Errorf("weather id %q missing label/glyph (label=%q glyph=%q)", w.ID, w.Label, w.Glyph)
		}
	}

	// 4. gmWeatherCategories() lists every category the catalog actually uses,
	//    so the console renders no empty section and drops none.
	listed := map[string]bool{}
	for _, c := range gmWeatherCategories() {
		listed[c] = true
	}
	for _, w := range got {
		if !listed[w.Category] {
			t.Errorf("category %q used by %q but missing from gmWeatherCategories()", w.Category, w.ID)
		}
	}
}
