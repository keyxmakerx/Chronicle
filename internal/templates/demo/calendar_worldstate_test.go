// calendar_worldstate_test.go — C-CAL-WORLDSTATE-EFFECTS-SYSTEM Wave 0.
//
// Static guards on the world-state spine in static/js/cal-almanac.js: ONE
// `worldState` object + setWorldState(patch) pub/sub front door, the unified
// EFFECTS registry (per-surface renderers, additive over the legacy
// WEATHER_EFFECTS/CELESTIAL_EFFECTS maps), both surfaces subscribing, the
// back→front layer-resolution order, and the no-regression invariant (the
// v5 applyTime/renderSkyForDay monkey-patch wraps are gone, replaced by
// shims into setWorldState). Runtime behaviour (changedKeys diffing, no-op
// guard, layer activation) is covered by test/js/worldstate.test.mjs.

package demo

import (
	"strings"
	"testing"
)

// TestCalAlmanac_WorldStateCore — the shared model + pub/sub front door.
func TestCalAlmanac_WorldStateCore(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"var worldState",
		"function setWorldState(patch)",
		"function subscribeWorldState(",
		"function wsEqual(", "function wsMerge(",
		// changedKeys-gated notify (no-op patch notifies nobody).
		"if (!changed.length) return changed;",
		// CATALOG Part 8 shape, seeded in its own block.
		"registerInitBlock('world-state'",
		"timeOfDay:", "moodTint", "timeControl", "moons:", "events:",
		// exposed for the demo harness + tests.
		"window.__calSetWorldState", "window.__calWorldState", "window.__calSubscribeWorldState",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("world-state core marker missing in JS: %q", m)
		}
	}
}

// TestCalAlmanac_UnifiedEffectsRegistry — ONE EFFECTS registry with the
// per-surface renderer shape, projected ADDITIVELY over the legacy maps.
func TestCalAlmanac_UnifiedEffectsRegistry(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"var EFFECTS",
		"registerInitBlock('unified-effects'",
		"function projectIntoEffects(",
		"projectIntoEffects(WEATHER_EFFECTS, 'weather')",
		"projectIntoEffects(CELESTIAL_EFFECTS, 'celestial')",
		"window.__calEffects",
		// per-surface renderer fields (CATALOG Part 0).
		"skyBand", "hgTop", "hgBottom", "hgSand", "timeline",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("unified-effects marker missing in JS: %q", m)
		}
	}
	// Additive: the legacy maps must still exist (thin projections over EFFECTS).
	for _, m := range []string{"WEATHER_EFFECTS", "CELESTIAL_EFFECTS"} {
		if !strings.Contains(js, m) {
			t.Errorf("legacy registry %q must remain (additive Wave 0)", m)
		}
	}
}

// TestCalAlmanac_BothSurfacesSubscribe — sky-band + hourglass both subscribe
// to worldState, in back→front layer order (sky core → sun → hourglass).
func TestCalAlmanac_BothSurfacesSubscribe(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"registerInitBlock('world-state-subscribers'",
		"subscribeWorldState(function (st, changed)",
		// the three surface concerns the subscribers drive.
		"renderTimePipeline(st.timeOfDay)",
		"renderDayPipeline(st.date.month, st.date.day)",
		"applySunState(currentSunState())",
		"applyHourglassLevels(st.timeOfDay)",
		"applySandTheme(st.date.month, st.date.day)",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("subscriber wiring marker missing in JS: %q", m)
		}
	}
	// exactly three subscribeWorldState() registrations (sky / sun / hourglass).
	if c := strings.Count(js, "subscribeWorldState(function"); c != 3 {
		t.Errorf("expected exactly 3 surface subscribers; got %d", c)
	}
}

// TestCalAlmanac_LayerResolutionOrder — the explicit back→front order.
func TestCalAlmanac_LayerResolutionOrder(t *testing.T) {
	js := readCalAlmanacJS(t)
	const order = "['timeOfDay', 'season', 'celestial', 'weather', 'events', 'moodTint', 'timeControl']"
	for _, m := range []string{
		"var WS_LAYER_ORDER = " + order,
		"function resolveLayers(",
		"window.__calLayerOrder", "window.__calResolveLayers",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("layer-resolution marker missing in JS: %q", m)
		}
	}
}

// TestCalAlmanac_NoMonkeyPatchChain — the no-regression invariant: the v5
// applyTime/renderSkyForDay reassignment wraps are gone; the public entry
// points are now thin shims into setWorldState (every caller preserved).
func TestCalAlmanac_NoMonkeyPatchChain(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, forbidden := range []string{
		"applyTime = function",
		"renderSkyForDay = function",
		"__applyTimeOrig",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("v5 monkey-patch wrap must be gone, found: %q", forbidden)
		}
	}
	// Base pipelines + shims present.
	for _, m := range []string{
		"function renderTimePipeline(t)",
		"function renderDayPipeline(m, day)",
		"function applyTime(t)",
		"function renderSkyForDay(m, day)",
		"setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, t)) })",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("pipeline/shim marker missing in JS: %q", m)
		}
	}
}

// TestCalAlmanac_ReducedMotionPreserved — Wave 0 must not weaken the engine's
// reduced-motion / pooled-cap safety (binding convention).
func TestCalAlmanac_ReducedMotionPreserved(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"prefers-reduced-motion",
		"function drawStaticFrame(",
		"globalCap",
		"requestAnimationFrame",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("engine safety marker missing/weakened in JS: %q", m)
		}
	}
}
