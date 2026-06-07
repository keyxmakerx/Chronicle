// entity_worldstate_block.go — the entity-page worldState timepiece embed
// (C-CAL-WORLDSTATE-WIDGETS, Phase 6 widgetization). Graduates the showcase
// hourglass-on-shelf ("the mini shelf view") into an entity-page block,
// completing "all three views entity-able" (calendar #411/#413, timeline
// Tuner #414, and now the hourglass mini-shelf).
//
// Data-prep mirrors EntityCalendarBlock: build the #401 BuildWorldStateSeed
// for the campaign's active calendar, then hand it to the read-only view. The
// view renders the worldstate widget mount (data-widget="worldstate") seeded
// server-side so the engine paints the first frame with zero client fetch; the
// worldState provider singleton drives live refreshes from there. Registered
// (calendar service injected via closure) in internal/app/routes.go — same
// pattern as entity_calendar / map_editor.
package calendar

import (
	"context"
	"encoding/json"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityWorldStateBlock builds the worldState timepiece component. Unlike
// entity_calendar (which shows an entity's linked events), the worldState is
// campaign-level — it needs only the campaign, so it works in BOTH the entity
// page (template) and campaign dashboard contexts (dispatch decision D). Does
// its IO synchronously inside the block-render path (the established
// service-backed-block pattern). Degrades gracefully: no campaign context →
// friendly unavailable state; no calendar → "Create calendar" CTA; seed errors
// → the band omits rather than failing the page.
func EntityWorldStateBlock(svc CalendarService, cc *campaigns.CampaignContext, userID string) templ.Component {
	// No campaign context → render the friendly not-found state rather than
	// leaking a raw error / blank (mirrors entity_calendar item 2).
	if cc == nil || cc.Campaign == nil {
		return entityWorldStateUnavailable()
	}
	ctx := context.Background()
	role := cc.VisibilityRole()

	var (
		cal      *Calendar
		seed     *WorldStateSeed
		seedJSON string
	)
	if c, err := svc.GetCalendar(ctx, cc.Campaign.ID); err == nil {
		cal = c
	}
	if cal != nil {
		if s, err := svc.BuildWorldStateSeed(ctx, cal.ID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, userID); err == nil {
			seed = s
			if b, e := json.Marshal(s); e == nil {
				seedJSON = string(b)
			}
		}
	}

	data := CalendarV2ViewData{ActiveCalendar: cal, WorldState: seed, WorldStateJSON: seedJSON}
	return entityWorldStateBlockView(cc.Campaign.ID, cal, data)
}
