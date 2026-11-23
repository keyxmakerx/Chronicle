// category_merge.go — deleting a category that still has pages by moving
// them into another category first.

package entities

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// errMergeOutOfDMOnly is why a DM-only category's pages cannot move into a
// category players can see: the category, not each page, is what hides them
// (ADR-068), so the move would publish every one.
const errMergeOutOfDMOnly = "its pages are for the DM team only, so they can only move into another category only the DM team sees"

// checkMerge is every rule a merge keeps, checked before anything is
// written. It is shared by the merge itself and the plans that preview one
// (the Customizer's Save, an AI review row).
func checkMerge(from, to *EntityType, fromHasKinds bool) error {
	switch {
	case from.ID == to.ID:
		return apperror.NewBadRequest("a category cannot move its pages into itself")
	case from.CampaignID != to.CampaignID:
		return apperror.NewNotFound("category not found")
	case fromHasKinds:
		return apperror.NewBadRequest("move or delete its kinds first")
	case from.DMOnly() && !to.DMOnly():
		return apperror.NewBadRequest(errMergeOutOfDMOnly)
	}
	return nil
}

// MergeEntityType moves every page of fromID into toID, Trash included, with
// fromID's sidebar folders and saved filters, then deletes fromID, all in one
// transaction. The pages keep their words, tags, links and field values;
// values for fields toID does not have stay stored and come back if the
// pages move to a category that has them. fromID's own page layout, home and
// fields go with it. Returns the number of pages moved.
func (s *entityService) MergeEntityType(ctx context.Context, campaignID string, fromID, toID int) (int, error) {
	from, err := s.types.FindByID(ctx, fromID)
	if err != nil {
		return 0, err
	}
	to, err := s.types.FindByID(ctx, toID)
	if err != nil {
		return 0, err
	}
	// A category of another campaign reads as missing, so a request cannot
	// probe or touch one.
	if from.CampaignID != campaignID || to.CampaignID != campaignID {
		return 0, apperror.NewNotFound("category not found")
	}
	kids, err := s.types.ListChildTypes(ctx, fromID)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("listing kinds of entity type %d: %w", fromID, err))
	}
	if err := checkMerge(from, to, len(kids) > 0); err != nil {
		return 0, err
	}

	moved, err := s.types.MoveEntitiesAndDeleteType(ctx, campaignID, fromID, toID)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("merging entity type %d into %d: %w", fromID, toID, err))
	}
	// The delete is guarded against a page added in between; a category still
	// standing is reported rather than counted as merged.
	if _, err := s.types.FindByID(ctx, fromID); err == nil {
		return int(moved), apperror.NewConflict("a page was added to it while its pages were moving; try again")
	} else if ae := (*apperror.AppError)(nil); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		return int(moved), apperror.NewInternal(fmt.Errorf("checking entity type %d is gone: %w", fromID, err))
	}

	slog.Info("entity type merged",
		slog.String("campaign_id", campaignID),
		slog.Int("from_type_id", fromID),
		slog.Int("to_type_id", toID),
		slog.Int64("pages_moved", moved),
	)
	s.events.PublishEntityTypeEvent("deleted", campaignID, from)
	return int(moved), nil
}
