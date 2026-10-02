// Package colour is a Go port of the colour maths in the Customize page's
// browser editor. The server emits the same readable shades the live preview
// shows, so every function here must agree with the JavaScript to the hex
// digit; change both together and re-run the table tests, which hold values
// generated from the JS.
package colour

import (
	"math"
	"strconv"
	"strings"
)

// ink is the dark text colour used on light fills; it must match the JS INK.
const ink = "#111827"

// clamp mirrors Math.max(a, Math.min(b, v)).
func clamp(v, a, b float64) float64 { return math.Max(a, math.Min(b, v)) }

// jsRound mirrors Math.round for the non-negative values used here: halves
// round up, where Go's math.Round rounds them away from zero.
func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

// ValidHex reports whether s is exactly #RRGGBB (case-insensitive). Callers
// validate user input with it before using any other function in this package.
func ValidHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// Lower normalises a hex colour to the lowercase form the JS emits.
func Lower(hex string) string { return strings.ToLower(hex) }

// hexToRgb parses #rgb or #rrggbb into 0..1 channels. Unparseable pairs read
// as 0 rather than NaN because callers are expected to have validated input.
func hexToRgb(h string) [3]float64 {
	h = strings.Replace(h, "#", "", 1)
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var out [3]float64
	for i := 0; i < 3; i++ {
		lo, hi := i*2, i*2+2
		if hi > len(h) {
			continue
		}
		n, err := strconv.ParseInt(h[lo:hi], 16, 32)
		if err != nil {
			continue
		}
		out[i] = float64(n) / 255
	}
	return out
}

// rgbToHex clamps and quantises 0..1 channels to lowercase #rrggbb.
func rgbToHex(c [3]float64) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 0, 7)
	b = append(b, '#')
	for _, v := range c {
		n := int(jsRound(clamp(v, 0, 1) * 255))
		b = append(b, digits[n>>4], digits[n&15])
	}
	return string(b)
}

// lin converts an sRGB channel to linear light.
func lin(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// delin converts a linear-light channel back to sRGB.
func delin(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

// luminance is the WCAG relative luminance of a hex colour.
func luminance(hex string) float64 {
	c := hexToRgb(hex)
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// Contrast returns the WCAG contrast ratio between two hex colours (1..21).
func Contrast(a, b string) float64 {
	A, B := luminance(a), luminance(b)
	return (math.Max(A, B) + 0.05) / (math.Min(A, B) + 0.05)
}

// toLab converts sRGB channels to OKLab.
func toLab(rgb [3]float64) [3]float64 {
	r, g, b := lin(rgb[0]), lin(rgb[1]), lin(rgb[2])
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// fromLab converts OKLab to (possibly out-of-gamut) sRGB channels.
func fromLab(lab [3]float64) [3]float64 {
	l := math.Pow(lab[0]+0.3963377774*lab[1]+0.2158037573*lab[2], 3)
	m := math.Pow(lab[0]-0.1055613458*lab[1]-0.0638541728*lab[2], 3)
	s := math.Pow(lab[0]-0.0894841775*lab[1]-1.2914855480*lab[2], 3)
	return [3]float64{
		delin(4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		delin(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		delin(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s),
	}
}

// oklch returns lightness, chroma and hue (degrees) of a hex colour.
func oklch(hex string) [3]float64 {
	L := toLab(hexToRgb(hex))
	return [3]float64{L[0], math.Hypot(L[1], L[2]), math.Mod(math.Atan2(L[2], L[1])*180/math.Pi+360, 360)}
}

// lchRgb converts OKLCH to sRGB channels without gamut handling.
func lchRgb(L, C, h float64) [3]float64 {
	r := h * math.Pi / 180
	return fromLab([3]float64{L, C * math.Cos(r), C * math.Sin(r)})
}

// inGamut allows a tiny overshoot so rounding noise does not trigger mapping.
func inGamut(c [3]float64) bool {
	for _, v := range c {
		if !(v >= -0.0005 && v <= 1.0005) {
			return false
		}
	}
	return true
}

// lchHex renders OKLCH as hex, reducing chroma by bisection until in gamut.
func lchHex(L, C, h float64) string {
	c := lchRgb(L, C, h)
	if inGamut(c) {
		return rgbToHex(c)
	}
	lo, hi := 0.0, C
	for i := 0; i < 24; i++ {
		mid := (lo + hi) / 2
		if inGamut(lchRgb(L, mid, h)) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return rgbToHex(lchRgb(L, lo, h))
}

// DeltaE is the Euclidean distance between two colours in OKLab.
func DeltaE(a, b string) float64 {
	A, B := toLab(hexToRgb(a)), toLab(hexToRgb(b))
	return math.Hypot(math.Hypot(A[0]-B[0], A[1]-B[1]), A[2]-B[2])
}

// Mix interpolates between two hex colours in OKLab at t (0 gives a, 1 gives b).
func Mix(a, b string, t float64) string {
	A, B := toLab(hexToRgb(a)), toLab(hexToRgb(b))
	return rgbToHex(fromLab([3]float64{
		A[0] + (B[0]-A[0])*t,
		A[1] + (B[1]-A[1])*t,
		A[2] + (B[2]-A[2])*t,
	}))
}

// Over composites fg at the given alpha over bg in sRGB space, as CSS does.
func Over(fg, bg string, alpha float64) string {
	F, B := hexToRgb(fg), hexToRgb(bg)
	var out [3]float64
	for i := range out {
		out[i] = F[i]*alpha + B[i]*(1-alpha)
	}
	return rgbToHex(out)
}

// RGBChannels renders a hex colour as space-separated 0..255 channels
// ("99 102 241"), the form Tailwind-style alpha tokens consume.
func RGBChannels(hex string) string {
	c := hexToRgb(hex)
	parts := make([]string, 3)
	for i, v := range c {
		parts[i] = strconv.Itoa(int(jsRound(v * 255)))
	}
	return strings.Join(parts, " ")
}

// Shade is a colour together with how far it moved from the one requested.
type Shade struct {
	// Hex is the resulting lowercase #rrggbb colour.
	Hex string
	// DeltaE is the OKLab distance from the input colour (0 if unchanged).
	DeltaE float64
}

// Readable nudges hex's lightness (darker or lighter) until it reaches the
// contrast target against the worst of backgrounds, keeping hue and chroma.
// If the target is unreachable it returns the last candidate tried, matching
// the JS preview.
func Readable(hex string, backgrounds []string, target float64, darker bool) Shade {
	worst := func(h string) float64 {
		w := math.Inf(1)
		for _, b := range backgrounds {
			w = math.Min(w, Contrast(h, b))
		}
		return w
	}
	if worst(hex) >= target {
		return Shade{Hex: hex}
	}
	c := oklch(hex)
	L := c[0]
	step := 0.004
	if darker {
		step = -0.004
	}
	out := hex
	for i := 0; i < 240; i++ {
		L += step
		if L <= 0.02 || L >= 0.995 {
			break
		}
		out = lchHex(L, c[1], c[2])
		if worst(out) >= target {
			break
		}
	}
	return Shade{Hex: out, DeltaE: DeltaE(hex, out)}
}

// Fill is a button/badge background with the text colour to put on it.
type Fill struct {
	// Fill is the background colour, possibly adjusted from the request.
	Fill string
	// On is the text colour that reaches 4.5:1 on Fill.
	On string
	// DeltaE is how far Fill moved from the requested colour.
	DeltaE float64
	// Flipped is true when dark text is used instead of white.
	Flipped bool
}

// FillFor picks a fill and its text colour for an accent. It prefers white
// text, darkens slightly if that suffices, and otherwise switches to dark ink
// (or whichever adjustment moves the colour least).
func FillFor(hex string) Fill {
	if Contrast(hex, "#ffffff") >= 4.5 {
		return Fill{Fill: hex, On: "#ffffff"}
	}
	dk := Readable(hex, []string{"#ffffff"}, 4.5, true)
	if dk.DeltaE <= 0.06 {
		return Fill{Fill: dk.Hex, On: "#ffffff", DeltaE: dk.DeltaE}
	}
	if Contrast(hex, ink) >= 4.5 {
		return Fill{Fill: hex, On: ink, Flipped: true}
	}
	lt := Readable(hex, []string{ink}, 4.5, false)
	if lt.DeltaE < dk.DeltaE {
		return Fill{Fill: lt.Hex, On: ink, DeltaE: lt.DeltaE, Flipped: true}
	}
	return Fill{Fill: dk.Hex, On: "#ffffff", DeltaE: dk.DeltaE}
}

// chromaCap limits chroma at high lightness where the sRGB gamut narrows.
func chromaCap(L float64) float64 { return 0.25 - math.Max(0, L-0.72)*1.625 }

// floorL is the darkest lightness allowed; deep themes may go darker.
func floorL(deep bool) float64 {
	if deep {
		return 0.14
	}
	return 0.30
}

// Tame pulls an accent into a usable lightness/chroma range for a theme and
// reports whether it visibly changed (OKLab distance above 0.02).
func Tame(hex string, deep bool) (out string, toned bool) {
	c := oklch(hex)
	L := clamp(c[0], floorL(deep), 0.80)
	C := math.Min(c[1], chromaCap(L))
	if L == c[0] && C == c[1] {
		out = strings.ToLower(hex)
	} else {
		out = lchHex(L, C, c[2])
	}
	return out, DeltaE(hex, out) > 0.02
}
