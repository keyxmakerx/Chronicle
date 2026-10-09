package quests

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Caps keep one page's boards bounded; they are abuse limits, so the small
// race between counting and inserting is accepted.
const (
	MaxBoardsPerPage = 12
	MaxItemsPerBoard = 60
	maxBoardName     = 80
	maxNoteText      = 400
	defaultItemWidth = 20
)

// BoardService owns the notice-board rules: who may change which board, what
// each viewer may see, and the caps.
type BoardService interface {
	// View returns the page's boards as v may see them.
	View(ctx context.Context, campaignID, entityID string, v Viewer) (*BoardsView, error)

	CreateBoard(ctx context.Context, campaignID, entityID string, v Viewer, name, who string) (*BoardView, error)
	PatchBoard(ctx context.Context, campaignID, entityID, boardID string, v Viewer, p BoardPatch) (*BoardSummary, error)
	DeleteBoard(ctx context.Context, campaignID, entityID, boardID string, v Viewer) error
	SetOrder(ctx context.Context, campaignID, entityID string, v Viewer, ids []string) error
	SetLooks(ctx context.Context, campaignID, entityID string, v Viewer, p LooksPatch) (Looks, error)
	// ClearPlayerItems removes everything players pinned (not the DM's own
	// pieces, and never notices) and returns how many items went.
	ClearPlayerItems(ctx context.Context, campaignID, entityID, boardID string, v Viewer) (int, error)

	CreateItem(ctx context.Context, campaignID, entityID, boardID string, v Viewer, in ItemInput) (*ItemView, error)
	PatchItem(ctx context.Context, campaignID, entityID, boardID, itemID string, v Viewer, p ItemPatch) (*ItemView, error)
	DeleteItem(ctx context.Context, campaignID, entityID, boardID, itemID string, v Viewer) error
}

type boardService struct {
	repo   BoardRepository
	quests questDocs
	gate
	maps    MapDirectory
	members MemberNames
}

// questDocs is the slice of QuestRepository the boards need, to show a
// notice's title and reward.
type questDocs interface {
	GetMany(ctx context.Context, campaignID string, entityIDs []string) (map[string][]byte, error)
}

// NewBoardService builds the service.
func NewBoardService(repo BoardRepository, quests QuestRepository, entities EntityDirectory, maps MapDirectory, members MemberNames) BoardService {
	return &boardService{repo: repo, quests: quests, gate: gate{entities: entities}, maps: maps, members: members}
}

// canChange says who may pin to / edit a board.
func canChange(b *Board, v Viewer) bool {
	switch b.Who {
	case WhoAll:
		return v.IsPlayer()
	case WhoScribe:
		return v.IsScribe()
	default:
		return v.IsDM
	}
}

// isManager may edit or remove anyone's item on the board.
func isManager(b *Board, v Viewer) bool {
	return v.IsDM || (b.Who == WhoScribe && v.IsScribe())
}

func requireDM(v Viewer) error {
	if !v.IsDM {
		return errForbidden("only the campaign owner or a member with DM access may do this")
	}
	return nil
}

func validWho(w string) bool { return w == WhoDM || w == WhoScribe || w == WhoAll }

func cleanBoardName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxBoardName {
		return "", errInvalid("board name must be 1 to 80 characters")
	}
	return s, nil
}

func (s *boardService) summary(b *Board, v Viewer) BoardSummary {
	return BoardSummary{ID: b.ID, Name: b.Name, Who: b.Who, CanChange: canChange(b, v)}
}

func (s *boardService) View(ctx context.Context, campaignID, entityID string, v Viewer) (*BoardsView, error) {
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	looks, err := s.repo.GetLooks(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	boards, err := s.repo.ListBoards(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ids := make([]string, len(boards))
	for i, b := range boards {
		ids[i] = b.ID
	}
	items, err := s.repo.ListItems(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	views, _, err := s.buildViews(ctx, campaignID, v, items)
	if err != nil {
		return nil, err
	}
	out := &BoardsView{
		CanManage: v.IsDM,
		Me:        Me{UserID: v.UserID, IsDM: v.IsDM, Role: v.MemberRole},
		Looks:     looks,
		Boards:    make([]BoardView, 0, len(boards)),
	}
	for i := range boards {
		bv := BoardView{BoardSummary: s.summary(&boards[i], v), Items: views[boards[i].ID]}
		if bv.Items == nil {
			bv.Items = []ItemView{}
		}
		out.Boards = append(out.Boards, bv)
	}
	return out, nil
}

// buildViews turns stored items into what v may see, grouped by board. It
// drops what v must not know exists: hidden items, notices of quests v cannot
// open, items whose target is gone, and strings with a missing end. Pages v
// cannot open stay as anonymous "concealed" pins so the board layout holds.
func (s *boardService) buildViews(ctx context.Context, campaignID string, v Viewer, items []Item) (map[string][]ItemView, map[string]bool, error) {
	var entIDs, mapIDs, noticeIDs, ownerIDs []string
	for _, it := range items {
		switch it.Kind {
		case KindNotice:
			entIDs = append(entIDs, it.RefID)
			noticeIDs = append(noticeIDs, it.RefID)
		case KindPage:
			entIDs = append(entIDs, it.RefID)
		case KindMap:
			mapIDs = append(mapIDs, it.RefID)
		}
		if it.OwnerUserID != "" {
			ownerIDs = append(ownerIDs, it.OwnerUserID)
		}
	}
	var ents map[string]EntityInfo
	var viewable map[string]bool
	var maps map[string]MapInfo
	var docs map[string][]byte
	var names map[string]string
	var err error
	if len(entIDs) > 0 {
		if ents, viewable, err = s.viewable(ctx, campaignID, v, entIDs); err != nil {
			return nil, nil, err
		}
	}
	if len(mapIDs) > 0 {
		if maps, err = s.maps.Maps(ctx, campaignID, mapIDs); err != nil {
			return nil, nil, apperrorInternal(err)
		}
	}
	if len(noticeIDs) > 0 {
		if docs, err = s.quests.GetMany(ctx, campaignID, noticeIDs); err != nil {
			return nil, nil, apperrorInternal(err)
		}
	}
	if len(ownerIDs) > 0 {
		if names, err = s.members.DisplayNames(ctx, campaignID, ownerIDs); err != nil {
			return nil, nil, apperrorInternal(err)
		}
	}

	out := map[string][]ItemView{}
	visible := map[string]bool{}
	base := func(it Item) ItemView {
		iv := ItemView{ID: it.ID, Kind: it.Kind, X: it.X, Y: it.Y, W: it.W, R: it.R}
		iv.ByDM = it.ByDM
		iv.Mine = it.OwnerUserID != "" && it.OwnerUserID == v.UserID
		// Member names are for members; a guest of a public campaign sees
		// the pins without who pinned them.
		if it.OwnerUserID != "" && v.IsPlayer() {
			iv.OwnerName = names[it.OwnerUserID]
			iv.OwnerInitial = initial(iv.OwnerName)
		}
		if v.IsDM {
			h := it.Hidden
			iv.Hidden = &h
		}
		return iv
	}
	// Strings last: they depend on which ends survived.
	var strs []Item
	for _, it := range items {
		if it.Hidden && !v.IsDM {
			continue
		}
		var iv ItemView
		switch it.Kind {
		case KindString:
			strs = append(strs, it)
			continue
		case KindNote:
			iv = base(it)
			iv.Text = it.Text
		case KindMap:
			m, ok := maps[it.RefID]
			if !ok {
				continue
			}
			iv = base(it)
			iv.MapID, iv.Name = m.ID, m.Name
			iv.URL = fmt.Sprintf("/campaigns/%s/maps/%s", campaignID, m.ID)
		case KindPage:
			e, exists := ents[it.RefID]
			if !exists {
				continue
			}
			iv = base(it)
			if viewable[it.RefID] {
				iv.EntityID, iv.Name, iv.TypeName, iv.ImagePath = e.ID, e.Name, e.TypeName, e.ImagePath
				iv.URL = fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, e.ID)
			} else {
				iv.Concealed = true
			}
		case KindNotice:
			e, exists := ents[it.RefID]
			if !exists || !viewable[it.RefID] {
				continue
			}
			q := defaultQuest(e.Name)
			if raw, ok := docs[it.RefID]; ok {
				_ = jsonUnmarshal(raw, &q) // a damaged sheet reads as defaults, never as an error on the board
			}
			fillDefaults(&q, e.Name)
			// A notice the DM hid on its own quest page stays hidden here too.
			if q.Layout.Notice.Hidden && !v.IsDM {
				continue
			}
			iv = base(it)
			iv.QuestID, iv.Title, iv.Kicker = e.ID, q.Notice.Title, q.Notice.Kicker
			iv.Blurb, iv.Status, iv.HasSheet = q.Notice.Blurb, q.Status, true
			// The reward travels with its tag: hidden on the quest page, hidden here.
			if v.IsDM || !q.Layout.Tag.Hidden {
				iv.Reward = q.Notice.Reward
			}
		default:
			continue
		}
		visible[it.ID] = true
		out[it.BoardID] = append(out[it.BoardID], iv)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	for _, it := range strs {
		f, t := byID[it.FromID], byID[it.ToID]
		if !visible[it.FromID] || !visible[it.ToID] || f.BoardID != it.BoardID || t.BoardID != it.BoardID {
			continue
		}
		iv := base(it)
		iv.From, iv.To = it.FromID, it.ToID
		visible[it.ID] = true
		out[it.BoardID] = append(out[it.BoardID], iv)
	}
	return out, visible, nil
}

func initial(name string) string {
	for _, r := range name {
		return string(unicode.ToUpper(r))
	}
	return ""
}

func (s *boardService) CreateBoard(ctx context.Context, campaignID, entityID string, v Viewer, name, who string) (*BoardView, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	name, err := cleanBoardName(name)
	if err != nil {
		return nil, err
	}
	if who == "" {
		who = WhoDM
	}
	if !validWho(who) {
		return nil, errInvalid("who must be dm, scribe or all")
	}
	boards, err := s.repo.ListBoards(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(boards) >= MaxBoardsPerPage {
		return nil, errInvalid("a page can hold at most 12 boards")
	}
	order := 0
	for _, b := range boards {
		if b.SortOrder >= order {
			order = b.SortOrder + 1
		}
	}
	b := Board{ID: uuid.NewString(), CampaignID: campaignID, EntityID: entityID, Name: name, Who: who, SortOrder: order}
	if err := s.repo.InsertBoard(ctx, b); err != nil {
		return nil, apperrorInternal(err)
	}
	return &BoardView{BoardSummary: s.summary(&b, v), Items: []ItemView{}}, nil
}

func (s *boardService) PatchBoard(ctx context.Context, campaignID, entityID, boardID string, v Viewer, p BoardPatch) (*BoardSummary, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	b, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID)
	if err != nil {
		return nil, passThrough(err)
	}
	if n, ok := p.Name.Get(); ok {
		if b.Name, err = cleanBoardName(n); err != nil {
			return nil, err
		}
	}
	if w, ok := p.Who.Get(); ok {
		if !validWho(w) {
			return nil, errInvalid("who must be dm, scribe or all")
		}
		b.Who = w
	}
	if err := s.repo.UpdateBoard(ctx, *b); err != nil {
		return nil, apperrorInternal(err)
	}
	sum := s.summary(b, v)
	return &sum, nil
}

func (s *boardService) DeleteBoard(ctx context.Context, campaignID, entityID, boardID string, v Viewer) error {
	if err := requireDM(v); err != nil {
		return err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return err
	}
	if _, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID); err != nil {
		return passThrough(err)
	}
	if err := s.repo.DeleteBoard(ctx, campaignID, entityID, boardID); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

func (s *boardService) SetOrder(ctx context.Context, campaignID, entityID string, v Viewer, ids []string) error {
	if err := requireDM(v); err != nil {
		return err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return err
	}
	boards, err := s.repo.ListBoards(ctx, campaignID, entityID)
	if err != nil {
		return apperrorInternal(err)
	}
	// Exactly the page's boards, each once: a partial or foreign list would
	// silently reorder or leak ids.
	have := map[string]bool{}
	for _, b := range boards {
		have[b.ID] = true
	}
	if len(ids) != len(boards) {
		return errInvalid("order must list every board on the page exactly once")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !have[id] || seen[id] {
			return errInvalid("order must list every board on the page exactly once")
		}
		seen[id] = true
	}
	if err := s.repo.SetOrder(ctx, campaignID, entityID, ids); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

func (s *boardService) SetLooks(ctx context.Context, campaignID, entityID string, v Viewer, p LooksPatch) (Looks, error) {
	if err := requireDM(v); err != nil {
		return Looks{}, err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return Looks{}, err
	}
	l, err := s.repo.GetLooks(ctx, campaignID, entityID)
	if err != nil {
		return Looks{}, apperrorInternal(err)
	}
	if b, ok := p.Board.Get(); ok {
		if !validLook(b) {
			return Looks{}, errInvalid("board look is not valid")
		}
		l.Board = b
	}
	if x, ok := p.Ledger.Get(); ok {
		if !validLook(x) {
			return Looks{}, errInvalid("ledger look is not valid")
		}
		l.Ledger = x
	}
	if err := s.repo.SetLooks(ctx, campaignID, entityID, l); err != nil {
		return Looks{}, apperrorInternal(err)
	}
	return l, nil
}

func (s *boardService) ClearPlayerItems(ctx context.Context, campaignID, entityID, boardID string, v Viewer) (int, error) {
	if err := requireDM(v); err != nil {
		return 0, err
	}
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return 0, err
	}
	if _, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID); err != nil {
		return 0, passThrough(err)
	}
	items, err := s.repo.ListItems(ctx, campaignID, []string{boardID})
	if err != nil {
		return 0, apperrorInternal(err)
	}
	// Anything posted by a DM-capable user stays, and notices never go.
	gone := map[string]bool{}
	for _, it := range items {
		if it.Kind != KindNotice && !it.ByDM {
			gone[it.ID] = true
		}
	}
	n := len(gone)
	var del []string
	for _, it := range items {
		// A string tied to a removed item would dangle.
		if gone[it.ID] || (it.Kind == KindString && (gone[it.FromID] || gone[it.ToID])) {
			del = append(del, it.ID)
		}
	}
	if err := s.repo.DeleteItems(ctx, campaignID, boardID, del); err != nil {
		return 0, apperrorInternal(err)
	}
	return n, nil
}

func (s *boardService) CreateItem(ctx context.Context, campaignID, entityID, boardID string, v Viewer, in ItemInput) (*ItemView, error) {
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	b, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID)
	if err != nil {
		return nil, passThrough(err)
	}
	if !canChange(b, v) {
		return nil, errForbidden("you cannot pin to this board")
	}
	it := Item{
		ID: uuid.NewString(), BoardID: b.ID, CampaignID: campaignID, Kind: in.Kind,
		X: clamp(in.X, 0, 100), Y: clamp(in.Y, 0, 100), W: in.W, R: clamp(in.R, -15, 15),
		OwnerUserID: v.UserID, ByDM: v.IsDM,
	}
	if math.IsNaN(in.W) || in.W == 0 {
		it.W = defaultItemWidth
	}
	it.W = clamp(it.W, 5, 70)

	existing, err := s.repo.ListItems(ctx, campaignID, []string{b.ID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(existing) >= MaxItemsPerBoard {
		return nil, errInvalid("this board is full")
	}

	switch in.Kind {
	case KindNotice:
		if !v.IsDM && (b.Who != WhoScribe || !v.IsScribe()) {
			return nil, errForbidden("only the DM or a scribe on a scribe board may pin a quest notice")
		}
		if err := s.requireTarget(ctx, campaignID, v, in.RefID); err != nil {
			return nil, err
		}
		it.RefID = in.RefID
	case KindPage:
		if err := s.requireTarget(ctx, campaignID, v, in.RefID); err != nil {
			return nil, err
		}
		it.RefID = in.RefID
	case KindMap:
		if !idPattern.MatchString(in.RefID) {
			return nil, errInvalid("map id is not valid")
		}
		found, err := s.maps.Maps(ctx, campaignID, []string{in.RefID})
		if err != nil {
			return nil, apperrorInternal(err)
		}
		if _, ok := found[in.RefID]; !ok {
			return nil, errInvalid("that map is not in this campaign")
		}
		it.RefID = in.RefID
	case KindNote:
		t := strings.TrimSpace(in.Text)
		if t == "" || utf8.RuneCountInString(t) > maxNoteText {
			return nil, errInvalid("a note must be 1 to 400 characters")
		}
		it.Text = t
	case KindString:
		_, vis, err := s.buildViews(ctx, campaignID, v, existing)
		if err != nil {
			return nil, err
		}
		kinds := map[string]string{}
		for _, e := range existing {
			kinds[e.ID] = e.Kind
		}
		if in.From == in.To || !vis[in.From] || !vis[in.To] || kinds[in.From] == KindString || kinds[in.To] == KindString {
			return nil, errInvalid("a string must join two different items on this board that you can see")
		}
		it.FromID, it.ToID = in.From, in.To
	default:
		return nil, errInvalid("kind must be notice, note, page, map or string")
	}
	if err := s.repo.InsertItem(ctx, it); err != nil {
		return nil, apperrorInternal(err)
	}
	return s.singleView(ctx, campaignID, v, it)
}

// requireTarget checks a pinned page is in the campaign and visible to the
// pinner, so pinning cannot be used to probe or reveal hidden pages.
func (s *boardService) requireTarget(ctx context.Context, campaignID string, v Viewer, id string) error {
	if !idPattern.MatchString(id) {
		return errInvalid("page id is not valid")
	}
	if _, ok, err := s.viewable(ctx, campaignID, v, []string{id}); err != nil {
		return err
	} else if !ok[id] {
		return errInvalid("that page is not available")
	}
	return nil
}

// singleView renders one item as v sees it. The whole board is built so a
// string finds its ends; an item v may not see reads as NotFound.
func (s *boardService) singleView(ctx context.Context, campaignID string, v Viewer, it Item) (*ItemView, error) {
	all, err := s.repo.ListItems(ctx, campaignID, []string{it.BoardID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	views, _, err := s.buildViews(ctx, campaignID, v, all)
	if err != nil {
		return nil, err
	}
	for i := range views[it.BoardID] {
		if views[it.BoardID][i].ID == it.ID {
			return &views[it.BoardID][i], nil
		}
	}
	return nil, errNotFound("item")
}

// authorize decides whether v may edit or remove it. Hidden items do not
// exist for non-DMs; the owner needs the board to still be open to them.
func authorize(b *Board, it *Item, v Viewer) error {
	if it.Hidden && !v.IsDM {
		return errNotFound("item")
	}
	owner := it.OwnerUserID != "" && it.OwnerUserID == v.UserID
	if (owner && canChange(b, v)) || isManager(b, v) {
		return nil
	}
	return errForbidden("you can only change your own pins")
}

func (s *boardService) PatchItem(ctx context.Context, campaignID, entityID, boardID, itemID string, v Viewer, p ItemPatch) (*ItemView, error) {
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	b, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID)
	if err != nil {
		return nil, passThrough(err)
	}
	it, err := s.repo.GetItem(ctx, campaignID, b.ID, itemID)
	if err != nil {
		return nil, passThrough(err)
	}
	if err := authorize(b, it, v); err != nil {
		return nil, err
	}
	if it.Kind == KindString {
		return nil, errInvalid("a string cannot be moved or edited; remove it instead")
	}
	if p.Hidden.Present() && !v.IsDM {
		return nil, errForbidden("only the DM can hide a pin")
	}
	it.X = clamp(p.X.Val(it.X), 0, 100)
	it.Y = clamp(p.Y.Val(it.Y), 0, 100)
	it.W = clamp(p.W.Val(it.W), 5, 70)
	it.R = clamp(p.R.Val(it.R), -15, 15)
	it.Hidden = p.Hidden.Val(it.Hidden)
	if t, ok := p.Text.Get(); ok {
		if it.Kind != KindNote {
			return nil, errInvalid("only a note has text")
		}
		t = strings.TrimSpace(t)
		if t == "" || utf8.RuneCountInString(t) > maxNoteText {
			return nil, errInvalid("a note must be 1 to 400 characters")
		}
		it.Text = t
	}
	if err := s.repo.UpdateItem(ctx, *it); err != nil {
		return nil, apperrorInternal(err)
	}
	return s.singleView(ctx, campaignID, v, *it)
}

func (s *boardService) DeleteItem(ctx context.Context, campaignID, entityID, boardID, itemID string, v Viewer) error {
	if _, err := s.requireViewable(ctx, campaignID, entityID, v); err != nil {
		return err
	}
	b, err := s.repo.GetBoard(ctx, campaignID, entityID, boardID)
	if err != nil {
		return passThrough(err)
	}
	it, err := s.repo.GetItem(ctx, campaignID, b.ID, itemID)
	if err != nil {
		return passThrough(err)
	}
	if err := authorize(b, it, v); err != nil {
		return err
	}
	all, err := s.repo.ListItems(ctx, campaignID, []string{b.ID})
	if err != nil {
		return apperrorInternal(err)
	}
	del := []string{it.ID}
	for _, o := range all {
		if o.Kind == KindString && (o.FromID == it.ID || o.ToID == it.ID) {
			del = append(del, o.ID)
		}
	}
	if err := s.repo.DeleteItems(ctx, campaignID, b.ID, del); err != nil {
		return apperrorInternal(err)
	}
	return nil
}
