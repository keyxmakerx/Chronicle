package colour

import "math"

// The state colours an accent must stay apart from. Amber also marks
// Director-only content, so a campaign colour in its range is refused rather
// than moved; red and green are moved because a near colour still has a
// usable neighbour. The JS in customize_look.js (nearAmber, offState) mirrors
// these; change both together.
const (
	stateRedLight, stateRedDark     = "#dc2626", "#f87171"
	stateGreenLight, stateGreenDark = "#16a34a", "#4ade80"
	stateHueWindow                  = 12.0 // degrees either side of a state hue
	stateApart                      = 0.10 // OKLab distance a moved colour must keep
)

// hsl returns hue (degrees), saturation and lightness (both 0..1).
func hsl(hex string) (h, s, l float64) {
	c := hexToRgb(hex)
	mx, mn := math.Max(c[0], math.Max(c[1], c[2])), math.Min(c[0], math.Min(c[1], c[2]))
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case c[0]:
		h = math.Mod((c[1]-c[2])/d+6, 6)
	case c[1]:
		h = (c[2]-c[0])/d + 2
	default:
		h = (c[0]-c[1])/d + 4
	}
	return h * 60, s, l
}

// NearAmber reports whether hex sits in the Director's amber: hue 22 to 58
// degrees, saturation above 45% and lightness above 30%.
func NearAmber(hex string) bool {
	h, s, l := hsl(hex)
	return h >= 22 && h <= 58 && s > 0.45 && l > 0.30
}

// hueGap is the shortest distance between two hues in degrees.
func hueGap(a, b float64) float64 {
	d := math.Abs(a - b)
	return math.Min(d, 360-d)
}

// OffState moves a colour that sits within 12 degrees of the state red or
// green away from it by lowering its chroma until it is told apart from the
// state colour in both themes. Other colours come back unchanged. moved says
// whether the colour changed.
func OffState(hex string) (out string, moved bool) {
	out = Lower(hex)
	h, _, _ := hsl(out)
	var near [2]string
	switch {
	case hueGap(h, 0) <= stateHueWindow:
		near = [2]string{stateRedLight, stateRedDark}
	case hueGap(h, 142) <= stateHueWindow:
		near = [2]string{stateGreenLight, stateGreenDark}
	default:
		return out, false
	}
	apart := func(c string) bool {
		return DeltaE(c, near[0]) >= stateApart && DeltaE(c, near[1]) >= stateApart
	}
	if apart(out) {
		return out, false
	}
	c := oklch(out)
	for f := 0.9; f >= 0; f -= 0.1 {
		cand := lchHex(c[0], c[1]*f, c[2])
		if apart(cand) {
			return cand, cand != out
		}
	}
	return lchHex(c[0], 0, c[2]), true
}

// AccentTame is the campaign colour's whole intake: the calm lightness and
// chroma range, then off the state colours.
func AccentTame(hex string) (out string, toned bool) {
	out, toned = Tame(hex, false)
	out, moved := OffState(out)
	return out, toned || moved
}
