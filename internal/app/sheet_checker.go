package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// campaignSheetChecker answers campaigns.SheetChecker from the campaign's
// enabled game system, the same one whose widget scripts the page loads. A
// campaign's custom system can hold no scripts, so it never has a sheet.
type campaignSheetChecker struct {
	addons interface {
		ListForCampaign(ctx context.Context, campaignID string) ([]addons.CampaignAddon, error)
	}
}

// CampaignHasSheet reports whether the campaign's system draws a character
// sheet. A failed lookup answers true, so a passing error never hides a part
// the member could otherwise set.
func (s campaignSheetChecker) CampaignHasSheet(ctx context.Context, campaignID string) bool {
	list, err := s.addons.ListForCampaign(ctx, campaignID)
	if err != nil {
		return true
	}
	_, sys := navAddonsFrom(list)
	return sys.Slug != "" && systems.Find(sys.Slug).HasCharacterSheet()
}
