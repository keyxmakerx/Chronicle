package app

import (
	"context"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
)

// screenKitMembers is the part of the campaigns service the Style drawer's
// kit row reads.
type screenKitMembers interface {
	MemberLookPicks(ctx context.Context, cc *campaigns.CampaignContext, userID string) (map[string]string, error)
}

// screenKitAdapter tells a docked screen's Style drawer which kit it wears
// and whether the viewer may pick their own. The pick is the member look
// pick for the DM Screen, so the owner's "who can change" setting decides,
// as it does on the Your look page.
type screenKitAdapter struct {
	members screenKitMembers
}

// ScreenKit implements dmscreen.KitSource.
func (a *screenKitAdapter) ScreenKit(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string) (dmscreen.KitInfo, error) {
	ap := cc.Campaign.ParseSettings().Appearance
	info := dmscreen.KitInfo{Campaign: campaignDockKit(ap), Owner: cc.MemberRole >= campaigns.RoleOwner}
	// The players' screen has no look part of its own yet: it follows the
	// campaign.
	if screen != dmscreen.ScreenDM {
		return info, nil
	}
	part, _ := campaigns.LookPartByKey(campaigns.LookPartDmScreen)
	if !part.MayPick(ap, cc) {
		return info, nil
	}
	info.CanPick, info.Part = true, part.Key
	picks, err := a.members.MemberLookPicks(ctx, cc, userID)
	if err != nil {
		return info, err
	}
	info.Mine = picks[part.Key]
	return info, nil
}

// campaignDockKit is the kit a docked screen wears from the campaign alone,
// as the stylesheet draws it: the DM Screen's own kit when the owner gave it
// one, else the pages' kit under the Material page look, else none.
func campaignDockKit(ap *campaigns.Appearance) string {
	if ap == nil {
		return "material"
	}
	if slices.Contains(campaigns.AppearanceStyleKits, ap.DmScreenKit) {
		return ap.DmScreenKit
	}
	if ap.PageLook == "annals" && slices.Contains(campaigns.AppearanceStyleKits, ap.StyleKit) {
		return ap.StyleKit
	}
	return "material"
}
