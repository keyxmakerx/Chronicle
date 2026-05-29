// v1_deprecation_banner.go — helpers for the V1 calendar deprecation
// banner. Wave 1.5 follow-up; ships alongside Wave 1.6 closer.
//
// V1 calendar surface deprecated 2026-05-28. Sunsets 2026-08-01 per
// operator decision in `decisions/2026-05-28-cal-timeline-v2-design.md`
// Wave 1 V1/V2 additive coexistence. V1 hard removal in dispatch
// C-CAL-V1-SUNSET (to be authored post-Wave-1.6).

package calendar

import (
	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// v1BannerSwitchHref builds the "Switch to V2" link target. Routes
// to the V2 calendar shell, which resolves the user's active
// calendar via PR #363's pointer table and lands on Month view by
// default. Calendar-specific URL state (active cal id, view,
// cursor) intentionally not preserved — operator expectation is
// "switch to the new view" not "preserve granular URL state".
func v1BannerSwitchHref(cc *campaigns.CampaignContext) templ.SafeURL {
	if cc == nil || cc.Campaign == nil {
		return templ.SafeURL("/")
	}
	return templ.SafeURL("/campaigns/" + cc.Campaign.ID + "/calendar/v2")
}
