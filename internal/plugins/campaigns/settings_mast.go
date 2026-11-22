// settings_mast.go builds the banner every Campaign settings page wears above
// its tabs: the campaign's name, its cover and emblem, whether it is private,
// its game system and how many people are in it. The frame lives in layouts
// and asks for it through layouts.SetManageMast.

package campaigns

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// ManageMast reads the banner for the campaign in the render context. The
// frame asks only for the owner; this checks again, since a banner names
// things only the owner should see on these pages. Nil when the campaign
// cannot be read, and the frame then draws its plain title.
func (h *Handler) ManageMast(ctx context.Context) *layouts.ManageMast {
	if !layouts.InCampaign(ctx) || layouts.GetCampaignRole(ctx) < int(RoleOwner) {
		return nil
	}
	campaign, err := h.service.GetByID(ctx, layouts.GetCampaignID(ctx))
	if err != nil || campaign == nil {
		return nil
	}
	// The same inputs the board reads, so the banner's system and head count
	// are the ones the Overview's tiles show.
	in := h.deskInputsFor(ctx, campaign, deskMembers|deskSystem)
	m := mastOf(campaign, in)
	return &m
}

// mastOf is the banner for a campaign: pure, so every state is table-tested.
// The emblem is the brand logo the Customizer sets; the cover is the backdrop.
// A system the list does not name ("custom:…") and a member count that could
// not be read are left out rather than guessed.
func mastOf(c *Campaign, in DeskInputs) layouts.ManageMast {
	s := c.ParseSettings()
	m := layouts.ManageMast{
		Name:     c.Name,
		Emblem:   strings.TrimSpace(s.BrandLogo),
		Public:   c.IsPublic,
		Archived: c.IsArchived(),
	}
	if c.BackdropPath != nil {
		m.Cover = strings.TrimSpace(*c.BackdropPath)
	}
	if in.SystemName != "" {
		m.Facts = append(m.Facts, in.SystemName)
	}
	if n := headCount(in.Members); n > 0 {
		m.Facts = append(m.Facts, plural(n, "member", "members"))
	}
	return m
}
