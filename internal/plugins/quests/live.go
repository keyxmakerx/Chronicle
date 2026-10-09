package quests

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Announcer tells open quest boards and notice boards that something they
// show has changed. It carries ids only: each browser fetches again through
// its own route, so what anyone sees is still decided by the services. It is
// implemented in internal/app over the websocket hub.
type Announcer interface {
	// QuestChanged says a quest sheet was saved at version.
	QuestChanged(campaignID, entityID string, version int, dmOnly bool)
	// BoardsChanged says a home's boards, items or looks changed.
	BoardsChanged(campaignID string, h Home, dmOnly bool)
}

// WithAnnouncer wraps the services so every successful change is announced.
// A nil announcer returns them unchanged.
func WithAnnouncer(q QuestService, b BoardService, entities EntityDirectory, a Announcer) (QuestService, BoardService) {
	if a == nil {
		return q, b
	}
	l := liveAudience{entities: entities}
	return &liveQuests{QuestService: q, a: a, aud: l}, &liveBoards{BoardService: b, a: a, aud: l}
}

// liveAudience decides who may hear about a page. Only a page a plain player
// may open is announced to players; anything else (hidden, DM-only, or shown
// to named players only) goes to the DM team alone, because even an id in a
// message would tell a player that the page exists.
type liveAudience struct {
	entities EntityDirectory
}

func (l liveAudience) dmOnly(ctx context.Context, campaignID, entityID string) bool {
	ok, err := l.entities.FilterViewable(ctx, campaignID, []string{entityID}, permissions.RolePlayer, "")
	return err != nil || !ok[entityID]
}

// homeDMOnly: a category has no visibility of its own (every member can open
// its page), so only a page home is checked.
func (l liveAudience) homeDMOnly(ctx context.Context, campaignID string, h Home) bool {
	if h.IsType() {
		return false
	}
	return l.dmOnly(ctx, campaignID, h.EntityID)
}

type liveQuests struct {
	QuestService
	a   Announcer
	aud liveAudience
}

func (s *liveQuests) Put(ctx context.Context, campaignID, entityID string, v Viewer, p QuestPatch) (*DMQuestView, error) {
	out, err := s.QuestService.Put(ctx, campaignID, entityID, v, p)
	if err == nil && out != nil {
		s.a.QuestChanged(campaignID, entityID, out.Version, s.aud.dmOnly(ctx, campaignID, entityID))
	}
	return out, err
}

type liveBoards struct {
	BoardService
	a   Announcer
	aud liveAudience
}

// done announces a change to h's boards when err is nil, and passes err on.
func (s *liveBoards) done(ctx context.Context, campaignID string, h Home, err error) error {
	if err == nil {
		s.a.BoardsChanged(campaignID, h, s.aud.homeDMOnly(ctx, campaignID, h))
	}
	return err
}

func (s *liveBoards) CreateBoard(ctx context.Context, campaignID string, h Home, v Viewer, name, who string) (*BoardView, error) {
	out, err := s.BoardService.CreateBoard(ctx, campaignID, h, v, name, who)
	return out, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) PatchBoard(ctx context.Context, campaignID string, h Home, boardID string, v Viewer, p BoardPatch) (*BoardSummary, error) {
	out, err := s.BoardService.PatchBoard(ctx, campaignID, h, boardID, v, p)
	return out, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) DeleteBoard(ctx context.Context, campaignID string, h Home, boardID string, v Viewer) error {
	return s.done(ctx, campaignID, h, s.BoardService.DeleteBoard(ctx, campaignID, h, boardID, v))
}

func (s *liveBoards) SetOrder(ctx context.Context, campaignID string, h Home, v Viewer, ids []string) error {
	return s.done(ctx, campaignID, h, s.BoardService.SetOrder(ctx, campaignID, h, v, ids))
}

func (s *liveBoards) SetLooks(ctx context.Context, campaignID string, h Home, v Viewer, p LooksPatch) (Looks, error) {
	out, err := s.BoardService.SetLooks(ctx, campaignID, h, v, p)
	return out, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) ClearPlayerItems(ctx context.Context, campaignID string, h Home, boardID string, v Viewer) (int, error) {
	n, err := s.BoardService.ClearPlayerItems(ctx, campaignID, h, boardID, v)
	return n, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) CreateItem(ctx context.Context, campaignID string, h Home, boardID string, v Viewer, in ItemInput) (*ItemView, error) {
	out, err := s.BoardService.CreateItem(ctx, campaignID, h, boardID, v, in)
	return out, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) PatchItem(ctx context.Context, campaignID string, h Home, boardID, itemID string, v Viewer, p ItemPatch) (*ItemView, error) {
	out, err := s.BoardService.PatchItem(ctx, campaignID, h, boardID, itemID, v, p)
	return out, s.done(ctx, campaignID, h, err)
}

func (s *liveBoards) DeleteItem(ctx context.Context, campaignID string, h Home, boardID, itemID string, v Viewer) error {
	return s.done(ctx, campaignID, h, s.BoardService.DeleteItem(ctx, campaignID, h, boardID, itemID, v))
}
