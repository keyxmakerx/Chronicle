package decks

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Looks handles GET /campaigns/:id/decks/looks: the Director's proof sheet of
// every look the card has. It stores nothing and invents no cards: every card
// on it is a labelled sample. It goes when the workshop's Look tab lands.
func (h *Handler) Looks(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// A player or scribe meets the same answer as a page that does not exist.
	if !cc.CanAuthorDmOnly() {
		return apperror.NewNotFound("page not found")
	}
	return middleware.Render(c, http.StatusOK, LooksPage(proofSections()))
}

// proofCell is one sample on the proof sheet.
type proofCell struct {
	Label string
	Look  Look
	State CardState
	Opts  CardOpts
	// Turn and Move name the playable ones; Reset restores them.
	Turn, Move string
	Pile       bool
}

// proofSection is a titled row of samples.
type proofSection struct {
	ID, Title, Note string
	Cells           []proofCell
}

// proofKinds paints the ten palette colours across the eight signs, so every
// colour and every sign is on the sheet.
func proofKinds() []KindDef {
	out := make([]KindDef, 0, len(Palette))
	for i, p := range Palette {
		out = append(out, KindDef{Key: "k-" + p, Name: "Kind " + p, Colour: p, Sign: signValues[i%len(signValues)]})
	}
	return out
}

// proofLook is the default look with the sample kinds and a back, changed by f.
func proofLook(f func(*Look)) Look {
	l := defaultLook()
	l.Kinds = proofKinds()
	l.Back = Back{Pattern: "lattice", Colour: "indigo", Words: "Sample deck", Sign: "star"}
	if f != nil {
		f(&l)
	}
	return l
}

func proofFace(kind, corner string, rare bool) Face {
	return Face{
		CardID: "sample", Name: "Sample card", KindKey: kind, Corner: corner, Rare: rare,
		Words: "Words go here. A few short lines show how they sit on the card.",
	}
}

func proofUp(kind, corner string, rare bool) CardState {
	return FaceUp(proofFace(kind, corner, rare), "")
}

func proofSections() []proofSection {
	var secs []proofSection
	row := CardOpts{Size: SizeRow}
	hand := CardOpts{Size: SizeHand}

	// Borders on every shape, then at the hand size where every frame thins.
	var bs []proofCell
	for _, b := range borderValues {
		for _, sh := range shapeValues {
			l := proofLook(func(l *Look) { l.Border = b; l.Shape = sh })
			bs = append(bs, proofCell{Label: b + ", " + sh, Look: l, State: proofUp("k-violet", "7", false), Opts: row})
		}
	}
	secs = append(secs, proofSection{ID: "borders", Title: "Borders on every shape", Cells: bs})
	var hs []proofCell
	for _, b := range borderValues {
		l := proofLook(func(l *Look) { l.Border = b })
		hs = append(hs, proofCell{Label: b + ", hand size", Look: l, State: proofUp("k-teal", "7", false), Opts: hand})
	}
	secs = append(secs, proofSection{ID: "hand", Title: "Hand size", Note: "About 92px wide: frames thin, rivets shrink, the corner mark goes.", Cells: hs})

	// Backs: every pattern, then every sign, then a pile.
	var bk []proofCell
	for i, p := range patternValues {
		l := proofLook(func(l *Look) { l.Back.Pattern = p; l.Back.Colour = Palette[i%len(Palette)] })
		bk = append(bk, proofCell{Label: p + " back", Look: l, State: FaceDown(), Opts: CardOpts{Size: SizeRow, DeckName: "Sample deck"}})
	}
	for i, sg := range backSignValues {
		l := proofLook(func(l *Look) { l.Back.Sign = sg; l.Back.Colour = Palette[(i+3)%len(Palette)] })
		bk = append(bk, proofCell{Label: sg + " sign", Look: l, State: FaceDown(), Opts: CardOpts{Size: SizeRow, DeckName: "Sample deck"}})
	}
	for _, sh := range []string{"standard", "small"} {
		l := proofLook(func(l *Look) { l.Shape = sh; l.Border = "metal"; l.Back.Pattern = "iron" })
		bk = append(bk, proofCell{Label: sh + " back, metal", Look: l, State: FaceDown(), Opts: CardOpts{Size: SizeRow, DeckName: "Sample deck"}})
	}
	secs = append(secs, proofSection{ID: "backs", Title: "Backs and signs", Cells: bk})

	// One list at a time, everything else left at its default.
	one := func(id, title string, vals []string, set func(*Look, string), corner string) proofSection {
		var cs []proofCell
		for _, v := range vals {
			v := v
			l := proofLook(func(l *Look) { set(l, v) })
			cs = append(cs, proofCell{Label: v, Look: l, State: proofUp("k-plum", corner, false), Opts: row})
		}
		return proofSection{ID: id, Title: title, Cells: cs}
	}
	secs = append(secs,
		one("windows", "Picture windows", windowValues, func(l *Look, v string) { l.Window = v }, "7"),
		one("names", "Name position", nameValues, func(l *Look, v string) { l.Name = v }, "7"),
		one("lettering", "Lettering", letteringValues, func(l *Look, v string) { l.Lettering = v }, "7"),
		one("corners", "Corner marks", cornerValues, func(l *Look, v string) { l.Corner = v }, "7"),
		one("markers", "Kind markers", markerValues, func(l *Look, v string) { l.Marker = v }, "7"),
		one("finishes", "Finishes", finishValues, func(l *Look, v string) { l.Finish = v }, "7"),
	)
	secs[len(secs)-1].Cells = append(secs[len(secs)-1].Cells,
		proofCell{Label: "rare", Look: proofLook(nil), State: proofUp("k-plum", "7", true), Opts: row},
		proofCell{Label: "foil, small", Look: proofLook(func(l *Look) { l.Finish = "foil"; l.Shape = "small" }), State: proofUp("k-plum", "7", false), Opts: row})

	var kd []proofCell
	for _, k := range proofKinds() {
		kd = append(kd, proofCell{Label: k.Name + ", " + k.Sign, Look: proofLook(nil), State: proofUp(k.Key, "", false), Opts: row})
	}
	secs = append(secs, proofSection{ID: "kinds", Title: "Kinds: ten colours, eight signs", Cells: kd})

	// The leaf, beside a plate at the open size.
	secs = append(secs, proofSection{ID: "leaf", Title: "Open size, with the leaf", Cells: []proofCell{
		{Label: "double", Look: proofLook(nil), State: proofUp("k-blue", "7", false), Opts: CardOpts{Size: SizeOpen, Foot: "In the deck"}},
		{Label: "metal", Look: proofLook(func(l *Look) { l.Border = "metal" }), State: proofUp("k-rose", "7", false), Opts: CardOpts{Size: SizeOpen, Foot: "In the deck"}},
		{Label: "stone, rare", Look: proofLook(func(l *Look) { l.Border = "stone" }), State: proofUp("k-moss", "7", true), Opts: CardOpts{Size: SizeOpen, Foot: "In the deck"}},
	}})

	// One playable card per turn-over, then one per fixed move.
	var ts []proofCell
	for i, t := range turnValues {
		t := t
		l := proofLook(func(l *Look) { l.Turn = t })
		ts = append(ts, proofCell{Label: t, Look: l, State: FaceDown(), Turn: t, Opts: CardOpts{Size: SizeRow, DeckName: "Sample deck", Position: strconv.Itoa(i+1) + " of " + strconv.Itoa(len(turnValues)), Turnable: true}})
	}
	secs = append(secs, proofSection{ID: "turns", Title: "Turn-overs", Note: "Press a card to turn it. Reset puts it back face down.", Cells: ts})
	var ms []proofCell
	for _, m := range moveNames() {
		m := m
		ms = append(ms, proofCell{Label: m, Look: proofLook(nil), State: proofUp("k-cyan", "7", false), Move: m, Opts: row, Pile: m == "shuffle"})
	}
	secs = append(secs, proofSection{ID: "moves", Title: "Fixed moves", Note: "The same for every deck.", Cells: ms})
	return secs
}

// moveNames are the fixed moves, the same for every deck.
func moveNames() []string {
	return []string{"draw", "deal", "shuffle", "play", "used", "gone", "ready"}
}

// moveOffset is where a travelling move sends a sample card, in pixels.
func moveOffset(m string) (dx, dy int) {
	switch m {
	case "draw":
		return 70, 0
	case "deal":
		return 40, 60
	case "play":
		return 40, -50
	case "used":
		return 40, 0
	}
	return 0, 0
}
