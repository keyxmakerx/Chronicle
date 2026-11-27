package campaigns

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// accountCampaignsMax is how many campaigns the account rail lists; the rest
// are counted, and My campaigns lists them all.
const accountCampaignsMax = 100

// AccountCampaignLister gives Account settings the campaigns a person belongs
// to (auth.CampaignLister), so auth never imports this plugin.
type AccountCampaignLister struct {
	Service CampaignService
}

// SettingsCampaigns lists the person's campaigns for the account rail.
func (l AccountCampaignLister) SettingsCampaigns(ctx context.Context, userID string) (auth.CampaignSettingsList, error) {
	return settingsCampaigns(ctx, l.Service, userID)
}

func settingsCampaigns(ctx context.Context, svc CampaignService, userID string) (auth.CampaignSettingsList, error) {
	list, total, err := svc.List(ctx, userID, ListOptions{Page: 1, PerPage: accountCampaignsMax})
	if err != nil {
		return auth.CampaignSettingsList{}, err
	}
	out := auth.CampaignSettingsList{Campaigns: make([]auth.CampaignSettingsRef, 0, len(list))}
	for i := range list {
		out.Campaigns = append(out.Campaigns, auth.CampaignSettingsRef{ID: list[i].ID, Name: list[i].Name, Archived: list[i].IsArchived()})
	}
	if total > len(list) {
		out.More = total - len(list)
	}
	return out, nil
}

// campaignSettingsView is what one campaign's page in Account settings shows.
type campaignSettingsView struct {
	Rail       auth.CampaignSettingsList
	Campaign   *Campaign
	IsOwner    bool
	Parts      []yourLookPart
	UserID     string
	Characters []string
}

// CampaignSettings is one campaign's page in Account settings:
// GET /account/campaigns/:id. It gathers the settings that change only what
// this person sees in this campaign. The look picks save through the same
// endpoint as Your look, so the owner's limits apply unchanged. It resolves
// membership itself, with the same guest-preview refusal as the campaign
// routes' middleware, so the page keeps the account's frame instead of the
// campaign's sidebar.
func (h *Handler) CampaignSettings(c *echo.Context) error {
	cc, err := resolveCampaignContext(c, h.service)
	if err != nil {
		return err
	}
	// An owner previewing the campaign as a guest is stopped here, as a
	// real guest is (guests never reach /account).
	if handled, err := refuseDuringGuestPreview(c, cc); handled {
		return err
	}
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	v := campaignSettingsView{Campaign: cc.Campaign, IsOwner: cc.MemberRole >= RoleOwner, UserID: userID}

	if rail, err := settingsCampaigns(ctx, h.service, userID); err == nil {
		v.Rail = rail
	} else {
		slog.Warn("listing campaigns for account settings", slog.String("user_id", userID), slog.Any("error", err))
	}

	if !v.IsOwner {
		ap := cc.Campaign.ParseSettings().Appearance
		picks, err := h.service.MemberLookPicks(ctx, cc, userID)
		if err != nil {
			return err
		}
		var hasSheet func() bool
		if h.sheetChecker != nil {
			hasSheet = func() bool { return h.sheetChecker.CampaignHasSheet(ctx, cc.Campaign.ID) }
		}
		for _, p := range ShownLookParts(PickableParts(ap, cc), hasSheet) {
			v.Parts = append(v.Parts, yourLookPart{
				Part:     p,
				URL:      fmt.Sprintf("/campaigns/%s/my-look/%s", cc.Campaign.ID, p.Key),
				Current:  picks[p.Key],
				Campaign: p.campaignChoice(ap),
			})
		}
		// A failed lookup only drops the Leave box's line about characters.
		if names, err := h.service.ClaimedCharacters(ctx, userID, []string{cc.Campaign.ID}); err == nil {
			v.Characters = names[cc.Campaign.ID]
		}
	}
	return middleware.Render(c, http.StatusOK, CampaignSettingsPage(v))
}

// brushShownIn reports whether the owner keeps the Customize brush on the
// header in this campaign, from the view prefs the layout already read. An
// unread choice counts as shown, the default.
func brushShownIn(ctx context.Context, campaignID string) bool {
	p := layouts.GetViewPrefs(ctx)
	if p == nil {
		return true
	}
	return !slices.Contains(p.BrushHidden, campaignID)
}

// storeShownIn reports whether the owner keeps the Store icon on the top bar
// in this campaign, read the same way as brushShownIn.
func storeShownIn(ctx context.Context, campaignID string) bool {
	p := layouts.GetViewPrefs(ctx)
	if p == nil {
		return true
	}
	return !slices.Contains(p.StoreHidden, campaignID)
}
