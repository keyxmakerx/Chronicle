package decks

// lookRow is one row of the Look tab: one choice a deck makes, drawn as a row
// of small real cards or backs. Key is the look key ("border") or, for the
// back, "back.pattern"; the page sends exactly that path and the value.
type lookRow struct {
	Key, Title string
	Values     []string
	Words      map[string]string
	// Kind is how a choice is drawn: "face" (a card with that value), "back"
	// (a back with that value), "pose" (a back in the turn-over's pose) or
	// "colour" (a dot).
	Kind string
	// TallOnly rows have no effect on the standard and small shapes.
	TallOnly bool
}

// foldAfter is how many choices a row shows before the rest fold behind
// "Show all".
const foldAfter = 8

var lookRows = []lookRow{
	{Key: "border", Title: "Border", Kind: "face", Values: borderValues, Words: map[string]string{
		"double": "Double line", "line": "Single line", "kit": "Cornered", "metal": "Dark metal", "brass": "Brass", "steel": "Steel", "bound": "Chained", "stone": "Stone", "none": "None"}},
	{Key: "shape", Title: "Shape", Kind: "face", Values: shapeValues, Words: map[string]string{"tall": "Tall", "standard": "Standard", "small": "Small"}},
	{Key: "window", Title: "Picture window", Kind: "face", TallOnly: true, Values: windowValues, Words: map[string]string{
		"arch": "Arch", "square": "Square", "pointed": "Pointed", "round": "Round", "full": "Full"}},
	{Key: "name", Title: "Where the name sits", Kind: "face", TallOnly: true, Values: nameValues, Words: map[string]string{"below": "Below the picture", "over": "Over the picture"}},
	{Key: "lettering", Title: "Lettering", Kind: "face", Values: letteringValues, Words: map[string]string{
		"plain": "Plain", "carved": "Carved", "classic": "Classic", "condensed": "Condensed", "hand": "Handwritten"}},
	{Key: "corner", Title: "Corner mark", Kind: "face", Values: cornerValues, Words: map[string]string{
		"roman": "Roman numerals", "number": "Numbers", "suit": "Suit", "pips": "Pips", "none": "None"}},
	{Key: "marker", Title: "Kind marker", Kind: "face", Values: markerValues, Words: map[string]string{
		"gem": "Gem", "banner": "Banner", "tab": "Tab", "ribbon": "Ribbon", "line": "Line"}},
	{Key: "finish", Title: "Finish", Kind: "face", Values: finishValues, Words: map[string]string{
		"matte": "Matte", "glow": "Glow", "sheen": "Sheen", "foil": "Foil"}},
	{Key: "back.pattern", Title: "Back pattern", Kind: "back", Values: patternValues, Words: map[string]string{
		"lattice": "Lattice", "rays": "Rays", "waves": "Waves", "stars": "Stars", "check": "Check", "plain": "Plain", "iron": "Iron"}},
	{Key: "back.sign", Title: "Back sign", Kind: "back", Values: backSignValues, Words: map[string]string{
		"star": "Star", "sun": "Sun", "moon": "Moon", "eye": "Eye", "crown": "Crown", "anchor": "Anchor", "flame": "Flame", "sword": "Sword", "none": "None"}},
	{Key: "back.colour", Title: "Back colour", Kind: "colour", Values: append([]string{""}, Palette...), Words: map[string]string{"": "The campaign's colour",
		"violet": "Violet", "indigo": "Indigo", "blue": "Blue", "cyan": "Cyan", "teal": "Teal", "moss": "Moss", "plum": "Plum", "rose": "Rose", "umber": "Umber", "slate": "Slate"}},
	{Key: "turn", Title: "Turning over", Kind: "pose", Values: turnValues, Words: map[string]string{
		"flip": "Flip", "slow": "Slow turn", "toss": "Tossed", "slam": "Slammed down", "sweep": "Sweep", "burn": "Burned through", "ink": "Inked in", "seal": "Wax seal", "bolt": "Lightning", "forge": "Forged"}},
}

// word is a value in words; an unlisted value is shown as it is.
func (r lookRow) word(v string) string {
	if w, ok := r.Words[v]; ok {
		return w
	}
	return v
}

// lookValue reads the look's value for a row key.
func lookValue(l Look, key string) string {
	switch key {
	case "shape":
		return l.Shape
	case "border":
		return l.Border
	case "turn":
		return l.Turn
	case "window":
		return l.Window
	case "name":
		return l.Name
	case "lettering":
		return l.Lettering
	case "corner":
		return l.Corner
	case "marker":
		return l.Marker
	case "finish":
		return l.Finish
	case "back.pattern":
		return l.Back.Pattern
	case "back.sign":
		return l.Back.Sign
	case "back.colour":
		return l.Back.Colour
	}
	return ""
}

// withChoice is the look with one row's value set, for drawing a swatch.
func withChoice(l Look, key, v string) Look {
	out := l
	switch key {
	case "shape":
		out.Shape = v
	case "border":
		out.Border = v
	case "turn":
		out.Turn = v
	case "window":
		out.Window = v
	case "name":
		out.Name = v
	case "lettering":
		out.Lettering = v
	case "corner":
		out.Corner = v
	case "marker":
		out.Marker = v
	case "finish":
		out.Finish = v
	case "back.pattern":
		out.Back.Pattern = v
	case "back.sign":
		out.Back.Sign = v
	case "back.colour":
		out.Back.Colour = v
	}
	return out
}

// unavailable says why a choice cannot be used with the deck's look as it is,
// or "" when it can. The server still stores these; the card plays or draws
// them as their twin, and the page says so rather than offering a choice that
// would not show.
func (r lookRow) unavailable(l Look, v string) string {
	if r.TallOnly && l.Shape != "tall" {
		return "Only on tall cards."
	}
	if l.Shape == "small" {
		switch {
		case r.Key == "turn" && v == "burn":
			return "Small cards turn over with a sweep instead."
		case r.Key == "turn" && v == "ink":
			return "Small cards turn over with a fade instead."
		case r.Key == "finish" && v == "foil":
			return "Foil shows as a sheen on small cards."
		}
	}
	return ""
}

// folded is how many choices sit behind "Show all" (0 when the row is short
// enough or its current value is among the hidden ones).
func (r lookRow) folded(l Look) int {
	if len(r.Values) <= foldAfter {
		return 0
	}
	cur := lookValue(l, r.Key)
	for i, v := range r.Values {
		if v == cur && i >= foldAfter {
			return 0
		}
	}
	return len(r.Values) - foldAfter
}

// poseClass is the static pose a turning-over choice is drawn in.
func poseClass(turn string) string { return "dkw-pose-" + turn }

// styleLine is the style's name with ", changed" joined to it directly; a
// template would put whitespace between the name and the comma.
func styleLine(name string, changed bool) string {
	if changed {
		return name + ", changed"
	}
	return name
}

// styleNote names the style closest to the deck's look and says "changed"
// once the look no longer matches it. A deck stores no style, only values, so
// the nearest style is the one it most likely started from.
func styleNote(l Look) (name string, changed bool) {
	best, bestN := deckStyles[0], -1
	for _, s := range deckStyles {
		m := s.Look()
		n := 0
		for _, same := range []bool{
			m.Border == l.Border, m.Shape == l.Shape, m.Window == l.Window, m.Lettering == l.Lettering, m.Corner == l.Corner,
			m.Marker == l.Marker, m.Finish == l.Finish, m.Turn == l.Turn, m.Back.Pattern == l.Back.Pattern,
			m.Back.Colour == l.Back.Colour, m.Back.Sign == l.Back.Sign,
		} {
			if same {
				n++
			}
		}
		if n > bestN {
			best, bestN = s, n
		}
	}
	return best.Name, bestN != 11
}
