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
//	ledger     the docked LEDGER panel — REMOVED FROM THE SEED, see below.
//	shelf      the Month / Upcoming / Filters / Almanac foot — REMOVED, see below.
//
// Absent, because the renders do not show them here: legend, horizon.
//
// THE SEED IS NOW THREE KEYS, NOT FIVE (C-CALV4-BENCH-R2 slice R2-1, [BR2-8]
// SIGNED). The operator used the product on a live client and said "on the
// entity it scrolls". The two keys removed are exactly the two that add a ZONE:
// `ledger` docks a 300px column beside the month at full tier and stacks UNDER
// it at std (which is where an entity embed lives, host 420px), and `shelf`
// adds a tabbed foot. The three kept are all INSIDE the month — discs in the
// dates, era bands above the week rows, the W1/W2/W3 gutter — and none of them
// adds a section. The tighter variant ["moons"] (= DEF) was refused: `eras` and
// `weeknums` carry the FICTION, and reducing this embed to DEF would erase the
// distinction three waves have maintained on purpose.
//
// THE CONSEQUENCE, STATED HONESTLY INCLUDING THE PART NOBODY WILL LIKE. A
// viewer who wants the Ledger back turns it on in the switchboard, once, and it
// PERSISTS — resolveBlockLayers + blockLayerPrefsFor already do this and this
// slice adds nothing to make it true. But the store's grain is
// (user_id, campaign_id) ([LYR-3] SIGNED), so turning `ledger` back on for the
// entity page ALSO TURNS IT ON FOR THE BENCH. That is the signed grain, not a
// bug, and it is written here so it is met in a document before it is met in a
// browser. Depth returns properly through R2-3's Block theater; this slice
// ships NO substitute — no expand chip, no "show more", no link to the Bench,
// no second embed — because a stopgap becomes the thing R2-3 has to delete.
//
// THE GEOMETRY WARNING THIS COMMENT USED TO CARRY WAS STALE, AND IT WAS
// MEASURED RATHER THAN ASSUMED. It read: "the full-tier column arithmetic
// subtracts the Ledger's 300px unconditionally (sizing.go), so an entity Block
// that skipped the zone would measure its own columns wrong and flip density at
// the wrong host width." Every clause of its premise is false in the render
// path, on three independent legs measured in entity_ledger_geometry_test.go:
//
//	1. ColWidth / IsNamed / IsNamedCSS have ZERO non-test callers anywhere under
//	   internal/ — the file walk in that test proves it, and a new caller fails
//	   it loudly. Nothing in the render path ever evaluates that arithmetic. The
//	   density decision is made by a CONTAINER QUERY against real layout,
//	   `@container cal-cell (min-width: 84px)`.
//	2. The full-tier body grid is `grid-template-columns: minmax(0, 1fr) auto`,
//	   so an ABSENT Ledger COLLAPSES ITS OWN TRACK rather than leaving a 300px
//	   hole. There is nothing to measure wrong.
//	3. A Block rendered without the key emits no Ledger DOM at all — the render
//	   is strictly shorter, not the same length with a placeholder in it.
//
// ONE REAL BEHAVIOUR CHANGE IS TRUE EITHER WAY, and it is a good one: without
// the Ledger beside it the month's cells are WIDER at the same host width, so
// the cal-cell container query flips NAMED COLUMNS ON at a NARROWER host. For a
// ten-day week that moves the flip from 1198px of host to 898px — a shift of
// exactly the dock's own 300px. The entity month becomes RICHER, not poorer.
//
// THE MOON CEILING STILL HAS A DESTINATION, checked rather than assumed. [S5]'s
// argument for the nameplate's "3 of 4 moons" chip is that a ceiling is only
// legitimate if the overflow goes somewhere, and the somewhere was the Shelf's
// almanac — which this seed removes. calendar_block/moons_badge_test.go already
// contemplates the no-Shelf state and asserts the honest behaviour:
// "all of them are in the Almanac" is NOT printed when the Shelf is gone, while
// "4 moons declared; the grid draws 3" still is. The ceiling keeps explaining
// itself and only the pointer drops. Nothing is owed, and no Block file is
// opened.
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
// THE SET IS NOW A SEED, NOT A VERDICT (C-CALV4-LAYERS-P9, [LYR-3] SIGNED),
// AND THAT IS PRECISELY WHY R2-1'S CHANGE WAS CHEAP. The keys below are what a
// viewer who has NEVER opened the switchboard sees. Until R2-1 they were
// byte-for-byte what wave 1 and wave 2 rendered, which is what kept every
// signed entity render valid on day one; R2-1 changed them deliberately, and
// the signed renders' five-key embed is now a NAMED DIVERGENCE rather than a
// contract (dispatch §14 item 4). Changing a seed is a producer decision with a
// designed mechanism behind it and it touches NO WIDGET FILE — which is the
// whole of what [LYR-3] bought. The first explicit switchboard write persists
// the viewer's own set, and from then on the store wins here and on the Bench
// alike, because L20 describes a viewer's preference for how they read
// calendars and "eras off" almost certainly means "off, everywhere".
//
// THE SET ITSELF DID NOT GAIN A KEY ([LYR-7] SIGNED). The HOST-P3/BENCH-P4
// bookings that reserved `moongraph` and `horizon` for this slice close as
// SUPERSEDED rather than DONE: their stated purpose was REACHABILITY, and the
// switchboard supplies reachability directly. L29 says the illumination graph's
// default is OFF, so seeding it would contradict the law that put it in W-F;
// and `horizon` is still chipped, so seeding it would ship a `needs backend`
// chip into a default view — the exact inverse of the DEF ruling.
// THE TWO SEEDS NOW DIFFER ON PURPOSE. benchBlockLayers (bench.go) and this
// function were byte-identical five-key lists from wave 1 until R2-1. The next
// reader's instinct will be to re-unify them; do not. The Bench is a COCKPIT
// and depth is its job; the entity page is an EMBED beside somebody's prose and
// glanceability is its job. blockDefaultLayers (DEF = ["moons"]) is untouched.
func entityBlockLayers(prefs blockLayerPrefs) calblock.LayerState {
	return resolveBlockLayers([]string{"moons", "eras", "weeknums"}, prefs)
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
		return entityCalendarBlockView(cc.Campaign.ID, nil, nil, CalendarV2ViewData{}, nil, entityID, source, false, cc.MemberRole >= campaigns.RoleOwner)
	}

	// The viewer's stored layer set + their persistence endpoint, read ONCE.
	// An anonymous viewer or a read failure both land on the host's seed with
	// no switchboard, which is the wave-1/2 surface exactly.
	prefs := blockLayerPrefsFor(ctx, svc, userID, cc.Campaign.ID)

	// THE BLOCK. The spine owns the visibility gate, the one viewer-filtered
	// pass, and both tie counts; this host owns only the layer SEED.
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
			d.Layers = entityBlockLayers(prefs)
			block = &d
		case isNotFound(err):
			// Hidden or missing — indistinguishable on purpose (stage 9).
			return entityCalendarBlockView(cc.Campaign.ID, nil, nil, CalendarV2ViewData{}, nil, entityID, source, false, cc.MemberRole >= campaigns.RoleOwner)
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
	return entityCalendarBlockView(cc.Campaign.ID, cal, block, data, ties, entityID, source, cc.MemberRole >= campaigns.RoleScribe, cc.MemberRole >= campaigns.RoleOwner)
}

// entityEventHref links a linked-event row to the campaign's calendar.
//
// C-CALV4-V2SUNSET R2-4, [VS-13] SIGNED — AND THE DATE CURSOR IS DROPPED, WHICH
// IS A LOSS AND IS STATED RATHER THAN HIDDEN. This link used to carry
// ?year=&month=&day= and land the reader on that event's own day, because
// ShowV2 parses those params. The Bench does not: AppDashboard reads `sort`,
// `y` and `m` and nothing else, and `y`/`m` are a MONTH cursor in the in-world
// calendar's own month list — there is no day, and no calendar selector to say
// WHICH calendar's month list a `y`/`m` pair means. So the row now lands on the
// Bench's own current view.
//
// WHY DROP IT RATHER THAN KEEP THE V2 LINK. The V1 embed's chip
// (calendar.templ:210 dayCellEventHref) makes the opposite trade and keeps its
// V2 target, and the difference is the ruling: a V1 surface already scheduled
// for retirement may keep a V2 link to preserve a date, but a v4 surface
// linking out to the legacy calendar IS the operator's complaint. The cursor
// rides with C-CALV4-BENCH-CALID.
//
// RENAMED from entityEventHref's V2-named sibling discipline: this one never
// said V2, but its target did.
func entityEventHref(campaignID string, evt Event) string {
	return "/campaigns/" + campaignID + "/apps/calendar"
}

// openCalendarHref is the "Open full calendar" target (C-WIDGET-BINDING-QA2
// Part B): the campaign's calendar, which since C-CALV4-V2SUNSET R2-4 is the
// Bench and no longer the V2 shell.
//
// RENAMED FROM openCalendarV2Href ([VS-2] SIGNED). A helper called
// openCalendarV2Href returning a v4 URL is the next reader's trap.
//
// THE calendarID ARGUMENT IS NOW IGNORED, and it is kept rather than removed
// because the caller's binding still knows which calendar it means. [VS-12]
// SIGNED, measured: AppDashboard (app_dashboard.go) reads only `sort`, `y` and
// `m` — `?calId=` is INERT on this handler, so a door carrying it would land on
// the Bench's default selection anyway, silently. Teaching the Bench to read it
// is C-CALV4-BENCH-CALID, booked; dropping the parameter here is the honest
// version of what already happens.
func openCalendarHref(campaignID, calendarID string) string {
	return "/campaigns/" + campaignID + "/apps/calendar"
}

// entityEventRole renders the participation role label (empty → "linked").
// (itoa for the href is the shared helper in subresource_v2.go.)
func entityEventRole(t EntityEventTie) string {
	if t.ParticipationRole == "" {
		return "linked"
	}
	return t.ParticipationRole
}
