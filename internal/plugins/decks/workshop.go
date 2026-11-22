package decks

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// The deck workshop: the Director's pages for making and running decks. They
// are server rendered; every write goes through the JSON routes in
// handler.go, so a page can never do what a route would refuse. A player or
// scribe is told the page is not there (404), never that it is not theirs.

// Tabs of a deck's page, in the order they are shown.
var workshopTabs = []struct{ Key, Name string }{
	{"cards", "Cards"}, {"look", "Look"}, {"rules", "Rules"}, {"players", "Players"},
}

// tabOf reads a tab key; an unknown one is the first tab.
func tabOf(s string) string {
	for _, t := range workshopTabs {
		if t.Key == s {
			return s
		}
	}
	return workshopTabs[0].Key
}

// director returns the campaign and the viewer, or the 404 a non-Director gets.
func (h *Handler) director(c *echo.Context) (string, Viewer, error) {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return "", Viewer{}, err
	}
	if !v.Director {
		return "", Viewer{}, apperror.NewNotFound("page not found")
	}
	return cid, v, nil
}

// pictureURL is where a card's picture is loaded from: the media route, with
// the 300px thumbnail at row and hand size.
func pictureURL(id string, size CardSize) string {
	if id == "" {
		return ""
	}
	if size == SizeOpen {
		return "/media/" + id
	}
	return "/media/" + id + "/thumb/300"
}

// dirFace is a card's face as a Director's page draws it.
func dirFace(c DirectorCard) Face {
	return Face{CardID: c.ID, Name: c.Name, KindKey: c.KindKey, Words: c.Words, Picture: c.Picture, Corner: c.Corner, Rare: c.Rare}
}

// upState is the card face up at a size, picture and all.
func upState(c DirectorCard, size CardSize) CardState {
	return FaceUp(dirFace(c), pictureURL(c.Picture, size))
}

// sampleCard is the card the Look tab and the style pickers draw: the deck's
// first card, or a plain sample when it has none.
func sampleCard(d DirectorDeck, kindKey string) DirectorCard {
	var c DirectorCard
	if len(d.Cards) > 0 {
		c = d.Cards[0]
	} else {
		c = DirectorCard{ID: "sample", Name: "A sample card", Corner: "7", Words: "The words of a card sit here, a few short lines."}
	}
	if kindKey != "" {
		c.KindKey = kindKey
	} else if c.KindKey == "" && len(d.Look.Kinds) > 0 {
		c.KindKey = d.Look.Kinds[0].Key
	}
	return c
}

// sampleCardFor is the card a style fan or a choice swatch draws for a look.
func sampleCardFor(l Look, c DirectorCard) DirectorCard {
	if c.KindKey == "" && len(l.Kinds) > 0 {
		c.KindKey = l.Kinds[0].Key
	}
	return c
}

// A page's deck as Directors see it, with the rest of the workshop's reads.
type deckPage struct {
	CampaignID string
	Deck       DirectorDeck
	Roster     Roster
	// Others are the campaign's other decks, for "put it in a deck for keeps".
	Others []DirectorDeck
	Tab    string
	// Card is the card open in the editor, "" for none; Kind is the kind the
	// Look tab's big card wears.
	Card, Kind string
	// Error is a plain sentence a fragment shows when it could not be read.
	Error string
}

func (p deckPage) base() string { return "/campaigns/" + p.CampaignID + "/decks" }

func (p deckPage) pageURL(tab string) string {
	return p.base() + "/workshop/" + p.Deck.ID + "?tab=" + tab
}

func (p deckPage) frag(name string) string {
	return p.base() + "/workshop/" + p.Deck.ID + "/" + name
}

func (p deckPage) cardByID(id string) (DirectorCard, bool) {
	for _, c := range p.Deck.Cards {
		if c.ID == id {
			return c, true
		}
	}
	return DirectorCard{}, false
}

func (p deckPage) heroName(id string) string {
	for _, h := range p.Roster.Heroes {
		if h.ID == id {
			return h.Name
		}
	}
	return ""
}

// totalCards is every copy of the deck, wherever it is.
func (p deckPage) totalCards() int {
	n := p.Deck.Counts
	return n.Pile + n.Shown + n.Hand + n.Play + n.Used + n.Gone
}

// forWhom says who the deck is for, in words.
func (p deckPage) forWhom() string {
	switch p.Deck.Kind {
	case KindHero:
		if n := p.heroName(p.Deck.HolderEntityID); n != "" {
			return "A hero's deck, for " + n
		}
		if p.Deck.HolderEntityID != "" {
			return "A hero's deck"
		}
		return "A hero's deck, for no one yet"
	case KindParty:
		return "The party's deck"
	}
	return "An event deck"
}

// load reads a deck for a Director with everything a page needs.
func (h *Handler) load(c *echo.Context, withOthers bool) (deckPage, error) {
	cid, v, err := h.director(c)
	if err != nil {
		return deckPage{}, err
	}
	ctx := c.Request().Context()
	view, err := h.svc.Get(ctx, cid, c.Param("did"), v)
	if err != nil {
		return deckPage{}, err
	}
	d, ok := view.(DirectorDeck)
	if !ok {
		return deckPage{}, apperror.NewNotFound("page not found")
	}
	roster, err := h.svc.Roster(ctx, cid, v)
	if err != nil {
		return deckPage{}, err
	}
	p := deckPage{CampaignID: cid, Deck: d, Roster: roster, Tab: tabOf(c.QueryParam("tab")), Card: c.QueryParam("card"), Kind: c.QueryParam("kind")}
	if withOthers {
		list, err := h.svc.List(ctx, cid, v)
		if err != nil {
			return deckPage{}, err
		}
		if dl, ok := list.(DirectorList); ok {
			for _, o := range dl.Decks {
				if o.ID != d.ID {
					p.Others = append(p.Others, o)
				}
			}
		}
	}
	return p, nil
}

// WorkshopList handles GET /campaigns/:id/decks/workshop: the Decks app page.
func (h *Handler) WorkshopList(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	list, err := h.svc.List(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	dl, _ := list.(DirectorList)
	return middleware.Render(c, http.StatusOK, WorkshopListPage(cid, dl.Decks))
}

// newPage is what the "making a deck" page draws.
type newPage struct {
	CampaignID string
	Styles     []DeckStyle
	Heroes     []Hero
}

// WorkshopNew handles GET /campaigns/:id/decks/workshop/new.
func (h *Handler) WorkshopNew(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	roster, err := h.svc.Roster(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, WorkshopNewPage(newPage{CampaignID: cid, Styles: DeckStyles(), Heroes: roster.Heroes}))
}

// WorkshopDeck handles GET /campaigns/:id/decks/workshop/:did.
func (h *Handler) WorkshopDeck(c *echo.Context) error {
	p, err := h.load(c, true)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, WorkshopDeckPage(p))
}

// WorkshopTable handles the table row's fragment.
func (h *Handler) WorkshopTable(c *echo.Context) error {
	p, err := h.load(c, true)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, DeckTable(p))
}

// WorkshopTab handles one tab's fragment.
func (h *Handler) WorkshopTab(tab string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		p, err := h.load(c, false)
		if err != nil {
			return err
		}
		p.Tab = tab
		return middleware.Render(c, http.StatusOK, deckTabPanel(p))
	}
}

// ---- table row -----------------------------------------------------------

// shownCard is one card in the shown spot.
type shownCard struct {
	CopyID string
	Card   DirectorCard
}

// playCard is one card in play, with where it lies.
type playCard struct {
	CopyID string
	Card   DirectorCard
	Where  string
}

// tableView is the table row: the pile, the shown spot, the used pile and
// what is in play, read from the deck a Director holds.
type tableView struct {
	Pile, Used, InHands int
	// MaxDraw is how many a draw may take now: five less what is shown.
	MaxDraw  int
	Shown    []shownCard
	UsedTop  *DirectorCard
	Play     []playCard
	HasHeros bool
}

func (p deckPage) table() tableView {
	d := p.Deck
	byID := map[string]DirectorCard{}
	for _, c := range d.Cards {
		byID[c.ID] = c
	}
	cps := append([]DirectorCopy(nil), d.Copies...)
	sort.SliceStable(cps, func(i, j int) bool { return cps[i].MovedAt.Before(cps[j].MovedAt) })
	t := tableView{Pile: d.Counts.Pile, Used: d.Counts.Used, InHands: d.Counts.Hand, HasHeros: len(p.Roster.Heroes) > 0}
	for _, cp := range cps {
		card, ok := byID[cp.CardID]
		if !ok {
			continue
		}
		switch cp.Zone {
		case ZoneShown:
			t.Shown = append(t.Shown, shownCard{cp.ID, card})
		case ZoneUsed:
			cc := card
			t.UsedTop = &cc
		case ZonePlay:
			where := "the table"
			if cp.HolderEntityID != "" {
				where = p.heroName(cp.HolderEntityID)
				if where == "" {
					where = "a hero"
				}
			}
			t.Play = append(t.Play, playCard{cp.ID, card, where})
		}
	}
	t.MaxDraw = MaxShown - len(t.Shown)
	if t.MaxDraw > MaxDrawCount {
		t.MaxDraw = MaxDrawCount
	}
	if t.MaxDraw < 0 {
		t.MaxDraw = 0
	}
	return t
}

// left says how many are in the pile, in words.
func (t tableView) left() string { return strconv.Itoa(t.Pile) + " left" }

// kindOfCard names a card's kind, "" for none.
func kindOfCard(l Look, c DirectorCard) string { return kindWord(l, dirFace(c)) }

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// fanLook is a style's look with one kind, so its sample face wears a marker.
func fanLook(s DeckStyle) Look {
	l := s.Look()
	col := s.Colour
	if col == "" {
		col = "violet"
	}
	l.Kinds = []KindDef{{Key: "sample", Name: "Boon", Colour: col, Sign: "round"}}
	return l
}

func fanFace() Face {
	return Face{CardID: "sample", Name: "A sample card", KindKey: "sample", Corner: "7", Words: "Words go here."}
}

// playerRow is one member on the Players tab.
type playerRow struct {
	UserID, Name, Left string
	Chosen             bool
}

// playerRows lists the campaign's players and scribes (Directors always draw)
// with whether the deck has chosen them and how many draws they have left.
func (p deckPage) playerRows() []playerRow {
	chosen := map[string]DirectorPlayer{}
	for _, pl := range p.Deck.Players {
		chosen[pl.UserID] = pl
	}
	var out []playerRow
	for _, m := range p.Roster.Members {
		if m.Director {
			continue
		}
		r := playerRow{UserID: m.UserID, Name: m.Name}
		if r.Name == "" {
			r.Name = "A player"
		}
		if c, ok := chosen[m.UserID]; ok {
			r.Chosen = true
			if c.DrawsLeft != nil {
				r.Left = strconv.Itoa(*c.DrawsLeft)
			}
		}
		out = append(out, r)
	}
	return out
}

// styleJSON is a style as the look body the deck routes take, so the page
// sends exactly what the style says and the service checks it as any look.
func styleJSON(s DeckStyle) string {
	b, err := json.Marshal(s.LookInput())
	if err != nil {
		return "{}"
	}
	return string(b)
}
