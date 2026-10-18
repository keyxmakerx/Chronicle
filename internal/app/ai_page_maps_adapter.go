package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/pageblocks"
)

// aiPageMapsAdapter implements records.PageMapsAPI over the page blocks
// service, which applies its own edit, DM-only and page-link rules.
type aiPageMapsAdapter struct{ svc pageblocks.Service }

var _ records.PageMapsAPI = aiPageMapsAdapter{}

// pageMapViewer is the operator as the page blocks service sees them. The
// AI page's routes are owner-only, so the membership role is the owner's.
func pageMapViewer(a records.Actor) pageblocks.Viewer {
	return pageblocks.Viewer{UserID: a.UserID, MemberRole: int(campaigns.RoleOwner), VisibilityRole: a.Role}
}

func (p aiPageMapsAdapter) FirstBlock(ctx context.Context, campaignID, pageID string) (string, bool, error) {
	return p.svc.FirstPageMapBlock(ctx, campaignID, pageID)
}

func (p aiPageMapsAdapter) Text(ctx context.Context, campaignID, pageID, blockID string, a records.Actor) (string, error) {
	text, _, err := p.svc.PageMapText(ctx, campaignID, pageID, blockID, pageMapViewer(a))
	return text, err
}

func (p aiPageMapsAdapter) Save(ctx context.Context, campaignID, pageID, blockID string, a records.Actor, text string) error {
	_, err := p.svc.SavePageMapText(ctx, campaignID, pageID, blockID, pageMapViewer(a), text)
	return err
}
