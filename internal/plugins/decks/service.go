package decks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Service owns the deck rules. Handlers only bind and render; every check of
// who may see or do what is here, so a mis-wired route cannot widen access.
type Service interface {
	// List returns the Directors' list, or the player's own list, by who asks.
	List(ctx context.Context, campaignID string, v Viewer) (DeckListView, error)
	// Get returns one deck as the viewer may see it; a deck they may not see
	// is a 404.
	Get(ctx context.Context, campaignID, deckID string, v Viewer) (DeckView, error)
	Create(ctx context.Context, campaignID string, v Viewer, in CreateDeckInput) (DeckView, error)
	Update(ctx context.Context, campaignID, deckID string, v Viewer, in UpdateDeckInput) (DeckView, error)
	Delete(ctx context.Context, campaignID, deckID string, v Viewer) error
	SetPlayers(ctx context.Context, campaignID, deckID string, v Viewer, in SetPlayersInput) (DeckView, error)

	CreateCard(ctx context.Context, campaignID, deckID string, v Viewer, in CreateCardInput) (DeckView, error)
	UpdateCard(ctx context.Context, campaignID, deckID, cardID string, v Viewer, in UpdateCardInput) (DeckView, error)
	DeleteCard(ctx context.Context, campaignID, deckID, cardID string, v Viewer) (DeckView, error)

	// Draw is a Director's draw to the shown spot, or a player's draw into the
	// named hero's hand.
	Draw(ctx context.Context, campaignID, deckID string, v Viewer, in DrawInput) (*ActionResult, error)
	// Act applies a verb to one copy.
	Act(ctx context.Context, campaignID, deckID, copyID, verb string, v Viewer, in ActInput) (*ActionResult, error)
	// DealCard deals a random pile copy of a named card.
	DealCard(ctx context.Context, campaignID, deckID string, v Viewer, in DealCardInput) (*ActionResult, error)
	Shuffle(ctx context.Context, campaignID, deckID string, v Viewer) (*ActionResult, error)
	Gather(ctx context.Context, campaignID, deckID string, v Viewer) (*ActionResult, error)

	// Table returns the cards lying in front of the whole table, from every
	// deck, to any member.
	Table(ctx context.Context, campaignID string, v Viewer) (*TableView, error)
}

// Notifier is told after every move, once it is stored, so live messages can
// follow. CopyID is empty for a shuffle or a gather.
//
// Anything that names a copy, a card or a holder is for Directors only. A
// notice sent to a player may name only the deck: a copy id, a card, or which
// hero holds it would tell a player what a face-down card is or where it went.
type Notifier interface {
	Moved(campaignID, deckID, copyID, action, holderEntityID, actorUserID string)
}

// Option configures the service.
type Option func(*service)

// WithNotifier sets who is told after a move.
func WithNotifier(n Notifier) Option { return func(s *service) { s.notify = n } }

// WithRandom replaces the draw's random pick; tests use it to fix a draw.
func WithRandom(pick func(n int) int) Option { return func(s *service) { s.pick = pick } }

type service struct {
	repo   Repository
	heroes Heroes
	people Members
	notify Notifier
	pick   func(n int) int
}

// NewService builds the service.
func NewService(repo Repository, heroes Heroes, members Members, opts ...Option) Service {
	s := &service{repo: repo, heroes: heroes, people: members, pick: cryptoPick}
	for _, o := range opts {
		o(s)
	}
	return s
}

// cryptoPick returns a uniform random index below n from crypto randomness,
// so a draw cannot be predicted from earlier ones.
func cryptoPick(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// The system source failing is not recoverable and a biased draw
		// would be worse than a stopped one.
		panic(fmt.Sprintf("decks: crypto/rand: %v", err))
	}
	return int(v.Int64())
}

func (s *service) moved(campaignID string, m Move, action string) {
	if s.notify != nil {
		s.notify.Moved(campaignID, m.DeckID, m.CopyID, action, m.Log.HolderEntityID, m.Log.ActorUserID)
	}
}

// request carries what one call learns about the viewer, so it asks the
// directory once.
type request struct {
	s      *service
	ctx    context.Context
	cid    string
	v      Viewer
	owned  []Hero
	ownedF bool
	held   []Copy
	heldF  bool
	joined map[string]time.Time
	joinF  bool
}

func (s *service) newRequest(ctx context.Context, cid string, v Viewer) *request {
	return &request{s: s, ctx: ctx, cid: cid, v: v}
}

// member reports whether v is a current member who may use decks at all: a
// former member or a visitor to a public campaign never is.
func (r *request) member() bool {
	return r.v.Director || (r.v.UserID != "" && r.v.MemberRole >= permissions.RolePlayer)
}

func (r *request) ownedHeroes() ([]Hero, error) {
	if !r.ownedF {
		h, err := r.s.heroes.Owned(r.ctx, r.cid, r.v.UserID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		r.owned, r.ownedF = h, true
	}
	return r.owned, nil
}

func (r *request) owns(entityID string) (bool, error) {
	if entityID == "" {
		return false, nil
	}
	owned, err := r.ownedHeroes()
	if err != nil {
		return false, err
	}
	for _, h := range owned {
		if h.ID == entityID {
			return true, nil
		}
	}
	return false, nil
}

// heldCopies are the viewer's own heroes' copies in hand or in play, across
// every deck.
func (r *request) heldCopies() ([]Copy, error) {
	if !r.heldF {
		owned, err := r.ownedHeroes()
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(owned))
		for _, h := range owned {
			ids = append(ids, h.ID)
		}
		held, err := r.s.repo.ListHeldBy(r.ctx, r.cid, ids)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		r.held, r.heldF = held, true
	}
	return r.held, nil
}

// chosen finds the viewer among a deck's chosen players. A grant counts only
// for a current member and only if it was made during their present stay.
func (r *request) chosen(d Deck) (Player, bool, error) {
	if !r.member() || r.v.UserID == "" {
		return Player{}, false, nil
	}
	var mine *Player
	for i := range d.Players {
		if d.Players[i].UserID == r.v.UserID {
			mine = &d.Players[i]
		}
	}
	if mine == nil {
		return Player{}, false, nil
	}
	if !r.joinF {
		list, err := r.s.people.Members(r.ctx, r.cid)
		if err != nil {
			return Player{}, false, apperrorInternal(err)
		}
		r.joined = make(map[string]time.Time, len(list))
		for _, m := range list {
			r.joined[m.UserID] = m.JoinedAt
		}
		r.joinF = true
	}
	j, ok := r.joined[mine.UserID]
	if !ok || mine.GrantedAt.Before(j) {
		return Player{}, false, nil
	}
	return *mine, true, nil
}

// listed says whether a player sees the deck at all: they are one of its
// chosen players, a hero they own is its holder or holds a copy of it in
// hand or in play, or it is a party deck. Event decks are otherwise never
// listed.
func (r *request) listed(d Deck) (bool, error) {
	if !r.member() {
		return false, nil
	}
	if d.Kind == KindParty {
		return true, nil
	}
	if _, ok, err := r.chosen(d); err != nil || ok {
		return ok, err
	}
	if own, err := r.owns(d.HolderEntityID); err != nil || own {
		return own, err
	}
	held, err := r.heldCopies()
	if err != nil {
		return false, err
	}
	for _, c := range held {
		if c.DeckID == d.ID {
			return true, nil
		}
	}
	return false, nil
}

// deck loads a deck the viewer may see; one they may not is a 404, exactly
// like a missing one.
func (r *request) deck(id string) (Deck, error) {
	if !validToken(id) || !r.member() {
		return Deck{}, errNotFound("deck")
	}
	d, found, err := r.s.repo.GetDeck(r.ctx, r.cid, id)
	if err != nil {
		return Deck{}, apperrorInternal(err)
	}
	if !found {
		return Deck{}, errNotFound("deck")
	}
	if !r.v.Director {
		ok, err := r.listed(d)
		if err != nil {
			return Deck{}, err
		}
		if !ok {
			return Deck{}, errNotFound("deck")
		}
	}
	return d, nil
}

func requireDirector(v Viewer) error {
	if !v.Director {
		return errForbidden("only the campaign's Directors can do that")
	}
	return nil
}

// directorDeck loads a deck for a Director; anyone else gets a 404 rather
// than a hint that the deck exists.
func (r *request) directorDeck(id string) (Deck, error) {
	d, err := r.deck(id)
	if err != nil {
		return Deck{}, err
	}
	return d, requireDirector(r.v)
}

// ---- reading ---------------------------------------------------------------

func (s *service) List(ctx context.Context, campaignID string, v Viewer) (DeckListView, error) {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() {
		return nil, errNotFound("deck")
	}
	all, err := s.repo.ListDecks(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if v.Director {
		counts, err := s.repo.ZoneCounts(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		out := DirectorList{Decks: []DirectorDeck{}}
		for _, d := range all {
			out.Decks = append(out.Decks, directorSummary(d, counts[d.ID]))
		}
		return out, nil
	}
	out := PlayerList{Decks: []PlayerDeck{}}
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
		out.Decks = append(out.Decks, pd)
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, campaignID, deckID string, v Viewer) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, err
	}
	return r.view(d)
}

// view builds the deck for the viewer: the Director type or the player type,
// never one filtered into the other.
func (r *request) view(d Deck) (DeckView, error) {
	if r.v.Director {
		return r.directorFull(d)
	}
	return r.playerDeck(d)
}

func directorPlayers(ps []Player) []DirectorPlayer {
	out := make([]DirectorPlayer, 0, len(ps))
	for _, p := range ps {
		out = append(out, DirectorPlayer{UserID: p.UserID, DrawsLeft: p.DrawsLeft})
	}
	return out
}

func directorSummary(d Deck, counts ZoneCounts) DirectorDeck {
	return DirectorDeck{ID: d.ID, Name: d.Name, Kind: d.Kind, HolderEntityID: d.HolderEntityID,
		NextCardID: d.NextCardID, LastCardID: d.LastCardID, Settings: d.Settings, Look: d.Look, Version: d.Version, Players: directorPlayers(d.Players), Counts: counts}
}

func (r *request) directorFull(d Deck) (DirectorDeck, error) {
	cards, err := r.s.repo.ListCards(r.ctx, r.cid, d.ID)
	if err != nil {
		return DirectorDeck{}, apperrorInternal(err)
	}
	copies, err := r.s.repo.ListCopies(r.ctx, r.cid, d.ID)
	if err != nil {
		return DirectorDeck{}, apperrorInternal(err)
	}
	log, err := r.s.repo.ListLog(r.ctx, r.cid, d.ID, LogShown)
	if err != nil {
		return DirectorDeck{}, apperrorInternal(err)
	}
	var counts ZoneCounts
	out := directorSummary(d, counts)
	out.Cards = make([]DirectorCard, 0, len(cards))
	for _, c := range cards {
		out.Cards = append(out.Cards, DirectorCard{ID: c.ID, Name: c.Name, KindKey: c.KindKey, Words: c.Words,
			DirectorNote: c.DirectorNote, Picture: c.Picture, Corner: c.Corner, Rare: c.Rare, Copies: c.Copies, Fate: c.Fate,
			When: c.When, WhenOpen: c.WhenOpen, AskFirst: c.AskFirst, SortOrder: c.SortOrder})
	}
	out.Copies = make([]DirectorCopy, 0, len(copies))
	for _, c := range copies {
		out.Counts.add(c.Zone, 1)
		out.Copies = append(out.Copies, DirectorCopy{ID: c.ID, CardID: c.CardID, Zone: c.Zone,
			HolderEntityID: c.HolderEntityID, FaceUp: c.FaceUp, Open: c.Open, AskedAt: c.AskedAt, DealtBy: c.DealtBy,
			DealtAt: c.DealtAt, TurnedAt: c.TurnedAt, MovedAt: c.MovedAt})
	}
	out.Log = log
	return out, nil
}

func faceOf(c Card) Face {
	return Face{CardID: c.ID, Name: c.Name, KindKey: c.KindKey, Words: c.Words, Picture: c.Picture, Corner: c.Corner, Rare: c.Rare}
}

// playerCopy is a copy as a player may know it: the face only when it is
// face up.
func playerCopy(c Copy, cards map[string]Card) PlayerCopy {
	out := PlayerCopy{ID: c.ID, DeckID: c.DeckID, FaceUp: c.FaceUp}
	// Every face-down copy carries exactly the same fields and values, so
	// nothing about it varies with which card it is: the waiting mark can
	// only be set by a play of an ask-first card, which a player makes face up.
	if card, ok := cards[c.CardID]; ok && c.FaceUp {
		f := faceOf(card)
		out.Face = &f
		out.Waiting = c.AskedAt != nil
		out.Open = c.Open
	}
	return out
}

// othersSeeFace says whether a player who does not hold a copy sees its face.
// In a hand that takes the holder's second step, showing it: turning a card
// over is private, so a turned card and an unturned one are the same to
// everyone else. A card played in front of a hero is on the table, so face up
// is enough.
func othersSeeFace(d Deck, c Copy) bool {
	if d.Settings.OthersSee != "face" {
		return false
	}
	if c.Zone == ZoneHand {
		return c.Open && c.FaceUp
	}
	return c.FaceUp
}

func (r *request) playerDeck(d Deck) (PlayerDeck, error) {
	cards, err := r.s.repo.ListCards(r.ctx, r.cid, d.ID)
	if err != nil {
		return PlayerDeck{}, apperrorInternal(err)
	}
	copies, err := r.s.repo.ListCopies(r.ctx, r.cid, d.ID)
	if err != nil {
		return PlayerDeck{}, apperrorInternal(err)
	}
	byID := make(map[string]Card, len(cards))
	for _, c := range cards {
		byID[c.ID] = c
	}
	out := PlayerDeck{ID: d.ID, Name: d.Name, Kind: d.Kind, Look: d.Look, Hands: []PlayerHand{}}

	if p, ok, err := r.chosen(d); err != nil {
		return PlayerDeck{}, err
	} else if ok {
		out.DrawsLeft = p.DrawsLeft
		out.CanDraw = d.Settings.Draws == "players" && (p.DrawsLeft == nil || *p.DrawsLeft > 0)
	}

	owned, err := r.ownedHeroes()
	if err != nil {
		return PlayerDeck{}, err
	}
	mine := map[string]*PlayerHand{}
	for _, h := range owned {
		out.Hands = append(out.Hands, PlayerHand{HeroID: h.ID, Hand: []PlayerCopy{}, Play: []PlayerCopy{}})
	}
	for i := range out.Hands {
		mine[out.Hands[i].HeroID] = &out.Hands[i]
	}

	// Copies are always listed by when they arrived where they are, then by
	// id, never by anything that comes from which card they are.
	sort.SliceStable(copies, func(i, j int) bool {
		if !copies[i].MovedAt.Equal(copies[j].MovedAt) {
			return copies[i].MovedAt.Before(copies[j].MovedAt)
		}
		return copies[i].ID < copies[j].ID
	})

	// Other heroes' holds are only for heroes the viewer may open.
	viewable := map[string]bool{}
	if d.Settings.OthersSee != "nothing" {
		for _, c := range copies {
			if (c.Zone != ZoneHand && c.Zone != ZonePlay) || c.HolderEntityID == "" || mine[c.HolderEntityID] != nil {
				continue
			}
			if _, done := viewable[c.HolderEntityID]; done {
				continue
			}
			ok, err := r.s.heroes.Viewable(r.ctx, r.cid, c.HolderEntityID, r.v)
			if err != nil {
				return PlayerDeck{}, apperrorInternal(err)
			}
			viewable[c.HolderEntityID] = ok
		}
	}

	pile := 0
	unseen := map[string]int{}
	others := map[string][]Copy{}
	for _, c := range copies {
		// seen is whether this player is shown the copy's face. Every other
		// copy counts toward its kind's total, so no move that only hides or
		// shows a card to someone else changes a count.
		seen := false
		switch c.Zone {
		case ZonePile:
			pile++
		case ZoneHand, ZonePlay:
			switch {
			case inPartyHand(d, c):
				// The party's hand belongs to every member, so a face-up card in
				// it is seen by all of them, and a face-down one is a face-down
				// entry like any other.
				out.PartyHand = append(out.PartyHand, playerCopy(c, byID))
				seen = c.FaceUp
			case c.HolderEntityID == "":
				seen = c.Zone == ZonePlay && c.FaceUp
			case mine[c.HolderEntityID] != nil:
				seen = c.FaceUp
				h := mine[c.HolderEntityID]
				if c.Zone == ZoneHand {
					h.Hand = append(h.Hand, playerCopy(c, byID))
				} else {
					h.Play = append(h.Play, playerCopy(c, byID))
				}
			default:
				if viewable[c.HolderEntityID] {
					others[c.HolderEntityID] = append(others[c.HolderEntityID], c)
					seen = othersSeeFace(d, c)
				}
			}
		case ZoneUsed:
			if d.Kind == KindParty {
				out.Used = append(out.Used, playerCopy(c, byID))
				seen = c.FaceUp
			}
		}
		if !seen {
			unseen[byID[c.CardID].KindKey]++
		}
	}

	if d.Settings.Knows != "nothing" {
		out.Pile = &pile
	}
	if d.Settings.Knows == "kinds" || d.Settings.Knows == "faces" {
		keys := make([]string, 0, len(unseen))
		for k := range unseen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.Kinds = []KindCount{}
		for _, k := range keys {
			out.Kinds = append(out.Kinds, KindCount{KindKey: k, Count: unseen[k]})
		}
	}
	if d.Settings.Knows == "faces" {
		out.Cards = []Face{}
		for _, c := range cards {
			out.Cards = append(out.Cards, faceOf(c))
		}
	}
	if d.Settings.OthersSee != "nothing" {
		ids := make([]string, 0, len(others))
		for id := range others {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			oh := OtherHolds{HeroID: id, Count: len(others[id])}
			if d.Settings.OthersSee == "face" {
				for _, c := range others[id] {
					if pc := playerCopy(c, byID); pc.Face != nil && othersSeeFace(d, c) {
						oh.Faces = append(oh.Faces, *pc.Face)
					}
				}
			}
			out.Others = append(out.Others, oh)
		}
	}
	return out, nil
}

func (s *service) Table(ctx context.Context, campaignID string, v Viewer) (*TableView, error) {
	r := s.newRequest(ctx, campaignID, v)
	if !r.member() {
		return nil, errNotFound("table")
	}
	rows, err := s.repo.ListTable(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &TableView{Decks: []TableDeck{}}
	index := map[string]int{}
	for _, row := range rows {
		i, ok := index[row.DeckID]
		if !ok {
			i = len(out.Decks)
			index[row.DeckID] = i
			out.Decks = append(out.Decks, TableDeck{DeckID: row.DeckID, Name: row.DeckName, Kind: row.DeckKind,
				Look: row.Look, Cards: []TableCard{}})
		}
		out.Decks[i].Cards = append(out.Decks[i].Cards, TableCard{CopyID: row.CopyID, Face: row.Face})
	}
	return out, nil
}

// ---- decks -----------------------------------------------------------------

// checkHolder makes sure a hero deck's page is a live page of this campaign.
func (s *service) checkHolder(ctx context.Context, campaignID, kind, holder string) error {
	if holder == "" {
		return nil
	}
	if kind != KindHero {
		return errInvalid("only a hero deck belongs to a hero page")
	}
	if !validToken(holder) {
		return errInvalid("that hero page is not in this campaign")
	}
	_, ok, err := s.heroes.InCampaign(ctx, campaignID, holder)
	if err != nil {
		return apperrorInternal(err)
	}
	if !ok {
		return errInvalid("that hero page is not in this campaign")
	}
	return nil
}

func (s *service) Create(ctx context.Context, campaignID string, v Viewer, in CreateDeckInput) (DeckView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	name, err := plainText("the deck's name", in.Name, MaxNameChars)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errInvalid("a deck needs a name")
	}
	if err := validKind(in.Kind); err != nil {
		return nil, err
	}
	if err := s.checkHolder(ctx, campaignID, in.Kind, in.HolderEntityID); err != nil {
		return nil, err
	}
	settings, err := applySettings(defaultSettings(in.Kind), in.Settings)
	if err != nil {
		return nil, err
	}
	look, err := applyLook(defaultLook(), in.Look)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.CountDecks(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if n >= MaxDecksPerCampaign {
		return nil, errInvalid(fmt.Sprintf("a campaign can have at most %d decks", MaxDecksPerCampaign))
	}
	d := Deck{ID: uuid.NewString(), CampaignID: campaignID, Name: name, Kind: in.Kind, HolderEntityID: in.HolderEntityID,
		Settings: settings, Look: look, SortOrder: n}
	if err := s.repo.CreateDeck(ctx, d); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errTaken("deck")
		}
		return nil, apperrorInternal(err)
	}
	return s.reread(ctx, campaignID, d.ID, v)
}

// reread returns the stored deck as the viewer now sees it.
func (s *service) reread(ctx context.Context, campaignID, deckID string, v Viewer) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.deck(deckID)
	if err != nil {
		return nil, err
	}
	return r.view(d)
}

func (s *service) Update(ctx context.Context, campaignID, deckID string, v Viewer, in UpdateDeckInput) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	cur, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	ver, ok := in.Version.Get()
	if !ok {
		return nil, errInvalid("version is required: send the version you loaded")
	}
	if ver != cur.Version {
		return nil, errStale()
	}
	next := cur
	rawName := in.Name.Val(cur.Name)
	if next.Name, err = plainText("the deck's name", rawName, MaxNameChars); err != nil {
		return nil, err
	}
	if next.Name == "" {
		return nil, errInvalid("a deck needs a name")
	}
	next.Kind = in.Kind.Val(cur.Kind)
	if err := validKind(next.Kind); err != nil {
		return nil, err
	}
	if cur.Kind == KindParty && next.Kind != KindParty {
		// A card in the party's hand has no holder, so in any other kind of
		// deck nobody could see or play it.
		copies, err := s.repo.ListCopies(ctx, campaignID, cur.ID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, c := range copies {
			if c.Zone == ZoneHand && c.HolderEntityID == "" {
				return nil, conflict("take the party's cards back before changing who this deck is for")
			}
		}
	}
	var curHolder *string
	if cur.HolderEntityID != "" {
		curHolder = &cur.HolderEntityID
	}
	next.HolderEntityID = ""
	if h := in.HolderEntityID.Ptr(curHolder); h != nil {
		next.HolderEntityID = *h
	}
	if err := s.checkHolder(ctx, campaignID, next.Kind, next.HolderEntityID); err != nil {
		return nil, err
	}
	if sv, ok := in.Settings.Get(); ok {
		if next.Settings, err = applySettings(cur.Settings, &sv); err != nil {
			return nil, err
		}
	}
	if lv, ok := in.Look.Get(); ok {
		if next.Look, err = applyLook(cur.Look, &lv); err != nil {
			return nil, err
		}
		cards, err := s.repo.ListCards(ctx, campaignID, cur.ID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, c := range cards {
			if c.KindKey != "" && !hasKind(next.Look, c.KindKey) {
				return nil, errInvalid(fmt.Sprintf("the card %q still uses the kind %q; change the card first", c.Name, c.KindKey))
			}
		}
	}
	picks, err := s.checkPicks(ctx, campaignID, cur, in)
	if err != nil {
		return nil, err
	}
	okSaved, err := s.repo.UpdateDeck(ctx, next, ver, picks)
	if errors.Is(err, errDuplicate) {
		return nil, errTaken("deck")
	}
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !okSaved {
		return nil, errStale()
	}
	return s.reread(ctx, campaignID, deckID, v)
}

// checkPicks reads the Director's next and last card from an update. Absent
// keeps, null or "" clears, and a value must be a card of this deck.
func (s *service) checkPicks(ctx context.Context, campaignID string, cur Deck, in UpdateDeckInput) (PickChange, error) {
	var pc PickChange
	one := func(f patch.Field[string], set *bool, dst *string, label string) error {
		if !f.Present() {
			return nil
		}
		*set = true
		// An explicit null and "" both clear.
		v, ok := f.Get()
		if !ok || v == "" {
			return nil
		}
		if !validToken(v) {
			return errInvalid("the " + label + " card is not a card of this deck")
		}
		if _, found, err := s.repo.GetCard(ctx, campaignID, cur.ID, v); err != nil {
			return apperrorInternal(err)
		} else if !found {
			return errInvalid("the " + label + " card is not a card of this deck")
		}
		*dst = v
		return nil
	}
	if err := one(in.NextCardID, &pc.SetNext, &pc.Next, "next"); err != nil {
		return PickChange{}, err
	}
	if err := one(in.LastCardID, &pc.SetLast, &pc.Last, "last"); err != nil {
		return PickChange{}, err
	}
	return pc, nil
}

func (s *service) Delete(ctx context.Context, campaignID, deckID string, v Viewer) error {
	r := s.newRequest(ctx, campaignID, v)
	if _, err := r.directorDeck(deckID); err != nil {
		return err
	}
	if err := s.repo.DeleteDeck(ctx, campaignID, deckID); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

func (s *service) SetPlayers(ctx context.Context, campaignID, deckID string, v Viewer, in SetPlayersInput) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	cur, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	if in.Version != cur.Version {
		return nil, errStale()
	}
	if len(in.Players) > MaxPlayersChosen {
		return nil, errInvalid("too many players chosen")
	}
	list, err := s.people.Members(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	members := make(map[string]Member, len(list))
	for _, m := range list {
		members[m.UserID] = m
	}
	existing := map[string]Player{}
	for _, p := range cur.Players {
		existing[p.UserID] = p
	}
	now := time.Now().UTC().Truncate(time.Second)
	seen := map[string]bool{}
	players := make([]Player, 0, len(in.Players))
	for _, p := range in.Players {
		m, ok := members[p.UserID]
		if !validToken(p.UserID) || !ok || m.Role < permissions.RolePlayer {
			return nil, errInvalid("every chosen player must be a member of this campaign")
		}
		if m.Director {
			return nil, errInvalid("a Director already sees every deck and cannot be chosen as a player")
		}
		if seen[p.UserID] {
			return nil, errInvalid("a player is chosen twice")
		}
		seen[p.UserID] = true
		if p.DrawsLeft != nil && (*p.DrawsLeft < 0 || *p.DrawsLeft > MaxDraws) {
			return nil, errInvalid(fmt.Sprintf("draws left must be between 0 and %d, or left empty for no limit", MaxDraws))
		}
		granted := now
		// A grant still counting from this stay keeps its time, so saving the
		// list again never re-dates it; a stale one starts over.
		if old, had := existing[p.UserID]; had && !old.GrantedAt.Before(m.JoinedAt) {
			granted = old.GrantedAt
		}
		players = append(players, Player{UserID: p.UserID, DrawsLeft: p.DrawsLeft, GrantedAt: granted})
	}
	saved, err := s.repo.SetPlayers(ctx, campaignID, deckID, players, in.Version)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !saved {
		return nil, errStale()
	}
	return s.reread(ctx, campaignID, deckID, v)
}

// ---- cards -----------------------------------------------------------------

// checkCard validates a whole card against its deck and normalizes its text.
func checkCard(d Deck, c *Card) error {
	var err error
	if c.Name, err = plainText("the card's name", c.Name, MaxNameChars); err != nil {
		return err
	}
	if c.Name == "" {
		return errInvalid("a card needs a name")
	}
	if c.KindKey != "" && !hasKind(d.Look, c.KindKey) {
		return errInvalid("that card kind is not one of the deck's kinds")
	}
	if c.Words, err = longText("the card's words", c.Words, MaxWordsChars); err != nil {
		return err
	}
	if c.DirectorNote, err = longText("the Director's note", c.DirectorNote, MaxNoteChars); err != nil {
		return err
	}
	// TODO(#2567): a card's picture is accepted once the workshop can upload one.
	if c.Picture != "" {
		return errInvalid("a card can't have a picture yet")
	}
	if c.Corner, err = plainText("the card's corner", c.Corner, MaxCornerChars); err != nil {
		return err
	}
	if c.Copies < 1 || c.Copies > MaxCopiesPerCard {
		return errInvalid(fmt.Sprintf("copies must be between 1 and %d", MaxCopiesPerCard))
	}
	if c.Fate != "" {
		if err := oneOf("fate", c.Fate, fateValues); err != nil {
			return err
		}
	}
	// TODO(#2569): a card's condition is accepted once conditions exist.
	if c.When != "" {
		return errInvalid("a card can't have a condition yet")
	}
	return nil
}

// otherCopies sums the copies of every card but one.
func otherCopies(cards []Card, except string) int {
	n := 0
	for _, c := range cards {
		if c.ID != except {
			n += c.Copies
		}
	}
	return n
}

func (s *service) CreateCard(ctx context.Context, campaignID, deckID string, v Viewer, in CreateCardInput) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	copies := 1
	if in.Copies != nil {
		copies = *in.Copies
	}
	c := Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: campaignID, Name: in.Name, KindKey: in.KindKey,
		Words: in.Words, DirectorNote: in.DirectorNote, Picture: in.Picture, Corner: in.Corner, Rare: in.Rare, Copies: copies,
		Fate: in.Fate, When: in.When, WhenOpen: in.WhenOpen, AskFirst: in.AskFirst}
	if err := checkCard(d, &c); err != nil {
		return nil, err
	}
	cards, err := s.repo.ListCards(ctx, campaignID, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(cards) >= MaxCardsPerDeck {
		return nil, errInvalid(fmt.Sprintf("a deck can have at most %d cards", MaxCardsPerDeck))
	}
	if otherCopies(cards, "")+c.Copies > MaxCopiesPerDeck {
		return nil, errInvalid(fmt.Sprintf("a deck can have at most %d copies in all", MaxCopiesPerDeck))
	}
	c.SortOrder = len(cards)
	if err := s.repo.CreateCard(ctx, c); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errTaken("card")
		}
		return nil, apperrorInternal(err)
	}
	return r.view(d)
}

func (s *service) UpdateCard(ctx context.Context, campaignID, deckID, cardID string, v Viewer, in UpdateCardInput) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	if !validToken(cardID) {
		return nil, errNotFound("card")
	}
	c, found, err := s.repo.GetCard(ctx, campaignID, d.ID, cardID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("card")
	}
	c.Name = in.Name.Val(c.Name)
	c.KindKey = in.KindKey.Val(c.KindKey)
	c.Words = in.Words.Val(c.Words)
	c.DirectorNote = in.DirectorNote.Val(c.DirectorNote)
	c.Picture = in.Picture.Val(c.Picture)
	c.Corner = in.Corner.Val(c.Corner)
	c.Rare = in.Rare.Val(c.Rare)
	c.Copies = in.Copies.Val(c.Copies)
	c.Fate = in.Fate.Val(c.Fate)
	c.When = in.When.Val(c.When)
	c.WhenOpen = in.WhenOpen.Val(c.WhenOpen)
	c.AskFirst = in.AskFirst.Val(c.AskFirst)
	if err := checkCard(d, &c); err != nil {
		return nil, err
	}
	cards, err := s.repo.ListCards(ctx, campaignID, d.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if otherCopies(cards, c.ID)+c.Copies > MaxCopiesPerDeck {
		return nil, errInvalid(fmt.Sprintf("a deck can have at most %d copies in all", MaxCopiesPerDeck))
	}
	if _, err := s.repo.UpdateCard(ctx, c); err != nil {
		var held *heldError
		switch {
		case errors.As(err, &held):
			return nil, errInvalid(fmt.Sprintf("%d %s of this card %s held in hands or in play; take them back before lowering the copies",
				held.N, plural(held.N, "copy", "copies"), plural(held.N, "is", "are")))
		case errors.Is(err, errDuplicate):
			return nil, errTaken("card")
		case errors.Is(err, errLostClaim):
			return nil, errNotFound("deck")
		}
		return nil, apperrorInternal(err)
	}
	return r.view(d)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *service) DeleteCard(ctx context.Context, campaignID, deckID, cardID string, v Viewer) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	if !validToken(cardID) {
		return nil, errNotFound("card")
	}
	if _, found, err := s.repo.GetCard(ctx, campaignID, d.ID, cardID); err != nil {
		return nil, apperrorInternal(err)
	} else if !found {
		return nil, errNotFound("card")
	}
	if err := s.repo.DeleteCard(ctx, campaignID, d.ID, cardID); err != nil {
		return nil, apperrorInternal(err)
	}
	return r.view(d)
}
