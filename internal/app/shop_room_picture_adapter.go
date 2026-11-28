package app

import (
	"context"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// shopRoomPictureAdapter answers the armory's question about a shop room's own
// picture from the media service, so the armory never reads media rows itself.
type shopRoomPictureAdapter struct {
	svc media.MediaService
}

// IsCampaignPicture holds a room's picture to the same rule a map's picture
// is held to: an image of this campaign, not in the Trash, and not bound to a
// note or a page (those have their own readers and are never shown inline).
func (a *shopRoomPictureAdapter) IsCampaignPicture(ctx context.Context, campaignID, mediaID string) (bool, error) {
	f, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == 404 {
			return false, nil
		}
		return false, err
	}
	if f == nil || f.CampaignID == nil || *f.CampaignID != campaignID {
		return false, nil
	}
	return f.IsImage() && !f.IsBound() && !f.InTrash(), nil
}
