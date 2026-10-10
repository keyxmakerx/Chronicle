package entities

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxPlacesPerPage bounds how many extra spots one page can be listed in, so
// a script or a slip can't turn one page into a wall of tree rows.
const maxPlacesPerPage = 25

// PlaceService lists one page in more than one place in the page tree.
//
// A listing changes only where the tree shows a page. Who may see the page
// stays the page's own setting: every read here asks the one visibility rule
// about the page AND about the parent it is listed under, so a viewer who
// cannot see the page never sees its extra row, and one who can see the page
// but not the extra parent simply never reaches that branch.
//
// Who may add or remove a listing (anyone who can edit the page) is decided
// by the handler with CheckEntityAccess, like every other page write.
type PlaceService interface {
	// AddPlace lists entityID under parentID. Adding an existing listing is a
	// no-op. Refuses the page itself, its own real parent, any page below it
	// (through real parents or other listings) and any other campaign's page.
	AddPlace(ctx context.Context, campaignID, entityID, parentID, userID string) error
	// RemovePlace takes one listing away. The page is never touched.
	RemovePlace(ctx context.Context, campaignID, entityID, parentID string) error
	// PlacesOf returns the extra places of one page that this viewer may see.
	PlacesOf(ctx context.Context, campaignID, entityID string, role int, userID string) ([]PlaceLink, error)
	// PlacesUnder returns the listings to draw under parentIDs for this viewer.
	PlacesUnder(ctx context.Context, campaignID string, parentIDs []string, role int, userID string) ([]PlaceLink, error)
	// ExportPlaces returns every listing in a campaign for export. No viewer
	// filtering: export is the owner's whole-campaign copy.
	ExportPlaces(ctx context.Context, campaignID string) ([]Place, error)

	// PlaceGuard is how page moves keep listings honest.
	PlaceGuard
}

// PlaceGuard is the part of the listings the entity service needs while it
// moves a page for real, so a move can't build a loop through a listing or
// leave a listing next to the page's new real parent.
type PlaceGuard interface {
	// WouldCycle reports whether making parentID the real or extra parent of
	// entityID would put entityID above itself.
	WouldCycle(ctx context.Context, campaignID, entityID, parentID string) (bool, error)
	// DropPlace removes the listing of entityID under parentID, if any.
	DropPlace(ctx context.Context, entityID, parentID string) error
}

// placeEntities is the slice of the entity repository the listings need.
type placeEntities interface {
	FindByID(ctx context.Context, id string) (*Entity, error)
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

type placeService struct {
	entities placeEntities
	places   PlaceRepository
}

// NewPlaceService creates the listings service.
func NewPlaceService(entities placeEntities, places PlaceRepository) PlaceService {
	return &placeService{entities: entities, places: places}
}

// livePage loads a page that is live and in this campaign. A trashed page is
// "not found" from FindByID, and a page of another campaign is made to look
// the same, so neither end of a listing can reach across campaigns or reveal
// that a page exists elsewhere.
func (s *placeService) livePage(ctx context.Context, campaignID, id string) (*Entity, error) {
	e, err := s.entities.FindByID(ctx, id)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, apperror.NewNotFound("page not found")
	}
	return e, nil
}

func (s *placeService) AddPlace(ctx context.Context, campaignID, entityID, parentID, userID string) error {
	if entityID == parentID {
		return apperror.NewBadRequest("a page can't be listed under itself")
	}
	e, err := s.livePage(ctx, campaignID, entityID)
	if err != nil {
		return err
	}
	if _, err := s.livePage(ctx, campaignID, parentID); err != nil {
		return err
	}
	if e.ParentID != nil && *e.ParentID == parentID {
		return apperror.NewBadRequest("that is already where this page lives")
	}
	cycle, err := s.WouldCycle(ctx, campaignID, entityID, parentID)
	if err != nil {
		return err
	}
	if cycle {
		return apperror.NewBadRequest("a page can't be listed under one of its own sub-pages")
	}
	existing, err := s.places.ListOf(ctx, campaignID, entityID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	for _, l := range existing {
		if l.ParentID == parentID {
			return nil
		}
	}
	if len(existing) >= maxPlacesPerPage {
		return apperror.NewBadRequest(fmt.Sprintf("a page can be listed in at most %d extra places", maxPlacesPerPage))
	}
	if err := s.places.Insert(ctx, &Place{
		EntityID: entityID, ParentEntityID: parentID, CampaignID: campaignID, CreatedBy: userID,
	}); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *placeService) RemovePlace(ctx context.Context, campaignID, entityID, parentID string) error {
	// Loading the page first keeps another campaign's page from being reached
	// by a guessed id; the delete itself is keyed on both ids.
	if _, err := s.livePage(ctx, campaignID, entityID); err != nil {
		return err
	}
	if err := s.places.Delete(ctx, entityID, parentID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *placeService) PlacesOf(ctx context.Context, campaignID, entityID string, role int, userID string) ([]PlaceLink, error) {
	links, err := s.places.ListOf(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.visibleLinks(ctx, campaignID, links, role, userID)
}

func (s *placeService) PlacesUnder(ctx context.Context, campaignID string, parentIDs []string, role int, userID string) ([]PlaceLink, error) {
	links, err := s.places.ListUnder(ctx, campaignID, parentIDs)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.visibleLinks(ctx, campaignID, links, role, userID)
}

// visibleLinks keeps a listing only when the viewer may see BOTH its page and
// its parent, through the same predicate every list uses. One query covers
// every id.
func (s *placeService) visibleLinks(ctx context.Context, campaignID string, links []PlaceLink, role int, userID string) ([]PlaceLink, error) {
	if len(links) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(links)*2)
	for _, l := range links {
		ids = append(ids, l.EntityID, l.ParentID)
	}
	visible, err := s.entities.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := make([]PlaceLink, 0, len(links))
	for _, l := range links {
		if visible[l.EntityID] && visible[l.ParentID] {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *placeService) ExportPlaces(ctx context.Context, campaignID string) ([]Place, error) {
	out, err := s.places.ListAll(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (s *placeService) WouldCycle(ctx context.Context, campaignID, entityID, parentID string) (bool, error) {
	if entityID == parentID {
		return true, nil
	}
	// The page sits above its new parent exactly when the parent's own chain
	// of real parents and listings leads back to the page.
	above, err := s.places.AncestorIDs(ctx, campaignID, parentID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return above[entityID], nil
}

func (s *placeService) DropPlace(ctx context.Context, entityID, parentID string) error {
	if err := s.places.Delete(ctx, entityID, parentID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
