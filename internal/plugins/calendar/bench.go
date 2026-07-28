// bench.go — THE BENCH: the nav Calendar tab's landing surface (calendar-v4
// wave 1, phase B / C-CALV4-BENCH-P4).
//
// SAME ROUTE, NEW SURFACE. GET /campaigns/:id/apps/calendar keeps its path
// exactly — the nav target is a bare string in internal/templates/layouts/
// app.templ (addonURLMap, ~:706) and the Extensions hub carries a SECOND bare
// string (campaigns/extensions_hub.go:62-64) pinned literally by
// extensions_hub_test.go:127 — and any path change would force a
// routes_snapshot.txt regeneration, which collides with every other slice in
// the wave. This file rebuilds what that route RENDERS, not where it lives.
//
// THE PROPORTION RULE is written twice in the signed contract, verbatim (the
// `@layer bench` header comment and the page caption):
//
//	one primary Block, one real-world Block, two subordinate rows, one slot.
//	There is no width at which this becomes four identical panels.
//
// It is the rule that stops the Bench decaying back into the card grid it
// replaces, so it is structural here rather than cosmetic: benchClassify picks
// exactly one primary and at most one real-world calendar and every other
// calendar becomes a ROW. No width, no sort key and no calendar count turns a
// row into a Block.
//
// WHAT IS COMPUTED AND WHAT IS DESIGN-AHEAD. Every count on this page is
// derived post-filter, per viewer. Where the signed contract asks for a fact
// the backend cannot answer — session RSVP, a queryable fog horizon,
// per-calendar sync linkage — the surface carries the signed
// `needs backend` chip instead of a fabricated number. That is the honesty
// state the operator signed; see benchRibbon and BenchRsvp.
package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// benchNextUpLimit bounds the NEXT UP index. The signed contract prints five
// rows and a "all N →" tail; the tail's N is the number of rows the VIEWER's
// index actually holds, never the mockup's hardcoded "all 11" (that literal is
// the same leak class as the un-filtered UpcomingByCalendar path — dispatch §5).
const benchNextUpLimit = 5

// benchNextUpScan is how many rows are resolved before the display slice is
// taken, so the "all N →" tail can state a real total without a second query.
// Well past any plausible bench render; the underlying read is one batched
// query either way.
const benchNextUpScan = 200

// benchMoonCap mirrors the renderer's own ceiling (calendar_block's moonCap):
// the grid draws at most three discs per day so a month can never grow with the
// fiction. The declared total still reaches the nameplate as "3 of 4 moons".
const benchMoonCap = 3

// --- the view model ---------------------------------------------------------

// BenchData is the Bench's complete render input. Assembled by buildBench and
// rendered by bench.templ; nothing in bench.templ queries.
type BenchData struct {
	CampaignID   string
	CampaignName string

	// IsGM is the dm-sight role (owner or co-DM). It gates the three GM ribbon
	// tiles and the warnrow. PERMISSION IS ABSENCE: those surfaces are not in a
	// player's DOM at all — not greyed, not disabled, not rendered-then-hidden.
	IsGM bool
	// IsOwner gates the MANAGEMENT affordances (settings, permissions, the
	// New-calendar slot), which is a narrower gate than IsGM.
	IsOwner   bool
	CSRFToken string
	// Sort is the subordinate-row ordering key (the pre-existing ?sort control,
	// which is the only HTMX on this page and keeps working).
	Sort string
	// LoadError degrades the whole surface to a friendly "couldn't load" card.
	LoadError bool

	// CalendarCount is how many calendars THIS VIEWER may see; NeedsSetup is how
	// many of those carry a structural fault.
	CalendarCount int
	NeedsSetup    int

	Ribbon []BenchTile

	// Primary is the campaign's principal in-world calendar, at full Block size.
	// RealWorld is the real-world calendar beneath it, rendered noShelf.
	// Either may be nil — a campaign may have no real-world calendar, and a
	// degraded spine produces no Block at all (see buildBench's degrade ladder).
	Primary   *BenchBlock
	RealWorld *BenchBlock

	Rsvp   BenchRsvp
	NextUp BenchNextUp

	// Rows are the subordinate calendars. SUBORDINATE IS PRESENTATION, NOT A
	// RELATION: the calendars table is flat (is_default + sort_order) and a
	// repo-wide grep for subordinate / parent_calendar / ParentCalendar returns
	// nothing. No migration, no parent/child, this wave.
	Rows []BenchRow
	// ShowNewSlot renders the New-calendar slot. Owner-only — see benchRowGrid.
	ShowNewSlot bool
}

// BenchTile is one ribbon tile. The ribbon is six tiles for a GM and THREE for
// a player: Today · Next up · Session, then the GM-only Sync · Needs attention
// · Horizon.
type BenchTile struct {
	// Key is the stable marker the tests and W-B key on ("today", "nextup",
	// "session", "sync", "attention", "horizon").
	Key      string
	Glyph    string
	Eyebrow  string
	Headline string
	// Tone tints the headline: "" | "ok" | "warn".
	Tone string
	// Qual is the line under the headline; Detail is the foot line.
	Qual   string
	Detail string
	// Class is the tile's own modifier: "accenttint" (Today) | "attn".
	Class string
	// NeedsBackend renders the signed chip. A tile that cannot answer says so;
	// it never prints a fabricated zero.
	NeedsBackend bool
	// Rows are the .arow entries of the Needs-attention tile.
	Rows []BenchTileRow
	// Actions are the tile's buttons. Wave 1's are INERT (see BenchAction).
	Actions []BenchAction
	// Ticks is the Today tile's 30-tick month rule; empty on every other tile.
	Ticks []BenchTick
	// Href makes the whole tile a door. Empty = not a link.
	Href string
}

// BenchTileRow is one line inside the Needs-attention tile.
type BenchTileRow struct {
	Label string
	// Bad raises the row from warn to bad ink.
	Bad  bool
	Href string
}

// BenchAction is a tile button.
//
// INERT BY DEFAULT, and deliberately so. The signed RSVP controls (Yes / No /
// Maybe, Nudge) have no store behind them — session scheduling does not exist
// on main. A live-looking control that swallows a click is worse than a
// disabled one, so wave 1 follows the Block's own layersInvoker precedent:
// present, so the tile's geometry is final, and disabled with a title that says
// why. Href promotes an action to a real link when a real target exists.
type BenchAction struct {
	Label string
	Href  string
	Fill  bool
	Title string
}

// BenchTick is one day of the Today tile's tick rule. Day carries the ANSWER
// key (data-day) even though nothing answers yet — a surface that forgets the
// key simply stops answering when W-B lands, and that is invisible in review
// (COMMON §6.6, guard B4).
type BenchTick struct {
	Day    int
	DayKey string
	// Event marks a day that carries at least one viewer-visible event.
	Event bool
	Today bool
	Axis  string
}

// BenchBlock is one Block on the bench, plus the owner-only management strip
// that rides beneath it.
//
// THE MANAGEMENT STRIP IS A DOCUMENTED ADDITION to the signed DOM. The signed
// Bench has no per-calendar management affordance for its two Blocks — the
// mockup has no permissions feature at all — and the calendars promoted to
// Blocks are precisely the campaign's two most important ones. Dropping the
// shipped W5b per-calendar permissions editor and the Settings link for them
// would be a functional regression, so the strip sits INSIDE the .stack item,
// after the Block, leaving every named contract element in its exact order.
type BenchBlock struct {
	Data   calblock.BlockData
	Manage BenchManage
}

// BenchManage is the owner-only per-calendar management cluster, shared by the
// Block strip and the subordinate rows so the two can never drift.
type BenchManage struct {
	CalendarID string
	Name       string
	// Visibility is the W5b badge kind: "everyone" | "dm_only" | "custom".
	Visibility string
	// VisMode / VisRules seed the shared permissions modal, exactly as the
	// pre-existing card did (calendar_permissions.js reads both attributes).
	VisMode  string
	VisRules string
	// IsActive / IsDefault drive the two badges.
	IsActive  bool
	IsDefault bool

	OpenHref     string
	SettingsHref string
}

// BenchRow is one subordinate calendar row (.calrow).
//
// THE FAULT PRINTS WHERE THE DATE WOULD GO. A misconfigured calendar swaps its
// NAME for the fault and carries NO date element at all — not a zero, not a
// placeholder, not an em dash. That is the signed .calrow.warnrow, and it is
// the same honesty rule the Block's own date line follows (blockDateLine).
type BenchRow struct {
	Manage BenchManage

	// CalHue / Pattern are the identity channel: a token name and one of the
	// eight locked stroke patterns, so the row still resolves in greyscale.
	CalHue  string
	Pattern string
	Letter  string

	// Name is empty exactly when Fault is set — the fault takes the name's slot.
	Name string
	// DateLabel is empty exactly when Fault is set.
	DateLabel string
	Fault     string
	// Detail is the quiet line under the name: the row's own qualifier
	// ("no events yet") or, on a warnrow, why the date cannot resolve.
	Detail string

	// Warn marks the row as a .calrow.warnrow. GM-ONLY: a misconfiguration is a
	// GM's problem and naming it to a player leaks the calendar's broken state.
	Warn bool
}

// BenchNextUp is the cross-calendar NEXT UP index.
type BenchNextUp struct {
	Rows []BenchNextUpRow
	// Total is how many rows the VIEWER's index holds in full (the "all N →"
	// tail). Computed post-filter, per viewer — never the mockup's hardcoded 11.
	Total int
	// Calendars is how many calendars contributed, for the section subtitle.
	Calendars int
}

// BenchNextUpRow is one printed line of the index.
type BenchNextUpRow struct {
	// DayKey is the ANSWER key (data-day), emitted in wave 1 and consumed in
	// W-B.
	DayKey string
	// DateLabel is the occurrence date in that calendar's own reckoning.
	DateLabel string
	// CalendarLabel is the short calendar name the index prints in the caps
	// column ("HARPTOS"); CalHue is its identity token.
	CalendarLabel string
	CalHue        string

	Name string
	// GMOnly renders the gold GM badge. Nil audience for a player, always —
	// permission is absence, and a player never receives the row at all.
	GMOnly bool
	// Axis / Pattern / Glyph are the event's locked identity triple.
	Axis    string
	Pattern string
	Glyph   string

	Href string
}

// BenchRsvp is the session/RSVP panel.
//
// DESIGN-AHEAD IN FULL. The signed panel draws per-member availability lanes, a
// density row, a recommended window and a member table with per-member time
// zones and answers. NONE of that has a store on main: there is no session
// entity, no RSVP table and no per-member time zone. Wave 1 therefore renders
// the panel's HEADER with the signed `needs backend` chip and a plain statement
// of what will live there — and draws no lanes, no densities and no member
// rows, because every one of them would be a fabricated fact on the one surface
// whose entire job is honesty (dispatch §4).
type BenchRsvp struct {
	Title string
	Note  string
}

// --- assembly ---------------------------------------------------------------

// benchInput is everything buildBench needs from the request, resolved by the
// handler. Keeping it a struct rather than six parameters means the handler
// stays thin and the builder stays testable.
type benchInput struct {
	Campaign *campaigns.Campaign
	UserID   string
	// Role is the VISIBILITY role (cc.VisibilityRole()), which is what every
	// filter in this plugin takes.
	Role      int
	IsOwner   bool
	CSRFToken string
	Sort      string
}

// buildBench assembles the Bench.
//
// THE OWNER/PLAYER LIST SPLIT IS PRESERVED (dispatch §8): owners get
// ListCalendars, players get ListVisibleCalendars. Unifying the two list calls
// reopens the W5a leak, so they stay separate even though the Bench renders one
// surface.
//
// THE DEGRADE LADDER:
//
//	list fails            → LoadError, the friendly card, nothing else
//	spine absent/errors   → no Block; that calendar falls back to a ROW, so no
//	                        calendar ever vanishes from the page
//	upcoming read fails   → the index is empty and the ribbon says so
//
// NO N+1. The subordinate rows need an in-world date label, which needs Months,
// and ListByCampaignID returns SHALLOW rows (repository.go:255-274) whose only
// eager member is data.Selected. The single-calendar eagerLoad (service.go:690-
// 724 — it moved from the dispatch's :668-701) is NINE sequential queries per
// calendar, and this is the page the nav Calendar tab lands on for every
// player. So the hydration goes through the spine's batched EagerLoadCalendars.
func (h *Handler) buildBench(ctx context.Context, in benchInput) BenchData {
	data := BenchData{
		CampaignID:   in.Campaign.ID,
		CampaignName: in.Campaign.Name,
		IsGM:         permissions.CanSeeDmOnly(in.Role),
		IsOwner:      in.IsOwner,
		CSRFToken:    in.CSRFToken,
		Sort:         normalizeCalendarSort(in.Sort),
		ShowNewSlot:  in.IsOwner,
		Rsvp:         benchRsvpPanel(),
	}

	var (
		cals []Calendar
		err  error
	)
	if in.IsOwner {
		cals, err = h.svc.ListCalendars(ctx, in.Campaign.ID)
	} else {
		cals, err = h.svc.ListVisibleCalendars(ctx, in.Campaign.ID, in.Role, in.UserID)
	}
	if err != nil {
		slog.Warn("bench: calendar list failed",
			slog.String("campaign_id", in.Campaign.ID), slog.Any("error", err))
		data.LoadError = true
		return data
	}
	data.CalendarCount = len(cals)
	if len(cals) == 0 {
		return data
	}

	viewer := BlockViewer{UserID: in.UserID, Role: in.Role}
	spine := BlockSpine()

	// Hydrate every listed calendar in a fixed number of queries, then apply the
	// viewer's active-calendar marker.
	hydrated := benchHydrate(ctx, spine, cals)
	activeID := ""
	if active, aerr := h.svc.GetActiveCalendar(ctx, in.UserID, in.Campaign.ID); aerr == nil && active != nil {
		activeID = active.ID
	}

	// The cross-calendar index, viewer-filtered at the source.
	upcoming := benchUpcoming(ctx, spine, in.Campaign.ID, viewer)
	data.NextUp = benchNextUp(upcoming, data.IsGM, in.Campaign.ID)

	// The subordinate-row ordering reuses the shipped ?sort control. The
	// nextevent key is fed from the SAME viewer-filtered index the NEXT UP panel
	// prints, never from UpcomingByCalendar (dispatch §5).
	sortDashboardCalendars(hydrated, data.Sort, benchSortKeys(upcoming))

	primary, realWorld, rows := benchClassify(hydrated, activeID)
	data.NeedsSetup = benchNeedsSetup(hydrated)

	if primary != nil {
		if b := h.benchBlock(ctx, spine, primary, viewer, activeID, false); b != nil {
			data.Primary = b
		} else {
			rows = append([]*Calendar{primary}, rows...)
		}
	}
	if realWorld != nil {
		// noShelf — the signed real-world Block on the Bench renders with its
		// Shelf docked but hidden, which is the ShelfHidden flag's whole purpose.
		if b := h.benchBlock(ctx, spine, realWorld, viewer, activeID, true); b != nil {
			data.RealWorld = b
		} else {
			rows = append(rows, realWorld)
		}
	}
	data.Rows = benchRows(rows, activeID, data.CampaignID, data.IsGM)
	data.Ribbon = benchRibbon(benchRibbonInput{
		IsGM:       data.IsGM,
		CampaignID: data.CampaignID,
		Primary:    primary,
		Block:      data.Primary,
		NextUp:     data.NextUp,
		Sync:       benchSyncPill(ctx, spine, in.Campaign.ID, data.Primary),
		Attention:  benchAttentionRows(hydrated, data.CampaignID),
	})
	return data
}

// benchHydrate returns the listed calendars WITH their sub-resources, through
// the spine's batched loader. A degraded spine (or a batch failure) returns the
// shallow rows unchanged rather than blanking the page: a row without Months
// prints its numeric date label, which is what the pre-Bench dashboard printed.
func benchHydrate(ctx context.Context, spine *BlockService, cals []Calendar) []Calendar {
	if spine == nil || len(cals) == 0 {
		return cals
	}
	ids := make([]string, 0, len(cals))
	for i := range cals {
		ids = append(ids, cals[i].ID)
	}
	full, err := spine.EagerLoadCalendars(ctx, ids)
	if err != nil {
		slog.Warn("bench: eager load failed", slog.Any("error", err))
		return cals
	}
	out := make([]Calendar, 0, len(cals))
	for i := range cals {
		if c := full[cals[i].ID]; c != nil {
			out = append(out, *c)
			continue
		}
		out = append(out, cals[i])
	}
	return out
}

// benchUpcoming reads the cross-calendar index. UpcomingAcrossCalendars is the
// ONLY path used here: the pre-existing UpcomingByCalendar /
// EventDatesForCalendars path filters base visibility in SQL only and never
// applies filterEventsByUser, so an event carrying visibility_rules leaks its
// NAME to a player there (dispatch §5, block_service.go:568-570).
func benchUpcoming(ctx context.Context, spine *BlockService, campaignID string, viewer BlockViewer) []BlockUpcoming {
	if spine == nil {
		return nil
	}
	rows, err := spine.UpcomingAcrossCalendars(ctx, campaignID, viewer, benchNextUpScan)
	if err != nil {
		slog.Warn("bench: upcoming index failed",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return rows
}

// benchSyncPill resolves the campaign's sync pill for the ribbon tile. The
// primary Block already carries its own pill; this reuses it when it is there
// and otherwise asks the spine, so the tile and the Block can never disagree.
func benchSyncPill(ctx context.Context, spine *BlockService, campaignID string, primary *BenchBlock) calblock.SyncPill {
	if primary != nil {
		return primary.Data.Sync
	}
	if spine == nil {
		return calblock.SyncPill{State: blockSyncStateNone}
	}
	pill, err := spine.CalendarLinkStatus(ctx, campaignID)
	if err != nil {
		slog.Warn("bench: sync pill failed",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	return pill
}

// benchBlock projects one calendar into a Block, or nil when it cannot be.
//
// The spine owns the visibility gate and the one viewer-filtered pass; this
// host owns only the layer set. A not-found answer is INDISTINGUISHABLE from a
// hidden calendar by construction (C-CALV4-SEAM-P5 stage 9) and this host must
// not undo that, so it does not branch on which — any failure simply demotes
// the calendar to a subordinate row.
func (h *Handler) benchBlock(ctx context.Context, spine *BlockService, cal *Calendar, viewer BlockViewer, activeID string, noShelf bool) *BenchBlock {
	if spine == nil || cal == nil {
		return nil
	}
	d, err := spine.Block(ctx, BlockRequest{
		CalendarID:  cal.ID,
		CampaignID:  cal.CampaignID,
		Viewer:      viewer,
		IsActive:    cal.ID == activeID,
		ShelfHidden: noShelf,
		MoonCap:     benchMoonCap,
	})
	if err != nil {
		return nil
	}
	d.Layers = benchBlockLayers()
	return &BenchBlock{Data: d, Manage: benchManage(cal, activeID, cal.CampaignID)}
}

// benchBlockLayers is THE HOST'S LAYER SET for the Bench.
//
// THE RULING IT SITS UNDER (cordinator decisions/2026-07-28-calv4-def-and-zone-
// chips-ruling.md §1): the producer's DEF stays ["moons"] — "the default surface
// is a month with its moon phases and nothing else" — and a host that wants more
// PASSES A LAYER SET rather than growing DEF. This is that host saying so.
//
// The signed bench renders (mockups/renders/v4-bench-*.png) show, on the
// primary Block: the per-week-row era bands, the W1/W2/W3 week gutter, the moon
// silhouettes and their "3 OF 4 MOONS" nameplate badge, the docked LEDGER and
// the Month/Upcoming/Filters/Almanac Shelf. Those five keys are this set.
//
// `ledger` also has a GEOMETRIC consequence and cannot simply be dropped: the
// full-tier column arithmetic subtracts the Ledger's 300px unconditionally
// (calendar_block/sizing.go), so a Block that skipped the zone would measure its
// own columns wrong and flip density at the wrong host width.
//
// TWO KEYS THE SIGNED BENCH SHOWS AND THIS SET DELIBERATELY OMITS — `moongraph`
// (the illumination strip) and `horizon` (the knowledge ribbon). Both are
// MEASURED omissions, not oversights, and both were already booked by
// C-CALV4-HOST-P3 for the same reason: wave 1 renders each of those zones as a
// `needs backend` chip and nothing else (there is no queryable fog horizon on
// main — COMMON §6.1), and at the std tier the extra need-zones stack against
// the docked Ledger and the Shelf. Two chip rows buy no information at all and
// cost the 390px reading, which this slice's own screenshots gate. W-F fills
// both zones and owns their placement; the host adds the keys then.
//
// HasSwitchboard stays false: layer preferences are per-viewer and PERSISTED
// (L20/L26/L29) and that store is W-F's.
func benchBlockLayers() calblock.LayerState {
	return calblock.LayerState{
		Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"},
	}
}

// benchSortKeys adapts the viewer-filtered cross-calendar index into the shape
// the shipped ?sort=nextevent comparator takes.
//
// It exists so the sort control keeps working WITHOUT the leaky
// UpcomingByCalendar read: the rows are already filterEventsByUser'd, so no
// event name and no event date reaches an ordering decision the viewer may not
// see. Only the FIRST row per calendar is kept — the comparator reads Next and
// nothing else.
func benchSortKeys(rows []BlockUpcoming) map[string]CalendarUpcoming {
	if len(rows) == 0 {
		return nil
	}
	out := make(map[string]CalendarUpcoming, len(rows))
	for i := range rows {
		r := &rows[i]
		if r.Calendar == nil {
			continue
		}
		if _, seen := out[r.Calendar.ID]; seen {
			continue
		}
		out[r.Calendar.ID] = CalendarUpcoming{Next: &CalendarEventDate{
			CalendarID: r.Calendar.ID,
			Year:       r.Date.Year,
			Month:      r.Date.Month,
			Day:        r.Date.Day,
			Name:       r.Event.Name,
		}}
	}
	return out
}

// --- classification ---------------------------------------------------------

// benchClassify applies the proportion rule to a flat calendar list.
//
// PRIMARY is the calendar a reader means when they say "the campaign calendar":
// the campaign default, else the viewer's active one, else the first in-world
// calendar, else the first calendar at all. REAL-WORLD is the first real-life
// calendar that is not already the primary. EVERYTHING ELSE IS A ROW.
//
// The rule is deliberately not "the first two calendars": a campaign whose
// first two rows happen to both be fantasy calendars must still not render two
// identical panels, and a campaign with one calendar renders one Block and no
// second panel rather than promoting an arbitrary second.
func benchClassify(cals []Calendar, activeID string) (primary, realWorld *Calendar, rows []*Calendar) {
	if len(cals) == 0 {
		return nil, nil, nil
	}
	pick := func(match func(*Calendar) bool) *Calendar {
		for i := range cals {
			if match(&cals[i]) {
				return &cals[i]
			}
		}
		return nil
	}
	inWorld := func(c *Calendar) bool { return !c.IsRealLife() }

	primary = pick(func(c *Calendar) bool { return c.IsDefault && inWorld(c) })
	if primary == nil && activeID != "" {
		primary = pick(func(c *Calendar) bool { return c.ID == activeID && inWorld(c) })
	}
	if primary == nil {
		primary = pick(inWorld)
	}
	if primary == nil {
		// A campaign with nothing but real-world calendars: the default (or the
		// first) is still the primary, and no second panel is promoted.
		primary = pick(func(c *Calendar) bool { return c.IsDefault })
	}
	if primary == nil {
		primary = &cals[0]
	}
	realWorld = pick(func(c *Calendar) bool { return c.IsRealLife() && c.ID != primary.ID })

	for i := range cals {
		c := &cals[i]
		if c.ID == primary.ID || (realWorld != nil && c.ID == realWorld.ID) {
			continue
		}
		rows = append(rows, c)
	}
	return primary, realWorld, rows
}

// benchNeedsSetup counts the calendars that cannot resolve a date — the
// "1 needs setup" half of the section subtitle. It reuses blockDateLine so the
// Bench's count, the row's fault text and the Block's own date line can never
// disagree about what "broken" means.
func benchNeedsSetup(cals []Calendar) int {
	n := 0
	for i := range cals {
		if _, fault := blockDateLine(&cals[i]); fault != "" {
			n++
		}
	}
	return n
}

// benchRows shapes the subordinate calendars.
//
// THE WARN TREATMENT IS GM-ONLY, THE ROW IS NOT. The signed Bench's Dwarven
// warnrow is absent from the player render because that calendar is not shared
// with players at all — a VISIBILITY fact, already settled upstream by
// ListVisibleCalendars — not because the fault is secret. Conflating the two
// would make a player's own misconfigured calendar vanish from their page
// silently, which is the worst of the three possible answers.
//
// So every calendar the viewer may see gets a row. A GM additionally gets the
// diagnosis (the fault where the name goes, the "setup" badge, the settings
// door); a player gets the calendar's name and a calm statement that its date
// is not available — and, either way, NO date element at all: not a zero, not a
// placeholder, not an em dash.
func benchRows(cals []*Calendar, activeID, campaignID string, isGM bool) []BenchRow {
	out := make([]BenchRow, 0, len(cals))
	for _, c := range cals {
		if c == nil {
			continue
		}
		label, fault := blockDateLine(c)
		row := BenchRow{
			Manage:  benchManage(c, activeID, campaignID),
			CalHue:  blockCalHue(c),
			Pattern: blockCalPattern(c),
			Letter:  blockCalLetter(c),
			Name:    blockCalendarName(c),
			Detail:  benchRowDetail(c),
		}
		switch {
		case fault == "":
			row.DateLabel = label
		case isGM:
			row.Warn = true
			row.Name = ""
			row.Fault = benchFaultHeadline(fault)
			row.Detail = benchFaultDetail(fault)
		default:
			row.Detail = "date not available yet"
		}
		out = append(out, row)
	}
	return out
}

// benchFaultHeadline is the short form that takes the NAME's slot, and
// benchFaultDetail the explanation that takes the qualifier's. blockDateLine
// returns them as one "<what> — <why>" string; the row prints them in two
// places, exactly as the signed warnrow does ("Needs eras" / "0 eras defined —
// dates cannot resolve").
func benchFaultHeadline(fault string) string {
	if head, _, ok := strings.Cut(fault, " — "); ok {
		return head
	}
	return fault
}

func benchFaultDetail(fault string) string {
	if _, tail, ok := strings.Cut(fault, " — "); ok {
		return tail
	}
	return ""
}

// benchRowDetail is the quiet qualifier under a row's name.
func benchRowDetail(c *Calendar) string {
	switch {
	case c == nil:
		return ""
	case c.IsRealLife():
		return "real-world calendar · tracks wall-clock time"
	case len(c.Months) == 0:
		return "no months defined"
	default:
		return fmt.Sprintf("%d months · %d-day week", len(c.Months), blockWeekLen(c))
	}
}

// benchManage builds the owner-only management cluster for one calendar. The
// visibility mode + rules are the SAME pair the pre-Bench card handed the
// shared permissions modal, so calendar_permissions.js keeps working unchanged.
func benchManage(c *Calendar, activeID, campaignID string) BenchManage {
	if c == nil {
		return BenchManage{}
	}
	return BenchManage{
		CalendarID:   c.ID,
		Name:         blockCalendarName(c),
		Visibility:   calVisibilityKind(*c),
		VisMode:      calVisModeForCard(*c),
		VisRules:     calVisRulesAttr(*c),
		IsActive:     c.ID == activeID,
		IsDefault:    c.IsDefault,
		OpenHref:     fmt.Sprintf("/campaigns/%s/calendar/v2/%s", campaignID, c.ID),
		SettingsHref: fmt.Sprintf("/campaigns/%s/calendars/%s/settings", campaignID, c.ID),
	}
}

// --- the ribbon -------------------------------------------------------------

// benchRibbonInput is what the ribbon is built from. Everything here is already
// viewer-filtered; the ribbon computes no facts of its own.
type benchRibbonInput struct {
	IsGM       bool
	CampaignID string
	// Primary is the calendar the Today tile reads; Block is its projection,
	// which supplies the tick rule without a second query.
	Primary *Calendar
	Block   *BenchBlock
	NextUp  BenchNextUp
	Sync    calblock.SyncPill
	// Attention is the resolved Needs-attention row set (empty = all clear).
	Attention []BenchTileRow
}

// benchRibbon builds the ribbon.
//
// PLAYERS GET THREE TILES, NOT SIX. Today · Next up · Session, and then the
// GM-only Sync · Needs attention · Horizon. The three GM tiles are ABSENT FROM
// THE DOM for a player — not greyed, not disabled, not rendered-then-hidden.
// Permission is absence, and bench_test.go asserts the three markers do not
// appear in a player render.
func benchRibbon(in benchRibbonInput) []BenchTile {
	tiles := []BenchTile{
		benchTodayTile(in),
		benchNextUpTile(in),
		benchSessionTile(in.IsGM),
	}
	if !in.IsGM {
		return tiles
	}
	return append(tiles,
		benchSyncTile(in.Sync),
		benchAttentionTile(in.Attention),
		benchHorizonTile(),
	)
}

// benchTodayTile states the campaign's in-world date and carries the tick rule.
// It is the one tile that is entirely computed.
func benchTodayTile(in benchRibbonInput) BenchTile {
	t := BenchTile{Key: "today", Glyph: "◆", Eyebrow: "Today", Class: "accenttint"}
	if in.Primary == nil {
		t.Headline = "No calendar"
		t.Qual = "this campaign has no calendar yet"
		return t
	}
	label, fault := blockDateLine(in.Primary)
	if fault != "" {
		// The fault takes the date's slot here for the same reason it does in
		// the Block and in the warnrow: a calendar that cannot resolve a date
		// says so where the reader is already looking.
		t.Headline = benchFaultHeadline(fault)
		t.Tone = "warn"
		t.Qual = benchFaultDetail(fault)
		return t
	}
	t.Headline = label
	t.Qual = benchTodayQual(in.Primary)
	season, era := blockSeasonEraLabels(in.Primary)
	t.Detail = strings.TrimSpace(strings.Join(benchNonEmpty(era, season), " · "))
	t.Href = fmt.Sprintf("/campaigns/%s/calendar/v2/%s", in.CampaignID, in.Primary.ID)
	if in.Block != nil {
		t.Ticks = benchTicks(&in.Block.Data)
	}
	return t
}

// benchTodayQual is the "Harptos of Imix · day 14 of 30" line.
func benchTodayQual(cal *Calendar) string {
	name := blockCalendarName(cal)
	if cal.CurrentMonth < 1 || cal.CurrentMonth > len(cal.Months) {
		return name
	}
	return fmt.Sprintf("%s · day %d of %d", name, cal.CurrentDay,
		cal.MonthDays(cal.CurrentMonth-1, cal.CurrentYear))
}

// benchNextUpTile prints the soonest row of the viewer's own index — never a
// campaign-wide "next event" a player may not see.
func benchNextUpTile(in benchRibbonInput) BenchTile {
	t := BenchTile{Key: "nextup", Glyph: "✦", Eyebrow: "Next up"}
	if len(in.NextUp.Rows) == 0 {
		t.Headline = "Nothing scheduled"
		t.Qual = "no upcoming events on the calendars you can see"
		return t
	}
	r := in.NextUp.Rows[0]
	t.Headline = r.Name
	t.Qual = strings.Join(benchNonEmpty(r.DateLabel, r.CalendarLabel), " · ")
	t.Detail = fmt.Sprintf("%d upcoming across %d calendars", in.NextUp.Total, in.NextUp.Calendars)
	t.Href = r.Href
	return t
}

// benchSessionTile is DESIGN-AHEAD and says so.
//
// There is no session entity and no RSVP store on main, so this tile prints the
// signed `needs backend` chip rather than the mockup's "RSVP 3 / 5" — which is
// a fabricated number, and §4 of the dispatch is explicit that design-ahead
// surfaces carry the chip and never a fabricated zero. The signed controls
// still render, INERT: present so the tile's geometry is final, disabled so
// they are not dead controls that swallow a click (the same reading the Block's
// own layersInvoker ships under).
func benchSessionTile(isGM bool) BenchTile {
	t := BenchTile{
		Key: "session", Glyph: "◷", Eyebrow: "Session",
		Headline: "Not scheduled here yet", NeedsBackend: true,
		Qual:   "session dates and RSVPs have no store on Chronicle yet",
		Detail: "this tile fills in when session scheduling lands",
	}
	const why = "session scheduling and RSVP storage do not exist yet"
	if isGM {
		t.Actions = []BenchAction{{Label: "Nudge", Title: why}}
		return t
	}
	t.Actions = []BenchAction{
		{Label: "Yes", Fill: true, Title: why},
		{Label: "No", Title: why},
		{Label: "Maybe", Title: why},
	}
	return t
}

// benchSyncTile prints the resolved sync pill.
//
// THE DENOMINATOR NEVER DROPS. The numerator is DEFINED, not queried (COMMON
// §6.3): sync_mappings has no calendar type and every syncapi calendar endpoint
// resolves the campaign default, so Linked is 1 when a module is connected and
// 0 otherwise, and Total is the number of calendars in the campaign. The chip
// stays on the tile because that per-calendar linkage is exactly what does not
// exist — the state is real, the per-calendar attribution is not.
func benchSyncTile(p calblock.SyncPill) BenchTile {
	t := BenchTile{
		Key: "sync", Glyph: "●", Eyebrow: "Sync", NeedsBackend: true,
		Detail: fmt.Sprintf("%d of %d calendars linked — the denominator is the point", p.Linked, p.Total),
	}
	switch p.State {
	case blockSyncStateOK:
		t.Headline, t.Tone = "In sync", "ok"
	case blockSyncStateDrift:
		t.Headline, t.Tone = "Drifted", "warn"
	case blockSyncStateBad:
		t.Headline, t.Tone = "Incompatible structure", "warn"
	case blockSyncStatePause:
		t.Headline = "Paused"
	default:
		t.Headline = "Not linked"
	}
	t.Qual = p.Full
	return t
}

// benchAttentionTile is the instrument that DOES NOT VANISH WHEN HEALTHY — an
// empty attention list renders the all-clear state rather than dropping the
// tile, because a missing tile reads as "not checked", not as "nothing wrong".
func benchAttentionTile(rows []BenchTileRow) BenchTile {
	if len(rows) == 0 {
		return BenchTile{
			Key: "attention", Glyph: "✓", Eyebrow: "Needs attention",
			Headline: "all clear", Tone: "ok",
			Qual:   "every calendar resolves its dates",
			Detail: "the instrument does not vanish when healthy",
		}
	}
	return BenchTile{
		Key: "attention", Glyph: "▲", Eyebrow: "Needs attention", Class: "attn",
		Headline: benchItemCount(len(rows)), Tone: "warn",
		Rows: rows,
	}
}

// benchItemCount renders "1 item" / "2 items".
func benchItemCount(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// benchAttentionRows lists what actually needs a GM's hand. Wave 1 can see
// exactly one class of problem — a calendar whose date will not resolve — so
// that is what it reports. The signed tile's second row ("2 RSVPs unanswered")
// is not synthesised: no RSVP store exists, and inventing the row would put a
// fabricated fact on the tile whose whole job is to be trusted.
func benchAttentionRows(cals []Calendar, campaignID string) []BenchTileRow {
	var out []BenchTileRow
	for i := range cals {
		c := &cals[i]
		if _, fault := blockDateLine(c); fault != "" {
			out = append(out, BenchTileRow{
				Label: blockCalendarName(c) + " — " + benchFaultDetail(fault),
				Bad:   true,
				Href:  fmt.Sprintf("/campaigns/%s/calendars/%s/settings", campaignID, c.ID),
			})
		}
	}
	return out
}

// benchHorizonTile is design-ahead in full.
//
// WAVE-1 RULING (COMMON §6.1): there is no queryable fog horizon on main —
// m.horizon is a literal on the mockup's month object, DayCell.Fogged stays
// false everywhere, and the Horizon surfaces render the signed `needs backend`
// chip. The mockup's "9 days fogged" is therefore a number this tile must not
// print. W-F builds the horizon.
func benchHorizonTile() BenchTile {
	return BenchTile{
		Key: "horizon", Glyph: "◐", Eyebrow: "Horizon", NeedsBackend: true,
		Headline: "Not tracked yet",
		Qual:     "there is no stored knowledge horizon to read",
		Detail:   "what the party knows is not modelled on Chronicle yet",
	}
}

// benchTicks derives the Today tile's month rule from the primary Block's own
// cells — no second query, and the tile can never disagree with the grid
// beside it.
//
// Every tick carries data-day, the ANSWER key, even though nothing answers yet:
// a surface that forgets the key simply stops answering when W-B lands, and
// that is invisible in review (COMMON §6.6; guard B4).
func benchTicks(d *calblock.BlockData) []BenchTick {
	if d == nil {
		return nil
	}
	out := make([]BenchTick, 0, d.Month.Days)
	for _, row := range d.Month.Rows {
		for _, c := range row.Cells {
			if c.Day == 0 {
				continue
			}
			t := BenchTick{
				Day:    c.Day,
				DayKey: benchDayKey(d.CalendarSlug, c.Day),
				Event:  len(c.Marks) > 0,
				Today:  c.IsToday,
			}
			if len(c.Marks) > 0 {
				t.Axis = c.Marks[0].Axis
			}
			out = append(out, t)
		}
	}
	return out
}

// benchDayKey is the ANSWER key format the signed contract uses:
// "<calendar>-<day>".
func benchDayKey(slug string, day int) string {
	return fmt.Sprintf("%s-%d", slug, day)
}

// benchNonEmpty drops blanks so a joined line never renders " · · ".
func benchNonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- NEXT UP ----------------------------------------------------------------

// benchNextUp shapes the cross-calendar index.
//
// EVERY COUNT IS COMPUTED POST-FILTER, PER VIEWER. The mockup's own hardcoded
// "all 11" footer is the same class of bug as the un-filtered UpcomingByCalendar
// path and is not ported: Total is the length of the viewer's own resolved
// index, and Calendars is how many calendars actually contributed to it.
func benchNextUp(rows []BlockUpcoming, isGM bool, campaignID string) BenchNextUp {
	out := BenchNextUp{Total: len(rows)}
	seen := map[string]bool{}
	for i := range rows {
		if rows[i].Calendar != nil {
			seen[rows[i].Calendar.ID] = true
		}
	}
	out.Calendars = len(seen)
	limit := len(rows)
	if limit > benchNextUpLimit {
		limit = benchNextUpLimit
	}
	for i := 0; i < limit; i++ {
		out.Rows = append(out.Rows, benchNextUpRow(&rows[i], isGM, campaignID))
	}
	return out
}

// benchNextUpRow prints one row of the index. The event's identity triple
// (axis, pattern, glyph) comes from the SAME resolver the Block's marks use, so
// an event reads identically in the grid and in the index.
func benchNextUpRow(r *BlockUpcoming, isGM bool, campaignID string) BenchNextUpRow {
	key, color, icon := blockEventAxisKey(r.Calendar, &r.Event)
	row := BenchNextUpRow{
		DateLabel:     benchOccurrenceLabel(r.Calendar, r.Date),
		CalendarLabel: benchShortCalName(r.Calendar),
		CalHue:        blockCalHue(r.Calendar),
		Name:          r.Event.Name,
		Axis:          color,
		Pattern:       blockPatternFor(key),
		Glyph:         icon,
		GMOnly:        blockAudienceFor(&r.Event, isGM) != nil,
		Href: fmt.Sprintf("/campaigns/%s/calendar/v2?year=%d&month=%d&day=%d",
			campaignID, r.Date.Year, r.Date.Month, r.Date.Day),
	}
	if r.Calendar != nil {
		row.DayKey = benchDayKey(blockCalendarSlug(r.Calendar), r.Date.Day)
	}
	return row
}

// benchOccurrenceLabel formats an occurrence in its OWN calendar's reckoning —
// "17 Deepwinter" in world, "Sat 25 Jul" on a real-world calendar. It is the
// arbitrary-date sibling of blockDateLabel, which formats only the calendar's
// CURRENT date.
func benchOccurrenceLabel(cal *Calendar, d BlockDate) string {
	if cal == nil || d.Month < 1 || d.Month > len(cal.Months) {
		return fmt.Sprintf("%d/%d", d.Month, d.Day)
	}
	month := cal.Months[d.Month-1].Name
	if cal.IsRealLife() {
		wd := ""
		if wl := blockWeekLen(cal); wl > 0 && len(cal.Weekdays) > 0 {
			if idx := v2WeekdayIndexFor(cal, d.Year, d.Month, d.Day); idx >= 0 && idx < len(cal.Weekdays) {
				wd = blockAbbrev(cal.Weekdays[idx].Name, 3) + " "
			}
		}
		return fmt.Sprintf("%s%d %s", wd, d.Day, blockAbbrev(month, 3))
	}
	return fmt.Sprintf("%d %s", d.Day, month)
}

// benchShortCalName is the index's caps column: the calendar's first word, which
// is what the signed index prints ("HARPTOS", "REAL", "ELVEN").
func benchShortCalName(cal *Calendar) string {
	name := blockCalendarName(cal)
	if first, _, ok := strings.Cut(name, " "); ok {
		return first
	}
	return name
}

// --- the RSVP panel ---------------------------------------------------------

// benchRsvpPanel is the design-ahead session panel. See BenchRsvp for why it
// draws no lanes, no densities and no member rows.
func benchRsvpPanel() BenchRsvp {
	return BenchRsvp{
		Title: "RSVP · Schedule",
		Note: "Per-member availability, time zones and session answers land here. " +
			"None of it is stored on Chronicle yet, so this panel states that " +
			"rather than drawing a schedule nobody entered.",
	}
}
