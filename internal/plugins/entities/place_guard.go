package entities

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SetPlaceGuard wires the extra-listing checks into page moves. Unset (unit
// tests), moves behave as they did before listings existed.
func (s *entityService) SetPlaceGuard(g PlaceGuard) {
	s.placeGuard = g
}

// refuseListingCycle stops a real move that would put a page above itself
// through one of the listings, which the plain parent walk cannot see.
func (s *entityService) refuseListingCycle(ctx context.Context, campaignID, entityID, parentID string) error {
	if s.placeGuard == nil {
		return nil
	}
	cycle, err := s.placeGuard.WouldCycle(ctx, campaignID, entityID, parentID)
	if err != nil {
		return err
	}
	if cycle {
		return apperror.NewBadRequest("circular reference: the selected parent is listed below this page")
	}
	return nil
}

// dropListingUnderRealParent removes the listing a page no longer needs once
// that same parent becomes its real home. Best effort: the move already
// happened, and a leftover row is hidden by the tree's own de-duplication.
func (s *entityService) dropListingUnderRealParent(ctx context.Context, entityID, parentID string) {
	if s.placeGuard == nil {
		return
	}
	_ = s.placeGuard.DropPlace(ctx, entityID, parentID)
}
