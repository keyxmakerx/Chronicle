package decks

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The player's screens: the Hand leaf, the strip on a hero's page, an opened
// card and the one request that turns a card over. Every type here is built
// from the player types (PlayerDeck, PlayerCopy, Face, TableCard) or from a
// holder's own view of a copy, never from a Director type with parts hidden by
// a template, so what a fragment can draw is what a player may be sent.

// HandCard is one copy drawn in a hand: the copy as its viewer may know it,
// and the deck that gives it a look and a name.
type HandCard struct {
	DeckID   string
	DeckName string
	Look     Look
	Copy     PlayerCopy
}

// HandSpot is one hand: a hero's, or a party deck's own. Hand and Play are
// the cards held and the cards lying in front of the hero.
type HandSpot struct {
	// HeroID is "" for a party deck's hand.
	HeroID string
	// Name is the hero's name, or the party deck's.
	Name string
	// Acts is whether the viewer may turn, play and show these cards.
	Acts bool
	Hand []HandCard
	Play []HandCard
}

// HandParty is a party deck as every member sees it.
type HandParty struct {
	DeckID   string
	DeckName string
	Look     Look
	Pile     *int
	Hand     []HandCard
	Used     []HandCard
}

// HandOther is what one deck lets the viewer see of another hero's hand.
type HandOther struct {
	HeroID   string
	HeroName string
	DeckID   string
	DeckName string
	Look     Look
	Count    int
	Faces    []Face
}

// HandDraw is a deck opened to the viewer to draw from, with what the deck
// lets them know of its pile.
type HandDraw struct {
	DeckID    string
	DeckName  string
	Look      Look
	Pile      *int
	Kinds     []KindCount
	DrawsLeft *int
	CanDraw   bool
	// Heroes are the viewer's own heroes a draw may go to.
	Heroes []Hero
	// Invited is set only for the player the Director invited to draw: the draw
	// goes into the invited hero's hand and spends no draw, so the count of draws
	// left is not shown.
	Invited bool
}

// HandView is everything the Hand leaf draws.
type HandView struct {
	Mine   []HandSpot
	Party  []HandParty
	Table  []TableDeck
	Others []HandOther
	Draws  []HandDraw
}

// Empty says the viewer holds nothing, sees nothing on the table or in other
// hands, and has no deck open to draw from; a party deck with no hand is not a card.
func (h *HandView) Empty() bool {
	for _, p := range h.Party {
		if len(p.Hand) > 0 || len(p.Used) > 0 {
			return false
		}
	}
	return len(h.Mine) == 0 && len(h.Table) == 0 && len(h.Others) == 0 && len(h.Draws) == 0
}

func handCards(d PlayerDeck, list []PlayerCopy) []HandCard {
	out := make([]HandCard, 0, len(list))
	for _, c := range list {
		out = append(out, HandCard{DeckID: d.ID, DeckName: d.Name, Look: d.Look, Copy: c})
	}
	return out
}

// Hand reads what the Hand leaf draws for the viewer, from the same player
// deck every other read builds. A Director reads it as a player does: their
// own heroes' hands and what any player is let see.
func (s *service) Hand(ctx context.Context, campaignID string, v Viewer) (*HandView, error) {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() {
		return nil, errNotFound("hand")
	}
	all, err := s.repo.ListDecks(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	owned, err := r.ownedHeroes()
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, h := range owned {
		names[h.ID] = h.Name
	}
	spots := map[string]*HandSpot{}
	var order []string
	out := &HandView{Mine: []HandSpot{}, Party: []HandParty{}, Table: []TableDeck{}, Others: []HandOther{}, Draws: []HandDraw{}}
	for _, d := range all {
		ok, err := r.listed(d)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		pd, err := r.playerDeck(d)
		if err != nil {
			return nil, err
		}
		for _, h := range pd.Hands {
			if len(h.Hand) == 0 && len(h.Play) == 0 {
				continue
			}
			sp := spots[h.HeroID]
			if sp == nil {
				sp = &HandSpot{HeroID: h.HeroID, Name: names[h.HeroID], Acts: true}
				spots[h.HeroID] = sp
				order = append(order, h.HeroID)
			}
			sp.Hand = append(sp.Hand, handCards(pd, h.Hand)...)
			sp.Play = append(sp.Play, handCards(pd, h.Play)...)
		}
		if d.Kind == KindParty {
			out.Party = append(out.Party, HandParty{DeckID: d.ID, DeckName: d.Name, Look: pd.Look, Pile: pd.Pile,
				Hand: handCards(pd, pd.PartyHand), Used: handCards(pd, pd.Used)})
		}
		for _, o := range pd.Others {
			name := ""
			if h, ok, err := s.heroes.InCampaign(ctx, campaignID, o.HeroID); err != nil {
				return nil, apperrorInternal(err)
			} else if ok {
				name = h.Name
			}
			out.Others = append(out.Others, HandOther{HeroID: o.HeroID, HeroName: name, DeckID: d.ID, DeckName: d.Name,
				Look: pd.Look, Count: o.Count, Faces: o.Faces})
		}
		// An invitation is the one thing a player is told of a moment. It names the
		// hero the card goes to, who must be one of the viewer's own.
		var invited []Hero
		if pd.InvitedHeroID != "" {
			for _, h := range owned {
				if h.ID == pd.InvitedHeroID {
					invited = append(invited, h)
				}
			}
		}
		switch {
		case len(invited) > 0:
			out.Draws = append(out.Draws, HandDraw{DeckID: d.ID, DeckName: d.Name, Look: pd.Look, Pile: pd.Pile, Kinds: pd.Kinds,
				CanDraw: true, Heroes: invited, Invited: true})
		case pd.DrawsOpen:
			out.Draws = append(out.Draws, HandDraw{DeckID: d.ID, DeckName: d.Name, Look: pd.Look, Pile: pd.Pile, Kinds: pd.Kinds,
				DrawsLeft: pd.DrawsLeft, CanDraw: pd.CanDraw, Heroes: owned})
		}
	}
	for _, id := range order {
		out.Mine = append(out.Mine, *spots[id])
	}
	sort.SliceStable(out.Mine, func(i, j int) bool { return out.Mine[i].Name < out.Mine[j].Name })
	t, err := s.Table(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	out.Table = t.Decks
	return out, nil
}

// HeroHandView is the strip on a hero's page.
type HeroHandView struct {
	HeroID string
	Name   string
	// Own is the hero's own hand, for the hero's player and for Directors.
	Own *HandSpot
	// Others is what the viewer may see of the hero's hand, for anyone else.
	Others []HandOther
}

// Empty says the strip has nothing to draw.
func (h *HeroHandView) Empty() bool {
	if h.Own != nil && (len(h.Own.Hand) > 0 || len(h.Own.Play) > 0) {
		return false
	}
	return len(h.Others) == 0
}

// Shown is how many cards the strip draws for this viewer: the cards held and
// in play for a holder or Director, and for anyone else what each deck lets
// them see, a face or a back apiece. It counts what the viewer is shown and
// nothing more, so a caller can offer the strip without reading its fields.
func (h *HeroHandView) Shown() int {
	if h == nil {
		return 0
	}
	n := 0
	if h.Own != nil {
		n += len(h.Own.Hand) + len(h.Own.Play)
	}
	for _, o := range h.Others {
		// The strip draws every face and then backs up to Count; a Count below
		// the faces cannot hide a face it draws.
		n += max(o.Count, len(o.Faces))
	}
	return n
}

// HeroHand reads the strip for one hero page. The hero's player reads what the
// Hand leaf shows for that hero. A Director reads the hand as its holder does
// (a face-down copy is its back) and acts on it in the workshop, not here.
// Anyone else reads only
// what each deck lets them see of it, and only when they may open the page; a
// page they may not open, or one that is not a hero of this campaign, is a 404
// like a missing one.
func (s *service) HeroHand(ctx context.Context, campaignID, heroID string, v Viewer) (*HeroHandView, error) {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() || !validToken(heroID) {
		return nil, errNotFound("page")
	}
	hero, ok, err := s.heroes.InCampaign(ctx, campaignID, heroID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("page")
	}
	if !v.Director {
		may, err := s.heroes.Viewable(ctx, campaignID, heroID, v)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		if !may {
			return nil, errNotFound("page")
		}
	}
	out := &HeroHandView{HeroID: heroID, Name: hero.Name, Others: []HandOther{}}
	own, err := r.owns(heroID)
	if err != nil {
		return nil, err
	}
	switch {
	case own:
		h, err := s.Hand(ctx, campaignID, v)
		if err != nil {
			return nil, err
		}
		out.Own = &HandSpot{HeroID: heroID, Name: hero.Name, Acts: true}
		for _, sp := range h.Mine {
			if sp.HeroID == heroID {
				out.Own = &sp
			}
		}
	case v.Director:
		sp, err := s.holderSpot(ctx, campaignID, hero, r)
		if err != nil {
			return nil, err
		}
		out.Own = sp
	default:
		h, err := s.Hand(ctx, campaignID, v)
		if err != nil {
			return nil, err
		}
		for _, o := range h.Others {
			if o.HeroID == heroID {
				out.Others = append(out.Others, o)
			}
		}
	}
	return out, nil
}

// holderSpot is a hero's hand as its holder sees it, for a Director reading
// a hero page: faces for face-up copies, a back for the rest. It is to read:
// turning a card over is its holder's to do, and a Director who must act on a
// hand does it in the workshop, where every move is deliberate.
func (s *service) holderSpot(ctx context.Context, campaignID string, hero Hero, r *request) (*HandSpot, error) {
	held, err := s.repo.ListHeldBy(ctx, campaignID, []string{hero.ID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	sort.SliceStable(held, func(i, j int) bool {
		if !held[i].MovedAt.Equal(held[j].MovedAt) {
			return held[i].MovedAt.Before(held[j].MovedAt)
		}
		return held[i].ID < held[j].ID
	})
	sp := &HandSpot{HeroID: hero.ID, Name: hero.Name}
	decks := map[string]Deck{}
	cards := map[string]map[string]Card{}
	for _, c := range held {
		d, ok := decks[c.DeckID]
		if !ok {
			var found bool
			if d, found, err = s.repo.GetDeck(ctx, campaignID, c.DeckID); err != nil {
				return nil, apperrorInternal(err)
			} else if !found {
				continue
			}
			decks[c.DeckID] = d
			list, err := s.repo.ListCards(ctx, campaignID, c.DeckID)
			if err != nil {
				return nil, apperrorInternal(err)
			}
			s.printLines(ctx, campaignID, d, r.v, list)
			cards[c.DeckID] = make(map[string]Card, len(list))
			for _, k := range list {
				cards[c.DeckID][k.ID] = k
			}
		}
		hc := HandCard{DeckID: d.ID, DeckName: d.Name, Look: d.Look, Copy: playerCopy(c, cards[c.DeckID])}
		if c.Zone == ZonePlay {
			sp.Play = append(sp.Play, hc)
		} else {
			sp.Hand = append(sp.Hand, hc)
		}
	}
	return sp, nil
}

// OpenedCard is one face-up card read at full size, with what its viewer may
// do with it.
type OpenedCard struct {
	DeckID   string
	DeckName string
	Look     Look
	Copy     PlayerCopy
	// InHand is a copy in a hand (not on the table or in play).
	InHand bool
	// CanPlay, CanShow and CanDiscard say which controls are on. They follow
	// the rules Act applies, which still decide every request.
	CanPlay    bool
	CanShow    bool
	CanDiscard bool
	// Shown is a card its holder has shown to the table; it cannot be taken back.
	Shown bool
}

// sameNotFound makes every "not there" answer of a player's fragment read the
// same, so a deck, copy or card that is hidden cannot be told from one that is
// missing by the words of the refusal.
func sameNotFound(err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
		return errNotFound("page")
	}
	return err
}

// Open reads a face-up copy the viewer may read: one in their own hero's hand
// or play, in a party deck's hand or used pile, or on the table. A face-down
// copy has no face to open and says so; any other copy is a 404 like a
// missing one.
func (s *service) Open(ctx context.Context, campaignID, deckID, copyID string, v Viewer) (*OpenedCard, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		if card, onTable, terr := r.openOnTable(deckID, copyID); terr != nil || onTable {
			return card, terr
		}
		return nil, sameNotFound(err)
	}
	c, err := r.lookupCopy(d, copyID)
	if err != nil {
		return nil, sameNotFound(err)
	}
	if !c.FaceUp {
		return nil, errConflictMsg("that card is face down; turn it over first")
	}
	card, err := r.opened(d, c)
	return card, sameNotFound(err)
}

// openOnTable reads a card lying on the table from a deck the viewer may not
// list. The table is every member's to read (Table sends the same face), so
// such a card opens to read and nothing more; anything else of that deck stays
// as hidden as the deck is.
func (r *request) openOnTable(deckID, copyID string) (*OpenedCard, bool, error) {
	if !r.member() || !validToken(deckID) || !validToken(copyID) {
		return nil, false, nil
	}
	d, found, err := r.s.repo.GetDeck(r.ctx, r.cid, deckID)
	if err != nil {
		return nil, false, apperrorInternal(err)
	}
	if !found {
		return nil, false, nil
	}
	c, found, err := r.s.repo.GetCopy(r.ctx, r.cid, d.ID, copyID)
	if err != nil {
		return nil, false, apperrorInternal(err)
	}
	if !found || c.Zone != ZonePlay || c.HolderEntityID != "" || !c.FaceUp {
		return nil, false, nil
	}
	card, found, err := r.s.repo.GetCard(r.ctx, r.cid, d.ID, c.CardID)
	if err != nil {
		return nil, false, apperrorInternal(err)
	}
	if !found {
		return nil, false, nil
	}
	look := d.Look
	look.Kinds = kindsOf(d.Look, map[string]bool{card.KindKey: true})
	one := []Card{card}
	r.s.printLines(r.ctx, r.cid, d, r.v, one)
	f := faceOf(one[0])
	return &OpenedCard{DeckID: d.ID, DeckName: d.Name, Look: look,
		Copy: PlayerCopy{ID: c.ID, DeckID: c.DeckID, FaceUp: true, Face: &f}}, true, nil
}

func (r *request) opened(d Deck, c Copy) (*OpenedCard, error) {
	card, found, err := r.s.repo.GetCard(r.ctx, r.cid, d.ID, c.CardID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("card")
	}
	one := []Card{card}
	r.s.printLines(r.ctx, r.cid, d, r.v, one)
	pc := playerCopy(c, map[string]Card{card.ID: one[0]})
	out := &OpenedCard{DeckID: d.ID, DeckName: d.Name, Look: d.Look, Copy: pc, InHand: c.Zone == ZoneHand}
	own, err := r.owns(c.HolderEntityID)
	if err != nil {
		return nil, err
	}
	party := inPartyHand(d, c)
	mine := (own || party) && c.Zone == ZoneHand
	out.CanPlay = mine && c.AskedAt == nil
	// Showing means something only where others read faces.
	out.CanShow = mine && !party && !c.Open && d.Settings.OthersSee == "face"
	out.Shown = c.Open
	out.CanDiscard = mine && d.Settings.Discard
	return out, nil
}

// TurnFace turns one face-down copy over and returns it, face and all, in one
// step: the one request that both performs the turn and brings the face. A
// copy that is already face up answers 409 and brings nothing, so a repeat
// never flips it back or hands out a face twice; one the viewer may not turn
// is a 404 like a missing one. A face is returned only for a copy that is
// face up when the move is done.
func (s *service) TurnFace(ctx context.Context, campaignID, deckID, copyID string, v Viewer) (*OpenedCard, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, sameNotFound(err)
	}
	c, err := r.lookupCopy(d, copyID)
	if err != nil {
		return nil, sameNotFound(err)
	}
	if c.FaceUp {
		return nil, conflict("that card is already face up")
	}
	if _, err := s.Act(ctx, campaignID, deckID, copyID, verbTurn, v, ActInput{}); err != nil {
		return nil, sameNotFound(err)
	}
	fresh, found, err := s.repo.GetCopy(ctx, campaignID, d.ID, c.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found || !fresh.FaceUp {
		return nil, errMoved()
	}
	card, err := r.opened(d, fresh)
	return card, sameNotFound(err)
}
