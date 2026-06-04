// calendar_hourglass_test.go — C-CAL-WORLDSTATE-EFFECTS-SYSTEM Wave 1b.
//
// Static guards on the hourglass interior: the minimal CalParticleEngine
// per-surface frame hook, the heightmap/day-night interior sim wired onto the
// glass surface, the PRESERVED v4 dawn/dusk flip (watch-item), and the engine
// perf caps that must survive. The interior sim's pure math is runtime-tested
// in test/js/hourglass.test.mjs; the visual render is the operator's local gate.

package demo

import (
	"strings"
	"testing"
)

// TestCalAlmanac_EngineFrameHook — the minimal per-surface frame hook (NOT a
// refactor): a surface frame draws under the particles on the one shared rAF,
// keeps the loop alive with zero particles, and runs once for the static frame.
func TestCalAlmanac_EngineFrameHook(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"setFrame: function (fn)",              // public hook on the surface handle
		"frame: opts.frame || null",            // surface carries a frame fn
		"function hasFrames()",                 // loop/sync consider frame-only surfaces
		"hasEmitters() || hasFrames()",         // frame-only surface keeps the rAF alive
		"s.frame(ctx, s.w, s.h, dt, s.frameT)", // drawn under particles in step()
		"s.frame(ctx, s.w, s.h, 0, s.frameT)",  // static frame (dt=0) for reduced-motion
	} {
		if !strings.Contains(js, m) {
			t.Errorf("engine frame-hook marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_HourglassInterior — the interior sim (heightmap + day/night)
// is wired onto the glass surface via the frame hook and exposes its pure math.
func TestCalAlmanac_HourglassInterior(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"var HG_INTERIOR",
		"GLASS_SURFACE.setFrame(function (ctx, w, h, dt) { HG_INTERIOR.frame",
		"function hgAvalanche(",       // slope-limited avalanche
		"function hgSkyForTimeOfDay(", // day→night keyframes
		"function hgSunPos(",          // sun arc/visibility
		"window.__calHgSim",           // pure math exposed for tests
		"worldState.timeOfDay",        // bottom chamber driven by worldState
	} {
		if !strings.Contains(js, m) {
			t.Errorf("hourglass-interior marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_FlipPreserved — WATCH-ITEM: the v4 dawn/dusk flip must NOT be
// retired by the day/night port. The flip is the hourglass subscriber calling
// applyHourglassFlip on a timeOfDay change; the canvas counter-rotates so the
// interior stays upright while the glass shell flips.
func TestCalAlmanac_FlipPreserved(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "function applyHourglassFlip(") {
		t.Errorf("applyHourglassFlip removed — the v4 flip mechanic must survive")
	}
	if !strings.Contains(js, "applyHourglassFlip(st.timeOfDay)") {
		t.Errorf("flip no longer driven by the worldState hourglass subscriber")
	}
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, `[data-cal-hourglass-flipped="true"] .cal-almanac-hourglass__canvas`) {
		t.Errorf("canvas counter-rotate on flip missing (interior would render upside-down at night)")
	}
}

// TestCalAlmanac_EnginePerfCapsSurvive — the frame hook must not weaken the v4
// perf safety: pooled caps (80/40/20), DPR clamp ≤2, IO + visibility pause,
// reduced-motion gate.
func TestCalAlmanac_EnginePerfCapsSurvive(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"high: 80", "normal: 40", "low: 20", // pooled caps
		"Math.min(window.devicePixelRatio || 1, 2)", // DPR clamp ≤2
		"IntersectionObserver",                      // offscreen pause
		"visibilitychange",                          // tab-hidden pause
		"reducedNow",                                // reduced-motion gate
	} {
		if !strings.Contains(js, m) {
			t.Errorf("engine perf-cap marker missing/weakened: %q", m)
		}
	}
}

// TestCalAlmanac_HourglassMaterials — WAVE 1c: dark-wood frame (feTurbulence
// grain + feDiffuseLighting bevel) + procedural glass (feSpecularLighting
// gloss). All filters are static; only the canvas interior animates.
func TestCalAlmanac_HourglassMaterials(t *testing.T) {
	html := renderAlmanac(t)
	for _, m := range []string{
		"feTurbulence",                 // wood grain
		"feDiffuseLighting",            // wood bevel
		"feSpecularLighting",           // glass gloss
		`id="calHgWood"`,               // wood gradient
		`url(#calHgWoodBevel)`,         // bevel applied to the caps
		`url(#calHgWoodGrain)`,         // grain overlay
		"cal-almanac-hourglass__gloss", // gloss path
	} {
		if !strings.Contains(html, m) {
			t.Errorf("hourglass material marker missing: %q", m)
		}
	}
	// Filters must be static (no SMIL <animate> inside the hourglass SVG defs).
	if strings.Contains(html, "<animate") {
		t.Errorf("hourglass SVG must use static filters, found an <animate> element")
	}
}

// TestCalAlmanac_OldSandRectsGone — the v4 SVG sand-fill rects are superseded
// by the canvas heightmap.
func TestCalAlmanac_OldSandRectsGone(t *testing.T) {
	html := renderAlmanac(t)
	for _, gone := range []string{
		`data-cal-hourglass-fill="top"`,
		`data-cal-hourglass-fill="bot"`,
		"cal-almanac-hourglass__sand",
	} {
		if strings.Contains(html, gone) {
			t.Errorf("superseded SVG sand markup still present: %s", gone)
		}
	}
}
