// calendar_timecontrol_test.go — C-CAL-WORLDSTATE-W3-TIME-CONTROL (CATALOG Part 6).
//
// Static guards on the time-control verb layer (D&D narrative-chunk model, NOT
// VCR): the shared-rAF tween mechanism + atmosphere-pause on the engine, the
// verbs + fill-cap boundary + step-back reverse-sand, the worldState fields,
// and the demo controls. Runtime verb behaviour is in
// test/js/timecontrol.test.mjs; visual fidelity is the operator's local gate.

package demo

import (
	"strings"
	"testing"
)

func TestCalAlmanac_TimeControlEngine(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"function addTick(fn)",  // shared-rAF tween hook
		"ENGINE_TICKS",          // tween list driven inside step()
		"function setPaused(b)", // atmosphere-pause (freeze the loop)
		"addTick: addTick",      // exposed on the engine API
		"setPaused: setPaused",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("engine time-control marker missing: %q", m)
		}
	}
}

func TestCalAlmanac_TimeControlVerbs(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"var TC_FILL_CAP = 0.33",       // capped fill (~1/3)
		"function tcAdvanceHours(",     // +N time
		"function tcAdvanceDateBy(",    // +1day moves the calendar cursor
		"function tcSetTime(",          // jump + crossfade
		"function tcStepBack(",         // single-undo
		"function tcTogglePause(",      // atmosphere-pause
		"function tcPeriodBoundary(",   // fill cap → flip + reset
		"function forceHourglassFlip(", // reuse the shipped dawn/dusk flip
		"HG_INTERIOR.setFill",          // verb-controlled hourglass fill
		"HG_INTERIOR.reverseSand",      // 400ms step-back flourish
		"window.__calTimeControl",      // reusable verb API (future GM panel)
		"timepieceFill: 0",             // worldState field
		"atmospherePaused: false",      // worldState field
	} {
		if !strings.Contains(js, m) {
			t.Errorf("time-control verb marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_TimeControlNoNewRAF — the ~600ms/400ms tweens run on the shared
// engine rAF via addTick (no parallel loop); reduced-motion → instant snaps.
func TestCalAlmanac_TimeControlNoNewRAF(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "CalParticleEngine.addTick(function (dt)") {
		t.Errorf("time tweens must run on the shared engine rAF via addTick")
	}
	// The tween helper must NOT spin its own rAF loop.
	if strings.Contains(js, "function tcTween") {
		seg := js[strings.Index(js, "function tcTween"):]
		end := strings.Index(seg, "function tcSnapshot")
		if end > 0 {
			seg = seg[:end]
		}
		if strings.Contains(seg, "requestAnimationFrame") {
			t.Errorf("tcTween must use the shared engine tick, not its own requestAnimationFrame")
		}
		if !strings.Contains(seg, "tcReduced()") || !strings.Contains(seg, "onUpdate(1)") {
			t.Errorf("tcTween must snap instantly to the end-state under reduced-motion")
		}
	}
}

// TestCalAlmanac_TimeControlControls — the showcase verb buttons + pause freeze.
func TestCalAlmanac_TimeControlControls(t *testing.T) {
	html := renderAlmanac(t)
	for _, m := range []string{
		`data-cal-democtl-tc="hour"`,
		`data-cal-democtl-tc="day"`,
		`data-cal-democtl-tc="rest"`,
		`data-cal-democtl-tc="stepback"`,
		`data-cal-democtl-tc="pause"`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("time-control button missing: %q", m)
		}
	}
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, `[data-cal-atmosphere-paused="true"]`) || !strings.Contains(css, "animation-play-state: paused") {
		t.Errorf("atmosphere-pause must freeze CSS animations under the shell")
	}
}
