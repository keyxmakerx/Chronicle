package decks

import (
	"context"
	"errors"
)

// Verbs a copy route accepts.
const (
	verbDeal     = "deal"
	verbPutBack  = "put-back"
	verbAnother  = "another"
	verbTurn     = "turn"
	verbPlay     = "play"
	verbApprove  = "approve"
	verbTurnDown = "turn-down"
	verbTakeOff  = "take-off"
	verbDiscard  = "discard"
	verbTakeBack = "take-back"
)

var verbs = map[string]bool{
	verbDeal: true, verbPutBack: true, verbAnother: true, verbTurn: true, verbPlay: true, verbApprove: true,
	verbTurnDown: true, verbTakeOff: true, verbDiscard: true, verbTakeBack: true,
}

// fateOf is where a card goes after use: its own fate, or the deck's.
func fateOf(d Deck, c Card) string {
	if c.Fate != "" {
		return c.Fate
	}
	return d.Settings.Fate
}

// afterPlay is where a played copy lands. Fate "play" leaves it on the table
// in front of the hero that played it.
func afterPlay(fate, holder string) Place {
	switch fate {
	case FateBack:
		return Place{Zone: ZonePile}
	case FateGone:
		return Place{Zone: ZoneGone, FaceUp: true}
	case FatePlay:
		return Place{Zone: ZonePlay, HolderEntityID: holder, FaceUp: true}
	default:
		return Place{Zone: ZoneUsed, FaceUp: true}
	}
}

// afterLeaving is where a copy lands when it is taken off the table or
// discarded: fate "play" only ever holds a card on the table, so it ends in
// the used pile.
func afterLeaving(fate string) Place {
	if fate == FatePlay {
		fate = FateUsed
	}
	return afterPlay(fate, "")
}

// lookupCopy finds a copy the viewer may know about. A Director knows every
// copy. A player knows the copies in their own heroes' hands and play, the
// cards lying on the table, and the used pile of a party deck; any other
// copy answers like a missing one.
func (r *request) lookupCopy(d Deck, id string) (Copy, error) {
	if !validToken(id) {
		return Copy{}, errNotFound("card")
	}
	c, found, err := r.s.repo.GetCopy(r.ctx, r.cid, d.ID, id)
	if err != nil {
		return Copy{}, apperrorInternal(err)
	}
	if !found {
		return Copy{}, errNotFound("card")
	}
	if r.v.Director {
		return c, nil
	}
	own, err := r.owns(c.HolderEntityID)
	if err != nil {
		return Copy{}, err
	}
	switch {
	case (c.Zone == ZoneHand || c.Zone == ZonePlay) && own:
	case c.Zone == ZonePlay && c.HolderEntityID == "":
	case c.Zone == ZoneUsed && d.Kind == KindParty:
	default:
		return Copy{}, errNotFound("card")
	}
	return c, nil
}

func conflict(msg string) error { return errConflictMsg(msg) }

// applyMove stores a move and turns a lost race into a conflict.
func (s *service) applyMove(ctx context.Context, m Move, action string) error {
	m.Log.Action = action
	if err := s.repo.Move(ctx, m); err != nil {
		if errors.Is(err, errLostClaim) {
			return errMoved()
		}
		return apperrorInternal(err)
	}
	s.moved(m.CampaignID, m, action)
	return nil
}

func (r *request) result(d Deck, copyID string) (*ActionResult, error) {
	fresh, found, err := r.s.repo.GetDeck(r.ctx, r.cid, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("deck")
	}
	view, err := r.view(fresh)
	if err != nil {
		return nil, err
	}
	return &ActionResult{CopyID: copyID, Deck: view}, nil
}

// target resolves where a deal goes: a hero's hand (a page of this campaign)
// or the whole table.
func (s *service) target(ctx context.Context, campaignID string, d Deck, to, heroID string) (Place, error) {
	switch to {
	case "hand":
		if !validToken(heroID) {
			return Place{}, errInvalid("name the hero page to deal to")
		}
		if _, ok, err := s.heroes.InCampaign(ctx, campaignID, heroID); err != nil {
			return Place{}, apperrorInternal(err)
		} else if !ok {
			return Place{}, errInvalid("that hero page is not in this campaign")
		}
		return Place{Zone: ZoneHand, HolderEntityID: heroID, FaceUp: d.Settings.Arrives == "up"}, nil
	case "table":
		return Place{Zone: ZonePlay, FaceUp: true}, nil
	}
	return Place{}, errInvalid(`"to" must be "hand" or "table"`)
}

func (s *service) drawError(err error, named bool) error {
	switch {
	case errors.Is(err, errEmptyPile):
		if named {
			return errConflictMsg("No copy of that card is left in the pile.")
		}
		return errNothingLeft()
	case errors.Is(err, errNoDraws):
		return errForbidden("you have no draws left")
	case errors.Is(err, errShownTaken):
		return errConflictMsg("A card is already shown; deal it, put it back or draw another.")
	case errors.Is(err, errLostClaim):
		return errMoved()
	}
	return apperrorInternal(err)
}

func (s *service) Draw(ctx context.Context, campaignID, deckID string, v Viewer, in DrawInput) (*ActionResult, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, err
	}
	plan := DrawPlan{CampaignID: campaignID, DeckID: d.ID, Reshuffle: d.Settings.Empty == "reshuffle",
		Pick: s.pick, Log: LogLine{Action: ActDraw, ActorUserID: v.UserID}}
	if v.Director {
		if in.HeroID != "" {
			return nil, errInvalid("a Director's draw goes to the shown spot; deal it from there")
		}
		plan.To = Place{Zone: ZoneShown}
	} else {
		p, ok, err := r.chosen(d)
		if err != nil {
			return nil, err
		}
		if !ok || d.Settings.Draws != "players" {
			return nil, errForbidden("you can't draw from this deck")
		}
		if p.DrawsLeft != nil && *p.DrawsLeft <= 0 {
			return nil, errForbidden("you have no draws left")
		}
		own, err := r.owns(in.HeroID)
		if err != nil {
			return nil, err
		}
		if !own {
			return nil, errForbidden("you can only draw into your own hero's hand")
		}
		plan.To = Place{Zone: ZoneHand, HolderEntityID: in.HeroID, FaceUp: d.Settings.Arrives == "up"}
		plan.DealtBy, plan.SpendUserID = v.UserID, v.UserID
		plan.Log.HolderEntityID = in.HeroID
	}
	got, err := s.repo.Draw(ctx, plan)
	if err != nil {
		return nil, s.drawError(err, false)
	}
	s.moved(campaignID, Move{DeckID: d.ID, CopyID: got.ID, Log: plan.Log}, ActDraw)
	return r.result(d, got.ID)
}

func (s *service) DealCard(ctx context.Context, campaignID, deckID string, v Viewer, in DealCardInput) (*ActionResult, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	if !validToken(in.CardID) {
		return nil, errNotFound("card")
	}
	if _, found, err := s.repo.GetCard(ctx, campaignID, d.ID, in.CardID); err != nil {
		return nil, apperrorInternal(err)
	} else if !found {
		return nil, errNotFound("card")
	}
	to, err := s.target(ctx, campaignID, d, in.To, in.HeroID)
	if err != nil {
		return nil, err
	}
	plan := DrawPlan{CampaignID: campaignID, DeckID: d.ID, To: to, DealtBy: v.UserID, CardID: in.CardID, Pick: s.pick,
		Log: LogLine{Action: ActDeal, ActorUserID: v.UserID, HolderEntityID: to.HolderEntityID}}
	got, err := s.repo.Draw(ctx, plan)
	if err != nil {
		return nil, s.drawError(err, true)
	}
	s.moved(campaignID, Move{DeckID: d.ID, CopyID: got.ID, Log: plan.Log}, ActDeal)
	return r.result(d, got.ID)
}

func (s *service) Act(ctx context.Context, campaignID, deckID, copyID, verb string, v Viewer, in ActInput) (*ActionResult, error) {
	if !verbs[verb] {
		return nil, errNotFound("page")
	}
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, err
	}
	c, err := r.lookupCopy(d, copyID)
	if err != nil {
		return nil, err
	}
	card, found, err := s.repo.GetCard(ctx, campaignID, d.ID, c.CardID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("card")
	}

	here := Place{Zone: c.Zone, HolderEntityID: c.HolderEntityID, FaceUp: c.FaceUp}
	m := Move{CampaignID: campaignID, DeckID: d.ID, CopyID: c.ID, From: here, To: here,
		Log: LogLine{ActorUserID: v.UserID, HolderEntityID: c.HolderEntityID}}
	// mine is whether the copy is in front of or in the hand of a hero the
	// viewer owns; a Director may act on any.
	own, err := r.owns(c.HolderEntityID)
	if err != nil {
		return nil, err
	}
	mine := v.Director || own
	wrongZone := func(want string) error {
		return conflict("that card is not " + want)
	}

	var action string
	switch verb {
	case verbDeal:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		if c.Zone != ZoneShown {
			return nil, wrongZone("in the shown spot")
		}
		to, err := s.target(ctx, campaignID, d, in.To, in.HeroID)
		if err != nil {
			return nil, err
		}
		m.To, m.DealtBy, m.Asked, action = to, v.UserID, AskedClear, ActDeal
		m.Log.HolderEntityID = to.HolderEntityID

	case verbPutBack:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		if c.Zone != ZoneShown {
			return nil, wrongZone("in the shown spot")
		}
		m.To, action = Place{Zone: ZonePile}, ActPutBack

	case verbAnother:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		if c.Zone != ZoneShown {
			return nil, wrongZone("in the shown spot")
		}
		plan := DrawPlan{CampaignID: campaignID, DeckID: d.ID, To: Place{Zone: ZoneShown}, PutBackShown: true,
			Reshuffle: d.Settings.Empty == "reshuffle", Pick: s.pick,
			Log: LogLine{Action: ActAnother, ActorUserID: v.UserID}}
		got, err := s.repo.Draw(ctx, plan)
		if err != nil {
			return nil, s.drawError(err, false)
		}
		s.moved(campaignID, Move{DeckID: d.ID, CopyID: got.ID, Log: plan.Log}, ActAnother)
		return r.result(d, got.ID)

	case verbTurn:
		if c.Zone != ZoneHand {
			return nil, wrongZone("in a hand")
		}
		if !mine {
			return nil, errForbidden("you can only turn a card in your own hero's hand")
		}
		if !v.Director && c.FaceUp {
			// A player only ever turns a card up. A repeat is refused before
			// anything is written, so flipping cannot fill the log.
			return nil, conflict("that card is already face up")
		}
		m.To.FaceUp, m.Turned, action = !c.FaceUp, true, ActTurn

	case verbPlay:
		if c.Zone != ZoneHand {
			return nil, wrongZone("in a hand")
		}
		if !mine {
			return nil, errForbidden("you can only play a card from your own hero's hand")
		}
		if !v.Director && !c.FaceUp {
			// Playing a face-down card would show by what happens next (a wait
			// for a Director, where it lands) what the card is.
			return nil, conflict("turn the card over before playing it")
		}
		if card.AskFirst && !v.Director {
			if c.AskedAt != nil {
				return nil, conflict("that card is already waiting for a Director's answer")
			}
			m.Asked, action = AskedMark, ActAsk
			break
		}
		m.To, m.Asked, action = afterPlay(fateOf(d, card), c.HolderEntityID), AskedClear, ActPlay

	case verbApprove, verbTurnDown:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		if c.Zone != ZoneHand {
			return nil, wrongZone("in a hand")
		}
		if c.AskedAt == nil {
			return nil, conflict("that card is not waiting for an answer")
		}
		m.ExpectAsked, m.Asked = true, AskedClear
		if verb == verbApprove {
			m.To, action = afterPlay(fateOf(d, card), c.HolderEntityID), ActApprove
		} else {
			action = ActTurnDown
		}

	case verbTakeOff:
		if c.Zone != ZonePlay {
			return nil, wrongZone("in play")
		}
		// A card lying in front of nobody belongs to the whole table, which
		// only a Director clears.
		if !v.Director && !own {
			return nil, errForbidden("you can only take off a card in front of your own hero")
		}
		m.To, action = afterLeaving(fateOf(d, card)), ActTakeOff

	case verbDiscard:
		if c.Zone != ZoneHand {
			return nil, wrongZone("in a hand")
		}
		if !v.Director && !(own && d.Settings.Discard) {
			return nil, errForbidden("you can't discard from this deck")
		}
		if !v.Director && !c.FaceUp {
			// Where a discarded card lands depends on its fate, so discarding
			// one unseen would tell the player something about it.
			return nil, conflict("turn the card over before discarding it")
		}
		m.To, m.Asked, action = afterLeaving(fateOf(d, card)), AskedClear, ActDiscard

	case verbTakeBack:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		if c.Zone != ZoneHand && c.Zone != ZonePlay {
			return nil, wrongZone("in a hand or in play")
		}
		m.To, m.Asked, action = Place{Zone: ZonePile}, AskedClear, ActTakeBack
		m.Log.Seen = c.FaceUp
	}

	if err := s.applyMove(ctx, m, action); err != nil {
		return nil, err
	}
	// A copy back in the pile has a new id, so the old one names nothing.
	if m.To.Zone == ZonePile {
		return r.result(d, "")
	}
	return r.result(d, c.ID)
}

func (s *service) Shuffle(ctx context.Context, campaignID, deckID string, v Viewer) (*ActionResult, error) {
	return s.bulk(ctx, campaignID, deckID, v, ActShuffle)
}

func (s *service) Gather(ctx context.Context, campaignID, deckID string, v Viewer) (*ActionResult, error) {
	return s.bulk(ctx, campaignID, deckID, v, ActGather)
}

func (s *service) bulk(ctx context.Context, campaignID, deckID string, v Viewer, action string) (*ActionResult, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	log := LogLine{Action: action, ActorUserID: v.UserID}
	if action == ActShuffle {
		_, err = s.repo.Shuffle(ctx, campaignID, d.ID, log)
	} else {
		_, err = s.repo.Gather(ctx, campaignID, d.ID, log)
	}
	if err != nil {
		if errors.Is(err, errLostClaim) {
			return nil, errNotFound("deck")
		}
		return nil, apperrorInternal(err)
	}
	s.moved(campaignID, Move{DeckID: d.ID, Log: log}, action)
	return r.result(d, "")
}
