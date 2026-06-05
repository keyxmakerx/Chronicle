// calendar_harden_test.go — C-CAL-WORLDSTATE-PREPORT-HARDENING (Slice 1).
//
// Static guards on the engine-hygiene hardening: the rAF resilience structure,
// the dead-code purge (the drifted demoApplySky + snowglobe/anchored-popover
// JS+CSS are gone), the §5 perf knobs, the §6 moon fix, and the §7 browser-
// compat guard. Behavioural coverage (a throwing spec doesn't kill the loop,
// setProfile cap-trim, parseTime) is in test/js/engine.test.mjs.

package demo

import (
	"strings"
	"testing"
)

func TestCalAlmanac_RAFResilience(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"function renderSurface(s, dt)",       // extracted, guarded surface render
		"function engineLogOnce(",             // rate-limited logging
		"em.disabled",                         // disable a persistently-throwing emitter
		"if (em.errCount >= 3)",               // after repeated errors
		"try { renderSurface(s, dt); } catch", // surface render guarded
		"finally {",                           // reschedule ALWAYS runs
	} {
		if !strings.Contains(js, m) {
			t.Errorf("rAF-resilience marker missing: %q", m)
		}
	}
	// The reschedule must live in the finally so nothing can skip it.
	fin := js[strings.LastIndex(js, "finally {"):]
	if !strings.Contains(fin[:200], "requestAnimationFrame(step)") {
		t.Errorf("the rAF reschedule must be inside the step() finally block")
	}
}

func TestCalAlmanac_DeadCodeGone(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, gone := range []string{
		"demoApplySky", "composeSand", "__currentCelestialIDs",
		"cal-almanac-globe__moon", "data-cal-globe-sun",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("dead JS still present: %q", gone)
		}
	}
	// demo-controls now drive the REAL worldState path.
	for _, m := range []string{"function demoSetWeather(", "function demoSetCelestial("} {
		if !strings.Contains(js, m) {
			t.Errorf("demo controls must call the real path: %q missing", m)
		}
	}
	css := readCalAlmanacCSS(t)
	for _, gone := range []string{".cal-almanac-globe", ".cal-almanac-drawer", ".cal-almanac-pop", "cal-almanac-eras__band", "cell__weather--"} {
		if strings.Contains(css, gone) {
			t.Errorf("dead CSS still present: %q", gone)
		}
	}
}

func TestCalAlmanac_PerfAndMoonFix(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"function pcap(base)", // §5 profile-scaled weather caps
		"pcap(W * 0.25)",      // applied to a renderer
		"_skyGradKey",         // §5 cached hourglass sky gradient
		"var designId = MOON_DESIGNS[moon.baseDesign] ? moon.baseDesign", // §6 resolved id (no 404)
	} {
		if !strings.Contains(js, m) {
			t.Errorf("perf/moon-fix marker missing: %q", m)
		}
	}
}

func TestCalAlmanac_BrowserCompatGuard(t *testing.T) {
	css := readCalAlmanacCSS(t)
	if !strings.Contains(css, "@supports not ((color: oklch(0 0 0))") {
		t.Errorf("§7 @supports OKLCH/color-mix guard missing")
	}
	if !strings.Contains(css, ".cal-almanac-unsupported") {
		t.Errorf("§7 unsupported-browser notice CSS missing")
	}
	if !strings.Contains(css, "Chrome/Edge 111+") {
		t.Errorf("§7 documented support matrix missing from the CSS header")
	}
	html := renderAlmanac(t)
	if !strings.Contains(html, "cal-almanac-unsupported") {
		t.Errorf("§7 unsupported-browser notice element missing from the markup")
	}
}
