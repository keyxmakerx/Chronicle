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
// source is the widget-binding resolution layer ("own" | "entity_type" |
// "default" | "none") threaded from the block closure (C-WIDGET-BINDING-P4a),
// so Scribe+ can see bound-vs-default and open the create-or-pick picker. Empty
// when the framework isn't wired (treated as default).
func EntityCalendarBlock(svc CalendarService, cc *campaigns.CampaignContext, entityID, userID, calendarID, source string) templ.Component {
	// No campaign context at all → soft unavailable state (never a raw error).
	// No concrete entity (entityID == "") → this template-context block is being
	// rendered on a surface without an entity (the layout/customization editor or
	// a preview), so show a CALM "previews on the entity page" placeholder rather
	// than the alarming can't-load copy (C-WIDGET-BINDING-QA1 Bug 2).
	if cc == nil || cc.Campaign == nil {
		return entityCalendarUnavailable()
	}
	if entityID == "" {
		return entityCalendarPreviewPlaceholder()
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
	return entityCalendarBlockView(cc.Campaign.ID, cal, data, ties, entityID, source, cc.MemberRole >= campaigns.RoleScribe)
}

// entityEventHref links a linked-event row to the v2 calendar at that event's
// date so the reader can jump to it in context.
func entityEventHref(campaignID string, evt Event) string {
	return "/campaigns/" + campaignID + "/calendar/v2?year=" +
		itoa(evt.Year) + "&month=" + itoa(evt.Month) + "&day=" + itoa(evt.Day)
}

// openCalendarV2Href is the "Open full calendar" target (C-WIDGET-BINDING-QA2
// Part B): the V2 shell for the resolved calendar (the bound one, or the
// campaign-active default when unbound). Empty calendarID → the active-calendar
// V2 entry. V2 not V1 — consistent with QA1 Bug 1.
func openCalendarV2Href(campaignID, calendarID string) string {
	if calendarID == "" {
		return "/campaigns/" + campaignID + "/calendar/v2"
	}
	return "/campaigns/" + campaignID + "/calendar/v2/" + calendarID
}

// entityEventRole renders the participation role label (empty → "linked").
// (itoa for the href is the shared helper in subresource_v2.go.)
func entityEventRole(t EntityEventTie) string {
	if t.ParticipationRole == "" {
		return "linked"
	}
	return t.ParticipationRole
}
