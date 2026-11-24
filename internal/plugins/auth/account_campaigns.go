package auth

import (
	"context"
	"log/slog"
	"strconv"
)

// CampaignSettingsRef is one campaign in Account settings' "Your campaigns"
// list, which leads to that campaign's own settings page.
type CampaignSettingsRef struct {
	ID       string
	Name     string
	Archived bool
}

// CampaignSettingsList is the campaigns a person belongs to, as the account
// rail lists them. More counts the campaigns past the ones listed, so the
// rail can say so rather than silently stopping.
type CampaignSettingsList struct {
	Campaigns []CampaignSettingsRef
	More      int
}

// CampaignLister lists the campaigns a person belongs to. It lives outside
// auth (the campaigns plugin provides it) so auth never imports campaigns.
type CampaignLister interface {
	SettingsCampaigns(ctx context.Context, userID string) (CampaignSettingsList, error)
}

// SetCampaignLister wires the "Your campaigns" list into Account settings.
// Unwired, the rail lists only the account's own sections.
func (h *Handler) SetCampaignLister(l CampaignLister) {
	h.campaigns = l
}

// settingsCampaigns reads the rail's campaigns. A failed read leaves the
// list empty: the account's own settings still work.
func (h *Handler) settingsCampaigns(ctx context.Context, userID string) CampaignSettingsList {
	if h.campaigns == nil {
		return CampaignSettingsList{}
	}
	list, err := h.campaigns.SettingsCampaigns(ctx, userID)
	if err != nil {
		slog.Warn("listing campaigns for account settings", slog.String("user_id", userID), slog.Any("error", err))
		return CampaignSettingsList{}
	}
	return list
}

// moreCampaignsText names the campaigns the rail leaves out.
func moreCampaignsText(n int) string {
	if n == 1 {
		return "1 more campaign"
	}
	return strconv.Itoa(n) + " more campaigns"
}
