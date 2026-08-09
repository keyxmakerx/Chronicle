// theater.go — C-CALV4-THEATER (slice R2-3). The producer half of the Block
// theater: the seed, the copy, and the two names the copy needs so that two
// Blocks for one calendar can sit on one page without driving each other.
//
// WHAT THE THEATER IS, IN ONE SENTENCE. An entity-page reader opens the
// calendar they are already looking at into a full-tier surface, over the page
// they are on, without navigating anywhere and without changing what any other
// surface looks like tomorrow.
//
// THERE ARE TWO THINGS CALLED "FULL TIER" AND THIS FILE EXISTS BECAUSE OF THE
// DIFFERENCE (dispatch §2). The CSS tier — the 60px nameplate, the named
// weekday columns, the Ledger docked BESIDE the month — is a container query
// (`@container cal-block (min-width: 900px)`, calendar-block.css:2168) against
// `.cal-block-host`'s inline size, and widening the box buys all of it for
// free. The ZONE SET — whether a Ledger and a Shelf exist in the render at all
// — is the PRODUCER's, resolved from LayerState. Widening the box buys NONE of
// it. So moving the embed's existing Block into a wide overlay would produce a
// stretched glanceable month, not a full-tier Block, and a surface that called
// that "the full calendar" would be a lie in the affordance's own name.
//
// [TH-2] SIGNED: ONE PROJECTION, TWO RENDERS. The measurement that settled it
// is worth carrying here because the next reader's instinct is to reach for a
// second `spine.Block` call. EntityCalendarBlock passes NEITHER `LedgerHidden`
// NOR `ShelfHidden` to the spine, and block_projection.go builds the Ledger and
// the Shelf stubs UNCONDITIONALLY; the host then overwrites the projection's
// `Layers` post-hoc (entity_calendar_block.go:253). `Layers` is therefore a
// post-hoc DISPLAY GATE and the entity page's existing projection ALREADY
// CONTAINS every mark the theater will print. The theater's Block is a COPY of
// that same value — one struct copy, one extra template render, ZERO extra
// service work. Two service round trips for one page is not a cost to weigh, it
// is a sign somebody built the refused Option B by accident.
//
// [TH-6] SIGNED — THE NO-WIDER LAW. For any viewer the set of events the
// theater's Block prints is EXACTLY the set the embed's Block prints. The two
// renders differ in LAYER SET and in DOM NAMESPACE and in nothing else, and
// that is pinned STRUCTURALLY by a reflect assertion over every exported field
// of BlockData (entity_calendar_block_test.go) rather than by a mark-set
// oracle — an oracle would compare a struct to itself and pass forever,
// including on the day a later slice forks the projection.
package calendar

import (
	"strings"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// theaterHostSuffix is the token appended to the host entity id on the
// theater's COPY, and it is the whole DOM re-namespace ([TH-2], and line 4 of
// the [TH-8] mount contract — the line that bites).
//
// WHY THIS IS CORE AND NOT POLISH. Every DOM id and every radio-group name the
// Block emits is a pure function of (CalendarSlug, Viewer.HostEntity):
// tieGroupName (helpers.go:503), dayPickGroupName (:954), shelfPickGroupName
// (:1358), almPickGroupName (:1587), skyPickGroupName (:2389), layerSheetID
// (:1833) and layerAnchorName (:1857). Two Blocks for the SAME calendar on the
// SAME entity page would therefore emit IDENTICAL ids and IDENTICAL radio-group
// names — the first time in the product the widget's own stated invariant is
// broken (calendar_block/.ai.md: "two Blocks on one page must not share one
// piece of state").
//
// It is not latent. nameplate.templ:98 gates the tie toggle on
// `HostEntity != ""` alone, so BOTH Blocks emit it, four radios land in one
// `name` group with two markup-`checked`, and the theater's `<label for=…>`
// resolves by document order to the EMBED's radio: the theater's tie toggle is
// visibly dead and pressing it silently re-inks the embed behind the backdrop.
// The day, Shelf and Almanac groups are dormant today only because the embed's
// three-key seed emits no day radio (instrument.templ:213, gated on
// ledgerDocked) and no Shelf — and they FIRE the moment a viewer turns `ledger`
// or `shelf` on in the switchboard, which is stored per (user_id, campaign_id)
// and is exactly the interim answer R2-1 told viewers to use.
//
// THE FIX OPENS NO WIDGET FILE. Viewer.HostEntity is a HOST-OWNED field on the
// returned BlockData, so the copy sets it AFTER the projection has run: every
// namespace above diverges while the tie COUNTS, the tie MODE and every mark
// stay byte-identical, because they were computed before the copy.
const theaterHostSuffix = "~theater"

// theaterHostEntity is the copy's host-entity token. domToken (the widget's own
// sanitiser) maps `~` to `-`, so `abc~theater` becomes `abc-theater` — distinct
// from `abc` in every id and every radio-group name the Block emits.
func theaterHostEntity(entityID string) string { return entityID + theaterHostSuffix }

// theaterBlockLayers is the theater's SEED — the Bench's own five keys, not a
// sixth invented set ([TH-2] §4.1, SIGNED).
//
// IT RESTORES EXACTLY WHAT [BR2-8] REMOVED AND NOTHING ELSE. R2-1 cut the
// entity embed's seed from five keys to three, dropping the two that add a
// ZONE — `ledger` (a 300px docked column at full tier) and `shelf` (a tabbed
// foot) — and depth left the surface. This puts those two back, in the theater
// rather than in the embed. `moongraph`, `legend` and `horizon` stay out for
// the reasons entity_calendar_block.go's header already measured.
//
// IT IS A SEED, NOT AN OVERRIDE. It goes through resolveBlockLayers like every
// other host, so a viewer who has switched `shelf` off in the switchboard gets
// a theater with NO Shelf — and that is correct, because the switchboard is the
// only place layers are chosen (L29) and the theater is not a second one. This
// is the one place a reader will expect an override and not get one.
//
// AFTER THIS SLICE THE PLUGIN CARRIES THREE SEEDS WITH THREE DIFFERENT JOBS —
// entityBlockLayers (glanceable, three keys), benchBlockLayers (the cockpit,
// five) and this one (depth on request, the same five). The next reader's
// instinct will be to unify two of them; do not. The Bench and the theater
// agree on their five by argument, not by accident, and re-unifying them with
// the embed is the trade [BR2-8] made the operator meet in a document first.
func theaterBlockLayers(prefs blockLayerPrefs) calblock.LayerState {
	return resolveBlockLayers([]string{"moons", "eras", "weeknums", "ledger", "shelf"}, prefs)
}

// theaterBlockCopy builds the theater's Block from the embed's, changing the
// LAYER SET and the DOM NAMESPACE and nothing else.
//
// [TH-13] SIGNED — THE EMBED KEEPS THE SWITCHBOARD; THE THEATER RENDERS WITHOUT
// ONE. Three reasons, written here so they are not re-argued: the theater
// exists to SHOW the Ledger and the Shelf, so an in-place control whose job is
// to turn them off is the one affordance it does not need; the embed loses
// nothing and remains the one place a viewer says "I want the Ledger
// permanently"; and two live switchboards on one page, both writing the same
// per-(user_id, campaign_id) preference — one inside a modal and one behind it
// — is a state fork of exactly the kind calendar_block/.ai.md forbids.
//
// THE SUPPRESSION IS A REAL ABSENCE IN THE RENDER, NEVER CSS HIDING.
// switchboardLive (helpers.go:1954) is `HasSwitchboard && PersistURL != ""` and
// BOTH are host-owned fields on LayerState, so clearing them on the COPY makes
// the widget emit no `[popover]`, no `⋯` invoker and no layerSheetID at all.
// A display:none switchboard would be a control that still exists, is still
// reachable by keyboard and by screen reader, and still writes the preference —
// that is the failure this refuses, not a smaller version of it.
//
// THIS RULING IS REVERSIBLE and is written down as reversible on purpose. A
// later slice may give the theater the switchboard and suppress the embed's, or
// teach the widget to share one instance across two Blocks. That would be a
// real design change with its own signature, not a fix, and it is not this
// slice's to take.
func theaterBlockCopy(embed calblock.BlockData, prefs blockLayerPrefs) calblock.BlockData {
	d := embed // ONE STRUCT COPY. Never a second projection ([TH-2]).
	d.Layers = theaterBlockLayers(prefs)
	// [TH-13]: real absence, at the producer, on the copy only.
	d.Layers.HasSwitchboard = false
	d.Layers.PersistURL = ""
	// [TH-2]: the DOM re-namespace, after the projection has run.
	d.Viewer.HostEntity = theaterHostEntity(embed.Viewer.HostEntity)
	return d
}

// theaterScaffoldID is the `<dialog>`'s DOM id, and it is the whole of [TH-15].
//
// ONCE PER PAGE MEANS ONCE PER (calendar, entity). EntityCalendarBlock is a
// PER-BLOCK registry entry (internal/app/routes.go:3264), so an entity layout
// that carried two calendar blocks would call it twice; written naively that
// emits two scaffolds carrying one id and the first Expand button on the page
// drives whichever one a querySelector found. The shipped answer to exactly
// this problem is one line above the theater's own mount — the worldstate
// seed's `"cal-v2-worldstate-cal-" + entityID` and its recorded reason (C-CAL-
// CLOSEOUT E7): namespace per block, and the singleton binds the first while
// the rest stay static SSR. This takes the same shape, with the calendar slug
// in the key as well because two DIFFERENT calendars on one entity are the
// case the worldstate seed never had.
//
// The module's half of the contract is in calendar_theater.js: it binds EVERY
// `[data-cal-theater]` it finds and never `querySelector`s the first.
func theaterScaffoldID(calendarSlug, entityID string) string {
	return "cal-theater-" + theaterDOMToken(calendarSlug) + "-" + theaterDOMToken(theaterHostEntity(entityID))
}

// theaterHeadingID is the `aria-labelledby` target — the theater's accessible
// name comes from its own heading ([TH-4], modelled on the day card's
// `aria-labelledby="cal-daycard-head"`).
func theaterHeadingID(calendarSlug, entityID string) string {
	return theaterScaffoldID(calendarSlug, entityID) + "-head"
}

// theaterDOMToken mirrors the widget package's unexported domToken so the
// scaffold's id is built from the same alphabet the Block's own ids are. It is
// reimplemented rather than exported from the widget because
// internal/widgets/calendar_block/** is not opened by this slice at all — not a
// template, not a helper, not a test (dispatch §12, and the Bounds line that
// calls breaking it a STOP-AND-FLAG).
func theaterDOMToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}
