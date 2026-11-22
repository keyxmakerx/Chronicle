package campaigns

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// AISettings is the campaigns' part of the AI engine (aiplug): the `settings`
// lookup, which says what the Game & features screen and the campaign's
// details say, so an AI knows which parts of Chronicle the campaign uses.
//
// It reads a fixed short list of fields instead of the settings JSON, so a
// field added to the settings later (a code, a key, a link) can never reach
// an AI by accident. People, roles, invites, join and guest codes, API keys,
// the Foundry link, the Your AI page, handover and deletion are not here.
type AISettings struct {
	camps  aiCampaignReader
	addons AddonLister
}

// aiCampaignReader is the part of CampaignService the lookup reads.
type aiCampaignReader interface {
	GetByID(ctx context.Context, id string) (*Campaign, error)
	GetEventTierDefinitions(ctx context.Context, campaignID string) ([]TierDefinition, error)
}

// NewAISettings builds the lookup. addons may be nil, which leaves the
// add-ons out of the Director's answer (a non-Director never gets them).
func NewAISettings(camps aiCampaignReader, addons AddonLister) *AISettings {
	return &AISettings{camps: camps, addons: addons}
}

var _ aiplug.Lookup = (*AISettings)(nil)

// LookupInfo describes `what: settings` and its tool. The tool is not
// owner-only: a player's key may call it, and gets what a player sees.
func (*AISettings) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "settings", Aliases: []string{"campaign-settings", "features"}, Keys: "none",
		Brief: "the campaign's name, description and game system, and for the Director team which add-ons are on or off and the names of the event importance levels",
		Tool: aiplug.Tool{
			Name: "campaign_settings", Title: "Campaign settings", Did: "Looked at the campaign's settings",
			Desc:       "The campaign's name, description and game system, and for the Director team which add-ons (features) are switched on and which are off and the names of the calendar's event importance levels. Nothing about people, invites, codes or keys.",
			PlayerDesc: "The campaign's name, description and game system.",
		},
	}
}

// Answer reads as the Actor. Everyone gets the name, description and game
// system. Only the Director team (owner or co-DM) also gets the add-ons on and
// off and the importance levels: an enabled add-on may have no player screen,
// so listing them would tell a player's key (or Safe privacy) more than the
// player can see.
func (s *AISettings) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "Campaign settings"
	c, err := s.camps.GetByID(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	director := a.CanAuthorDmOnly()
	var hub []PluginHubAddon
	if s.addons != nil {
		if hub, err = s.addons.ListForPluginHub(ctx, campaignID); err != nil {
			return chip, "", err
		}
	}

	var b strings.Builder
	b.WriteString("## Campaign settings\n\n")
	fmt.Fprintf(&b, "Name: %s\n", aiplug.OneLine(c.Name, 200))
	if c.Description != nil && strings.TrimSpace(*c.Description) != "" {
		fmt.Fprintf(&b, "Description: %s\n", aiplug.OneLine(*c.Description, 600))
	}
	if sys := gameSystemName(c.ParseSettings().SystemID, hub); sys != "" {
		fmt.Fprintf(&b, "Game system: %s\n", sys)
	}

	var on, off []string
	for _, ad := range featureAddons(hub) {
		switch {
		case ad.Enabled:
			on = append(on, ad.Name)
		case ad.Installed:
			off = append(off, ad.Name)
		}
	}
	if director {
		if s.addons != nil {
			fmt.Fprintf(&b, "\nAdd-ons on: %s\n", listOrNone(on))
			fmt.Fprintf(&b, "Add-ons off: %s\n", listOrNone(off))
		}
		defs, err := s.camps.GetEventTierDefinitions(ctx, campaignID)
		if err != nil {
			return chip, "", err
		}
		names := make([]string, 0, len(defs))
		for _, d := range defs {
			names = append(names, aiplug.OneLine(d.Name, 60))
		}
		fmt.Fprintf(&b, "\nEvent importance levels: %s\n", listOrNone(names))
	}
	return chip, b.String(), nil
}

// gameSystemName is the game system's name as the picker shows it. A custom
// system's id is a link, so it is named only as custom; an id with no add-on
// to name it is given as it is.
func gameSystemName(id string, hub []PluginHubAddon) string {
	if id == "" {
		id = currentSystemID("", hub)
	}
	switch {
	case id == "":
		return ""
	case strings.HasPrefix(id, "custom:"):
		return "a custom game system"
	}
	for _, ad := range hub {
		if ad.Category == addonCategorySystem && ad.Slug == id {
			return ad.Name
		}
	}
	return id
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ", ")
}
