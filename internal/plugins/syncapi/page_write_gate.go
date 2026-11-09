package syncapi

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pageWriteGate holds the checks a page write on the sync API must pass so
// it refuses exactly what the web page editor refuses the same caller
// (entities.Handler.pageWriteErr). PermWrite alone admits a Scribe's own
// session, and a Scribe sees and edits only what players see.
type pageWriteGate struct {
	entitySvc   entities.EntityService
	campaignSvc campaigns.CampaignService
}

// visibilityRole promotes a member with DM access to Owner, as
// CampaignContext.VisibilityRole does on the web.
func (g pageWriteGate) visibilityRole(ctx context.Context, campaignID, userID string, role int) int {
	if role >= int(campaigns.RoleOwner) {
		return role
	}
	granted, err := g.campaignSvc.IsUserDmGranted(ctx, campaignID, userID)
	if err != nil || !granted {
		return role
	}
	return int(campaigns.RoleOwner)
}

// check answers 404 for a page the caller cannot see and 403 for one they
// can see but not edit. The page must already be known to be in campaignID.
func (g pageWriteGate) check(ctx context.Context, campaignID, entityID, userID string, role int) error {
	visRole := g.visibilityRole(ctx, campaignID, userID, role)
	if visRole >= int(campaigns.RoleOwner) {
		// The owner and Co-DMs write every page, as on the web.
		return nil
	}
	access, err := g.entitySvc.CheckEntityAccess(ctx, entityID, visRole, userID)
	if err != nil || access == nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}
	if !access.CanEdit {
		return apperror.NewForbidden("you can't edit this page")
	}
	return nil
}

// canHideOrShow reports whether the caller may change whether a page is
// hidden: the owner and Co-DMs only, as on the web.
func (g pageWriteGate) canHideOrShow(ctx context.Context, campaignID, userID string, role int) bool {
	return g.visibilityRole(ctx, campaignID, userID, role) >= int(campaigns.RoleOwner)
}
