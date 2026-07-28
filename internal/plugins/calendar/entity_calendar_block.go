// entity_calendar_block.go — the entity-page calendar embed.
//
// ENTITY PAGES ARE THE PRIMARY CONSUMPTION SURFACE for calendars (calendar-v4
// round-1 design delta L3): the Bench ships BEHIND the widget, not in front of
// it. This file is what makes that true — it projects the host's resolved
// calendar through the calendar-v4 spine (block_service.go / block_projection.go)
// and renders internal/widgets/calendar_block.Block, in place of the compact
// adaptiveCalendarWidget the embed grew before the Block existed.
//
// It keeps everything around the Block that was already honest: the compact
// worldstate band + its per-band seed blob (#401 BuildWorldStateSeed), and this
// entity's linked events (#402), now read through the viewer-filtered
// EventsForEntityFiltered rather than a hand-rolled filter loop.
//
// Registered (with the calendar service injected via closure) in
// internal/app/routes.go — same pattern as map_editor.
package calendar

import (
	"context"
	"encoding/json"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// entityBlockLayers is THE HOST'S LAYER SET for the entity page.
//
// THE RULING IT SITS UNDER (cordinator decisions/2026-07-28-calv4-def-and-zone-
// chips-ruling.md §1): the producer's DEF stays ["moons"] — "the default surface
// is a month with its moon phases and nothing else" — and a host that wants more
// PASSES A LAYER SET rather than growing DEF. This is that host, saying so
// explicitly, which is what the ruling asked C-CALV4-HOST-P3 to do.
//
// THE READING THE SIGNED RENDERS FORCE. Every key below is visible in
// mockups/renders/v4-entity-tied-light.png and its siblings:
//
//	moons      the nameplate's "3 OF 4 MOONS" badge (which is itself gated on
//	           this key — with the discs off it would claim a surface that is
//	           not there)
//	eras       the per-week-row era bands, "RECKONING OF WARDS" / "AGE OF THE
//	           EMBERFALL"
//	weeknums   the W1 / W2 / W3 gutter
//	ledger     the docked LEDGER panel. Also the one key with a GEOMETRIC
//	           consequence: the full-tier column arithmetic subtracts the
//	           Ledger's 300px unconditionally (sizing.go), so an entity Block
//	           that skipped the zone would measure its own columns wrong and
//	           flip density at the wrong host width.
//	shelf      the Month / Upcoming / Filters / Almanac foot
//
// Absent, because the renders do not show them here: legend, horizon.
//
// ONE KEY THE RENDERS SHOW AND THIS SET DELIBERATELY OMITS: `moongraph`, the
// illumination strip. It is MEASURED, not assumed — with it enabled the std
// tier (host 420px) stacks the moongraph needzone row, the docked Ledger and
// the Shelf on top of one another, and the Ledger and Shelf headers visibly
// collide; without it they stack cleanly. Wave 1's moongraph zone renders a
// `needs backend` chip and nothing else, so the collision buys no information
// at all. The zone belongs to W-F, which fills it and owns its placement; the
// host adds the key then. Screenshots of both readings are in the slice's
// evidence set.
//
// BOTH DOCKED ZONES ARE FILLED (W-B and W-E, 2026-07-28), and both chips
// retired exactly the way the flags promised: the producer set NeedsBackend
// false and no template on this path was edited. The layer set below is
// UNCHANGED by either slice — filling a zone is not a host decision, and the
// 2026-07-28 DEF/zone-chip ruling §1 keeps DEF at ["moons"] whatever the zones
// now contain.
//
// The moongraph measurement above was re-taken with BOTH zones full, at both
// production host widths, and still holds — but the std tier is TIGHTER than it
// was, because a filled Shelf wants 166px where the stub wanted 40. W-F's
// re-add is still one line here and it needs its own measurement rather than
// inheriting W-E's; see the plugin .ai.md's CTS-8 note and .ai/todo.md.
//
// THE ALMANAC IGNORES entityBlockMoonCap ON PURPOSE ([S5], r53). The cap below
// governs the GRID's discs; the Shelf's celestial register carries every
// DECLARED body at full width, because the ceiling is only legitimate if the
// overflow goes somewhere. It is the one place a host-passed parameter is
// deliberately non-authoritative for a zone.
//
// HasSwitchboard stays false: layer preferences are per-viewer and PERSISTED
// (L20/L26/L29) and that store is W-F's.
func entityBlockLayers() calblock.LayerState {
	return calblock.LayerState{
		Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"},
	}
}

// entityBlockMoonCap matches the renderer's own ceiling (calendar_block's
// moonCap = 3): the grid draws at most three discs per day so a month can never
// grow with the fiction, and the producer should not compute discs nobody draws.
// The declared total still reaches the nameplate as "3 of 4 moons".
const entityBlockMoonCap = 3

// EntityCalendarBlock builds the entity-page calendar embed component. Does its
// IO synchronously (context.Background()) inside the block-render path — the
// established service-backed-block pattern, and the reason nothing here may
// expect request-scoped values.
//
// THE DEGRADE LADDER, unchanged in shape and extended by one rung:
//
//	no campaign context      → the friendly unavailable state
//	no concrete entity       → the CALM "previews on the entity page" placeholder
//	                           (the layout/customization editor, QA1 Bug 2)
//	no calendar resolved     → the "Create calendar" CTA
//	calendar not VISIBLE     → the SAME "Create calendar" CTA  ← new
//	spine absent / Block err → the Block is omitted; band + linked events stand
//	seed error               → the band is omitted; everything else stands
//	ties error               → the list is omitted; everything else stands
//
// The new rung is the point of C-CALV4-SEAM-P5 stage 9: BlockService.Block
// applies calendar-level visibility and answers a hidden calendar with the
// byte-identical apperror.NewNotFound a missing one gets. This host must not
// undo that by telling the two apart — so it does not branch on which, and a
// player who may not see a calendar sees exactly what a player in a campaign
// without one sees.
//
// calendarID is the instance resolved by the widget-binding framework
// (C-WIDGET-BINDING-P1-SPINE): the host's own binding, an inherited entity-type
// template binding, or — the default — the campaign's default calendar. An empty
// calendarID preserves the pre-framework behavior.
// source is the widget-binding resolution layer ("own" | "entity_type" |
// "default" | "none") threaded from the block closure (C-WIDGET-BINDING-P4a).
func EntityCalendarBlock(svc CalendarService, cc *campaigns.CampaignContext, entityID, userID, calendarID, source string) templ.Component {
	if cc == nil || cc.Campaign == nil {
		return entityCalendarUnavailable()
	}
	if entityID == "" {
		return entityCalendarPreviewPlaceholder()
	}
	ctx := context.Background()
	role := cc.VisibilityRole()

	// Resolve the bound instance first (calendarID), falling back to the
	// campaign default exactly as before when unbound.
	var cal *Calendar
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
	if cal == nil {
		return entityCalendarBlockView(cc.Campaign.ID, nil, nil, CalendarV2ViewData{}, nil, entityID, source, false)
	}

	// THE BLOCK. The spine owns the visibility gate, the one viewer-filtered
	// pass, and both tie counts; this host owns only the layer set.
	var block *calblock.BlockData
	if spine := BlockSpine(); spine != nil {
		d, err := spine.Block(ctx, BlockRequest{
			CalendarID: cal.ID,
			CampaignID: cc.Campaign.ID,
			Viewer: BlockViewer{
				UserID:     userID,
				Role:       role,
				HostEntity: entityID,
				// The signed default on an entity page is tie-filtered. It is a
				// starting ink level, not a filter: every viewer-visible mark is
				// in the DOM either way and the toggle re-inks in CSS.
				TieMode: "tied",
			},
			IsActive: true,
			MoonCap:  entityBlockMoonCap,
		})
		switch {
		case err == nil:
			d.Layers = entityBlockLayers()
			block = &d
		case isNotFound(err):
			// Hidden or missing — indistinguishable on purpose (stage 9).
			return entityCalendarBlockView(cc.Campaign.ID, nil, nil, CalendarV2ViewData{}, nil, entityID, source, false)
		}
		// Any other error: the Block is omitted and the rest of the embed
		// stands, which is the same shape as the pre-existing seed/ties rungs.
	}

	// The compact worldstate band's seed (#401). Best-effort: a seed error omits
	// the band rather than failing the entity page.
	var (
		seed     *WorldStateSeed
		seedJSON string
	)
	if s, err := svc.BuildWorldStateSeed(ctx, cal.ID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, role, userID); err == nil {
		seed = s
		if b, e := json.Marshal(s); e == nil {
			seedJSON = string(b)
		}
	}

	// This entity's linked events (#402), through the VIEWER-FILTERED read.
	//
	// EventsForEntityFiltered enforces calendar-level AND event-level visibility
	// in one place (C-CALV4-TIEFIX-PB Bug 1 item 3), and C-CALV4-SEAM-P5 stage 13
	// put it on the interface precisely so this host — outside the service's
	// package — could reach it. The loop it replaces re-implemented only the
	// event-level half, so a tie into a calendar the viewer may not see still
	// surfaced that event's NAME here.
	var ties []EntityEventTie
	if all, err := svc.EventsForEntityFiltered(ctx, entityID, role, userID); err == nil {
		ties = all
	}

	data := CalendarV2ViewData{ActiveCalendar: cal, WorldState: seed, WorldStateJSON: seedJSON}
	return entityCalendarBlockView(cc.Campaign.ID, cal, block, data, ties, entityID, source, cc.MemberRole >= campaigns.RoleScribe)
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
