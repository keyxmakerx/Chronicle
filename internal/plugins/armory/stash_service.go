// stash_service.go holds the business rules for stashes: who can see one, who
// can change one, and the queries behind the stashes page and the character
// panel. Moving things between holders lives in stash_moves.go.
//
// The service never imports Echo or another plugin. Everything it needs from
// the entities and relations plugins arrives through the small interfaces
// below, wired in internal/app/routes.go.
package armory

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// noMoneyMessage is shown when a character's game system has no money field.
const noMoneyMessage = "This game system has no money field on characters."

// StashDirectory answers the entity questions the stash rules need.
// Implemented by an adapter over entities.EntityService.
type StashDirectory interface {
	// GetEntity returns the entity, or nil (and no error) when it does not
	// exist in campaignID, so a foreign id and a missing id look the same.
	GetEntity(ctx context.Context, campaignID, entityID string) (*EntityRef, error)
	// ListCharacters returns the character-family entities in the campaign
	// that the viewer may see.
	ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]EntityRef, error)
	// ListItems returns up to limit item entities the viewer may see.
	ListItems(ctx context.Context, campaignID string, role int, userID string, limit int) ([]EntityRef, error)
	// OwnedCharacterIDs returns the entities the user has claimed.
	OwnedCharacterIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error)
}

// HasItemRelation is one "Has Item" relation from a character to an item.
type HasItemRelation struct {
	ID           int
	ItemEntityID string
	ItemName     string
	Metadata     []byte
	DmOnly       bool
}

// HasItemStore reads and writes the "Has Item" relations that hold a
// character's inventory. Implemented over the relations widget service.
type HasItemStore interface {
	// ListByCharacter returns the character's "Has Item" relations.
	ListByCharacter(ctx context.Context, campaignID, characterID string) ([]HasItemRelation, error)
	// Create adds a Has Item relation (and its In Inventory Of reverse).
	Create(ctx context.Context, campaignID, characterID, itemID, createdBy string, metadata []byte) (int, error)
	// UpdateMetadata overwrites a relation's metadata.
	UpdateMetadata(ctx context.Context, id int, metadata []byte) error
	// UpdateMetadataIf overwrites metadata only while it still equals expected.
	UpdateMetadataIf(ctx context.Context, id int, expected, metadata []byte) (bool, error)
	// Delete removes a relation and its reverse.
	Delete(ctx context.Context, id int) error
}

// UserNamer resolves campaign members' display names.
type UserNamer interface {
	DisplayNames(ctx context.Context, campaignID string, userIDs []string) (map[string]string, error)
}

// StashDeps are the collaborators a stash service needs.
type StashDeps struct {
	Repo       StashRepository
	Directory  StashDirectory
	Visibility EntityVisibilityFilter
	// Actor says whether a user may act as (move things out of) a character.
	Actor     BuyerAccessChecker
	Fields    EntityFieldUpdater
	Relations HasItemStore
	UserNames UserNamer
}

// Actor is the calling user as the service sees them. Role is the campaign's
// VisibilityRole (a DM-granted co-DM counts as Owner), so every rule below is
// the same promotion the entity paths apply.
type Actor struct {
	UserID string
	Role   int
}

// IsGM reports Owner or Scribe, who can already edit anything.
func (a Actor) IsGM() bool { return a.Role >= permissions.RoleScribe }

// IsOwner reports Owner visibility: the Owner or a DM-granted co-DM. Only
// these answer requests; a Scribe does not.
func (a Actor) IsOwner() bool { return a.Role >= permissions.RoleOwner }

// StashService defines the contract the handlers call.
type StashService interface {
	StashesPage(ctx context.Context, campaignID string, a Actor) (*StashesPageView, error)
	CreateStash(ctx context.Context, campaignID string, a Actor, in CreateStashInput) (*Stash, error)
	UpdateStash(ctx context.Context, campaignID string, a Actor, id int, in UpdateStashInput) error
	DeleteStash(ctx context.Context, campaignID string, a Actor, id int) error
	SetStashViewers(ctx context.Context, campaignID string, a Actor, id int, characterIDs []string) error
	AddStashItem(ctx context.Context, campaignID string, a Actor, id int, itemID string, quantity int) error

	MoveDialog(ctx context.Context, campaignID string, a Actor, kind string, from Endpoint, itemID string) (*MoveDialogView, error)
	Move(ctx context.Context, campaignID string, a Actor, in MoveInput) (*MoveOutcome, error)
	Approve(ctx context.Context, campaignID string, a Actor, moveID int64) (*Move, error)
	Decline(ctx context.Context, campaignID string, a Actor, moveID int64) (*Move, error)

	IsDowntimeOpen(ctx context.Context, campaignID string) (bool, error)
	SetDowntime(ctx context.Context, campaignID string, a Actor, open bool) (DowntimeResult, error)

	// CharacterPanel returns nil when the viewer has no business with the
	// character's items and money (not a character, or not theirs).
	CharacterPanel(ctx context.Context, campaignID string, a Actor, characterID string) (*CharacterPanelView, error)
	CharacterHistory(ctx context.Context, campaignID string, a Actor, characterID string) ([]MoveLine, error)
	StashHistory(ctx context.Context, campaignID string, a Actor, stashID int) ([]MoveLine, error)
}

// DowntimeResult counts what opening downtime did to waiting requests.
type DowntimeResult struct {
	Applied int
	Failed  int
}

type stashService struct {
	StashDeps
	locks campaignLocks
}

// NewStashService creates the stash service.
func NewStashService(deps StashDeps) StashService {
	return &stashService{StashDeps: deps}
}

// campaignLocks serializes balance changes per campaign. The deployment is a
// single instance, so an in-process lock is enough to stop two applies from
// both spending the same item; the conditional SQL debits are the second line.
type campaignLocks struct {
	m sync.Map // campaign id -> *sync.Mutex
}

func (l *campaignLocks) lock(campaignID string) func() {
	v, _ := l.m.LoadOrStore(campaignID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// --- access helpers ---

func forbidden() error { return apperror.NewForbidden("You can't do that.") }

// notFound hides whether an id exists in another campaign.
func notFound(what string) error { return apperror.NewNotFound(what) }

func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// loadCharacter returns a character-family entity in the campaign, else NotFound.
func (s *stashService) loadCharacter(ctx context.Context, campaignID, id string) (*EntityRef, error) {
	ref, err := s.Directory.GetEntity(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if ref == nil || !ref.IsCharacter {
		return nil, notFound("character")
	}
	return ref, nil
}

// canAct reports whether the actor may move things out of the character.
func (s *stashService) canAct(ctx context.Context, campaignID string, a Actor, characterID string) (bool, error) {
	if a.IsGM() {
		return true, nil
	}
	if s.Actor == nil {
		return false, nil
	}
	return s.Actor.CanUserActAsBuyer(ctx, campaignID, characterID, a.UserID, a.Role)
}

// viewable reports whether the actor may see the entity.
func (s *stashService) viewable(ctx context.Context, campaignID string, a Actor, id string) (bool, error) {
	if a.IsOwner() {
		return true, nil
	}
	m, err := s.Visibility.FilterViewableEntityIDs(ctx, campaignID, []string{id}, a.Role, a.UserID)
	if err != nil {
		return false, err
	}
	return m[id], nil
}

// stashVisible reports whether the actor sees the stash: the GM always does,
// a player does when they own (claimed) one of its viewer characters.
func (s *stashService) stashVisible(ctx context.Context, campaignID string, a Actor, viewers []string) (bool, error) {
	if a.IsGM() {
		return true, nil
	}
	if len(viewers) == 0 {
		return false, nil
	}
	owned, err := s.Directory.OwnedCharacterIDs(ctx, campaignID, a.UserID)
	if err != nil {
		return false, err
	}
	for _, v := range viewers {
		if owned[v] {
			return true, nil
		}
	}
	return false, nil
}

// visibleStash loads a stash the actor can see, else NotFound — an unseen
// stash looks exactly like a missing one.
func (s *stashService) visibleStash(ctx context.Context, campaignID string, a Actor, id int) (*Stash, error) {
	st, err := s.Repo.GetStash(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if a.IsGM() {
		return st, nil
	}
	all, err := s.Repo.ListViewers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	ok, err := s.stashVisible(ctx, campaignID, a, all[id])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound("stash")
	}
	return st, nil
}

func (s *stashService) requireGM(a Actor) error {
	if !a.IsGM() {
		return forbidden()
	}
	return nil
}

// --- GM management ---

func cleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", apperror.NewBadRequest("Give the stash a name.")
	}
	if utf8.RuneCountInString(s) > maxStashName {
		return "", apperror.NewBadRequest("The stash name is too long.")
	}
	return s, nil
}

func cleanLocation(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxStashLocation {
		return "", apperror.NewBadRequest("The location is too long.")
	}
	return s, nil
}

func (s *stashService) CreateStash(ctx context.Context, campaignID string, a Actor, in CreateStashInput) (*Stash, error) {
	if err := s.requireGM(a); err != nil {
		return nil, err
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	loc, err := cleanLocation(in.Location)
	if err != nil {
		return nil, err
	}
	st := &Stash{CampaignID: campaignID, Name: name, Location: loc, CreatedBy: a.UserID}
	if err := s.Repo.CreateStash(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *stashService) UpdateStash(ctx context.Context, campaignID string, a Actor, id int, in UpdateStashInput) error {
	if err := s.requireGM(a); err != nil {
		return err
	}
	cur, err := s.Repo.GetStash(ctx, campaignID, id)
	if err != nil {
		return err
	}
	// Absent keeps, null clears (location only: a name can't be empty), a
	// value replaces. Merging onto the stored row is what lets a rename-only
	// push leave the location alone.
	name, err := cleanName(in.Name.Val(cur.Name))
	if err != nil {
		return err
	}
	var curLoc *string
	if cur.Location != "" {
		curLoc = &cur.Location
	}
	loc := ""
	if p := in.Location.Ptr(curLoc); p != nil {
		loc = *p
	}
	loc, err = cleanLocation(loc)
	if err != nil {
		return err
	}
	cur.Name, cur.Location = name, loc
	return s.Repo.UpdateStash(ctx, cur)
}

func (s *stashService) DeleteStash(ctx context.Context, campaignID string, a Actor, id int) error {
	if err := s.requireGM(a); err != nil {
		return err
	}
	unlock := s.locks.lock(campaignID)
	defer unlock()
	return s.Repo.DeleteStash(ctx, campaignID, id)
}

func (s *stashService) SetStashViewers(ctx context.Context, campaignID string, a Actor, id int, characterIDs []string) error {
	if err := s.requireGM(a); err != nil {
		return err
	}
	if _, err := s.Repo.GetStash(ctx, campaignID, id); err != nil {
		return err
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(characterIDs))
	for _, cid := range characterIDs {
		cid = strings.TrimSpace(cid)
		if cid == "" || seen[cid] {
			continue
		}
		seen[cid] = true
		// Only characters of this campaign can see a stash.
		if _, err := s.loadCharacter(ctx, campaignID, cid); err != nil {
			return err
		}
		clean = append(clean, cid)
	}
	return s.Repo.SetViewers(ctx, campaignID, id, clean)
}

func (s *stashService) AddStashItem(ctx context.Context, campaignID string, a Actor, id int, itemID string, quantity int) error {
	if err := s.requireGM(a); err != nil {
		return err
	}
	if quantity < 1 || quantity > maxMoveQuantity {
		return apperror.NewBadRequest("Enter a quantity of at least 1.")
	}
	if _, err := s.Repo.GetStash(ctx, campaignID, id); err != nil {
		return err
	}
	ref, err := s.Directory.GetEntity(ctx, campaignID, itemID)
	if err != nil {
		return err
	}
	if ref == nil || !ref.IsItem {
		return notFound("item")
	}
	unlock := s.locks.lock(campaignID)
	defer unlock()
	return s.Repo.CreditItem(ctx, campaignID, id, itemID, quantity)
}

// --- stashes page ---

func (s *stashService) IsDowntimeOpen(ctx context.Context, campaignID string) (bool, error) {
	d, err := s.Repo.GetDowntime(ctx, campaignID)
	if err != nil {
		return false, err
	}
	return d.IsOpen, nil
}

func (s *stashService) StashesPage(ctx context.Context, campaignID string, a Actor) (*StashesPageView, error) {
	open, err := s.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	stashes, err := s.Repo.ListStashes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	viewers, err := s.Repo.ListViewers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	items, err := s.Repo.ListItems(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	chars, err := s.Directory.ListCharacters(ctx, campaignID, a.Role, a.UserID)
	if err != nil {
		return nil, err
	}
	charName := map[string]string{}
	for _, c := range chars {
		charName[c.ID] = c.Name
	}

	var owned map[string]bool
	if !a.IsGM() {
		if owned, err = s.Directory.OwnedCharacterIDs(ctx, campaignID, a.UserID); err != nil {
			return nil, err
		}
	}

	// Items a player can't see are left out, so the page never offers a Move
	// the service would refuse, and a GM-secret item never leaks by name.
	itemNames := map[string]string{}
	var visibleItems map[string]bool
	if !a.IsOwner() {
		ids := map[string]bool{}
		for _, rows := range items {
			for _, r := range rows {
				ids[r.ItemEntityID] = true
			}
		}
		if len(ids) > 0 {
			list := make([]string, 0, len(ids))
			for id := range ids {
				list = append(list, id)
			}
			if visibleItems, err = s.Visibility.FilterViewableEntityIDs(ctx, campaignID, list, a.Role, a.UserID); err != nil {
				return nil, err
			}
		}
	}
	itemName := func(id string) (string, bool) {
		if n, ok := itemNames[id]; ok {
			return n, n != ""
		}
		ref, err := s.Directory.GetEntity(ctx, campaignID, id)
		name := ""
		if err == nil && ref != nil {
			name = ref.Name
		}
		itemNames[id] = name
		return name, name != ""
	}

	view := &StashesPageView{
		CampaignID:   campaignID,
		DowntimeOpen: open,
		CanManage:    a.IsGM(),
		CanApprove:   a.IsOwner(),
	}
	for _, st := range stashes {
		if !a.IsGM() {
			seen := false
			for _, v := range viewers[st.ID] {
				if owned[v] {
					seen = true
					break
				}
			}
			if !seen {
				continue
			}
		}
		sv := StashView{Stash: st}
		for _, cid := range viewers[st.ID] {
			name, ok := charName[cid]
			if !ok {
				name = "another character"
			}
			sv.Viewers = append(sv.Viewers, NamedRef{ID: cid, Name: name})
		}
		for _, r := range items[st.ID] {
			if visibleItems != nil && !visibleItems[r.ItemEntityID] {
				continue
			}
			name, ok := itemName(r.ItemEntityID)
			if !ok {
				continue
			}
			sv.Items = append(sv.Items, StashItemView{ItemID: r.ItemEntityID, Name: name, Quantity: r.Quantity})
		}
		sort.Slice(sv.Items, func(i, j int) bool { return strings.ToLower(sv.Items[i].Name) < strings.ToLower(sv.Items[j].Name) })
		view.Stashes = append(view.Stashes, sv)
	}

	if a.IsGM() {
		for _, c := range chars {
			view.Characters = append(view.Characters, NamedRef{ID: c.ID, Name: c.Name})
		}
		cat, err := s.Directory.ListItems(ctx, campaignID, a.Role, a.UserID, 500)
		if err != nil {
			return nil, err
		}
		for _, it := range cat {
			view.Items = append(view.Items, NamedRef{ID: it.ID, Name: it.Name})
		}
	}

	if a.IsOwner() {
		pending, err := s.Repo.ListPending(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		if view.Pending, err = s.lines(ctx, campaignID, a, pending); err != nil {
			return nil, err
		}
	}
	return view, nil
}

// --- history ---

// lines resolves names for a set of moves. Names of entities the actor can't
// see, and of stashes they can't see, are replaced rather than leaked.
func (s *stashService) lines(ctx context.Context, campaignID string, a Actor, moves []Move) ([]MoveLine, error) {
	if len(moves) == 0 {
		return nil, nil
	}
	ids := map[string]bool{}
	users := map[string]bool{}
	for _, m := range moves {
		if m.ItemEntityID != "" {
			ids[m.ItemEntityID] = true
		}
		for _, e := range []Endpoint{m.From, m.To} {
			if e.Kind == EndpointCharacter {
				ids[e.ID] = true
			}
		}
		users[m.RequestedBy] = true
	}
	var viewable map[string]bool
	if !a.IsOwner() && len(ids) > 0 {
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		var err error
		if viewable, err = s.Visibility.FilterViewableEntityIDs(ctx, campaignID, list, a.Role, a.UserID); err != nil {
			return nil, err
		}
	}
	entityName := func(id string) string {
		if viewable != nil && !viewable[id] {
			return "someone"
		}
		ref, err := s.Directory.GetEntity(ctx, campaignID, id)
		if err != nil || ref == nil {
			return "something that was removed"
		}
		return ref.Name
	}

	stashNames := map[string]string{}
	if sl, err := s.Repo.ListStashes(ctx, campaignID); err == nil {
		var vs map[int][]string
		if !a.IsGM() {
			vs, _ = s.Repo.ListViewers(ctx, campaignID)
		}
		for _, st := range sl {
			if !a.IsGM() {
				if ok, _ := s.stashVisible(ctx, campaignID, a, vs[st.ID]); !ok {
					continue
				}
			}
			stashNames[StashEndpoint(st.ID).ID] = st.Name
		}
	}
	endpointName := func(e Endpoint) string {
		if e.Kind == EndpointStash {
			if n, ok := stashNames[e.ID]; ok {
				return n
			}
			return "a stash"
		}
		return entityName(e.ID)
	}

	userList := make([]string, 0, len(users))
	for u := range users {
		userList = append(userList, u)
	}
	var userNames map[string]string
	if s.UserNames != nil {
		userNames, _ = s.UserNames.DisplayNames(ctx, campaignID, userList)
	}

	out := make([]MoveLine, 0, len(moves))
	for _, m := range moves {
		l := MoveLine{Move: m, FromName: endpointName(m.From), ToName: endpointName(m.To)}
		if m.ItemEntityID != "" {
			l.ItemName = entityName(m.ItemEntityID)
		}
		l.RequesterName = userNames[m.RequestedBy]
		if l.RequesterName == "" {
			l.RequesterName = "A player"
		}
		out = append(out, l)
	}
	return out, nil
}

// ownPending drops other people's pending requests for viewers who are not
// GM; applied, declined and failed rows stay.
func ownPending(a Actor, moves []Move) []Move {
	if a.IsGM() {
		return moves
	}
	out := moves[:0:0]
	for _, m := range moves {
		if m.Status == MovePending && m.RequestedBy != a.UserID {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (s *stashService) CharacterHistory(ctx context.Context, campaignID string, a Actor, characterID string) ([]MoveLine, error) {
	ref, err := s.loadCharacter(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	ok, err := s.canAct(ctx, campaignID, a, ref.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound("character")
	}
	ep := Endpoint{Kind: EndpointCharacter, ID: ref.ID}
	moves, err := s.Repo.ListMoves(ctx, campaignID, MoveFilter{Endpoint: &ep, Limit: historyPageSize})
	if err != nil {
		return nil, err
	}
	return s.lines(ctx, campaignID, a, ownPending(a, moves))
}

func (s *stashService) StashHistory(ctx context.Context, campaignID string, a Actor, stashID int) ([]MoveLine, error) {
	if _, err := s.visibleStash(ctx, campaignID, a, stashID); err != nil {
		return nil, err
	}
	ep := StashEndpoint(stashID)
	moves, err := s.Repo.ListMoves(ctx, campaignID, MoveFilter{Endpoint: &ep, Limit: historyPageSize})
	if err != nil {
		return nil, err
	}
	return s.lines(ctx, campaignID, a, ownPending(a, moves))
}

// --- character panel ---

func (s *stashService) CharacterPanel(ctx context.Context, campaignID string, a Actor, characterID string) (*CharacterPanelView, error) {
	ref, err := s.Directory.GetEntity(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, notFound("character")
	}
	if !ref.IsCharacter {
		return nil, nil
	}
	ok, err := s.canAct(ctx, campaignID, a, ref.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	open, err := s.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	view := &CharacterPanelView{CampaignID: campaignID, Character: *ref, DowntimeOpen: open}

	held, err := s.carried(ctx, campaignID, a, ref.ID)
	if err != nil {
		return nil, err
	}
	view.Items = held

	if ref.MoneyKey != "" {
		bal, err := s.characterMoney(ctx, ref)
		if err != nil {
			return nil, err
		}
		view.HasMoney, view.Money = true, bal
	}

	ep := Endpoint{Kind: EndpointCharacter, ID: ref.ID}
	moves, err := s.Repo.ListMoves(ctx, campaignID, MoveFilter{Endpoint: &ep, Limit: panelHistorySize + 1})
	if err != nil {
		return nil, err
	}
	if len(moves) > panelHistorySize {
		view.HistoryMore = true
		moves = moves[:panelHistorySize]
	}
	if view.History, err = s.lines(ctx, campaignID, a, ownPending(a, moves)); err != nil {
		return nil, err
	}
	return view, nil
}
