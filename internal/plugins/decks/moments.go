package decks

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Moments: a card whose tests hold has a moment. A moment is one waiting notice
// for one deck, and it only remembers the card the deck picked blind among the
// cards that fit. Nothing leaves the pile until a Director deals, and Chronicle
// never deals on its own.
//
// Everything here is Director-only: that a moment came, which card the deck
// picked, whether a hidden test holds, and what it read are never sent to a
// player by any channel. The one thing a player can learn is that they were
// invited to draw, and only the invited player learns it.

// BellInvited is the bell entry for an invitation to draw. It rides on the
// existing card-decks choice.
const BellInvited = "deck_draw_invited"

// momentLookEvery is how soon a Director's read of the moments looks again.
const momentLookEvery = 5 * time.Second

// Answers a moment offers a Director.
const (
	AnswerDeal    = "deal"
	AnswerAnother = "another"
	AnswerInvite  = "invite"
	AnswerDismiss = "dismiss"
)

// MomentView is an open moment as a Director reads it.
type MomentView struct {
	ID       string `json:"id"`
	DeckID   string `json:"deckId"`
	DeckName string `json:"deckName"`
	DeckKind string `json:"deckKind"`
	// Notice is the deck's own setting for how loudly it tells the Director.
	Notice string        `json:"notice"`
	Card   *DirectorCard `json:"card"`
	// SubjectID and Subject are the hero the card's "any" tests held for.
	SubjectID string `json:"subjectId,omitempty"`
	Subject   string `json:"subject,omitempty"`
	// Hero is the name of the hero the card would go to, "" when it goes to the
	// party's hand or the table (or the page cannot be read).
	Hero          string     `json:"hero,omitempty"`
	FitCount      int        `json:"fitCount"`
	Reading       string     `json:"reading,omitempty"`
	ReadingSource string     `json:"readingSource,omitempty"`
	ReadAt        *time.Time `json:"readAt,omitempty"`
	Quiet         bool       `json:"quiet"`
	State         string     `json:"state"`
	InvitedUserID string     `json:"invitedUserId,omitempty"`
	InvitedName   string     `json:"invitedName,omitempty"`
	// Answers are the answers this moment can be given right now.
	Answers   []string  `json:"answers"`
	CreatedAt time.Time `json:"createdAt"`
}

// momentGone is the one answer for a moment that is not open any more.
func momentGone() error {
	return errConflictMsg("That moment has already been answered.")
}

func (s *service) momentChanged(cid string) {
	if s.momentNote != nil {
		s.momentNote.MomentChanged(cid)
	}
}

// poked tells the watcher that something a card's tests read may have changed.
func (s *service) poked(cid string) {
	if s.poke != nil {
		s.poke(cid)
	}
}

// momentHero is the hero a moment's card would go to: a hero deck's hero, or a
// party deck's subject. A deck dealt to the table, or with no hero to name, has
// none.
func momentHero(d Deck, m Moment) string {
	switch d.Kind {
	case KindHero:
		return d.HolderEntityID
	case KindParty:
		return m.SubjectID
	}
	return ""
}

// momentPlace says where a deal of the moment's card goes: "hand" with the
// hero ("" for the party's own hand), or "table". ok is false for a hero deck
// that belongs to no hero.
func momentPlace(d Deck, m Moment) (to, hero string, ok bool) {
	switch d.Kind {
	case KindHero:
		return "hand", d.HolderEntityID, d.HolderEntityID != ""
	case KindParty:
		return "hand", m.SubjectID, true
	}
	return "table", "", true
}

// invitedHero is the hero the viewer is invited to draw for, "" for everyone
// else. It is asked of every player's deck, so asking costs the same whether
// or not anyone is invited.
func (s *service) invitedHero(ctx context.Context, cid string, d Deck, v Viewer) string {
	if s.moments == nil || v.Director || v.UserID == "" {
		return ""
	}
	m, ok, err := s.moments.InvitedFor(ctx, cid, d.ID, v.UserID)
	if err != nil || !ok {
		return ""
	}
	return momentHero(d, m)
}

// weightedPick chooses one card among those that fit, blind, by how many
// copies of each are in the pile.
func (s *service) weightedPick(cards []Card, fit map[string]Fit, piles map[string]int, skip string) (Card, bool) {
	var pool []Card
	for _, c := range cards {
		if _, ok := fit[c.ID]; !ok || c.ID == skip {
			continue
		}
		for i := 0; i < piles[c.ID]; i++ {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		return Card{}, false
	}
	return pool[s.pick(len(pool))], true
}

// look judges every card of a deck that has tests and a copy in the pile.
func (s *service) look(ctx context.Context, cid string, d Deck, cards []Card, piles map[string]int) map[string]Fit {
	var withCopy []Card
	for _, c := range cards {
		if len(c.When) > 0 && piles[c.ID] > 0 {
			withCopy = append(withCopy, c)
		}
	}
	return s.fittingWithTests(ctx, cid, d, withCopy)
}

// Recheck looks at every watched deck of the campaign.
func (s *service) Recheck(ctx context.Context, cid string) error {
	if s.moments == nil {
		return nil
	}
	ids, err := s.moments.WatchedDecks(ctx, cid)
	if err != nil {
		return apperrorInternal(err)
	}
	var first error
	changed := false
	var told []string
	for _, id := range ids {
		c, invited, err := s.recheckDeck(ctx, cid, id)
		if err != nil && first == nil {
			first = err
		}
		changed = changed || c
		if invited != "" {
			told = append(told, invited)
		}
	}
	if len(told) > 0 && s.notify != nil {
		sort.Strings(told)
		s.notify.Changed(cid, Audience{Users: told})
	}
	if changed {
		s.momentChanged(cid)
	}
	return first
}

// recheckDeck judges one deck. changed says a Director would see something
// different; invitedLapsed names the player whose invitation ended.
func (s *service) recheckDeck(ctx context.Context, cid, deckID string) (changed bool, invitedLapsed string, err error) {
	d, found, err := s.repo.GetDeck(ctx, cid, deckID)
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	if !found {
		return false, "", nil
	}
	cards, err := s.repo.ListCards(ctx, cid, d.ID)
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	om, hasOpen, err := s.moments.Open(ctx, cid, d.ID)
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	watched := d.Settings.Notice != "quiet" && cardsWithTests(cards)
	if !watched {
		// A deck set to quiet, or with no test left, has nothing to wait for.
		if hasOpen {
			return s.lapse(ctx, cid, om)
		}
		return false, "", nil
	}
	piles, err := s.moments.PileCounts(ctx, cid, d.ID)
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	fit := s.look(ctx, cid, d, cards, piles)

	var start, stop []string
	started := false
	for _, c := range cards {
		if len(c.When) == 0 {
			continue
		}
		_, ok := fit[c.ID]
		switch {
		case ok && c.FitsSince == nil:
			start, started = append(start, c.ID), true
		case !ok && c.FitsSince != nil:
			stop = append(stop, c.ID)
		}
	}
	if err := s.moments.SetFits(ctx, cid, d.ID, start, stop); err != nil {
		return false, "", apperrorInternal(err)
	}

	if hasOpen {
		return s.keepOrRepick(ctx, cid, d, cards, piles, fit, om)
	}
	if !started {
		return false, "", nil
	}
	pick, ok := s.weightedPick(cards, fit, piles, "")
	if !ok {
		return false, "", nil
	}
	quiet := false
	if at, any, err := s.moments.LastClosed(ctx, cid, d.ID); err != nil {
		return false, "", apperrorInternal(err)
	} else if any && time.Since(at) < quietWindow {
		quiet = true
	}
	f := fit[pick.ID]
	created, err := s.moments.Create(ctx, Moment{ID: uuid.NewString(), CampaignID: cid, DeckID: d.ID, CardID: pick.ID, SubjectID: f.Subject,
		FitCount: len(fit), Reading: f.Reading, Quiet: quiet})
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	return created, "", nil
}

// keepOrRepick handles an open moment: it stays while its card fits, is picked
// again while another card fits, and lapses when none does. An invited moment
// stays while any card fits, because the invited draw picks among them when it
// happens and not the card the moment remembered.
func (s *service) keepOrRepick(ctx context.Context, cid string, d Deck, cards []Card, piles map[string]int, fit map[string]Fit, om Moment) (bool, string, error) {
	cur, stays := fit[om.CardID]
	if om.State == MomentInvited {
		if len(fit) == 0 {
			return s.lapse(ctx, cid, om)
		}
		return s.refresh(ctx, cid, om, om.SubjectID, len(fit), cur.Reading)
	}
	if stays && piles[om.CardID] > 0 {
		return s.refresh(ctx, cid, om, cur.Subject, len(fit), cur.Reading)
	}
	pick, ok := s.weightedPick(cards, fit, piles, om.CardID)
	if !ok {
		return s.lapse(ctx, cid, om)
	}
	f := fit[pick.ID]
	moved, err := s.moments.Repick(ctx, cid, om.ID, om.CardID, Moment{CardID: pick.ID, SubjectID: f.Subject, FitCount: len(fit), Reading: f.Reading})
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	return moved, "", nil
}

// refresh updates what an open moment says when it has changed, and says
// whether it did.
func (s *service) refresh(ctx context.Context, cid string, om Moment, subject string, count int, r Reading) (bool, string, error) {
	if om.SubjectID == subject && om.FitCount == count && om.Reading.Text == r.Text && om.Reading.Source == r.Source {
		return false, "", nil
	}
	if err := s.moments.Refresh(ctx, cid, om.ID, Moment{SubjectID: subject, FitCount: count, Reading: r}); err != nil {
		return false, "", apperrorInternal(err)
	}
	return true, "", nil
}

// lapse closes a moment nothing fits any more.
func (s *service) lapse(ctx context.Context, cid string, om Moment) (bool, string, error) {
	closed, err := s.moments.Close(ctx, cid, om.ID, []string{MomentWaiting, MomentInvited}, MomentLapsed, closedBySystem)
	if err != nil {
		return false, "", apperrorInternal(err)
	}
	if !closed {
		return false, "", nil
	}
	if om.State == MomentInvited {
		return true, om.InvitedUserID, nil
	}
	return true, "", nil
}

// ---- reading and answering --------------------------------------------------

// directorOnly is the 404 anyone but a Director gets for every moment route.
func directorOnly(v Viewer) error {
	if !v.Director {
		return errNotFound("page")
	}
	return nil
}

// view builds one open moment for a Director.
func (r *request) momentView(m Moment, members map[string]Member) (MomentView, error) {
	d, found, err := r.s.repo.GetDeck(r.ctx, r.cid, m.DeckID)
	if err != nil {
		return MomentView{}, apperrorInternal(err)
	}
	if !found {
		return MomentView{}, errNotFound("page")
	}
	card, found, err := r.s.repo.GetCard(r.ctx, r.cid, d.ID, m.CardID)
	if err != nil {
		return MomentView{}, apperrorInternal(err)
	}
	if !found {
		return MomentView{}, errNotFound("page")
	}
	j := r.s.judgeFor(r.ctx, r.cid, d)
	nm := r.s.newNamer(r.ctx, r.cid, d, r.v)
	dc := r.s.directorCardOf(card, j, nm, r.s.newTestChecker(r.ctx, r.cid, d))
	out := MomentView{ID: m.ID, DeckID: d.ID, DeckName: d.Name, DeckKind: d.Kind, Notice: d.Settings.Notice, Card: &dc,
		SubjectID: m.SubjectID, FitCount: m.FitCount, Reading: m.Reading.Text, ReadingSource: m.Reading.Source,
		Quiet: m.Quiet, State: m.State, InvitedUserID: m.InvitedUserID, CreatedAt: m.CreatedAt, Answers: []string{}}
	if !m.Reading.At.IsZero() {
		at := m.Reading.At
		out.ReadAt = &at
	}
	if m.SubjectID != "" {
		out.Subject = nm.hero(m.SubjectID)
	}
	if h := momentHero(d, m); h != "" {
		out.Hero = nm.hero(h)
	}
	if mem, ok := members[m.InvitedUserID]; ok {
		out.InvitedName = mem.Name
	}
	if m.Open() {
		out.Answers = r.s.answersFor(r.ctx, r.cid, d, m)
	}
	return out, nil
}

// answersFor lists what a Director can do with an open moment now.
func (s *service) answersFor(ctx context.Context, cid string, d Deck, m Moment) []string {
	var out []string
	if _, _, ok := momentPlace(d, m); ok {
		out = append(out, AnswerDeal)
	}
	if m.State == MomentWaiting {
		out = append(out, AnswerAnother)
		if _, ok := s.invitee(ctx, cid, d, m); ok {
			out = append(out, AnswerInvite)
		}
	}
	return append(out, AnswerDismiss)
}

// invitee is the player a moment could invite: the claimed player of the hero
// the card would go to, a current member who is not a Director.
func (s *service) invitee(ctx context.Context, cid string, d Deck, m Moment) (string, bool) {
	hero := momentHero(d, m)
	if hero == "" {
		return "", false
	}
	r, err := s.members(ctx, cid)
	if err != nil {
		return "", false
	}
	owner, err := s.ownerOf(ctx, cid, hero, r)
	if err != nil || owner == "" || !r.player(owner) {
		return "", false
	}
	return owner, true
}

func (s *service) Moments(ctx context.Context, cid string, v Viewer) ([]MomentView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if s.moments == nil {
		return []MomentView{}, nil
	}
	// A change no hook saw is caught the next time a Director looks, at most
	// once every few seconds per campaign.
	s.lookedMu.Lock()
	last, seen := s.looked[cid]
	due := !seen || time.Since(last) >= momentLookEvery
	if due {
		s.looked[cid] = time.Now()
	}
	s.lookedMu.Unlock()
	if due {
		if err := s.Recheck(ctx, cid); err != nil {
			slog.Warn("decks: recheck on read failed", slog.String("campaign_id", cid), slog.Any("error", err))
		}
	}
	list, err := s.moments.ListOpen(ctx, cid)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	r := s.newRequest(ctx, cid, v)
	members := map[string]Member{}
	if people, err := s.people.Members(ctx, cid); err == nil {
		for _, m := range people {
			members[m.UserID] = m
		}
	}
	out := make([]MomentView, 0, len(list))
	for _, m := range list {
		mv, err := r.momentView(m, members)
		if err != nil {
			continue
		}
		out = append(out, mv)
	}
	return out, nil
}

// openMoment loads a moment a Director may answer: a missing one is a 404 and
// one that is closed a 409, and nothing changes either way.
func (s *service) openMoment(ctx context.Context, cid, id string, v Viewer) (Moment, Deck, *request, error) {
	if err := directorOnly(v); err != nil {
		return Moment{}, Deck{}, nil, err
	}
	if s.moments == nil || !validToken(id) {
		return Moment{}, Deck{}, nil, errNotFound("page")
	}
	m, found, err := s.moments.Get(ctx, cid, id)
	if err != nil {
		return Moment{}, Deck{}, nil, apperrorInternal(err)
	}
	if !found {
		return Moment{}, Deck{}, nil, errNotFound("page")
	}
	if !m.Open() {
		return Moment{}, Deck{}, nil, momentGone()
	}
	d, found, err := s.repo.GetDeck(ctx, cid, m.DeckID)
	if err != nil {
		return Moment{}, Deck{}, nil, apperrorInternal(err)
	}
	if !found {
		return Moment{}, Deck{}, nil, errNotFound("page")
	}
	return m, d, s.newRequest(ctx, cid, v), nil
}

func (s *service) viewOf(r *request, id string) (*MomentView, error) {
	m, found, err := s.moments.Get(r.ctx, r.cid, id)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("page")
	}
	members := map[string]Member{}
	if people, err := s.people.Members(r.ctx, r.cid); err == nil {
		for _, p := range people {
			members[p.UserID] = p
		}
	}
	mv, err := r.momentView(m, members)
	if err != nil {
		return nil, err
	}
	return &mv, nil
}

func (s *service) DealMoment(ctx context.Context, cid, id string, v Viewer) (*ActionResult, error) {
	m, d, r, err := s.openMoment(ctx, cid, id, v)
	if err != nil {
		return nil, err
	}
	to, hero, ok := momentPlace(d, m)
	if !ok {
		return nil, errInvalid("this hero deck belongs to no hero page yet; choose one, or deal the card from the deck")
	}
	place, err := s.target(ctx, cid, d, to, hero)
	if err != nil {
		return nil, err
	}
	plan := DrawPlan{CampaignID: cid, DeckID: d.ID, To: place, DealtBy: v.UserID, CardID: m.CardID, Pick: s.pick,
		Moment: &MomentClaim{ID: m.ID, From: []string{MomentWaiting, MomentInvited}, To: MomentDealt, By: v.UserID},
		Log:    LogLine{Action: ActDeal, ActorUserID: v.UserID, HolderEntityID: place.HolderEntityID}}
	got, err := s.repo.Draw(ctx, plan)
	if err != nil {
		switch {
		case errors.Is(err, errMomentGone):
			return nil, momentGone()
		case errors.Is(err, errEmptyPile):
			// The card has no copy left in the pile; look again so the moment
			// moves on, and say so.
			if rerr := s.Recheck(ctx, cid); rerr != nil {
				slog.Warn("decks: recheck after a deal failed", slog.String("campaign_id", cid), slog.Any("error", rerr))
			}
			return nil, errConflictMsg("No copy of that card is left in the pile any more; the moment was looked at again.")
		}
		return nil, s.drawError(err, true)
	}
	s.announce(ctx, cid, moveNote{Deck: d, Action: ActDeal,
		Move: Move{CampaignID: cid, DeckID: d.ID, CopyID: got[0].ID, From: Place{Zone: ZoneShown}, To: place, Log: plan.Log}})
	s.afterAnswer(ctx, cid, m)
	s.poked(cid)
	return r.result(d, got[0].ID)
}

// afterAnswer tells the people a moment's closing concerns: Directors always,
// and the player whose invitation it ended.
func (s *service) afterAnswer(_ context.Context, cid string, m Moment) {
	if m.State == MomentInvited && m.InvitedUserID != "" && s.notify != nil {
		s.notify.Changed(cid, Audience{Users: []string{m.InvitedUserID}})
	}
	s.momentChanged(cid)
}

func (s *service) AnotherMoment(ctx context.Context, cid, id string, v Viewer) (*MomentView, error) {
	m, d, r, err := s.openMoment(ctx, cid, id, v)
	if err != nil {
		return nil, err
	}
	if m.State != MomentWaiting {
		return nil, errConflictMsg("Take the invitation back before picking another card.")
	}
	cards, err := s.repo.ListCards(ctx, cid, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	piles, err := s.moments.PileCounts(ctx, cid, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	fit := s.look(ctx, cid, d, cards, piles)
	pick, ok := s.weightedPick(cards, fit, piles, m.CardID)
	if !ok {
		return nil, errConflictMsg("This is the only card that fits right now.")
	}
	f := fit[pick.ID]
	moved, err := s.moments.Repick(ctx, cid, m.ID, m.CardID, Moment{CardID: pick.ID, SubjectID: f.Subject, FitCount: len(fit), Reading: f.Reading})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !moved {
		return nil, momentGone()
	}
	s.momentChanged(cid)
	return s.viewOf(r, m.ID)
}

func (s *service) InviteMoment(ctx context.Context, cid, id string, v Viewer) (*MomentView, error) {
	m, d, r, err := s.openMoment(ctx, cid, id, v)
	if err != nil {
		return nil, err
	}
	if m.State != MomentWaiting {
		return nil, momentGone()
	}
	owner, ok := s.invitee(ctx, cid, d, m)
	if !ok {
		return nil, errConflictMsg("There is no player to invite: the card would not go to a hero that a player plays.")
	}
	done, err := s.moments.Invite(ctx, cid, m.ID, owner)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !done {
		return nil, momentGone()
	}
	if s.bell != nil {
		if err := s.bell.Ring(ctx, cid, BellEntry{Users: []string{owner}, Kind: BellInvited,
			Message: "Your Director invites you to draw from " + d.Name, Link: handLink(cid)}); err != nil {
			slog.Warn("decks: bell notice not written", slog.Any("error", err))
		}
	}
	if s.notify != nil {
		s.notify.Changed(cid, Audience{Users: []string{owner}})
	}
	s.momentChanged(cid)
	return s.viewOf(r, m.ID)
}

func (s *service) DismissMoment(ctx context.Context, cid, id string, v Viewer) (*MomentView, error) {
	m, _, r, err := s.openMoment(ctx, cid, id, v)
	if err != nil {
		return nil, err
	}
	closed, err := s.moments.Close(ctx, cid, m.ID, []string{MomentWaiting, MomentInvited}, MomentDismissed, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !closed {
		return nil, momentGone()
	}
	// Cards that fit keep fits_since, so they raise nothing again until they
	// stop fitting and start again.
	s.afterAnswer(ctx, cid, m)
	return s.viewOf(r, m.ID)
}

// invitation finds the moment that invites this player to draw into this hero
// from this deck. A player with none draws by the deck's ordinary rule.
func (s *service) invitation(ctx context.Context, cid string, d Deck, v Viewer, heroID string) (Moment, bool) {
	if s.moments == nil || v.UserID == "" || heroID == "" {
		return Moment{}, false
	}
	m, ok, err := s.moments.InvitedFor(ctx, cid, d.ID, v.UserID)
	if err != nil || !ok || momentHero(d, m) != heroID {
		return Moment{}, false
	}
	return m, true
}
