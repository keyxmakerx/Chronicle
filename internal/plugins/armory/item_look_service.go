package armory

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// LookEntityFinder finds the pages a look can sit on. Implemented in
// internal/app over the entities service. A missing entity, or one in
// another campaign, is a clean ok=false.
type LookEntityFinder interface {
	EntityName(ctx context.Context, campaignID, entityID string) (name string, ok bool, err error)
}

// LookSource is the page an item was made from, as one viewer may see it.
// ID, Name and URL are empty when the viewer cannot see that page, so the
// item still carries its look without naming a hidden page.
type LookSource struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name,omitempty"`
	URL  string          `json:"url,omitempty"`
	Look json.RawMessage `json:"look"`
}

// LookView is what the Look block reads: the page's own look (nil when it
// has none) and the look it carries from the page it was made from (nil
// when it was made from nothing, or from a page with no look).
type LookView struct {
	Look   json.RawMessage `json:"look"`
	Source *LookSource     `json:"source"`
}

// ItemLookService reads and saves looks. A look can sit on any page in the
// campaign: items carry them, and a Remnant's page holds the look the
// items made from it inherit.
type ItemLookService interface {
	// GetLook returns the page's look for a viewer who can see the page. A
	// page that is missing or hidden from the viewer is NotFound, so a
	// hidden page is not confirmed.
	GetLook(ctx context.Context, campaignID, entityID string, role int, userID string) (*LookView, error)
	// SaveLook validates raw and stores it, or clears the look when raw
	// names none, returning what the page now shows. Who may save is
	// decided by the route (Owner).
	SaveLook(ctx context.Context, campaignID, entityID, userID string, raw []byte) (*LookView, error)
}

type itemLookService struct {
	repo       ItemLookRepository
	entities   LookEntityFinder
	visibility EntityVisibilityFilter
}

// NewItemLookService creates the service. With a nil visibility filter every
// non-Owner viewer is treated as unable to see the page (fail closed).
func NewItemLookService(repo ItemLookRepository, entities LookEntityFinder, visibility EntityVisibilityFilter) ItemLookService {
	return &itemLookService{repo: repo, entities: entities, visibility: visibility}
}

func (s *itemLookService) requireEntity(ctx context.Context, campaignID, entityID string) error {
	_, ok, err := s.entities.EntityName(ctx, campaignID, entityID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("page")
	}
	return nil
}

func (s *itemLookService) canSee(ctx context.Context, campaignID, entityID string, role int, userID string) (bool, error) {
	if role >= permissions.RoleOwner {
		return true, nil
	}
	if s.visibility == nil {
		return false, nil
	}
	viewable, err := s.visibility.FilterViewableEntityIDs(ctx, campaignID, []string{entityID}, role, userID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return viewable[entityID], nil
}

func (s *itemLookService) GetLook(ctx context.Context, campaignID, entityID string, role int, userID string) (*LookView, error) {
	if err := s.requireEntity(ctx, campaignID, entityID); err != nil {
		return nil, err
	}
	ok, err := s.canSee(ctx, campaignID, entityID, role, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperror.NewNotFound("page")
	}
	look, err := s.repo.Get(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, campaignID, entityID, look, role, userID)
}

// view pairs a stored look with the look of the page it was made from.
func (s *itemLookService) view(ctx context.Context, campaignID, entityID string, look json.RawMessage, role int, userID string) (*LookView, error) {
	v := &LookView{Look: look}
	if look == nil {
		return v, nil
	}
	var l ItemLook
	if err := json.Unmarshal(look, &l); err != nil || l.MadeFrom == "" || l.MadeFrom == entityID {
		return v, nil
	}
	name, ok, err := s.entities.EntityName(ctx, campaignID, l.MadeFrom)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		// The page was deleted: the item keeps its own look, if any.
		return v, nil
	}
	src, err := s.repo.Get(ctx, campaignID, l.MadeFrom)
	if err != nil {
		return nil, err
	}
	seen, err := s.canSee(ctx, campaignID, l.MadeFrom, role, userID)
	if err != nil {
		return nil, err
	}
	if src == nil && !seen {
		return v, nil
	}
	v.Source = &LookSource{Look: ownLook(src)}
	if seen {
		v.Source.ID, v.Source.Name = l.MadeFrom, name
		v.Source.URL = "/campaigns/" + url.PathEscape(campaignID) + "/entities/" + url.PathEscape(l.MadeFrom)
	}
	return v, nil
}

// ownLook strips the source page's own MadeFrom, so inheriting is one hop
// and a viewer never learns where that page came from.
func ownLook(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var l ItemLook
	if err := json.Unmarshal(raw, &l); err != nil || l.Look == "" {
		return nil
	}
	l.MadeFrom = ""
	out, err := json.Marshal(l)
	if err != nil {
		return nil
	}
	return out
}

func (s *itemLookService) SaveLook(ctx context.Context, campaignID, entityID, userID string, raw []byte) (*LookView, error) {
	if err := s.requireEntity(ctx, campaignID, entityID); err != nil {
		return nil, err
	}
	look, cleared, err := NormalizeItemLook(raw)
	if err != nil {
		return nil, err
	}
	if cleared {
		return &LookView{}, s.repo.Delete(ctx, campaignID, entityID)
	}
	var l ItemLook
	if err := json.Unmarshal(look, &l); err != nil {
		return nil, apperror.NewInternal(err)
	}
	if l.MadeFrom != "" {
		if l.MadeFrom == entityID {
			return nil, apperror.NewBadRequest("a page cannot be made from itself")
		}
		if _, ok, err := s.entities.EntityName(ctx, campaignID, l.MadeFrom); err != nil {
			return nil, apperror.NewInternal(err)
		} else if !ok {
			return nil, apperror.NewBadRequest("unknown page")
		}
	}
	if err := s.repo.Upsert(ctx, campaignID, entityID, userID, look); err != nil {
		return nil, err
	}
	// Only the Owner saves, so the Owner's view is the right answer.
	return s.view(ctx, campaignID, entityID, look, permissions.RoleOwner, userID)
}
