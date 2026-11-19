package colour

// A tag chip shows the owner's colour as words on a faint tint of that same
// colour. The tint is fixed (the "22" alpha byte the chips have always used),
// so the words are shaded, per theme, until they read on it. The owner's
// colour itself stays as the chip's tint and any swatch.
const chipTint = 34.0 / 255.0

// Surfaces a chip can sit on in each theme: the card and the alternate row.
var (
	chipLightSurfaces = []string{"#ffffff", "#f3f4f6"}
	chipDarkSurfaces  = []string{"#1f2937", "#111827"}
)

// ChipInk returns the word colours for a chip tinted from hex, one for the
// light theme and one for the dark, each reaching 4.5:1 on the tint over the
// theme's surfaces with the hue kept. Input that is not #rrggbb comes back
// unchanged in both.
func ChipInk(hex string) (light, dark string) {
	if !ValidHex(hex) {
		return hex, hex
	}
	hex = Lower(hex)
	tints := func(surfaces []string) []string {
		out := make([]string, len(surfaces))
		for i, s := range surfaces {
			out[i] = Over(hex, s, chipTint)
		}
		return out
	}
	light = Readable(hex, tints(chipLightSurfaces), 4.5, true).Hex
	dark = Readable(hex, tints(chipDarkSurfaces), 4.5, false).Hex
	return light, dark
}

// ChipStyle is the inline style for a tag chip: the tint, and both word
// colours as custom properties the .chip-ink class picks between by theme. An
// invalid colour gives the empty style, so the chip falls back to plain words.
func ChipStyle(hex string) string {
	if !ValidHex(hex) {
		return ""
	}
	l, d := ChipInk(hex)
	return "background-color: " + Lower(hex) + "22; --chip-l: " + l + "; --chip-d: " + d
}

// BadgeColours returns the background and word colours for a solid badge
// painted from hex: the fill is nudged only as far as the words need to reach
// 4.5:1. Input that is not #rrggbb gives white words on a neutral grey.
func BadgeColours(hex string) (bg, fg string) {
	if !ValidHex(hex) {
		return "#6b7280", "#ffffff"
	}
	f := FillFor(Lower(hex))
	return f.Fill, f.On
}
