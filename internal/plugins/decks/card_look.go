package decks

import (
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// CardSize is how big a card is drawn. A smaller size shows fewer lines of
// words, never smaller type, and the hand size thins every frame.
type CardSize string

const (
	// SizeRow is a card in a list or a row.
	SizeRow CardSize = "row"
	// SizeHand is a card in a player's hand, about 92px wide.
	SizeHand CardSize = "hand"
	// SizeOpen is a card opened to be read, with its leaf beside it.
	SizeOpen CardSize = "open"
)

// CardState is what a card shows. It is built only by FaceDown and FaceUp, so
// a face-down card cannot carry a name, words, kind, picture or id: FaceDown
// takes nothing, and the components draw a face only from a FaceUp state.
type CardState struct {
	up      bool
	face    Face
	picture string
	// deferPicture holds the picture back for the motion widget to fetch once a
	// turn is under way, so a face that has not been asked for never loads.
	deferPicture bool
}

// FaceDown is a card showing its back, drawn from the deck's look alone.
func FaceDown() CardState { return CardState{} }

// FaceUp is a card showing its face. picture is an optional picture address;
// without one the face shows its material and the kind's sign.
func FaceUp(f Face, picture string) CardState { return CardState{up: true, face: f, picture: picture} }

// FaceUpDeferred is FaceUp for the markup a turn-over fetches: the picture is
// named but loaded by the widget once the turn is under way.
func FaceUpDeferred(f Face, picture string) CardState {
	return CardState{up: true, face: f, picture: picture, deferPicture: true}
}

// Up reports whether the state shows a face.
func (s CardState) Up() bool { return s.up }

// CardOpts are the parts of a card that are not its look or its state.
type CardOpts struct {
	Size CardSize
	// DeckName and Position name which card of which deck a face-down card is,
	// for the turn-over button ("card 3 of 5", "Orrin"). Neither is card data.
	DeckName string
	Position string
	// Turnable draws the turn-over button on a face-down card.
	Turnable bool
	// FaceSrc is the address the widget fetches the face from when the card is
	// turned. It is never requested before. It must name a copy, never a card:
	// the address sits in the markup of a face-down card, so one that named the
	// card (by id, or by anything that identifies it) would tell the page which
	// card it is before it is turned.
	FaceSrc string
	// Foot is a quiet last line on the leaf (a card's fate, say).
	Foot string
}

// Lookup tables: a stored key becomes a class only through one of these, one
// per list in settings.go. A key with no entry draws as the list's first value,
// and card_look_test.go fails a list value that has no entry or no rule.
var (
	shapeClass = map[string]string{"tall": "dk-s-tall", "standard": "dk-s-standard", "small": "dk-s-small"}
	// A border is its class and, for the five forged ones, the shared material class.
	borderClass = map[string]string{
		"double": "dk-b-double", "line": "dk-b-line", "kit": "dk-b-kit", "none": "dk-b-none",
		"metal": "dk-b-metal dk-mat", "brass": "dk-b-brass dk-mat", "steel": "dk-b-steel dk-mat",
		"bound": "dk-b-bound dk-mat", "stone": "dk-b-stone dk-mat",
	}
	turnClass = map[string]string{
		"flip": "dk-t-flip", "slow": "dk-t-slow", "toss": "dk-t-toss", "slam": "dk-t-slam", "sweep": "dk-t-sweep",
		"burn": "dk-t-burn", "ink": "dk-t-ink", "seal": "dk-t-seal", "bolt": "dk-t-bolt", "forge": "dk-t-forge",
	}
	windowClass    = map[string]string{"arch": "dk-w-arch", "square": "dk-w-square", "pointed": "dk-w-pointed", "round": "dk-w-round", "full": "dk-w-full"}
	nameClass      = map[string]string{"below": "dk-n-below", "over": "dk-n-over"}
	letteringClass = map[string]string{"plain": "dk-l-plain", "carved": "dk-l-carved", "classic": "dk-l-classic", "condensed": "dk-l-condensed", "hand": "dk-l-hand"}
	cornerClass    = map[string]string{"roman": "dk-c-roman", "number": "dk-c-number", "suit": "dk-c-suit", "pips": "dk-c-pips", "none": "dk-c-none"}
	markerClass    = map[string]string{"gem": "dk-m-gem", "banner": "dk-m-banner", "tab": "dk-m-tab", "ribbon": "dk-m-ribbon", "line": "dk-m-line"}
	finishClass    = map[string]string{"matte": "dk-f-matte", "glow": "dk-f-glow", "sheen": "dk-f-sheen", "foil": "dk-f-foil"}
	patternClass   = map[string]string{
		"lattice": "dk-p-lattice", "rays": "dk-p-rays", "waves": "dk-p-waves", "stars": "dk-p-stars",
		"check": "dk-p-check", "plain": "dk-p-plain", "iron": "dk-p-iron",
	}
	paletteClass = map[string]string{
		"violet": "dk-pal-violet", "indigo": "dk-pal-indigo", "blue": "dk-pal-blue", "cyan": "dk-pal-cyan", "teal": "dk-pal-teal",
		"moss": "dk-pal-moss", "plum": "dk-pal-plum", "rose": "dk-pal-rose", "umber": "dk-pal-umber", "slate": "dk-pal-slate",
	}
	sizeClass = map[CardSize]string{SizeRow: "dk-sz-row", SizeHand: "dk-sz-hand", SizeOpen: "dk-sz-open"}

	// kindSignPath is the drawn shape of each kind sign, on a 20 by 20 box.
	kindSignPath = map[string]string{
		"round":    "M10 2a8 8 0 1 1 0 16a8 8 0 1 1 0-16z",
		"diamond":  "M10 1l8 9-8 9-8-9z",
		"point":    "M2 3h16L10 18z",
		"square":   "M3 3h14v14H3z",
		"star":     "M10 1l2.6 6 6.4.6-4.9 4.3 1.5 6.3L10 15l-5.6 3.2 1.5-6.3L1 7.6l6.4-.6z",
		"cross":    "M7 2h6v5h5v6h-5v5H7v-5H2V7h5z",
		"crescent": "M12 1.5a8.6 8.6 0 1 0 6.5 14A7 7 0 0 1 12 1.5z",
		"drop":     "M10 1c3 5 6.5 8 6.5 12a6.5 6.5 0 0 1-13 0C3.5 9 7 6 10 1z",
	}
	// emblemPath is the drawn shape of each back sign, on a 40 by 40 box. The
	// anchor's stroked ring and bar are added by the component.
	emblemPath = map[string]string{
		"star":   "M20 3l4.2 12.8L37 20l-12.8 4.2L20 37l-4.2-12.8L3 20l12.8-4.2z",
		"sun":    "M20 12a8 8 0 1 1 0 16a8 8 0 1 1 0-16zM20 1l3 8h-6zM20 39l-3-8h6zM1 20l8-3v6zM39 20l-8 3v-6zM6.6 6.6l7.8 3.6-4.2 4.2zM33.4 33.4l-7.8-3.6 4.2-4.2zM33.4 6.6l-3.6 7.8-4.2-4.2zM6.6 33.4l3.6-7.8 4.2 4.2z",
		"moon":   "M25 3a17 17 0 1 0 12 25A14 14 0 0 1 25 3z",
		"eye":    "M1 20C8 8 32 8 39 20C32 32 8 32 1 20zM20 12a8 8 0 1 0 0 16a8 8 0 1 0 0-16zM20 16.5a3.5 3.5 0 1 1 0 7a3.5 3.5 0 1 1 0-7z",
		"crown":  "M5 29L3 10l9 8 8-13 8 13 9-8-2 19zM5 32h30v4H5z",
		"anchor": "M2 26l3-6 4 5zM38 26l-3-6-4 5z",
		"flame":  "M20 2c3 7 12 11 12 22a12 12 0 0 1-24 0c0-6 4-9 5-14 2 3 2 6 4 7 2-4 3-9 3-15z",
		"sword":  "M18.5 2h3l1.5 23h-6zM11 25h18v3.5H11zM18.2 28.5h3.6v6h-3.6zM16.5 34.5h7v3.5h-7z",
	}
)

// fittingsFor are the rivets, chains, lock and keystone each forged border
// wears, as empty spans the stylesheet draws. Only the stud (g) is the kind's.
var fittingsFor = map[string][]string{
	"metal": {"dk-rv dk-rv-a", "dk-rv dk-rv-b", "dk-rv dk-rv-c", "dk-rv dk-rv-d", "dk-rv dk-rv-g"},
	"brass": {"dk-rv dk-rv-g"},
	"steel": {"dk-rv dk-rv-g"},
	"bound": {"dk-rv dk-rv-a", "dk-rv dk-rv-b", "dk-rv dk-rv-c", "dk-rv dk-rv-d", "dk-ch dk-ch-l", "dk-ch dk-ch-r", "dk-lk"},
	"stone": {"dk-ks"},
}

// classOf looks key up, falling back to the list's first value.
func classOf(table map[string]string, list []string, key string) string {
	if c, ok := table[key]; ok {
		return c
	}
	return table[list[0]]
}

// lookClasses are the classes every part of a card shares, from the deck's look
// alone: shape, border, window, lettering and so on. It reads no card data.
func lookClasses(l Look) string {
	parts := []string{
		classOf(shapeClass, shapeValues, l.Shape),
		classOf(borderClass, borderValues, l.Border),
		classOf(windowClass, windowValues, l.Window),
		classOf(nameClass, nameValues, l.Name),
		classOf(letteringClass, letteringValues, l.Lettering),
		classOf(cornerClass, cornerValues, l.Corner),
		classOf(markerClass, markerValues, l.Marker),
		classOf(finishClass, finishValues, effectiveFinish(l)),
	}
	return strings.Join(parts, " ")
}

// effectiveFinish is the finish a card actually wears: foil is too much for
// the small shape, so it shows as sheen there.
func effectiveFinish(l Look) string {
	if l.Shape == "small" && l.Finish == "foil" {
		return "sheen"
	}
	return l.Finish
}

func sizeOf(s CardSize) CardSize {
	if _, ok := sizeClass[s]; ok {
		return s
	}
	return SizeRow
}

func sizeClassOf(s CardSize) string { return sizeClass[sizeOf(s)] }

// palette resolves a kind's colour key; "" and an unknown key give "", which
// the stylesheet draws in the campaign's accent.
func paletteOf(key string) string { return paletteClass[key] }

// kindOf finds a face's kind in the deck's look; ok is false for a card with
// no kind or one the look no longer lists.
func kindOf(l Look, key string) (KindDef, bool) {
	if key == "" {
		return KindDef{}, false
	}
	for _, k := range l.Kinds {
		if k.Key == key {
			return k, true
		}
	}
	return KindDef{}, false
}

// kindWord is a card's kind in words, "" when it has none.
func kindWord(l Look, f Face) string {
	if k, ok := kindOf(l, f.KindKey); ok {
		return k.Name
	}
	return ""
}

// accessibleName is the name a screen reader gives a face-up card: the card's
// name and its kind in words, so no kind depends on colour alone.
func accessibleName(l Look, f Face) string {
	if w := kindWord(l, f); w != "" {
		return f.Name + ", " + w
	}
	return f.Name
}

// romanNumeral writes 1 to 3999 in roman numerals; "" for anything else.
func romanNumeral(n int) string {
	if n < 1 || n > 3999 {
		return ""
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

// wholeNumber reads text as a whole number with no sign, spaces or decimals.
func wholeNumber(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// cornerMark is what the corner shows for a card's corner text under the
// deck's corner style: text for roman, number and suit, and a dot count for
// pips. Pips need a whole number from 1 to 10 and fall back to the number
// style (text) otherwise; roman falls back to the text as typed.
func cornerMark(style, text string) (shown string, pips int) {
	text = strings.TrimSpace(text)
	if text == "" || style == "none" {
		return "", 0
	}
	n, whole := wholeNumber(text)
	switch style {
	case "pips":
		if whole && n >= 1 && n <= 10 {
			return text, n
		}
		return text, 0
	case "roman":
		if whole {
			if r := romanNumeral(n); r != "" {
				return r, 0
			}
		}
	}
	return text, 0
}

// turnButtonLabel says which card of which deck the turn-over button turns,
// without naming the card, which is still face down.
func turnButtonLabel(o CardOpts) string {
	label := "Turn over this card"
	if o.Position != "" {
		label = "Turn over card " + o.Position
	}
	if o.DeckName != "" {
		label += ", " + o.DeckName
	}
	return label
}

// backLabel is the accessible name of a face-down card.
func backLabel(o CardOpts) string {
	label := "Face-down card"
	if o.DeckName != "" {
		label += ", " + o.DeckName
	}
	return label
}

// cardStylesheet is the card's stylesheet address, versioned by the asset
// hash so a change reaches viewers at once.
func cardStylesheet() string { return layouts.AssetURL("/static/css/decks_card.css") }
