package dmscreen

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The Adventures tab is a board of two kinds of item: Adventures pages the DM
// pinned and short loose notes. Both are for the DM team only; a pin holds
// just a page id, so what the tab shows of the page is read fresh and through
// the page's own visibility every time.

// Board item kinds.
const (
	BoardKindPage = "page"
	BoardKindNote = "note"
)

const (
	// maxBoardItems keeps the tab a handful of things at hand, not a second
	// wiki; the Adventures category is where adventures live.
	maxBoardItems = 60
	// maxBoardNote is the loose note's length in characters.
	maxBoardNote = 500
	// pickLimit is how many pages the pin picker offers at once.
	pickLimit = 12
)

// BoardItem is one stored item.
type BoardItem struct {
	ID         string
	CampaignID string
	Kind       string
	// EntityID is the pinned page; empty for a note.
	EntityID  string
	Body      string
	CreatedBy string
	CreatedAt time.Time
}

// ErrAlreadyPinned is returned by BoardRepository.AddPin when the page is
// already on the board; the service treats it as success.
var ErrAlreadyPinned = errors.New("page already pinned")

// BoardRepository stores the board. It never decides who may see what: the
// service gates every call and reads pages through AdventureSource.
type BoardRepository interface {
	// List returns the campaign's items, oldest first.
	List(ctx context.Context, campaignID string) ([]BoardItem, error)
	Count(ctx context.Context, campaignID string) (int, error)
	// Add stores an item; a second pin of the same page returns ErrAlreadyPinned.
	Add(ctx context.Context, it *BoardItem) error
	// Delete removes the item of that campaign and reports whether it existed.
	Delete(ctx context.Context, campaignID, id string) (bool, error)
}

// AdventurePage is a page of the DM-only Adventures category, as the tab shows it.
type AdventurePage struct {
	ID       string
	Name     string
	TypeName string
	// Subtitle is a short line under the name (sessions, status); may be empty.
	Subtitle string
}

// AdventureSource reads Adventures pages through the entities plugin's own
// visibility, so the tab can never name a page the viewer could not open.
type AdventureSource interface {
	// Pages returns the requested pages that are live, in the campaign, in the
	// Adventures category and viewable by v; anything else is simply absent.
	Pages(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]AdventurePage, error)
	// Search lists Adventures pages matching query (all, newest first, when
	// empty). hasCategory is false when the campaign has no Adventures
	// category, so the picker can say where to add it.
	Search(ctx context.Context, campaignID, query string, v Viewer, limit int) (pages []AdventurePage, hasCategory bool, err error)
}

// AdventuresView is the tab: items in the order they were added.
type AdventuresView struct {
	Items []AdventureItem
	// Full is true when nothing more can be added.
	Full bool
}

// AdventureItem is one card on the tab. For a pin, Kicker/Title/Subtitle name
// the page; for a note, Text is the note.
type AdventureItem struct {
	ID       string
	Kind     string
	EntityID string
	Kicker   string
	Title    string
	Subtitle string
	Text     string
}

// PickPage is one choice in the pin picker.
type PickPage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TypeName string `json:"type_name"`
	// Pinned is true for a page already on the board.
	Pinned bool `json:"pinned"`
}

// PickResult is the picker's answer.
type PickResult struct {
	Pages []PickPage `json:"pages"`
	// HasCategory is false when the campaign has no Adventures category.
	HasCategory bool `json:"has_category"`
}

func requireTeam(v Viewer) error {
	if v.Role < 2 {
		return apperror.NewForbidden("the DM Screen is for the people running this campaign")
	}
	return nil
}

// adventures builds the tab from the stored items. A pin whose page is gone,
// trashed, moved out of Adventures or hidden from the viewer is left out
// without being deleted, so restoring the page brings it back.
func (s *service) adventures(ctx context.Context, campaignID string, v Viewer) (*AdventuresView, error) {
	items, err := s.src.Board.List(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range items {
		if it.Kind == BoardKindPage && it.EntityID != "" {
			ids = append(ids, it.EntityID)
		}
	}
	pages := map[string]AdventurePage{}
	if len(ids) > 0 {
		if pages, err = s.src.Adventures.Pages(ctx, campaignID, ids, v); err != nil {
			return nil, err
		}
	}
	out := &AdventuresView{Full: len(items) >= maxBoardItems}
	for _, it := range items {
		switch it.Kind {
		case BoardKindPage:
			p, ok := pages[it.EntityID]
			if !ok {
				continue
			}
			out.Items = append(out.Items, AdventureItem{
				ID: it.ID, Kind: BoardKindPage, EntityID: p.ID,
				Kicker: p.TypeName, Title: p.Name, Subtitle: p.Subtitle,
			})
		case BoardKindNote:
			out.Items = append(out.Items, AdventureItem{ID: it.ID, Kind: BoardKindNote, Text: it.Body})
		}
	}
	return out, nil
}

func (s *service) boardReady() error {
	if s.src.Board == nil || s.src.Adventures == nil {
		return apperror.NewNotFound("the Adventures tab is not available")
	}
	return nil
}

// PinPage puts an Adventures page on the board. Pinning a page already there
// succeeds and changes nothing.
func (s *service) PinPage(ctx context.Context, campaignID string, v Viewer, entityID string) error {
	if err := requireTeam(v); err != nil {
		return err
	}
	if err := s.boardReady(); err != nil {
		return err
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return apperror.NewBadRequest("page is required")
	}
	// The page must be one the viewer can open, in this campaign, in
	// Adventures; the reply is the same not-found for all of those.
	pages, err := s.src.Adventures.Pages(ctx, campaignID, []string{entityID}, v)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if _, ok := pages[entityID]; !ok {
		return apperror.NewNotFound("page not found")
	}
	if err := s.checkRoom(ctx, campaignID); err != nil {
		return err
	}
	err = s.src.Board.Add(ctx, &BoardItem{
		ID: uuid.NewString(), CampaignID: campaignID, Kind: BoardKindPage,
		EntityID: entityID, CreatedBy: v.UserID,
	})
	if errors.Is(err, ErrAlreadyPinned) {
		return nil
	}
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// AddNote adds a loose note.
func (s *service) AddNote(ctx context.Context, campaignID string, v Viewer, text string) error {
	if err := requireTeam(v); err != nil {
		return err
	}
	if err := s.boardReady(); err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return apperror.NewBadRequest("a note needs some text")
	}
	if utf8.RuneCountInString(text) > maxBoardNote {
		return apperror.NewBadRequest("a note can be at most 500 characters")
	}
	if err := s.checkRoom(ctx, campaignID); err != nil {
		return err
	}
	if err := s.src.Board.Add(ctx, &BoardItem{
		ID: uuid.NewString(), CampaignID: campaignID, Kind: BoardKindNote,
		Body: text, CreatedBy: v.UserID,
	}); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// RemoveItem unpins a page or deletes a note.
func (s *service) RemoveItem(ctx context.Context, campaignID string, v Viewer, itemID string) error {
	if err := requireTeam(v); err != nil {
		return err
	}
	if err := s.boardReady(); err != nil {
		return err
	}
	if _, err := uuid.Parse(itemID); err != nil {
		return apperror.NewNotFound("item not found")
	}
	ok, err := s.src.Board.Delete(ctx, campaignID, itemID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("item not found")
	}
	return nil
}

// PickPages lists Adventures pages the viewer can pin, marking those already
// on the board.
func (s *service) PickPages(ctx context.Context, campaignID string, v Viewer, query string) (*PickResult, error) {
	if err := requireTeam(v); err != nil {
		return nil, err
	}
	if err := s.boardReady(); err != nil {
		return nil, err
	}
	pages, hasCat, err := s.src.Adventures.Search(ctx, campaignID, strings.TrimSpace(query), v, pickLimit)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	items, err := s.src.Board.List(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	pinned := map[string]bool{}
	for _, it := range items {
		if it.Kind == BoardKindPage {
			pinned[it.EntityID] = true
		}
	}
	res := &PickResult{HasCategory: hasCat, Pages: []PickPage{}}
	for _, p := range pages {
		res.Pages = append(res.Pages, PickPage{ID: p.ID, Name: p.Name, TypeName: p.TypeName, Pinned: pinned[p.ID]})
	}
	return res, nil
}

func (s *service) checkRoom(ctx context.Context, campaignID string) error {
	n, err := s.src.Board.Count(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n >= maxBoardItems {
		return apperror.NewConflict("the Adventures tab is full; remove something first")
	}
	return nil
}
