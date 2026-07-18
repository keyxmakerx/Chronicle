// calendar_weather_test.go — C-CAL-WORLDSTATE-EFFECTS-SYSTEM Wave 2
// (weather + celestial bundle, CATALOG §12.2).
//
// Static guards on the 10 sky-band weather/celestial renderers, their EFFECTS
// registry entries, and the frame-hook wiring. Runtime behaviour (each frame
// runs without throwing; per-surface entry shape) is covered by
// test/js/weather.test.mjs; visual fidelity is the operator's local gate.

package demo

import (
	"strings"
	"testing"
)

func TestCalAlmanac_WeatherBundle(t *testing.T) {
	js := readCalAlmanacJS(t)
	// All 10 locked effect ids present as renderers.
	for _, id := range []string{
		"weather-clear", "weather-cloudy", "weather-rain", "weather-thunderstorm",
		"weather-snow", "weather-fog", "weather-tornado", "weather-ashfall",
		"celestial-meteor-shower", "celestial-aurora",
	} {
		if !strings.Contains(js, "'"+id+"'") {
			t.Errorf("weather/celestial effect id missing: %q", id)
		}
	}
	// Module + registry + mapping + exposure.
	for _, m := range []string{
		"var WEATHER_RENDERERS",
		"registerInitBlock('weather-fx'",
		"window.__calWeatherRenderers",
		"function weatherV2Id(",
		"var SKY_FX_META",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("weather-bundle marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_WeatherFrameWiring — the rich renderers run through the shared
// engine's per-surface frame hook (not a parallel rAF), and sun-bloom still
// layers on top; the hourglass sand recolors from the effect's hgSand.
// C-SKYBOX-MULTI-INSTANCE replaced the SKY_SURFACE singleton with one surface
// per live SKY_BANDS entry, so the compositor hook is now per-band.
func TestCalAlmanac_WeatherFrameWiring(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"band.surface.setFrame(composeFrames(L.back))", // layered compositor (per-band back canvas)
		"EFFECTS[v2].hgSand.color",                     // hourglass sand syncs to the weather effect
	} {
		if !strings.Contains(js, m) {
			t.Errorf("weather frame-hook wiring marker missing: %q", m)
		}
	}
	// Must not spin up a parallel rAF for weather (reuse the shared engine).
	if strings.Contains(js, "requestAnimationFrame(loop)") {
		t.Errorf("weather renderers must run on the shared engine frame hook, not a parallel rAF loop")
	}
}
