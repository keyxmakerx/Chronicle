package rolls

import (
	"context"
	"io"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// DiceSettingsReader is the slice of the rolls service the Dice tab's summary
// reads: the campaign's dice.
type DiceSettingsReader interface {
	Settings(ctx context.Context, campaignID string) (*Settings, error)
}

// diceSetNames are the sets' names as the dice engine shows them in the
// pickers (static/js/dice/), in DiceSetKeys order. TestDiceSetNamesMatchEngine
// keeps the two equal.
var diceSetNames = map[string]string{
	"jewelled": "Jewelled", "confetti": "Confetti glass", "acrylic": "Acrylic", "cage": "Verdant cage",
	"heartlight": "Heartlight", "gothic": "Gothic", "obsidian": "Ember obsidian", "moonstone": "Moonstone",
	"oak": "Druid's oak", "nebula": "Nebula resin", "bone": "Dragonbone", "scale": "Dragonscale",
}

// The engine's own defaults (dice.js DEFAULTS), what an empty pick means.
const (
	diceDefaultSet    = "jewelled"
	diceDefaultColour = "onyx"
)

// diceSummary words the campaign's dice for the Customize row: the set by its
// name and the colour by its key ("Jewelled, onyx"), and whether players pick
// their own. The colour's longer name lives only in the browser's catalogue,
// so its key, which is its first word, stands for it.
func diceSummary(s *Settings) campaigns.CustomizeSummary {
	set, colour := s.DiceSet, s.DiceColour
	if set == "" {
		set = diceDefaultSet
	}
	if colour == "" {
		colour = diceDefaultColour
	}
	name := diceSetNames[set]
	if name == "" {
		name = set
	}
	out := campaigns.CustomizeSummary{Text: name + ", " + strings.ReplaceAll(colour, "-", " "), Sub: "same for everyone"}
	if s.PlayersPick {
		out.Sub = "players pick their own"
	}
	return out
}

// CustomizeTabFactory contributes the "Dice" tab to the campaigns plugin's
// Customize page (campaigns.Handler.RegisterCustomizeTab). The campaigns
// plugin only hosts the tab; the rolls plugin owns what is in it and where
// it saves, and says what is chosen now from its own settings. The Customize
// page is the owner's, so every viewer is a Director. A nil reader gives the
// tab no summary.
func CustomizeTabFactory(dice DiceSettingsReader) func(*campaigns.CampaignContext) campaigns.CustomizeTab {
	return func(cc *campaigns.CampaignContext) campaigns.CustomizeTab {
		tab := campaigns.CustomizeTab{
			ID:        "dice",
			Label:     "Dice",
			Icon:      "fa-solid fa-dice-d20",
			SortOrder: 20,
			// Read at render time: the Director's own word for themselves is a
			// per-campaign setting.
			Content: templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
				return diceCustomizeSection(cc.Campaign.ID, layouts.GMTerm(ctx)).Render(ctx, w)
			}),
		}
		if dice != nil {
			tab.Summary = func(ctx context.Context) campaigns.CustomizeSummary {
				s, err := dice.Settings(ctx, cc.Campaign.ID)
				if err != nil || s == nil {
					return campaigns.CustomizeSummary{}
				}
				return diceSummary(s)
			}
		}
		return tab
	}
}
