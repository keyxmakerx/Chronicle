package rolls

import (
	"context"
	"io"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// CustomizeTabFactory contributes the "Dice" tab to the campaigns plugin's
// Customize page (campaigns.Handler.RegisterCustomizeTab). The campaigns
// plugin only hosts the tab; the rolls plugin owns what is in it and where
// it saves. The Customize page is the owner's, so every viewer is a Director.
func CustomizeTabFactory() func(*campaigns.CampaignContext) campaigns.CustomizeTab {
	return func(cc *campaigns.CampaignContext) campaigns.CustomizeTab {
		return campaigns.CustomizeTab{
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
	}
}
