package entities

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// character_lists.go decides which page types a campaign shows as characters
// (The Party, stashes, quests) and as NPCs (the NPC gallery, DM Screen, system
// panels). The owner picks the types; nothing here guesses from names at
// request time. The one guess left is seedCharacterLists, used once to fill a
// campaign that never chose, so existing campaigns keep what they showed. It
// runs on first read as well as at boot and creation, because a campaign can
// arrive without the lists later (an import, a seed that failed).

// CharacterListKind names one of the two lists.
type CharacterListKind string

const (
	// CharacterListCharacters is the list behind The Party and stashes.
	CharacterListCharacters CharacterListKind = "characters"
	// CharacterListNPCs is the list behind the NPC gallery and DM Screen.
	CharacterListNPCs CharacterListKind = "npcs"
)

// Valid reports whether k names a list.
func (k CharacterListKind) Valid() bool {
	return k == CharacterListCharacters || k == CharacterListNPCs
}

// CharacterLists is what the owner chose: the page types listed explicitly.
// Sub-types of a listed type count as listed too (see ResolveCharacterTypeIDs),
// so they are not stored.
type CharacterLists struct {
	CharacterTypeIDs []int
	NPCTypeIDs       []int
}

// CharacterListStore persists the lists. The composition root implements it
// over the campaign settings so this plugin never touches the campaigns
// repository.
type CharacterListStore interface {
	// Get returns the stored lists; ok is false when the campaign never chose.
	Get(ctx context.Context, campaignID string) (lists CharacterLists, ok bool, err error)
	Save(ctx context.Context, campaignID string, lists CharacterLists) error
}

// CharacterTypeSource is the slice of the entity service the lists need.
type CharacterTypeSource interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
	CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error)
}

// CharacterListReader is what other plugins' adapters read: the page types to
// treat as characters or NPCs, sub-types included, deleted types gone.
type CharacterListReader interface {
	CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
	NPCTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// CharacterListManager is what the Characters page handler needs.
type CharacterListManager interface {
	CharacterListReader
	// Chosen returns the explicit lists, stale IDs pruned.
	Chosen(ctx context.Context, campaignID string) (CharacterLists, error)
	// Editor describes one band's chips and the types still addable.
	Editor(ctx context.Context, campaignID string, kind CharacterListKind) (CastBandEditor, error)
	// Add and Remove return the type's name for the confirmation message.
	Add(ctx context.Context, campaignID string, kind CharacterListKind, typeID int) (string, error)
	Remove(ctx context.Context, campaignID string, kind CharacterListKind, typeID int) (string, error)
}

// CastChip is one chosen type on a band.
type CastChip struct {
	ID   int
	Name string
}

// CastTypeOption is one type the owner can still add to a band.
type CastTypeOption struct {
	ID    int
	Name  string
	IsSub bool
	Count int
	// With names the sub-types that come along when this type is added.
	With []string
}

// CastBandEditor is what the owner's controls on one band render from.
type CastBandEditor struct {
	Kind    CharacterListKind
	Label   string
	Chips   []CastChip
	Options []CastTypeOption
}

// CharacterListService implements CharacterListManager and the campaigns
// plugin's CharacterListSeeder.
type CharacterListService struct {
	types    CharacterTypeSource
	store    CharacterListStore
	onChange func(campaignID string)
}

// NewCharacterListService wires the service.
func NewCharacterListService(types CharacterTypeSource, store CharacterListStore) *CharacterListService {
	return &CharacterListService{types: types, store: store}
}

// SetChangeHook registers a callback run after the lists change, so a cache
// keyed on them can drop its copy.
func (s *CharacterListService) SetChangeHook(fn func(campaignID string)) { s.onChange = fn }

func (s *CharacterListService) changed(campaignID string) {
	if s.onChange != nil {
		s.onChange(campaignID)
	}
}

// Chosen returns the explicit lists with IDs of deleted types dropped. A
// campaign that never chose is seeded first (seedOnRead).
func (s *CharacterListService) Chosen(ctx context.Context, campaignID string) (CharacterLists, error) {
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return CharacterLists{}, err
	}
	return s.chosenFrom(ctx, campaignID, types)
}

func (s *CharacterListService) chosenFrom(ctx context.Context, campaignID string, types []EntityType) (CharacterLists, error) {
	stored, ok, err := s.store.Get(ctx, campaignID)
	if err != nil {
		return CharacterLists{}, err
	}
	if !ok {
		stored = s.seedOnRead(ctx, campaignID, types)
	}
	exists := make(map[int]bool, len(types))
	for _, t := range types {
		exists[t.ID] = true
	}
	return CharacterLists{
		CharacterTypeIDs: pruneTypeIDs(stored.CharacterTypeIDs, exists),
		NPCTypeIDs:       pruneTypeIDs(stored.NPCTypeIDs, exists),
	}, nil
}

// CharacterTypeIDs is the characters list with sub-types included.
func (s *CharacterListService) CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	chosen, err := s.chosenFrom(ctx, campaignID, types)
	if err != nil {
		return nil, err
	}
	return ResolveCharacterTypeIDs(types, chosen.CharacterTypeIDs, false), nil
}

// NPCTypeIDs is the NPC list with sub-types included, minus the player
// character sub-type: claimed player characters are the party, not NPCs.
func (s *CharacterListService) NPCTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	chosen, err := s.chosenFrom(ctx, campaignID, types)
	if err != nil {
		return nil, err
	}
	return ResolveCharacterTypeIDs(types, chosen.NPCTypeIDs, true), nil
}

// Add lists a type. The type must belong to the campaign. Listing a type that
// is already covered changes nothing; listing a parent absorbs its listed
// sub-types, which it now covers.
func (s *CharacterListService) Add(ctx context.Context, campaignID string, kind CharacterListKind, typeID int) (string, error) {
	if !kind.Valid() {
		return "", apperror.NewBadRequest("unknown list")
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return "", err
	}
	name, ok := typeName(types, typeID)
	if !ok {
		return "", apperror.NewBadRequest("that page type is not in this campaign")
	}
	chosen, err := s.chosenFrom(ctx, campaignID, types)
	if err != nil {
		return "", err
	}
	ids := chosen.list(kind)
	if coveredBy(types, ids, typeID) {
		return name, nil
	}
	next := []int{typeID}
	for _, id := range ids {
		if !isDescendant(types, id, typeID) {
			next = append(next, id)
		}
	}
	sort.Ints(next)
	chosen.set(kind, next)
	return name, s.save(ctx, campaignID, chosen)
}

// Remove unlists a type. Saving also drops IDs of deleted types.
func (s *CharacterListService) Remove(ctx context.Context, campaignID string, kind CharacterListKind, typeID int) (string, error) {
	if !kind.Valid() {
		return "", apperror.NewBadRequest("unknown list")
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return "", err
	}
	chosen, err := s.chosenFrom(ctx, campaignID, types)
	if err != nil {
		return "", err
	}
	ids := chosen.list(kind)
	next := make([]int, 0, len(ids))
	found := false
	for _, id := range ids {
		if id == typeID {
			found = true
			continue
		}
		next = append(next, id)
	}
	if !found {
		return "", apperror.NewNotFound("that page type is not on this list")
	}
	name, _ := typeName(types, typeID)
	chosen.set(kind, next)
	return name, s.save(ctx, campaignID, chosen)
}

func (s *CharacterListService) save(ctx context.Context, campaignID string, lists CharacterLists) error {
	if err := s.store.Save(ctx, campaignID, lists); err != nil {
		return err
	}
	s.changed(campaignID)
	return nil
}

// Seed records the starting lists for a campaign that has none, from
// seedCharacterLists. A campaign that already chose is left alone, so running
// it twice, or after the owner edited, changes nothing.
func (s *CharacterListService) Seed(ctx context.Context, campaignID string) error {
	_, ok, err := s.store.Get(ctx, campaignID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return err
	}
	chars, npcs := seedCharacterLists(types)
	if len(chars) == 0 && len(npcs) == 0 {
		// Nothing to record yet (no types, or none that look like
		// characters); leaving the lists unset lets a later read seed them
		// once the campaign has its types.
		return nil
	}
	return s.save(ctx, campaignID, CharacterLists{CharacterTypeIDs: chars, NPCTypeIDs: npcs})
}

// seedOnRead fills a campaign that never chose, the first time its lists are
// read. A failed save still answers with the seed, so the page is right now
// and the save is tried again on the next read.
func (s *CharacterListService) seedOnRead(ctx context.Context, campaignID string, types []EntityType) CharacterLists {
	chars, npcs := seedCharacterLists(types)
	lists := CharacterLists{CharacterTypeIDs: chars, NPCTypeIDs: npcs}
	if len(chars) == 0 && len(npcs) == 0 {
		return lists
	}
	if err := s.save(ctx, campaignID, lists); err != nil {
		slog.Warn("saving seeded character lists failed",
			slog.String("campaign_id", campaignID), slog.String("error", err.Error()))
	}
	return lists
}

// Editor builds a band's controls: the chosen types as chips and every other
// enabled type, not already covered, as an option with its page count.
func (s *CharacterListService) Editor(ctx context.Context, campaignID string, kind CharacterListKind) (CastBandEditor, error) {
	if !kind.Valid() {
		return CastBandEditor{}, apperror.NewBadRequest("unknown list")
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return CastBandEditor{}, err
	}
	chosen, err := s.chosenFrom(ctx, campaignID, types)
	if err != nil {
		return CastBandEditor{}, err
	}
	// Counts are best effort: a failure only blanks the numbers.
	counts, _ := s.types.CountByType(ctx, campaignID, int(campaigns.RoleOwner), "")

	ids := chosen.list(kind)
	byID := make(map[int]EntityType, len(types))
	for _, t := range types {
		byID[t.ID] = t
	}
	ed := CastBandEditor{Kind: kind, Label: bandLabel(kind)}
	for _, id := range ids {
		ed.Chips = append(ed.Chips, CastChip{ID: id, Name: byID[id].Name})
	}

	avail := make([]EntityType, 0, len(types))
	for _, t := range types {
		if t.Enabled && !coveredBy(types, ids, t.ID) {
			avail = append(avail, t)
		}
	}
	inAvail := make(map[int]bool, len(avail))
	for _, t := range avail {
		inAvail[t.ID] = true
	}
	children := map[int][]EntityType{}
	var roots []EntityType
	for _, t := range avail {
		if t.ParentTypeID != nil && inAvail[*t.ParentTypeID] {
			children[*t.ParentTypeID] = append(children[*t.ParentTypeID], t)
		} else {
			roots = append(roots, t)
		}
	}
	byName := func(l []EntityType) {
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i].Name) < strings.ToLower(l[j].Name) })
	}
	byName(roots)
	var emit func(t EntityType, sub bool)
	emit = func(t EntityType, sub bool) {
		kids := children[t.ID]
		byName(kids)
		opt := CastTypeOption{ID: t.ID, Name: t.Name, IsSub: sub, Count: counts[t.ID]}
		for _, k := range kids {
			opt.With = append(opt.With, k.Name)
		}
		ed.Options = append(ed.Options, opt)
		for _, k := range kids {
			emit(k, true)
		}
	}
	for _, r := range roots {
		emit(r, false)
	}
	return ed, nil
}

func bandLabel(k CharacterListKind) string {
	if k == CharacterListNPCs {
		return "NPCs"
	}
	return "The Party"
}

func (l CharacterLists) list(k CharacterListKind) []int {
	if k == CharacterListNPCs {
		return l.NPCTypeIDs
	}
	return l.CharacterTypeIDs
}

func (l *CharacterLists) set(k CharacterListKind, ids []int) {
	if k == CharacterListNPCs {
		l.NPCTypeIDs = ids
		return
	}
	l.CharacterTypeIDs = ids
}

func pruneTypeIDs(ids []int, exists map[int]bool) []int {
	out := make([]int, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if exists[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func typeName(types []EntityType, id int) (string, bool) {
	for _, t := range types {
		if t.ID == id {
			return t.Name, true
		}
	}
	return "", false
}

// isDescendant reports whether id sits anywhere under ancestor. The depth
// bound guards a parent cycle.
func isDescendant(types []EntityType, id, ancestor int) bool {
	byID := make(map[int]EntityType, len(types))
	for _, t := range types {
		byID[t.ID] = t
	}
	cur, ok := byID[id]
	for depth := 0; ok && depth < 16; depth++ {
		if cur.ParentTypeID == nil {
			return false
		}
		if *cur.ParentTypeID == ancestor {
			return true
		}
		cur, ok = byID[*cur.ParentTypeID]
	}
	return false
}

// coveredBy reports whether id is listed or sits under a listed type.
func coveredBy(types []EntityType, listed []int, id int) bool {
	for _, l := range listed {
		if l == id || isDescendant(types, id, l) {
			return true
		}
	}
	return false
}

func isPCType(t EntityType) bool {
	return (t.PresetCategory != nil && *t.PresetCategory == PresetCategoryPlayerCharacter) ||
		t.Slug == SlugPlayerCharacter
}

// ResolveCharacterTypeIDs expands a list to the enabled types it shows: each
// listed type plus every enabled sub-type under it. Sub-types come along
// because that is how the lists always behaved, and a sub-type such as
// "Villain" under Character is a character page. skipPlayerCharacter leaves
// the player-character sub-type out of what a listed parent brings in (the NPC
// list: claimed characters are the party); a type listed by name is always
// kept. The result follows the order of types.
func ResolveCharacterTypeIDs(types []EntityType, listed []int, skipPlayerCharacter bool) []int {
	var ids []int
	for _, t := range types {
		if !t.Enabled {
			continue
		}
		named := false
		for _, l := range listed {
			if l == t.ID {
				named = true
				break
			}
		}
		if named {
			ids = append(ids, t.ID)
			continue
		}
		if skipPlayerCharacter && isPCType(t) {
			continue
		}
		if coveredBy(types, listed, t.ID) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// seedCharacterLists is the one-time starting point for a campaign that never
// chose. It records what the old name-based rule listed, except creature and
// monster types, which that rule only caught by accident. A type is a root when
// its preset category is character, its slug is character or npc, or it ends
// in -character; sub-types of a root come along. Only the topmost types of
// each family are stored, since a listed parent already covers its sub-types.
func seedCharacterLists(types []EntityType) (characters, npcs []int) {
	isCreature := func(t EntityType) bool {
		if t.PresetCategory != nil && *t.PresetCategory == "creature" {
			return true
		}
		return t.Slug == "creature" || strings.HasSuffix(t.Slug, "-monster")
	}
	isRoot := func(t EntityType) bool {
		if isCreature(t) {
			return false
		}
		if t.PresetCategory != nil && *t.PresetCategory == "character" {
			return true
		}
		return t.Slug == "character" || t.Slug == "npc" || strings.HasSuffix(t.Slug, "-character")
	}
	byID := make(map[int]EntityType, len(types))
	for _, t := range types {
		byID[t.ID] = t
	}
	inFamily := func(t EntityType) bool {
		for depth := 0; depth < 16; depth++ {
			if isCreature(t) {
				return false
			}
			if isRoot(t) {
				return true
			}
			if t.ParentTypeID == nil {
				return false
			}
			p, ok := byID[*t.ParentTypeID]
			if !ok {
				return false
			}
			t = p
		}
		return false
	}
	topmost := func(includePC bool) []int {
		in := map[int]bool{}
		for _, t := range types {
			if t.Enabled && (includePC || !isPCType(t)) && inFamily(t) {
				in[t.ID] = true
			}
		}
		out := []int{}
		for _, t := range types {
			if !in[t.ID] {
				continue
			}
			if t.ParentTypeID != nil && in[*t.ParentTypeID] {
				continue
			}
			out = append(out, t.ID)
		}
		return out
	}
	return topmost(true), topmost(false)
}
