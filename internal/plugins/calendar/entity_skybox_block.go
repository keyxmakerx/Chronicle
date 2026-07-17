// entity_skybox_block.go — the sky-only ambient dashboard/template block
// (C-SKYBOX-WIDGET). Sky-only sibling of entity_worldstate_block: no
// hourglass, and no per-entity binding — always the campaign's default
// calendar (mirrors entity_calendar's own no-calendarID fallback), since
// "which calendar drives the sky" isn't a meaningful per-host choice without
// an hourglass to bind to it. Campaign-level (no per-entity data), so it
// works in BOTH the entity page (template) and the campaign dashboard
// contexts — same reasoning as entity_worldstate (dispatch decision D).
// Registered (calendar service injected via closure) in
// internal/app/routes.go — same pattern as entity_calendar / entity_worldstate.
package calendar

import (
	"context"
	"encoding/json"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntitySkyboxBlock builds the sky-only ambient component. Does its IO
// synchronously inside the block-render path (the established
// service-backed-block pattern). Degrades gracefully: no campaign context →
// friendly unavailable state; no calendar → "Create calendar" CTA; seed
// errors → the band omits rather than failing the page (mirrors
// entity_worldstate's own degrade rules). entityID namespaces the embedded
// worldstate-seed element id so a page with multiple sky/worldstate embeds
// doesn't collide on a duplicate id — empty on the campaign-dashboard
// context (no per-entity host).
func EntitySkyboxBlock(svc CalendarService, cc *campaigns.CampaignContext, entityID, userID string) templ.Component {
	if cc == nil || cc.Campaign == nil {
		return entitySkyboxUnavailable()
	}
	ctx := context.Background()
	role := cc.VisibilityRole()

	cal, err := svc.GetCalendar(ctx, cc.Campaign.ID)
	if err != nil {
		cal = nil
	}

	var (
		seed     *WorldStateSeed
		seedJSON string
	)
	if cal != nil {
		if s, err := svc.BuildWorldStateSeed(ctx, cal.ID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, userID); err == nil {
			seed = s
			if b, e := json.Marshal(s); e == nil {
				seedJSON = string(b)
			}
		}
	}

	data := CalendarV2ViewData{ActiveCalendar: cal, WorldState: seed, WorldStateJSON: seedJSON}
	return entitySkyboxBlockView(cc.Campaign.ID, cal, data, entityID)
}
