package entities

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// allPagesBare is true when the campaign has no page this reader may see and
// nothing narrowed the list, so the opening carries the whole page.
func allPagesBare(total int, opts ListOptions) bool {
	return total == 0 && opts.Query == ""
}

// allPagesOpening is All pages' opening. Each reader gets a sentence that
// offers only what they can do: the owner and Co-DMs are told what will fill
// the list, a player is never told to make something, and the New page
// action is there only for people who may make pages.
func allPagesOpening(ctx context.Context, cc *campaigns.CampaignContext, heading string, total int, opts ListOptions) layouts.AppOpeningData {
	d := layouts.AppOpeningData{
		Name:  heading,
		Empty: allPagesBare(total, opts),
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("Pages your %s shares will show here: people, places and things.", layouts.GMTerm(ctx)),
			CoDM:   "Every page you or the owner add will show here: people, places and things.",
			Owner:  "Every page of the campaign is listed here: its people, places and things. None has been made yet.",
		},
	}
	if cc.MemberRole >= campaigns.RoleScribe {
		d.Action = &layouts.AppAction{
			Label: "New page",
			Icon:  "fa-plus",
			Href:  fmt.Sprintf("/campaigns/%s/entities/new", cc.Campaign.ID),
			Attrs: layouts.NewPageOpenAttrs(),
		}
	}
	return d
}

// allPagesCount is the count beside the title: the number of pages, or how
// many match while a search is on.
func allPagesCount(total int, query string) string {
	if query != "" {
		return searchMatchSummary(total, query)
	}
	return categoryCount(total)
}
