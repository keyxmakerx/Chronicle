package mapsketch

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ColorNames are the named colours a shape or marker can take. Their shades
// belong to the widget's theme, so they read on a light and a dark page.
var ColorNames = []string{"sky", "teal", "orchid", "rose", "green", "violet", "cyan", "lime"}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// NormalizeColor checks a colour word and returns it in the stored form
// (lower case). problem is a plain sentence when the colour is refused.
//
// Amber is the Director's colour: the page marks DM-only things with it, so a
// custom hex in the amber hue band is refused rather than left to look like a
// hidden marker to players.
func NormalizeColor(c string) (norm, problem string) {
	c = strings.ToLower(strings.TrimSpace(c))
	for _, n := range ColorNames {
		if c == n {
			return c, ""
		}
	}
	if !hexColorRe.MatchString(c) {
		return "", fmt.Sprintf("color %q isn't one of %s or a hex colour like #3366cc.", c, strings.Join(ColorNames, ", "))
	}
	if isAmberish(c) {
		return "", fmt.Sprintf("color %s is too close to amber, which marks what only the DM sees; pick another.", c)
	}
	return c, ""
}

// isAmberish applies the UI standard's amber band: hue 22 to 58 degrees,
// HSL saturation above 45% and lightness above 30%. Muted and dark colours in
// that hue stay allowed: only the bright ones read as the DM marker.
func isAmberish(hex string) bool {
	r64, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g64, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b64, _ := strconv.ParseUint(hex[5:7], 16, 8)
	r, g, b := float64(r64)/255, float64(g64)/255, float64(b64)/255
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	if mx == 0 || mx-mn == 0 {
		return false
	}
	l := (mx + mn) / 2
	sat := (mx - mn) / (1 - math.Abs(2*l-1))
	var hue float64
	switch mx {
	case r:
		hue = math.Mod((g-b)/(mx-mn), 6)
	case g:
		hue = (b-r)/(mx-mn) + 2
	default:
		hue = (r-g)/(mx-mn) + 4
	}
	hue *= 60
	if hue < 0 {
		hue += 360
	}
	return hue >= 22 && hue <= 58 && sat > 0.45 && l > 0.30
}
