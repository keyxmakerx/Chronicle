// Package calendar — moon_silhouette.go computes the lit-crescent SVG path
// for the v5 moon-silhouette mark (the .sil/.db/.dl classes in
// static/css/calendar_v5.css). This is a direct, small port of the signed
// mockup's litPath(p, r) — the photorealistic canvas-rendered moon engine
// the mockup also carries (MOONR) is out of scope for Part B (a separate,
// larger rendering system), so every place a moon appears here uses this
// simple silhouette instead.
package calendar

import (
	"fmt"
	"math"
)

// moonSilhouetteRadius is the fixed radius the silhouette is drawn at; the
// mark is scaled up or down purely with CSS (width/height on .sil), never by
// changing this, so every silhouette on a page shares one viewBox.
const moonSilhouetteRadius = 6.0

// MoonLitPath returns the SVG path "d" attribute describing the lit portion
// of a moon's disc at the given phase (0 = new, 0.5 = full, wrapping back to
// 1 = new), for a disc of radius r centered at the origin. Returns "" within
// 0.004 of new — no sliver worth drawing — matching the mockup exactly.
func MoonLitPath(phase, r float64) string {
	if phase < 0.004 || phase > 0.996 {
		return ""
	}
	waxing := phase < 0.5
	k := math.Cos(2 * math.Pi * phase)
	rx := math.Abs(k) * r
	limb := 0
	if waxing {
		limb = 1
	}
	// term: 0 when waxing matches the sign of k, else 1 (mirrors the
	// mockup's `(waxing === (k > 0)) ? 0 : 1`).
	term := 1
	if waxing == (k > 0) {
		term = 0
	}
	return fmt.Sprintf("M0,%s A%s,%s 0 0 %d 0,%s A%s,%s 0 0 %d 0,%s Z",
		trimFloat(-r), trimFloat(r), trimFloat(r), limb, trimFloat(r),
		trimFloat(rx), trimFloat(r), term, trimFloat(-r))
}

// trimFloat formats f without a trailing ".00" for a whole number, matching
// how JavaScript's default number-to-string conversion (used by the mockup's
// plain string concatenation) renders the same values — rx is the only
// operand that is ever fractional in practice.
func trimFloat(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.2f", f)
}
