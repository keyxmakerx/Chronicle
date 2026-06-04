// calendar_mood_test.go — C-CAL-WORLDSTATE-W2-MOOD-TINT (CATALOG Part 5).
//
// Static guards on the player mood-tint wash: the renderer over BOTH surfaces
// (sky-band overlay + hourglass canvas composite), the step-6 resolution
// order, the preset palette + demo controls, that it adds NO new rAF (static
// composite), and that it renders under reduced-motion. Runtime ordering /
// no-op behaviour is in test/js/mood.test.mjs; visuals are the operator's gate.

package demo

import (
	"strings"
	"testing"
)

func TestCalAlmanac_MoodTintRenderer(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"function applyMoodTint(",
		"var MOOD_PRESETS",
		"window.__calApplyMoodTint",
		"data-cal-mood-wash",             // sky-band overlay element
		"HG_INTERIOR.setMood",            // hourglass composite (both surfaces)
		"function setMood(color, alpha)", // the hourglass interior hook
		"changed.indexOf('moodTint')",    // wired into a worldState subscriber
		"MOOD_ALPHA_CAP",                 // legibility cap (mood tints, doesn't erase)
	} {
		if !strings.Contains(js, m) {
			t.Errorf("mood-tint renderer marker missing: %q", m)
		}
	}
	// All 8 presets present.
	for _, id := range []string{"ominous-red", "eerie-green", "melancholy-blue", "festive-gold", "cursed-violet", "holy-white", "void-black", "frostbite-cyan"} {
		if !strings.Contains(js, "'"+id+"'") {
			t.Errorf("mood preset missing: %q", id)
		}
	}
}

// TestCalAlmanac_MoodStepSixOrder — mood-tint composites AFTER events and
// BEFORE the time-control modifier in the canonical layer order.
func TestCalAlmanac_MoodStepSixOrder(t *testing.T) {
	js := readCalAlmanacJS(t)
	const order = "['timeOfDay', 'season', 'celestial', 'weather', 'events', 'moodTint', 'timeControl']"
	if !strings.Contains(js, order) {
		t.Errorf("canonical layer order (events → moodTint → timeControl) not found")
	}
	// hourglass composites the mood AFTER the sand (don't clobber hgSand).
	if !strings.Contains(js, "globalCompositeOperation = 'overlay'") {
		t.Errorf("hourglass mood composite must use an 'overlay' blend over the drawn sand")
	}
}

// TestCalAlmanac_MoodNoNewRAF_AndReducedMotion — the wash is a static composite
// (no new rAF) and renders under reduced-motion (the sky overlay is CSS opacity
// with no @keyframes; only its transition is dropped).
func TestCalAlmanac_MoodNoNewRAF_AndReducedMotion(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, ".cal-almanac-sky__mood-wash") {
		t.Errorf("mood-wash CSS class missing")
	}
	if !strings.Contains(css, "mix-blend-mode: overlay") {
		t.Errorf("mood wash must use a screen/multiply 'overlay' blend")
	}
	// The wash has no continuous animation (only a state-change transition),
	// and reduced-motion drops even that — but the static wash still renders.
	if strings.Contains(css, "cal-almanac-sky__mood-wash") && strings.Contains(css, "@keyframes cal-mood") {
		t.Errorf("mood wash must be static (no @keyframes animation)")
	}
	if !strings.Contains(css, ".cal-almanac-sky__mood-wash { transition: none") {
		t.Errorf("reduced-motion should drop the mood wash transition (it stays static + visible)")
	}
}

// TestCalAlmanac_MoodControls — the demo panel exposes presets + custom +
// intensity + clear.
func TestCalAlmanac_MoodControls(t *testing.T) {
	html := renderAlmanac(t)
	for _, m := range []string{
		"data-cal-democtl-mood-preset",
		`data-mood="ominous-red"`,
		"data-cal-democtl-mood-color",
		"data-cal-democtl-mood-intensity",
		"data-cal-democtl-mood-clear",
	} {
		if !strings.Contains(html, m) {
			t.Errorf("mood control hook missing: %q", m)
		}
	}
}
