// Package middleware provides HTTP middleware and shared handler helpers.

package middleware

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CampaignScoped is satisfied by any model that belongs to a campaign.
// Used by RequireInCampaign to validate resource ownership and prevent
// cross-campaign IDOR attacks.
type CampaignScoped interface {
	GetCampaignID() string
}

// RequireInCampaign fetches a resource by ID using the provided fetch function,
// then verifies it belongs to the expected campaign. Returns the resource if
// ownership matches, or a 404 error if the resource does not exist or belongs
// to a different campaign. This eliminates duplicated IDOR checks across
// plugin handlers.
func RequireInCampaign[T CampaignScoped](ctx context.Context, fetchFn func(ctx context.Context, id string) (T, error), resourceID, campaignID, resourceName string) (T, error) {
	resource, err := fetchFn(ctx, resourceID)
	if err != nil {
		var zero T
		return zero, err
	}
	if resource.GetCampaignID() != campaignID {
		var zero T
		return zero, apperror.NewNotFound(resourceName + " not found")
	}
	return resource, nil
}
