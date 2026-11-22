package decks

import (
	"context"
	"errors"
	"fmt"
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
	verbShow     = "show"
	verbKeep     = "keep"
)

var verbs = map[string]bool{
	verbDeal: true, verbPutBack: true, verbAnother: true, verbTurn: true, verbPlay: true, verbApprove: true,
	verbTurnDown: true, verbTakeOff: true, verbDiscard: true, verbTakeBack: true,
	verbShow: true, verbKeep: true,
}

// inPartyHand is a copy in the party deck's own hand: in a hand, held by no
// hero, on a party deck. Every member who may see the deck may handle it.
func inPartyHand(d Deck, c Copy) bool {
	return d.Kind == KindParty && c.Zone == ZoneHand && c.HolderEntityID == ""
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
// copy answers like a missing one. The party deck's own hand is every
// member's.
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
	case inPartyHand(d, c):
	default:
		return Copy{}, errNotFound("card")
	}
	return c, nil
}

func conflict(msg string) error { return errConflictMsg(msg) }

// applyMove stores a move and turns a lost race into a conflict.
func (s *service) applyMove(ctx context.Context, d Deck, card string, m Move, action string) error {
	m.Log.Action = action
	if err := s.repo.Move(ctx, m); err != nil {
		if errors.Is(err, errLostClaim) {
			return errMoved()
		}
		return apperrorInternal(err)
	}
	s.announce(ctx, m.CampaignID, moveNote{Deck: d, Move: m, Action: action, Card: card})
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

// drawResult is the reply to a draw: the copies it took, always a list, and
// the deck as the viewer now sees it. The first copy is also named as copyId so
// a caller written for a single draw keeps working.
func (r *request) drawResult(d Deck, got []Copy) (*ActionResult, error) {
	ids := make([]string, 0, len(got))
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	res, err := r.result(d, ids[0])
	if err != nil {
		return nil, err
	}
	res.CopyIDs = ids
	return res, nil
}

// target resolves where a deal goes: a hero's hand (a page of this campaign)
// or the whole table.
func (s *service) target(ctx context.Context, campaignID string, d Deck, to, heroID string) (Place, error) {
	switch to {
	case "hand":
		if heroID == "" && d.Kind == KindParty {
			// A party deck's hand deal with no hero is the party's own hand.
			return Place{Zone: ZoneHand, FaceUp: d.Settings.Arrives == "up"}, nil
		}
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
	case errors.Is(err, errShownFull):
		return errConflictMsg(fmt.Sprintf("At most %d cards can be shown at once; deal some or put them back first.", MaxShown))
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
	plan := DrawPlan{CampaignID: campaignID, DeckID: d.ID, Count: 1, Reshuffle: d.Settings.Empty == "reshuffle",
		Pick: s.pick, Log: LogLine{Action: ActDraw, ActorUserID: v.UserID}}
	if v.Director {
		if in.HeroID != "" {
			return nil, errInvalid("a Director's draw goes to the shown spot; deal it from there")
		}
		if in.Count != 0 {
			if in.Count < 1 || in.Count > MaxDrawCount {
				return nil, errInvalid(fmt.Sprintf("count must be between 1 and %d", MaxDrawCount))
			}
			plan.Count = in.Count
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
		if in.Count > 1 || in.Count < 0 {
			return nil, errInvalid("you draw one card at a time")
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
	for _, c := range got {
		s.announce(ctx, campaignID, moveNote{Deck: d, Action: ActDraw,
			Move: Move{CampaignID: campaignID, DeckID: d.ID, CopyID: c.ID, From: Place{Zone: ZonePile}, To: plan.To, Log: plan.Log}})
	}
	return r.drawResult(d, got)
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
	s.announce(ctx, campaignID, moveNote{Deck: d, Action: ActDeal,
		Move: Move{CampaignID: campaignID, DeckID: d.ID, CopyID: got[0].ID, From: Place{Zone: ZoneShown}, To: to, Log: plan.Log}})
	return r.result(d, got[0].ID)
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

	here := Place{Zone: c.Zone, HolderEntityID: c.HolderEntityID, FaceUp: c.FaceUp, Open: c.Open}
	m := Move{CampaignID: campaignID, DeckID: d.ID, CopyID: c.ID, From: here, To: here,
		Log: LogLine{ActorUserID: v.UserID, HolderEntityID: c.HolderEntityID}}
	// mine is whether the copy is in front of or in the hand of a hero the
	// viewer owns; a Director may act on any.
	own, err := r.owns(c.HolderEntityID)
	if err != nil {
		return nil, err
	}
	// The party's hand is held by every member, so it is "mine" to any of them.
	party := inPartyHand(d, c)
	mine := v.Director || own || party
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
		// Put this copy back and draw one more, in one step; the log reads as a
		// put-back and a draw.
		plan := DrawPlan{CampaignID: campaignID, DeckID: d.ID, Count: 1, To: Place{Zone: ZoneShown}, PutBackCopyID: c.ID,
			Reshuffle: d.Settings.Empty == "reshuffle", Pick: s.pick,
			Log: LogLine{Action: ActDraw, ActorUserID: v.UserID}}
		got, err := s.repo.Draw(ctx, plan)
		if err != nil {
			return nil, s.drawError(err, false)
		}
		s.announce(ctx, campaignID, moveNote{Deck: d, Action: ActDraw,
			Move: Move{CampaignID: campaignID, DeckID: d.ID, CopyID: got[0].ID, From: Place{Zone: ZonePile}, To: plan.To, Log: plan.Log}})
		return r.drawResult(d, got)

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
		// Turning is private: it changes what the holder and Directors see and
		// nothing else. A card turned down is no longer open.
		m.To.FaceUp, m.To.Open, m.Turned, action = !c.FaceUp, false, true, ActTurn

	case verbShow:
		if c.Zone != ZoneHand {
			return nil, wrongZone("in a hand")
		}
		if party {
			return nil, conflict("a card in the party's hand is already seen by the whole party")
		}
		if !mine {
			return nil, errForbidden("you can only show a card in your own hero's hand")
		}
		if !c.FaceUp {
			return nil, conflict("turn the card over before showing it")
		}
		if c.Open {
			// A repeat is refused before anything is written, so it cannot fill
			// the log or the notices.
			return nil, conflict("that card is already shown to the table")
		}
		m.To.Open, action = true, ActShow

	case verbKeep:
		if err := requireDirector(v); err != nil {
			return nil, err
		}
		return s.keep(r, d, c, here, in)

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
		if mayDiscard := v.Director || ((own || party) && d.Settings.Discard); !mayDiscard {
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

	if err := s.applyMove(ctx, d, card.Name, m, action); err != nil {
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
	var before []string
	if action == ActGather {
		before = s.holdersOf(ctx, campaignID, d)
	}
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
	s.announce(ctx, campaignID, moveNote{Deck: d, Action: action, Before: before,
		Move: Move{CampaignID: campaignID, DeckID: d.ID, From: Place{Zone: ZoneUsed}, To: Place{Zone: ZonePile}, Log: log}})
	return r.result(d, "")
}

// keep copies a card into another deck of the campaign and sends the source
// copy to gone. Only a card in play, in a hand or in the shown spot can be
// kept.
func (s *service) keep(r *request, d Deck, c Copy, here Place, in ActInput) (*ActionResult, error) {
	if c.Zone != ZoneShown && c.Zone != ZoneHand && c.Zone != ZonePlay {
		return nil, conflict("that card is not shown, in a hand or in play")
	}
	pick := errInvalid("choose another deck of this campaign to keep the card in")
	if !validToken(in.TargetDeckID) || in.TargetDeckID == d.ID {
		return nil, pick
	}
	target, found, err := s.repo.GetDeck(r.ctx, r.cid, in.TargetDeckID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, pick
	}
	kinds := make(map[string]bool, len(target.Look.Kinds))
	for _, k := range target.Look.Kinds {
		kinds[k.Key] = true
	}
	plan := KeepPlan{CampaignID: r.cid, DeckID: d.ID, CopyID: c.ID, From: here, TargetDeckID: target.ID, TargetKinds: kinds,
		Log: LogLine{Action: ActKeep, ActorUserID: r.v.UserID, HolderEntityID: c.HolderEntityID}}
	if err := s.repo.Keep(r.ctx, plan); err != nil {
		switch {
		case errors.Is(err, errLostClaim):
			return nil, errMoved()
		case errors.Is(err, errTargetFull):
			return nil, errInvalid(fmt.Sprintf("that deck already has %d cards", MaxCardsPerDeck))
		case errors.Is(err, errTargetCopies):
			return nil, errInvalid(fmt.Sprintf("that deck already has %d copies in all", MaxCopiesPerDeck))
		case errors.Is(err, errCardCopies):
			return nil, errInvalid(fmt.Sprintf("that deck already has %d copies of this card", MaxCopiesPerCard))
		case errors.Is(err, errDuplicate):
			return nil, conflict("that deck changed while the card was being kept; look again and try once more")
		}
		return nil, apperrorInternal(err)
	}
	s.announce(r.ctx, r.cid, moveNote{Deck: d, Action: ActKeep, Target: &target,
		Move: Move{CampaignID: r.cid, DeckID: d.ID, CopyID: c.ID, From: here, To: Place{Zone: ZoneGone}, Log: plan.Log}})
	return r.result(d, c.ID)
}
