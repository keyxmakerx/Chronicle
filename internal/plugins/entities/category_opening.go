package entities

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// categoryBare is true when a category has no pages and nothing narrowed the
// list, so the opening carries the whole page.
func categoryBare(total int, opts ListOptions) bool {
	return total == 0 && opts.Query == "" && len(opts.TagSlugs) == 0
}

// categoryOpening is a category page's opening. Scribes and the owner can add
// a page; everyone else is told where the pages come from and offered
// nothing to click.
func categoryOpening(ctx context.Context, cc *campaigns.CampaignContext, et *EntityType, total int, opts ListOptions) layouts.AppOpeningData {
	d := layouts.AppOpeningData{
		Name:  et.NamePlural,
		Empty: categoryBare(total, opts),
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("Pages your %s shares under %s will show here.", layouts.GMTerm(ctx), et.NamePlural),
			CoDM:   fmt.Sprintf("Pages you or the owner add under %s will show here.", et.NamePlural),
			Owner:  fmt.Sprintf("Add a page and it will show here under %s.", et.NamePlural),
		},
	}
	if cc.MemberRole >= campaigns.RoleScribe {
		d.Action = &layouts.AppAction{
			Label: "New page",
			Icon:  "fa-plus",
			Href:  fmt.Sprintf("/campaigns/%s/entities/new?type=%d", cc.Campaign.ID, et.ID),
		}
	}
	return d
}

// categoryCount is the count beside the title.
func categoryCount(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}
