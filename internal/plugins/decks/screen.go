package decks

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The Director's Card decks section (on the DM Screen) and the draw overlay's
// reads. A Director's reads are built from Director data; a player's draw view
// and fresh cards are built from the player types only (PlayerDeck, PlayerCopy,
// Face), never from a Director type with parts left out, so what they can
// draw is what a player may be sent.

const (
	// MaxScreenPicks is how many decks a Director may choose for the section.
	MaxScreenPicks = 12
	// screenRecent is how many decks the section shows before a Director has
	// chosen any: the ones with the newest moves.
	screenRecent = 5
	// freshWindow is how long a dealt card may still pop up for its player.
	freshWindow = 10 * time.Minute
	// freshMax is how many dealt cards one check brings.
	freshMax = 3
	// freshSweep is how many older dealt cards one pop-up marks seen with it.
	freshSweep = 100
	// fanMax is how many backs the draw overlay's fan shows.
	fanMax = 7
)

// screenKindWords says a deck's kind in the plural, the way a row names who it is for.
func screenKindWords(kind string) string {
	switch kind {
	case KindHero:
		return "Heroes"
	case KindParty:
		return "Party"
	}
	return "Events"
}

// deckSub is "Events · 9 left"; with no known pile it is the kind alone.
func deckSub(kind string, pile *int) string {
	if pile == nil {
		return screenKindWords(kind)
	}
	return screenKindWords(kind) + " · " + strconv.Itoa(*pile) + " left"
}

// shownFullWords is the refusal for a draw that would pass the shown limit,
// the same words the draw itself answers with.
func shownFullWords() string {
	return fmt.Sprintf("At most %d cards can be shown at once; deal some or put them back first.", MaxShown)
}

// ScreenShown is a card a Director has drawn and not dealt, as the section
// draws it.
type ScreenShown struct {
	CopyID string
	Face   Face
}

// ScreenDeck is one deck row of the section.
type ScreenDeck struct {
	ID    string
	Name  string
	Kind  string
	Look  Look
	Pile  int
	Drawn []ScreenShown
}

// Sub is the row's second line: "Events · 9 left · 2 drawn", with no drawn
// part when nothing is drawn.
func (d ScreenDeck) Sub() string {
	p := d.Pile
	s := deckSub(d.Kind, &p)
	if n := len(d.Drawn); n > 0 {
		s += " · " + strconv.Itoa(n) + " drawn"
	}
	return s
}

// ScreenChoice is one line of the "Choose decks" list.
type ScreenChoice struct {
	ID      string
	Name    string
	Checked bool
}

// ScreenView is the Card decks section, for a Director.
type ScreenView struct {
	CampaignID string
	// Shown are the decks the section lists: the Director's picks in their
	// order, or the decks with the newest moves.
	Shown []ScreenDeck
	// Rest are the other decks, folded behind "All decks".
	Rest []ScreenDeck
	// Total is every deck of the campaign.
	Total   int
	Choices []ScreenChoice
	// Heroes are the party's heroes, for "Deal to…".
	Heroes []Hero
}

// Summary is the section's one line on the screen's rail: "3 decks · 2 drawn".
func (v *ScreenView) Summary() string {
	if v.Total == 0 {
		return "No decks yet"
	}
	drawn := 0
	for _, d := range v.Shown {
		drawn += len(d.Drawn)
	}
	s := countWords(len(v.Shown), "deck", "decks")
	if drawn > 0 {
		s += " · " + strconv.Itoa(drawn) + " drawn"
	}
	return s
}

// Screen reads the section for a Director. Anyone else gets the same answer as
// a deck that does not exist.
func (s *service) Screen(ctx context.Context, campaignID string, v Viewer) (*ScreenView, error) {
	if !v.Director || s.screen == nil {
		return nil, errNotFound("deck")
	}
	all, err := s.repo.ListDecks(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	picks, err := s.screen.Picks(ctx, campaignID, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	byID := make(map[string]Deck, len(all))
	for _, d := range all {
		byID[d.ID] = d
	}
	var order []string
	for _, id := range picks {
		if _, ok := byID[id]; ok {
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		moves, err := s.screen.LastMoves(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		ranked := make([]Deck, len(all))
		copy(ranked, all)
		// Newest move first; decks never moved keep the list's own order.
		sort.SliceStable(ranked, func(i, j int) bool { return moves[ranked[i].ID] > moves[ranked[j].ID] })
		for i := 0; i < len(ranked) && i < screenRecent; i++ {
			order = append(order, ranked[i].ID)
		}
	}
	chosen := make(map[string]bool, len(order))
	for _, id := range order {
		chosen[id] = true
	}
	counts, err := s.repo.ZoneCounts(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &ScreenView{CampaignID: campaignID, Total: len(all)}
	row := func(d Deck) (ScreenDeck, error) {
		sd := ScreenDeck{ID: d.ID, Name: d.Name, Kind: d.Kind, Look: d.Look, Pile: counts[d.ID].Pile}
		if counts[d.ID].Shown > 0 {
			drawn, err := s.shownOf(ctx, campaignID, d)
			if err != nil {
				return sd, err
			}
			sd.Drawn = drawn
		}
		return sd, nil
	}
	for _, id := range order {
		sd, err := row(byID[id])
		if err != nil {
			return nil, err
		}
		out.Shown = append(out.Shown, sd)
	}
	for _, d := range all {
		out.Choices = append(out.Choices, ScreenChoice{ID: d.ID, Name: d.Name, Checked: chosen[d.ID]})
		if chosen[d.ID] {
			continue
		}
		sd, err := row(d)
		if err != nil {
			return nil, err
		}
		out.Rest = append(out.Rest, sd)
	}
	heroes, err := s.heroes.Party(ctx, campaignID, v)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out.Heroes = heroes
	return out, nil
}

// shownOf lists a deck's shown copies, oldest drawn first, each with its face.
func (s *service) shownOf(ctx context.Context, campaignID string, d Deck) ([]ScreenShown, error) {
	copies, err := s.repo.ListCopies(ctx, campaignID, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	cards, err := s.repo.ListCards(ctx, campaignID, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	byCard := make(map[string]Card, len(cards))
	for _, c := range cards {
		byCard[c.ID] = c
	}
	var shown []Copy
	for _, c := range copies {
		if c.Zone == ZoneShown {
			shown = append(shown, c)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool {
		if !shown[i].MovedAt.Equal(shown[j].MovedAt) {
			return shown[i].MovedAt.Before(shown[j].MovedAt)
		}
		return shown[i].ID < shown[j].ID
	})
	out := make([]ScreenShown, 0, len(shown))
	for _, c := range shown {
		card, ok := byCard[c.CardID]
		if !ok {
			continue
		}
		out = append(out, ScreenShown{CopyID: c.ID, Face: faceOf(card)})
	}
	return out, nil
}

// SetScreenPicks stores a Director's chosen decks. An id that is not a deck of
// this campaign is refused whole; repeats keep their first place; an empty list
// goes back to the recent decks.
func (s *service) SetScreenPicks(ctx context.Context, campaignID string, v Viewer, deckIDs []string) error {
	if !v.Director || s.screen == nil {
		return errNotFound("deck")
	}
	all, err := s.repo.ListDecks(ctx, campaignID)
	if err != nil {
		return apperrorInternal(err)
	}
	have := make(map[string]bool, len(all))
	for _, d := range all {
		have[d.ID] = true
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(deckIDs))
	for _, id := range deckIDs {
		if !validToken(id) || !have[id] {
			return apperror.NewBadRequest("one of those decks is not in this campaign")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) > MaxScreenPicks {
		return apperror.NewBadRequest(fmt.Sprintf("choose at most %d decks", MaxScreenPicks))
	}
	if err := s.screen.SetPicks(ctx, campaignID, v.UserID, clean); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

// DrawShown is a card a Director has drawn, for the overlay's tray.
type DrawShown struct {
	CopyID string
	Face   Face
}

// DrawView is what the draw overlay needs to open on a deck, as its viewer may
// know it. A player's is built from the player deck, so Pile is nil when the
// deck tells players nothing of its count, and Shown is always empty.
type DrawView struct {
	DeckID string
	Name   string
	// Sub is "Events · 9 left".
	Sub   string
	Shape string
	Turn  string
	Look  Look
	// Pile is the count the viewer may know.
	Pile *int
	// Fan is how many backs the overlay shows: the pile's count up to seven, or
	// seven when the count is not the viewer's to know.
	Fan     int
	CanDraw bool
	// Refusal says why a draw would be refused ("" when it would not).
	Refusal string
	// DrawsLeft is a player's remaining draws; nil is no limit or a Director.
	DrawsLeft *int
	// HeroID is the hero a player's draw goes to; "" for a Director.
	HeroID string
	Shown  []DrawShown
}

func fanOf(pile *int) int {
	if pile == nil || *pile > fanMax {
		return fanMax
	}
	return *pile
}

// DrawView reads one deck for the overlay. A Director reads any deck; a player
// reads only a deck open to them to draw from, and every other deck is a 404
// worded like a missing one.
func (s *service) DrawView(ctx context.Context, campaignID, deckID string, v Viewer) (*DrawView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, sameNotFound(err)
	}
	if v.Director {
		return r.directorDrawView(d)
	}
	return r.playerDrawView(d)
}

func (r *request) directorDrawView(d Deck) (*DrawView, error) {
	copies, err := r.s.repo.ListCopies(r.ctx, r.cid, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	var counts ZoneCounts
	for _, c := range copies {
		counts.add(c.Zone, 1)
	}
	pile := counts.Pile
	out := &DrawView{DeckID: d.ID, Name: d.Name, Sub: deckSub(d.Kind, &pile), Shape: d.Look.Shape, Turn: d.Look.Turn,
		Look: d.Look, Pile: &pile, Fan: fanOf(&pile), Shown: []DrawShown{}}
	// A draw from an empty pile that reshuffles brings the used cards back.
	empty := pile == 0 && (d.Settings.Empty != "reshuffle" || counts.Used == 0)
	switch {
	case counts.Shown >= MaxShown:
		out.Refusal = shownFullWords()
	case empty:
		out.Refusal = nothingLeftWords
	}
	out.CanDraw = out.Refusal == ""
	if counts.Shown > 0 {
		drawn, err := r.s.shownOf(r.ctx, r.cid, d)
		if err != nil {
			return nil, err
		}
		for _, sh := range drawn {
			out.Shown = append(out.Shown, DrawShown(sh))
		}
	}
	return out, nil
}

func (r *request) playerDrawView(d Deck) (*DrawView, error) {
	pd, err := r.playerDeck(d)
	if err != nil {
		return nil, err
	}
	owned, err := r.ownedHeroes()
	if err != nil {
		return nil, err
	}
	hero := ""
	invited := false
	if pd.InvitedHeroID != "" {
		for _, h := range owned {
			if h.ID == pd.InvitedHeroID {
				hero, invited = h.ID, true
			}
		}
	}
	if !invited {
		if !pd.DrawsOpen {
			return nil, errNotFound("page")
		}
		if len(owned) > 0 {
			hero = owned[0].ID
		}
	}
	out := &DrawView{DeckID: d.ID, Name: d.Name, Sub: deckSub(d.Kind, pd.Pile), Shape: pd.Look.Shape, Turn: pd.Look.Turn,
		Look: pd.Look, Pile: pd.Pile, Fan: fanOf(pd.Pile), HeroID: hero, Shown: []DrawShown{}}
	if !invited {
		out.DrawsLeft = pd.DrawsLeft
	}
	switch {
	case hero == "":
		out.Refusal = "You have no hero to draw into yet."
	case !invited && !pd.CanDraw:
		out.Refusal = "You have no draws left."
	case pd.Pile != nil && *pd.Pile == 0 && d.Settings.Empty != "reshuffle":
		out.Refusal = nothingLeftWords
	}
	out.CanDraw = out.Refusal == ""
	return out, nil
}

// ShownFace reads a shown copy's face for a Director, in the form a turn-over
// brings in. Anyone else, and any copy that is not in the shown spot, is a 404
// worded like a missing deck.
func (s *service) ShownFace(ctx context.Context, campaignID, deckID, copyID string, v Viewer) (*OpenedCard, error) {
	if !v.Director {
		return nil, errNotFound("deck")
	}
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, errNotFound("deck")
	}
	c, err := r.lookupCopy(d, copyID)
	if err != nil || c.Zone != ZoneShown {
		return nil, errNotFound("deck")
	}
	card, found, err := s.repo.GetCard(ctx, campaignID, d.ID, c.CardID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("deck")
	}
	one := []Card{card}
	s.printLines(ctx, campaignID, d, v, one)
	f := faceOf(one[0])
	return &OpenedCard{DeckID: d.ID, DeckName: d.Name, Look: d.Look,
		Copy: PlayerCopy{ID: c.ID, DeckID: c.DeckID, FaceUp: true, Face: &f}}, nil
}

// FreshCard is a face-down card dealt to one of the viewer's heroes that has
// not yet been put in front of them. It holds the deck's look and the copy's
// id, never a card.
type FreshCard struct {
	DeckID   string
	CopyID   string
	DeckName string
	Sub      string
	Look     Look
}

// FreshCards lists the face-down cards dealt to a hero the viewer plays, by
// someone else, in the last ten minutes and not yet marked seen by the viewer:
// newest first, at most three. A Director is never shown any.
func (s *service) FreshCards(ctx context.Context, campaignID string, v Viewer) ([]FreshCard, error) {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() {
		return nil, errNotFound("hand")
	}
	out := []FreshCard{}
	if v.Director || s.screen == nil {
		return out, nil
	}
	rows, err := s.freshRows(r, freshMax)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		d, err := r.deck(row.DeckID)
		if err != nil {
			continue
		}
		pd, err := r.playerDeck(d)
		if err != nil {
			return nil, err
		}
		out = append(out, FreshCard{DeckID: d.ID, CopyID: row.CopyID, DeckName: d.Name, Sub: deckSub(d.Kind, pd.Pile), Look: pd.Look})
	}
	return out, nil
}

// MarkFreshSeen records that the viewer was shown a card dealt to their hero.
// A copy that is not in the hand or play of one of their heroes is a 404.
func (s *service) MarkFreshSeen(ctx context.Context, campaignID, copyID string, v Viewer) error {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() || v.Director || s.screen == nil || !validToken(copyID) {
		return errNotFound("hand")
	}
	held, err := r.heldCopies()
	if err != nil {
		return err
	}
	found := false
	for _, c := range held {
		if c.ID == copyID {
			found = true
			break
		}
	}
	if !found {
		return errNotFound("hand")
	}
	// The pop-up stands for every card dealt up to the one it showed, so a
	// deal of several pops up once: the older ones are marked seen with it and
	// wait in the Hand. A card dealt after it still pops up on its own.
	rows, err := s.freshRows(r, freshSweep)
	if err != nil {
		return err
	}
	mark := []string{copyID}
	for i, row := range rows {
		if row.CopyID == copyID {
			for _, older := range rows[i+1:] {
				mark = append(mark, older.CopyID)
			}
			break
		}
	}
	for _, id := range mark {
		if err := s.screen.MarkSeen(ctx, campaignID, v.UserID, id); err != nil {
			return apperrorInternal(err)
		}
	}
	return nil
}

// freshRows lists, newest first, the unseen face-down cards dealt to a hero
// the viewer plays.
func (s *service) freshRows(r *request, limit int) ([]FreshRow, error) {
	owned, err := r.ownedHeroes()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(owned))
	for _, h := range owned {
		ids = append(ids, h.ID)
	}
	rows, err := s.screen.Fresh(r.ctx, r.cid, r.v.UserID, ids, freshWindow, limit)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	return rows, nil
}
