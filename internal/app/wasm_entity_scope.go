package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// entityByID is the one entity-service method entityInCampaign needs.
type entityByID interface {
	GetByID(ctx context.Context, id string) (*entities.Entity, error)
}

// entityInCampaign loads an entity and answers not-found unless it belongs to
// campaignID. Extension host functions take entity ids from the extension, so
// each one is bound to the campaign the extension was called for.
func entityInCampaign(ctx context.Context, svc entityByID, campaignID, entityID string) (*entities.Entity, error) {
	ent, err := svc.GetByID(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if campaignID == "" || ent.CampaignID != campaignID {
		return nil, apperror.NewNotFound("entity not found")
	}
	return ent, nil
}
