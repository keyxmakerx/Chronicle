package decks

import (
	"context"
	"log/slog"
	"reflect"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// moveNote is one stored change, as announce needs it.
type moveNote struct {
	Deck   Deck
	Move   Move
	Action string
	// Card is the card's name, for a bell notice that may name it.
	Card string
	// Target is the deck a card was kept in.
	Target *Deck
	// Before are the players who held cards of the deck before a gather.
	Before []string
	// Reply is a Director's written reply to an ask they turned down.
	Reply string
}

// cast is who is in the campaign, read once per announcement.
type cast struct {
	byID      map[string]Member
	directors []string
}

func (s *service) members(ctx context.Context, cid string) (cast, error) {
	list, err := s.people.Members(ctx, cid)
	if err != nil {
		return cast{}, err
	}
	r := cast{byID: make(map[string]Member, len(list))}
	for _, m := range list {
		r.byID[m.UserID] = m
		if m.Director {
			r.directors = append(r.directors, m.UserID)
		}
	}
	sort.Strings(r.directors)
	return r, nil
}

// player says whether the user is a current member who is not a Director.
func (r cast) player(id string) bool {
	m, ok := r.byID[id]
	return ok && !m.Director && m.Role >= permissions.RolePlayer
}

// ownerOf is the claimed player of a hero page, "" when it has none or the
// player is no longer a member.
func (s *service) ownerOf(ctx context.Context, cid, heroID string, r cast) (string, error) {
	if heroID == "" {
		return "", nil
	}
	h, ok, err := s.heroes.InCampaign(ctx, cid, heroID)
	if err != nil || !ok {
		return "", err
	}
	if _, member := r.byID[h.OwnerUserID]; !member {
		return "", nil
	}
	return h.OwnerUserID, nil
}

// seersOf are the players who may see the deck at all, by the same rule that
// lists it: a party deck is every member's, a chosen player (during their
// present stay), the player of the hero a hero deck belongs to, and the player
// of any hero holding one of its cards.
func (s *service) seersOf(ctx context.Context, cid string, d Deck, r cast) ([]string, error) {
	set := map[string]bool{}
	if d.Kind == KindParty {
		for id := range r.byID {
			set[id] = true
		}
	}
	for _, p := range d.Players {
		if m, ok := r.byID[p.UserID]; ok && !p.GrantedAt.Before(m.JoinedAt) {
			set[p.UserID] = true
		}
	}
	holders := map[string]bool{}
	if d.HolderEntityID != "" {
		holders[d.HolderEntityID] = true
	}
	copies, err := s.repo.ListCopies(ctx, cid, d.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range copies {
		if (c.Zone == ZoneHand || c.Zone == ZonePlay) && c.HolderEntityID != "" {
			holders[c.HolderEntityID] = true
		}
	}
	for h := range holders {
		o, err := s.ownerOf(ctx, cid, h, r)
		if err != nil {
			return nil, err
		}
		if o != "" {
			set[o] = true
		}
	}
	var out []string
	for id := range set {
		if r.player(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// viewersOf narrows seers to those who may open a hero's page.
func (s *service) viewersOf(ctx context.Context, cid, heroID string, seers []string, r cast) ([]string, error) {
	if heroID == "" {
		return nil, nil
	}
	var out []string
	for _, id := range seers {
		m := r.byID[id]
		ok, err := s.heroes.Viewable(ctx, cid, heroID, Viewer{UserID: id, MemberRole: m.Role, VisibilityRole: m.Role})
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// announce tells the people a stored change is for. It never fails a move:
// when who to tell cannot be worked out, only Directors are told.
func (s *service) announce(ctx context.Context, cid string, n moveNote) {
	if s.notify == nil && s.bell == nil {
		return
	}
	aud, bells, err := s.audienceFor(ctx, cid, n)
	if err != nil {
		slog.Warn("decks: could not work out who to tell", slog.Any("error", err))
		aud, bells = Audience{}, nil
	}
	if s.notify != nil {
		s.notify.Changed(cid, aud)
	}
	if s.bell == nil {
		return
	}
	for _, b := range bells {
		if err := s.bell.Ring(ctx, cid, b); err != nil {
			slog.Warn("decks: bell notice not written", slog.Any("error", err))
		}
	}
}

func (s *service) audienceFor(ctx context.Context, cid string, n moveNote) (Audience, []BellEntry, error) {
	d, m := n.Deck, n.Move
	f := moveFacts{Action: n.Action, DeckKind: d.Kind, Knows: d.Settings.Knows, OthersSee: d.Settings.OthersSee, From: m.From, To: m.To}
	// A Director-only move needs nothing read.
	if n.Action == ActPutBack {
		return audience(f), nil, nil
	}
	r, err := s.members(ctx, cid)
	if err != nil {
		return Audience{}, nil, err
	}
	if f.Seers, err = s.seersOf(ctx, cid, d, r); err != nil {
		return Audience{}, nil, err
	}
	if n.Action == actDeck {
		set := map[string]bool{}
		for _, id := range f.Seers {
			set[id] = true
		}
		for _, id := range n.Before {
			if r.player(id) {
				set[id] = true
			}
		}
		f.Seers = f.Seers[:0]
		for id := range set {
			f.Seers = append(f.Seers, id)
		}
		sort.Strings(f.Seers)
		return audience(f), nil, nil
	}
	for _, id := range n.Before {
		if r.player(id) {
			f.Before = append(f.Before, id)
		}
	}
	if f.FromOwner, err = s.ownerOf(ctx, cid, m.From.HolderEntityID, r); err != nil {
		return Audience{}, nil, err
	}
	if f.ToOwner, err = s.ownerOf(ctx, cid, m.To.HolderEntityID, r); err != nil {
		return Audience{}, nil, err
	}
	if f.FromViewers, err = s.viewersOf(ctx, cid, m.From.HolderEntityID, f.Seers, r); err != nil {
		return Audience{}, nil, err
	}
	if f.ToViewers, err = s.viewersOf(ctx, cid, m.To.HolderEntityID, f.Seers, r); err != nil {
		return Audience{}, nil, err
	}
	if n.Target != nil && n.Target.Settings.Knows != "nothing" {
		if f.TargetSeers, err = s.seersOf(ctx, cid, *n.Target, r); err != nil {
			return Audience{}, nil, err
		}
	}
	aud := audience(f)

	bf := bellFacts{Action: n.Action, CampaignID: cid, DeckID: d.ID, DeckName: d.Name, Card: n.Card, Reply: n.Reply, From: m.From, To: m.To,
		FromOwner: f.FromOwner, ToOwner: f.ToOwner, Actor: m.Log.ActorUserID, Directors: r.directors}
	if holder := m.From.HolderEntityID; holder != "" {
		if h, ok, err := s.heroes.InCampaign(ctx, cid, holder); err == nil && ok {
			bf.HeroName = h.Name
		}
	} else if n.Action == ActAsk {
		bf.HeroName = "The party"
	}
	return aud, bellFor(bf), nil
}

// seersNow are the players who may see a deck right now, read before a change
// that may take it from them. A failure reads as nobody: the change then tells
// only those who see the deck afterwards.
func (s *service) seersNow(ctx context.Context, cid string, d Deck) []string {
	if s.notify == nil {
		return nil
	}
	r, err := s.members(ctx, cid)
	if err != nil {
		return nil
	}
	seers, err := s.seersOf(ctx, cid, d, r)
	if err != nil {
		return nil
	}
	return seers
}

// readByPlayers says whether two versions of a deck differ in anything a
// player's view is built from. A change to how cards arrive, where a played
// card goes, an empty pile's rule or a Director's next and last card is not
// one, so saving those tells no player anything.
func readByPlayers(a, b Deck) bool {
	return a.Name != b.Name || a.Kind != b.Kind || a.HolderEntityID != b.HolderEntityID ||
		a.Settings.Draws != b.Settings.Draws || a.Settings.Knows != b.Settings.Knows ||
		a.Settings.OthersSee != b.Settings.OthersSee || a.Settings.Discard != b.Settings.Discard ||
		!reflect.DeepEqual(a.Look, b.Look)
}

// holdersOf are the players who hold cards of the deck right now, read before
// a gather empties every hand. A failure reads as nobody: the gather then
// tells the deck's seers only.
func (s *service) holdersOf(ctx context.Context, cid string, d Deck) []string {
	if s.notify == nil {
		return nil
	}
	r, err := s.members(ctx, cid)
	if err != nil {
		return nil
	}
	copies, err := s.repo.ListCopies(ctx, cid, d.ID)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range copies {
		if (c.Zone != ZoneHand && c.Zone != ZonePlay) || c.HolderEntityID == "" {
			continue
		}
		o, err := s.ownerOf(ctx, cid, c.HolderEntityID, r)
		if err == nil && o != "" && !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	return out
}
