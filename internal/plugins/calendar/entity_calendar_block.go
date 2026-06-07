// entity_calendar_block.go — the entity-page calendar embed
// (C-CAL-ENTITY-PAGE-EMBED, Phase 6 pulled forward). The template-context
// `entity_calendar` block's data-prep: builds the compact worldstate band
// seed (#401 BuildWorldStateSeed) + this entity's linked events (#402
// EventsForEntity), dm_only filtered by the viewer's role, then hands them to
// the read-only view component. Registered (with the calendar service injected
// via closure) in internal/app/routes.go — same pattern as map_editor.
package calendar

import (
	"context"
	"encoding/json"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityCalendarBlock builds the entity-page calendar embed component. Does
// its IO synchronously (context.Background()) inside the block-render path —
// the established service-backed-block pattern. Degrades gracefully: no
// calendar → empty-but-present state; seed/ties errors → the band/list simply
// omit rather than failing the entity page.
//
// calendarID is the instance resolved by the widget-binding framework
// (C-WIDGET-BINDING-P1-SPINE): the host's own binding, an inherited
// entity-type template binding, or — the default — the campaign's default
// calendar. An empty calendarID preserves the pre-framework behavior
// (fall back to the campaign default), so callers that don't yet resolve a
// binding keep working unchanged.
func EntityCalendarBlock(svc CalendarService, cc *campaigns.CampaignContext, entityID, userID, calendarID string) templ.Component {
	// No entity/campaign context → nothing to build from; render the friendly
	// not-found state rather than leaking a raw "entity not found" / blank
	// (C-CAL-EMBED-CONVERGE-POLISH item 2).
	if cc == nil || cc.Campaign == nil || entityID == "" {
		return entityCalendarUnavailable()
	}
	ctx := context.Background()
	role := cc.VisibilityRole()

	var (
		cal      *Calendar
		seed     *WorldStateSeed
		seedJSON string
	)
	// Resolve the bound instance first (calendarID), falling back to the
	// campaign default exactly as before when unbound. Identical output when
	// calendarID is the default calendar's id (the framework's default path).
	if calendarID != "" {
		if c, err := svc.GetCalendarByID(ctx, calendarID); err == nil && c != nil && c.CampaignID == cc.Campaign.ID {
			cal = c
		}
	}
	if cal == nil {
		if c, err := svc.GetCalendar(ctx, cc.Campaign.ID); err == nil {
			cal = c
		}
	}
	if cal != nil {
		if s, err := svc.BuildWorldStateSeed(ctx, cal.ID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, userID); err == nil {
			seed = s
			if b, e := json.Marshal(s); e == nil {
				seedJSON = string(b)
			}
		}
	}

	// This entity's linked events (#402), dm_only filtered by viewer role —
	// mirrors filterEventsByUser so players never see secret events.
	var ties []EntityEventTie
	if all, err := svc.EventsForEntity(ctx, entityID); err == nil {
		if permissions.CanSeeDmOnly(role) || userID == "" {
			ties = all
		} else {
			for _, t := range all {
				if canUserView(t.Event.Visibility, t.Event.VisibilityRules, role, userID) {
					ties = append(ties, t)
				}
			}
		}
	}

	data := CalendarV2ViewData{ActiveCalendar: cal, WorldState: seed, WorldStateJSON: seedJSON}
	return entityCalendarBlockView(cc.Campaign.ID, cal, data, ties)
}

// entityEventHref links a linked-event row to the v2 calendar at that event's
// date so the reader can jump to it in context.
func entityEventHref(campaignID string, evt Event) string {
	return "/campaigns/" + campaignID + "/calendar/v2?year=" +
		itoa(evt.Year) + "&month=" + itoa(evt.Month) + "&day=" + itoa(evt.Day)
}

// entityEventRole renders the participation role label (empty → "linked").
// (itoa for the href is the shared helper in subresource_v2.go.)
func entityEventRole(t EntityEventTie) string {
	if t.ParticipationRole == "" {
		return "linked"
	}
	return t.ParticipationRole
}
