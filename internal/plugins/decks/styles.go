package decks

// DeckStyle is a starting look for a new deck: plain data, one table. Picking
// one only writes these values into the new deck's look; nothing stays linked
// to the style afterwards.
type DeckStyle struct {
	Key, Name string
	// Line says what the style does, in words, for the picker.
	Line      string
	Border    string
	Shape     string
	Window    string
	Lettering string
	Corner    string
	Marker    string
	Finish    string
	// Back is the pattern, palette colour ("" is the campaign's colour) and sign.
	Pattern, Colour, Sign string
	Turn                  string
}

// deckStyles are the seven starting styles, in the order the picker shows them.
// The first is what a deck made without choosing wears.
var deckStyles = []DeckStyle{
	{Key: "plain", Name: "Plain", Line: "Where every new deck starts", Border: "double", Shape: "tall", Window: "arch", Lettering: "plain", Corner: "roman", Marker: "gem", Finish: "matte", Pattern: "lattice", Colour: "", Sign: "star", Turn: "flip"},
	{Key: "iron-omens", Name: "Iron omens", Line: "Dark metal, iron stripes, slammed down", Border: "metal", Shape: "tall", Window: "pointed", Lettering: "carved", Corner: "roman", Marker: "gem", Finish: "glow", Pattern: "iron", Colour: "slate", Sign: "eye", Turn: "slam"},
	{Key: "heros-boons", Name: "Hero's boons", Line: "Double line, rays in teal, a sheen, flip", Border: "double", Shape: "tall", Window: "arch", Lettering: "classic", Corner: "suit", Marker: "ribbon", Finish: "sheen", Pattern: "rays", Colour: "teal", Sign: "sun", Turn: "flip"},
	{Key: "tavern-game", Name: "Tavern game", Line: "Single line, small, umber check, tossed", Border: "line", Shape: "small", Window: "arch", Lettering: "hand", Corner: "pips", Marker: "tab", Finish: "matte", Pattern: "check", Colour: "umber", Sign: "crown", Turn: "toss"},
	{Key: "star-reading", Name: "Star reading", Line: "Cornered, indigo stars, foil, slow turn", Border: "kit", Shape: "tall", Window: "round", Lettering: "carved", Corner: "roman", Marker: "gem", Finish: "foil", Pattern: "stars", Colour: "indigo", Sign: "moon", Turn: "slow"},
	{Key: "road-events", Name: "Road events", Line: "No border, standard, blue waves, sweep", Border: "none", Shape: "standard", Window: "arch", Lettering: "condensed", Corner: "number", Marker: "banner", Finish: "matte", Pattern: "waves", Colour: "blue", Sign: "anchor", Turn: "sweep"},
	{Key: "ember-curses", Name: "Ember curses", Line: "Bound, square window, rose, burned through", Border: "bound", Shape: "tall", Window: "square", Lettering: "carved", Corner: "roman", Marker: "line", Finish: "glow", Pattern: "plain", Colour: "rose", Sign: "flame", Turn: "burn"},
}

// DeckStyles returns the styles in picker order. The slice is a copy.
func DeckStyles() []DeckStyle { return append([]DeckStyle(nil), deckStyles...) }

// StyleByKey finds a style; ok is false for a key the table does not have.
func StyleByKey(key string) (DeckStyle, bool) {
	for _, s := range deckStyles {
		if s.Key == key {
			return s, true
		}
	}
	return DeckStyle{}, false
}

// LookInput writes the style as a look change, so the service checks every
// value against the lists exactly as it does for any other look.
func (s DeckStyle) LookInput() LookInput {
	p := func(v string) *string { return &v }
	return LookInput{
		Shape: p(s.Shape), Border: p(s.Border), Turn: p(s.Turn), Window: p(s.Window), Lettering: p(s.Lettering),
		Corner: p(s.Corner), Marker: p(s.Marker), Finish: p(s.Finish),
		Back: &BackInput{Pattern: p(s.Pattern), Colour: p(s.Colour), Sign: p(s.Sign)},
	}
}

// Look is the style applied to the default look.
func (s DeckStyle) Look() Look {
	l, _ := applyLook(defaultLook(), ptr2(s.LookInput()))
	return l
}

func ptr2[T any](v T) *T { return &v }
