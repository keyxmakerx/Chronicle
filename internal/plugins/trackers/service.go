package trackers

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Service owns the tracker rules. Only Directors define kinds, place, rename,
// hide, strike or remove trackers, and see hidden ones. A player sees the
// trackers marked for everyone and those that name them, on pages they can
// open, without the log's notes except their own; a player named with "may
// change it" may also log add and set lines.
type Service interface {
	// Page is every tracker on a page as v may see it, with the kinds a
	// Director can add.
	Page(ctx context.Context, campaignID, entityID string, v Viewer) (*PageView, error)
	// OwnedPage is the trackers on a page from the kinds owned under prefix
	// (one recipe's), with a virtual tracker for each such kind not placed
	// yet: the block that shows them is their placement. It offers no kinds
	// to add.
	OwnedPage(ctx context.Context, campaignID, entityID, prefix string, v Viewer) (*PageView, error)
	Place(ctx context.Context, campaignID, entityID string, v Viewer, in PlaceInput) (*TrackerView, error)
	Patch(ctx context.Context, campaignID, trackerID string, v Viewer, in TrackerPatch) (*TrackerView, error)
	Remove(ctx context.Context, campaignID, trackerID string, v Viewer) error
	// Log writes one change on a tracker of the page and returns the
	// tracker as it now stands.
	Log(ctx context.Context, campaignID, entityID string, v Viewer, in LogInput) (*TrackerView, error)
	// Strike strikes a log line, or restores it when struck is false.
	Strike(ctx context.Context, campaignID string, entryID int64, struck bool, v Viewer) (*TrackerView, error)

	Kinds(ctx context.Context, campaignID string, v Viewer) ([]Kind, []TypeRef, error)
	CreateKind(ctx context.Context, campaignID string, v Viewer, in KindInput) (*Kind, error)
	UpdateKind(ctx context.Context, campaignID, kindID string, v Viewer, in KindInput) (*Kind, error)
	DeleteKind(ctx context.Context, campaignID, kindID string, v Viewer) error

	// CheckOwned says, without writing, whether SyncOwned would refuse want.
	CheckOwned(ctx context.Context, campaignID, prefix string, want []OwnedKind) error
	// SyncOwned makes the kinds owned under prefix match want. Its caller
	// has already decided the change is the Director's to make.
	SyncOwned(ctx context.Context, campaignID, prefix string, want []OwnedKind) error

	// LogOwned writes one change, marked as a rule's, on the trackers of
	// the kind owned as owner: on entityID's page, creating its row if
	// needed, or with entityID "" on every page that already has one.
	LogOwned(ctx context.Context, campaignID, entityID, owner string, v Viewer, op string, amount int, note string) ([]TrackerView, error)
	// Shared lists the placed trackers shown to everyone, on pages a plain
	// player may open, for the campaign home's clocks block. It reads as a
	// player whoever v is: a Director's hidden trackers and hidden pages
	// never appear, so the owner sees what the table sees.
	Shared(ctx context.Context, campaignID string, v Viewer) ([]SharedTracker, error)
	// Pages lists the pages that have trackers, by name, for the DM
	// Screen's Homebrew drawer.
	Pages(ctx context.Context, campaignID string, v Viewer) ([]PageRef, error)
}

// Change is a tracker of an owned kind moving from one value to another.
type Change struct {
	Owner    string
	EntityID string
	From, To int
}

// ChangeListener hears that a tracker of an owned kind moved, after the
// change is saved, so its owner can act on it (a recipe's rules run).
type ChangeListener interface {
	TrackerChanged(ctx context.Context, campaignID string, v Viewer, c Change) error
}

// ChangeObserver hears that something a tracker-reading rule may judge has
// changed on a campaign: a value, a placement, a kind's bounds or name. Unlike
// ChangeListener it is told for every kind and carries nothing but the
// campaign, so the observer must look again rather than trust the event. It is
// told after the change is saved and must not block.
type ChangeObserver interface {
	TrackersChanged(campaignID string)
}

// PageRef is a page that has trackers.
type PageRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type service struct {
	repo     Repository
	entities EntityDirectory
	members  MemberDirectory
	clock    GameClock
	listener ChangeListener
	observer ChangeObserver
	newID    func() string
}

// NewService builds the service. clock may be nil when the calendar plugin is
// unavailable; log lines are then undated. members may be nil, in which case
// no player can be chosen and no log line is named.
func NewService(repo Repository, entities EntityDirectory, members MemberDirectory, clock GameClock) Service {
	if members != nil {
		repo = &grantFilter{Repository: repo, members: members}
	}
	return &service{repo: repo, entities: entities, members: members, clock: clock, newID: func() string { return uuid.NewString() }}
}

func requireDirector(v Viewer) error {
	if !v.Director {
		return errForbidden("only the campaign's Directors can change trackers")
	}
	return nil
}

// page decides that a page exists in the campaign and v may see it. A hidden
// page is reported exactly like a missing one.
func (s *service) page(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	if !validToken(entityID) {
		return PageAccess{}, errNotFound("page")
	}
	a, err := s.entities.Access(ctx, campaignID, entityID, v)
	if err != nil {
		return PageAccess{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return PageAccess{}, errNotFound("page")
	}
	return a, nil
}

// sees reports whether v may see tracker t. A "players" tracker is seen only
// by a current member named on it: the member check keeps a former member, or
// a visitor to a public campaign, from matching a row left behind.
func sees(v Viewer, t Tracker) bool {
	if v.Director || t.Visibility == SeeEveryone {
		return true
	}
	_, ok := chosen(v, t)
	return ok
}

// seesKind is sees for a tracker no row exists for yet: a kind's own default,
// which is never "players" because that needs a list of players.
func seesKind(v Viewer, visibility string) bool { return v.Director || visibility == SeeEveryone }

// chosen finds v among a "players" tracker's players.
func chosen(v Viewer, t Tracker) (Player, bool) {
	if t.Visibility != SeePlayers || v.UserID == "" || v.MemberRole < permissions.RolePlayer {
		return Player{}, false
	}
	for _, p := range t.Players {
		if p.UserID == v.UserID {
			return p, true
		}
	}
	return Player{}, false
}

// canChange says whether v may log add and set lines on t. It assumes v
// already sees t.
func canChange(v Viewer, t Tracker) bool {
	if v.Director {
		return true
	}
	p, ok := chosen(v, t)
	return ok && p.CanChange
}

// roster is the campaign's members by user id.
type roster map[string]Member

// loadRoster reads the members when the answer needs names: always for a
// Director (names on log lines and the picker), and for anyone else only when
// one of ts is a "players" tracker.
func (s *service) loadRoster(ctx context.Context, campaignID string, v Viewer, ts []Tracker) (roster, error) {
	need := v.Director
	for _, t := range ts {
		need = need || t.Visibility == SeePlayers
	}
	if !need || s.members == nil {
		return roster{}, nil
	}
	list, err := s.members.Members(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ro := make(roster, len(list))
	for _, m := range list {
		ro[m.UserID] = m
	}
	return ro, nil
}

// view builds what v sees of one tracker. entries are oldest first.
func view(k Kind, t Tracker, entries []Entry, v Viewer, ro roster) TrackerView {
	value, after := fold(k, entries)
	tv := TrackerView{
		ID: t.ID, KindID: k.ID, Name: t.Name, Shape: k.Shape, Visibility: t.Visibility,
		Value: value, Min: k.Min, Max: k.Max, Steps: k.Steps, Bands: k.Bands,
		Now: nowText(k, value), LogCount: len(entries), Log: []EntryView{},
		CanChange: canChange(v, t),
	}
	if t.Visibility == SeePlayers {
		for _, p := range t.Players {
			m, ok := ro[p.UserID]
			switch {
			case !ok:
				// No longer a member: nothing to show or to match.
			case v.Director:
				tv.Players = append(tv.Players, PlayerView{UserID: p.UserID, Name: m.Name, CanChange: p.CanChange})
			case p.UserID != v.UserID:
				tv.AlsoSeenBy = append(tv.AlsoSeenBy, m.Name)
			}
		}
	}
	for i := len(entries) - 1; i >= 0 && len(tv.Log) < LogShown; i-- {
		e := entries[i]
		ev := EntryView{ID: e.ID, Op: e.Op, Amount: e.Amount, Source: e.Source, At: e.CreatedAt,
			Struck: e.StruckAt != nil, After: after[i]}
		// Notes are the DM's working words; a player gets the change and its
		// date, plus their own lines in full. Only hand lines are anyone's own:
		// a rule's line carries the player who set it off, but is the rule's.
		switch {
		case v.Director:
			ev.Note = e.Note
			if m, ok := ro[e.CreatedBy]; ok && e.Source == SourceHand && !m.Director && e.CreatedBy != v.UserID {
				ev.By = m.Name
			}
		case e.Source == SourceHand && e.CreatedBy != "" && e.CreatedBy == v.UserID:
			ev.Note, ev.By = e.Note, "You"
		}
		if e.Game != nil {
			ev.When = e.Game.Label
		}
		tv.Log = append(tv.Log, ev)
	}
	return tv
}

func (s *service) kindMap(ctx context.Context, campaignID string) (map[string]Kind, []Kind, error) {
	list, err := s.repo.ListKinds(ctx, campaignID)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	m := make(map[string]Kind, len(list))
	for _, k := range list {
		m[k.ID] = k
	}
	return m, list, nil
}

func (s *service) Page(ctx context.Context, campaignID, entityID string, v Viewer) (*PageView, error) {
	return s.pageView(ctx, campaignID, entityID, "", v)
}

func (s *service) OwnedPage(ctx context.Context, campaignID, entityID, prefix string, v Viewer) (*PageView, error) {
	if !validPrefix(prefix) {
		return nil, errNotFound("trackers")
	}
	return s.pageView(ctx, campaignID, entityID, prefix, v)
}

// pageView builds a page's trackers: every one when prefix is empty, else
// only those of kinds owned under prefix.
func (s *service) pageView(ctx context.Context, campaignID, entityID, prefix string, v Viewer) (*PageView, error) {
	a, err := s.page(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	kinds, list, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	placed, err := s.repo.ListTrackers(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ts := make([]Tracker, 0, len(placed))
	hasKind, hasName := map[string]bool{}, map[string]bool{}
	for _, t := range placed {
		hasKind[t.KindID], hasName[t.Name] = true, true
		k, ok := kinds[t.KindID]
		if ok && sees(v, t) && (prefix == "" || ownedBy(k, prefix)) {
			ts = append(ts, t)
		}
	}
	// Kinds that go on every page of this category, or that the block names,
	// show here before anyone has changed them; the first change creates the
	// row.
	for _, k := range list {
		if !virtualOn(k, a.TypeID, prefix) || hasKind[k.ID] || hasName[k.Name] {
			continue
		}
		if seesKind(v, k.Visibility) {
			ts = append(ts, Tracker{KindID: k.ID, Name: k.Name, Visibility: k.Visibility, SortOrder: 1 << 30})
		}
	}
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		if t.ID != "" {
			ids = append(ids, t.ID)
		}
	}
	entries, err := s.repo.ListEntries(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ro, err := s.loadRoster(ctx, campaignID, v, ts)
	if err != nil {
		return nil, err
	}
	out := &PageView{Trackers: []TrackerView{}, CanEdit: v.Director}
	for _, t := range ts {
		out.Trackers = append(out.Trackers, view(kinds[t.KindID], t, entries[t.ID], v, ro))
	}
	if v.Director {
		out.Members = pickable(ro)
	}
	if v.Director && prefix == "" {
		out.Kinds = []KindChoice{}
		for _, k := range list {
			out.Kinds = append(out.Kinds, KindChoice{ID: k.ID, Name: k.Name, Shape: k.Shape, Visibility: k.Visibility})
		}
	}
	return out, nil
}

// pickable is the members a Director can choose for a tracker: everyone but
// the Directors, by name.
func pickable(ro roster) []MemberChoice {
	out := []MemberChoice{}
	for _, m := range ro {
		if !m.Director {
			out = append(out, MemberChoice{UserID: m.UserID, Name: m.Name, Character: m.Character})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

// ownedBy says whether k is one of the kinds owned under prefix.
func ownedBy(k Kind, prefix string) bool {
	return prefix != "" && strings.HasPrefix(k.Owner, prefix)
}

// virtualOn says whether k shows on a page of typeID before it is placed:
// for the owned view, every kind owned under prefix; otherwise a kind set
// to go on every page of that category.
func virtualOn(k Kind, typeID int, prefix string) bool {
	if prefix != "" {
		return ownedBy(k, prefix)
	}
	return k.EveryTypeID != 0 && k.EveryTypeID == typeID
}

// one returns a single tracker's view after a change.
func (s *service) one(ctx context.Context, campaignID string, t Tracker, v Viewer) (*TrackerView, error) {
	k, ok, err := s.repo.GetKind(ctx, campaignID, t.KindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("tracker kind")
	}
	entries, err := s.repo.ListEntries(ctx, campaignID, []string{t.ID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ro, err := s.loadRoster(ctx, campaignID, v, []Tracker{t})
	if err != nil {
		return nil, err
	}
	tv := view(k, t, entries[t.ID], v, ro)
	return &tv, nil
}

func (s *service) Place(ctx context.Context, campaignID, entityID string, v Viewer, in PlaceInput) (*TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	if _, err := s.page(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	if !validToken(in.KindID) {
		return nil, errInvalid("pick a tracker kind")
	}
	k, ok, err := s.repo.GetKind(ctx, campaignID, in.KindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errInvalid("pick a tracker kind")
	}
	if in.Name == "" {
		in.Name = k.Name
	}
	name, err := cleanName("name", in.Name)
	if err != nil {
		return nil, err
	}
	if in.Visibility == "" {
		in.Visibility = k.Visibility
	}
	if !validVisibility(in.Visibility) {
		return nil, errInvalid("visibility must be directors or everyone; choose players on the tracker once it is placed")
	}
	existing, err := s.repo.ListTrackers(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(existing) >= MaxTrackersPage {
		return nil, errInvalid("a page can hold at most 40 trackers")
	}
	t := Tracker{ID: s.newID(), CampaignID: campaignID, EntityID: entityID, KindID: k.ID, Name: name, Visibility: in.Visibility}
	if err := s.repo.CreateTracker(ctx, t); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errInvalid("this page already has a tracker called " + name)
		}
		return nil, apperrorInternal(err)
	}
	s.touched(campaignID)
	return s.one(ctx, campaignID, t, v)
}

// tracker finds a placed tracker on a page v may see.
func (s *service) tracker(ctx context.Context, campaignID, trackerID string, v Viewer) (Tracker, error) {
	if !validToken(trackerID) {
		return Tracker{}, errNotFound("tracker")
	}
	t, ok, err := s.repo.GetTracker(ctx, campaignID, trackerID)
	if err != nil {
		return Tracker{}, apperrorInternal(err)
	}
	if !ok || !sees(v, t) {
		return Tracker{}, errNotFound("tracker")
	}
	if _, err := s.page(ctx, campaignID, t.EntityID, v); err != nil {
		return Tracker{}, errNotFound("tracker")
	}
	return t, nil
}

func (s *service) Patch(ctx context.Context, campaignID, trackerID string, v Viewer, in TrackerPatch) (*TrackerView, error) {
	// Find the tracker first: a player who cannot see it learns nothing,
	// while one who can is told it is not theirs to change.
	t, err := s.tracker(ctx, campaignID, trackerID, v)
	if err != nil {
		return nil, err
	}
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	wasPlayers := t.Visibility == SeePlayers
	if in.Name != nil {
		name, err := cleanName("name", *in.Name)
		if err != nil {
			return nil, err
		}
		t.Name = name
	}
	if in.Visibility != nil {
		if !validTrackerVisibility(*in.Visibility) {
			return nil, errInvalid("visibility must be directors, everyone or players")
		}
		t.Visibility = *in.Visibility
	}
	setPlayers := false
	switch {
	case in.Players != nil && t.Visibility != SeePlayers:
		return nil, errInvalid("choose players as the visibility before naming them")
	case t.Visibility == SeePlayers && in.Players != nil:
		picks, err := s.checkPlayers(ctx, campaignID, *in.Players, t.Players)
		if err != nil {
			return nil, err
		}
		t.Players, setPlayers = picks, true
	case t.Visibility == SeePlayers && !wasPlayers:
		// Newly "players" with nobody named: only the Directors see it.
		t.Players, setPlayers = nil, true
	case t.Visibility != SeePlayers:
		t.Players = nil
	}
	// Every refusal comes before the first write, so a rejected rename never
	// leaves a replaced player list behind.
	if in.Name != nil {
		siblings, err := s.repo.ListTrackers(ctx, campaignID, t.EntityID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, o := range siblings {
			if o.ID != t.ID && strings.EqualFold(o.Name, t.Name) {
				return nil, errInvalid("this page already has a tracker called " + t.Name)
			}
		}
	}
	// Players go in before the visibility opens to them, and out after it
	// closes, so no step leaves a tracker visible to the wrong people.
	if setPlayers {
		if err := s.repo.SetPlayers(ctx, campaignID, t.ID, t.Players); err != nil {
			return nil, apperrorInternal(err)
		}
	}
	if err := s.repo.UpdateTracker(ctx, t); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errInvalid("this page already has a tracker called " + t.Name)
		}
		return nil, apperrorInternal(err)
	}
	if wasPlayers && t.Visibility != SeePlayers {
		if err := s.repo.SetPlayers(ctx, campaignID, t.ID, nil); err != nil {
			return nil, apperrorInternal(err)
		}
	}
	s.touched(campaignID)
	return s.one(ctx, campaignID, t, v)
}

// checkPlayers turns a Director's picks into the stored list. Each must be a
// current member who is not a Director: a Director already sees everything,
// and an unknown id must not be stored as if it were a person.
func (s *service) checkPlayers(ctx context.Context, campaignID string, picks []PlayerPick, current []Player) ([]Player, error) {
	if len(picks) > MaxPlayersChosen {
		return nil, errInvalid("too many players chosen")
	}
	if len(picks) == 0 {
		return nil, nil
	}
	ro, err := s.loadRosterAll(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// A pick already in force keeps its grant time; a new or lapsed one is
	// granted now, so choosing a returning player again starts a fresh grant.
	since := make(map[string]time.Time, len(current))
	for _, c := range current {
		since[c.UserID] = c.GrantedAt
	}
	now := time.Now().UTC()
	out := make([]Player, 0, len(picks))
	at := map[string]int{}
	for _, p := range picks {
		m, ok := ro[p.UserID]
		if !ok {
			return nil, errInvalid("someone you chose is not in this campaign")
		}
		if m.Director {
			return nil, errInvalid(m.Name + " is a Director and already sees every tracker")
		}
		if i, dup := at[p.UserID]; dup {
			out[i].CanChange = p.CanChange
			continue
		}
		at[p.UserID] = len(out)
		granted, kept := since[p.UserID]
		if !kept {
			granted = now
		}
		out = append(out, Player{UserID: p.UserID, CanChange: p.CanChange, GrantedAt: granted})
	}
	return out, nil
}

// loadRosterAll reads the members regardless of who is asking, for a check
// the Director's write needs.
func (s *service) loadRosterAll(ctx context.Context, campaignID string) (roster, error) {
	return s.loadRoster(ctx, campaignID, Viewer{Director: true}, nil)
}

func (s *service) Remove(ctx context.Context, campaignID, trackerID string, v Viewer) error {
	t, err := s.tracker(ctx, campaignID, trackerID, v)
	if err != nil {
		return err
	}
	if err := requireDirector(v); err != nil {
		return err
	}
	if err := s.repo.DeleteTracker(ctx, campaignID, t.ID); err != nil {
		return apperrorInternal(err)
	}
	s.touched(campaignID)
	return nil
}

// ensure returns the tracker a change is for: a placed one by id, or the row
// for a kind that goes on every page of this page's category or is owned
// under the block's prefix, created on its first change.
func (s *service) ensure(ctx context.Context, campaignID, entityID string, a PageAccess, v Viewer, in LogInput) (Tracker, error) {
	if in.TrackerID != "" {
		t, err := s.tracker(ctx, campaignID, in.TrackerID, v)
		if err != nil {
			return Tracker{}, err
		}
		if t.EntityID != entityID {
			return Tracker{}, errNotFound("tracker")
		}
		return t, nil
	}
	if !validToken(in.KindID) {
		return Tracker{}, errNotFound("tracker")
	}
	k, ok, err := s.repo.GetKind(ctx, campaignID, in.KindID)
	if err != nil {
		return Tracker{}, apperrorInternal(err)
	}
	// A Director may place any kind on any page, so creating the row on a
	// first change only needs a reason the kind shows here unplaced.
	shows := virtualOn(k, a.TypeID, "") || (validPrefix(in.Owner) && ownedBy(k, in.Owner))
	if !ok || !shows {
		return Tracker{}, errNotFound("tracker")
	}
	return s.rowFor(ctx, campaignID, entityID, k)
}

// rowFor returns the page's tracker of kind k, creating it on first use.
func (s *service) rowFor(ctx context.Context, campaignID, entityID string, k Kind) (Tracker, error) {
	if t, found, err := s.repo.TrackerForKind(ctx, campaignID, entityID, k.ID); err != nil {
		return Tracker{}, apperrorInternal(err)
	} else if found {
		return t, nil
	}
	t := Tracker{ID: s.newID(), CampaignID: campaignID, EntityID: entityID, KindID: k.ID, Name: k.Name, Visibility: k.Visibility}
	if err := s.repo.CreateTracker(ctx, t); err != nil {
		if !errors.Is(err, errDuplicate) {
			return Tracker{}, apperrorInternal(err)
		}
		// Someone else's first change made it a moment ago.
		got, found, err := s.repo.TrackerForKind(ctx, campaignID, entityID, k.ID)
		if err != nil {
			return Tracker{}, apperrorInternal(err)
		}
		if !found {
			return Tracker{}, errInvalid("this page already has a tracker called " + k.Name)
		}
		return got, nil
	}
	return t, nil
}

func (s *service) Log(ctx context.Context, campaignID, entityID string, v Viewer, in LogInput) (*TrackerView, error) {
	a, err := s.page(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	if !v.Director {
		return s.playerLog(ctx, campaignID, entityID, v, in)
	}
	in, err = validateLog(in)
	if err != nil {
		return nil, err
	}
	t, err := s.ensure(ctx, campaignID, entityID, a, v, in)
	if err != nil {
		return nil, err
	}
	return s.writeLine(ctx, campaignID, t, v, in, SourceHand)
}

// playerLog is a non-Director's change. It works only on a placed tracker
// named by id (a player cannot make a row from a kind), found through sees so
// an unseen tracker answers 404, and only where the Director said "may change
// it". The line is the same add or set a Director's hand writes.
func (s *service) playerLog(ctx context.Context, campaignID, entityID string, v Viewer, in LogInput) (*TrackerView, error) {
	if in.TrackerID == "" {
		return nil, errForbidden("only the campaign's Directors can change trackers")
	}
	t, err := s.tracker(ctx, campaignID, in.TrackerID, v)
	if err != nil {
		return nil, err
	}
	if t.EntityID != entityID {
		return nil, errNotFound("tracker")
	}
	if !canChange(v, t) {
		return nil, errForbidden("you can look at this tracker but not change it")
	}
	in, err = validateLog(in)
	if err != nil {
		return nil, err
	}
	return s.writeLine(ctx, campaignID, t, v, in, SourceHand)
}

// writeLine adds one log line to a tracker and tells the listener when the
// value of an owned kind's tracker moved.
func (s *service) writeLine(ctx context.Context, campaignID string, t Tracker, v Viewer, in LogInput, source string) (*TrackerView, error) {
	n, err := s.repo.CountEntries(ctx, campaignID, t.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if n >= MaxEntries {
		return nil, errInvalid("this tracker's log is full")
	}
	before, err := s.one(ctx, campaignID, t, v)
	if err != nil {
		return nil, err
	}
	e := Entry{TrackerID: t.ID, Op: in.Op, Amount: in.Amount, Note: in.Note, Source: source, CreatedBy: v.UserID}
	if s.clock != nil {
		// A calendar that can't be read leaves the line undated rather
		// than losing the change.
		if d, err := s.clock.Today(ctx, campaignID, v); err == nil {
			e.Game = d
		}
	}
	if err := s.repo.AddEntry(ctx, campaignID, e); err != nil {
		return nil, apperrorInternal(err)
	}
	s.touched(campaignID)
	after, err := s.one(ctx, campaignID, t, v)
	if err != nil {
		return nil, err
	}
	s.changed(ctx, campaignID, t, v, before.Value, after.Value)
	return after, nil
}

// touched tells the observer, if any, that a campaign's trackers changed.
func (s *service) touched(campaignID string) {
	if s.observer != nil {
		s.observer.TrackersChanged(campaignID)
	}
}

// changed tells the listener a tracker of an owned kind moved. The change
// is already saved, so a listener's failure is logged, not returned.
func (s *service) changed(ctx context.Context, campaignID string, t Tracker, v Viewer, from, to int) {
	if s.listener == nil || from == to {
		return
	}
	k, ok, err := s.repo.GetKind(ctx, campaignID, t.KindID)
	if err != nil || !ok || k.Owner == "" {
		return
	}
	if err := s.listener.TrackerChanged(ctx, campaignID, v, Change{Owner: k.Owner, EntityID: t.EntityID, From: from, To: to}); err != nil {
		slog.Warn("trackers: a change listener failed", slog.String("tracker", t.ID), slog.Any("error", err))
	}
}

func (s *service) Strike(ctx context.Context, campaignID string, entryID int64, struck bool, v Viewer) (*TrackerView, error) {
	e, ok, err := s.repo.GetEntry(ctx, campaignID, entryID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("log line")
	}
	t, err := s.tracker(ctx, campaignID, e.TrackerID, v)
	if err != nil {
		return nil, err
	}
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	before, err := s.one(ctx, campaignID, t, v)
	if err != nil {
		return nil, err
	}
	if err := s.repo.StrikeEntry(ctx, campaignID, entryID, struck, v.UserID); err != nil {
		return nil, apperrorInternal(err)
	}
	s.touched(campaignID)
	after, err := s.one(ctx, campaignID, t, v)
	if err != nil {
		return nil, err
	}
	s.changed(ctx, campaignID, t, v, before.Value, after.Value)
	return after, nil
}

// SetChangeListener sets who hears that an owned kind's tracker moved.
// It is called once while the app starts.
func (s *service) SetChangeListener(l ChangeListener) { s.listener = l }

// SetChangeObserver sets who hears that any tracker of the campaign changed.
// It is called once while the app starts.
func (s *service) SetChangeObserver(o ChangeObserver) { s.observer = o }

func (s *service) LogOwned(ctx context.Context, campaignID, entityID, owner string, v Viewer, op string, amount int, note string) ([]TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	in, err := validateLog(LogInput{Op: op, Amount: amount, Note: note})
	if err != nil {
		return nil, err
	}
	_, list, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var k Kind
	for _, c := range list {
		if owner != "" && c.Owner == owner {
			k = c
		}
	}
	if k.ID == "" {
		return nil, errNotFound("tracker kind")
	}
	var ts []Tracker
	if entityID == "" {
		all, err := s.repo.ListAllTrackers(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, t := range all {
			if t.KindID == k.ID {
				ts = append(ts, t)
			}
		}
	} else {
		if _, err := s.page(ctx, campaignID, entityID, v); err != nil {
			return nil, err
		}
		t, err := s.rowFor(ctx, campaignID, entityID, k)
		if err != nil {
			return nil, err
		}
		ts = []Tracker{t}
	}
	out := []TrackerView{}
	for _, t := range ts {
		tv, err := s.writeLine(ctx, campaignID, t, v, in, SourceRule)
		if err != nil {
			return out, err
		}
		out = append(out, *tv)
	}
	return out, nil
}

func (s *service) Pages(ctx context.Context, campaignID string, v Viewer) ([]PageRef, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	all, err := s.repo.ListAllTrackers(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	var ids []string
	seen := map[string]bool{}
	for _, t := range all {
		if !seen[t.EntityID] {
			seen[t.EntityID] = true
			ids = append(ids, t.EntityID)
		}
	}
	out := []PageRef{}
	if len(ids) == 0 {
		return out, nil
	}
	names, err := s.entities.Names(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	for _, id := range ids {
		if n, ok := names[id]; ok {
			out = append(out, PageRef{ID: id, Name: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (s *service) Kinds(ctx context.Context, campaignID string, v Viewer) ([]Kind, []TypeRef, error) {
	if err := requireDirector(v); err != nil {
		return nil, nil, err
	}
	_, list, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	types, err := s.entities.Types(ctx, campaignID)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	sort.SliceStable(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	return list, types, nil
}

// checkType refuses a category that isn't the campaign's own.
func (s *service) checkType(ctx context.Context, campaignID string, id int) error {
	if id == 0 {
		return nil
	}
	types, err := s.entities.Types(ctx, campaignID)
	if err != nil {
		return apperrorInternal(err)
	}
	for _, t := range types {
		if t.ID == id {
			return nil
		}
	}
	return errInvalid("unknown category")
}

func dupName(err error, name string) error {
	if errors.Is(err, errDuplicate) {
		return errInvalid("there is already a tracker kind called " + name)
	}
	return apperrorInternal(err)
}

func (s *service) CreateKind(ctx context.Context, campaignID string, v Viewer, in KindInput) (*Kind, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	k, err := validateKind(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkType(ctx, campaignID, k.EveryTypeID); err != nil {
		return nil, err
	}
	n, err := s.repo.CountKinds(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if n >= MaxKindsPerCamp {
		return nil, errInvalid("a campaign can have at most 200 tracker kinds")
	}
	k.ID, k.CampaignID, k.Version = s.newID(), campaignID, 1
	if err := s.repo.CreateKind(ctx, k); err != nil {
		return nil, dupName(err, k.Name)
	}
	return &k, nil
}

func (s *service) UpdateKind(ctx context.Context, campaignID, kindID string, v Viewer, in KindInput) (*Kind, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	if !validToken(kindID) {
		return nil, errNotFound("tracker kind")
	}
	old, ok, err := s.repo.GetKind(ctx, campaignID, kindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("tracker kind")
	}
	if in.Version != old.Version {
		return nil, errConflict()
	}
	if old.Owner != "" {
		return nil, errManaged()
	}
	k, err := validateKind(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkType(ctx, campaignID, k.EveryTypeID); err != nil {
		return nil, err
	}
	// Changing the shape would turn every placed tracker's log into a
	// different kind of number.
	if k.Shape != old.Shape {
		list, err := s.repo.ListKinds(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, x := range list {
			if x.ID == kindID && x.InUse > 0 {
				return nil, errInvalid("this kind is on pages, so its shape can't change; make a new kind instead")
			}
		}
	}
	k.ID, k.CampaignID = kindID, campaignID
	saved, err := s.repo.UpdateKind(ctx, k, old.Version)
	if err != nil {
		return nil, dupName(err, k.Name)
	}
	if !saved {
		return nil, errConflict()
	}
	k.Version = old.Version + 1
	s.touched(campaignID)
	return &k, nil
}

func (s *service) DeleteKind(ctx context.Context, campaignID, kindID string, v Viewer) error {
	if err := requireDirector(v); err != nil {
		return err
	}
	if !validToken(kindID) {
		return errNotFound("tracker kind")
	}
	if k, ok, err := s.repo.GetKind(ctx, campaignID, kindID); err != nil {
		return apperrorInternal(err)
	} else if !ok {
		return errNotFound("tracker kind")
	} else if k.Owner != "" {
		return errManaged()
	}
	inUse, err := s.repo.DeleteKind(ctx, campaignID, kindID)
	if err != nil {
		return apperrorInternal(err)
	}
	if inUse {
		return errInvalid("this kind is still on pages; remove those trackers first")
	}
	s.touched(campaignID)
	return nil
}

// grantFilter drops a chosen player's grant once it is older than the
// member's joined_at, or the member is gone. Doing it where trackers are read
// means no caller can decide who sees a tracker from a grant left by an
// earlier stay: a player removed and invited back is chosen again, not
// silently restored.
type grantFilter struct {
	Repository
	members MemberDirectory
}

func (g *grantFilter) live(ctx context.Context, campaignID string, ts []Tracker) error {
	need := false
	for _, t := range ts {
		need = need || (t.Visibility == SeePlayers && len(t.Players) > 0)
	}
	if !need {
		return nil
	}
	list, err := g.members.Members(ctx, campaignID)
	if err != nil {
		return err
	}
	joined := make(map[string]time.Time, len(list))
	for _, m := range list {
		joined[m.UserID] = m.JoinedAt
	}
	for i := range ts {
		kept := ts[i].Players[:0:0]
		for _, p := range ts[i].Players {
			if j, ok := joined[p.UserID]; ok && !p.GrantedAt.Before(j) {
				kept = append(kept, p)
			}
		}
		ts[i].Players = kept
	}
	return nil
}

func (g *grantFilter) ListTrackers(ctx context.Context, campaignID, entityID string) ([]Tracker, error) {
	ts, err := g.Repository.ListTrackers(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	return ts, g.live(ctx, campaignID, ts)
}

func (g *grantFilter) ListAllTrackers(ctx context.Context, campaignID string) ([]Tracker, error) {
	ts, err := g.Repository.ListAllTrackers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return ts, g.live(ctx, campaignID, ts)
}

func (g *grantFilter) one(ctx context.Context, campaignID string, t Tracker, found bool, err error) (Tracker, bool, error) {
	if err != nil || !found {
		return t, found, err
	}
	ts := []Tracker{t}
	if err := g.live(ctx, campaignID, ts); err != nil {
		return Tracker{}, false, err
	}
	return ts[0], true, nil
}

func (g *grantFilter) GetTracker(ctx context.Context, campaignID, id string) (Tracker, bool, error) {
	t, found, err := g.Repository.GetTracker(ctx, campaignID, id)
	return g.one(ctx, campaignID, t, found, err)
}

func (g *grantFilter) TrackerForKind(ctx context.Context, campaignID, entityID, kindID string) (Tracker, bool, error) {
	t, found, err := g.Repository.TrackerForKind(ctx, campaignID, entityID, kindID)
	return g.one(ctx, campaignID, t, found, err)
}
