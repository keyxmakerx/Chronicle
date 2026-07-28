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
	"time"

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
	// NeedsBackend renders a VISIBLE `.badge.need` beside the disabled control
	// (WG-5, adopted product-wide and implemented only where wave 3 already
	// touches). Chronicle's shipped precedent was the opposite — an inert
	// control carried its reason in `title` alone — and that is independently an
	// a11y defect: `title` is not announced by several screen readers and is
	// unreachable by touch.
	//
	// THE CHIP'S TEXT IS ALWAYS THE LITERAL "needs backend", never the specific
	// reason, because `.badge.need` is not diluted (§7; REVIEW.md:428-432 and
	// the calendar-settings 9-badges-3-meanings failure). The specific reason is
	// carried VISIBLY too — in the surrounding surface's `.cap`, not only in
	// `title` — so the a11y rule and the non-dilution rule are both satisfied
	// instead of trading against each other.
	//
	// A chip is GM-tier by the audience rule, so a control that carries one is
	// GM-tier too: for a player it is ABSENT, not greyed
	// (decisions/2026-07-27-needs-backend-audience.md).
	NeedsBackend bool
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
// ── A CORRECTION, STATED AS ONE (C-CALV4-RSVP-P8 §2, ADR-048 §15) ──────────
//
// Wave 1 shipped this type carrying, verbatim: "there is no session entity, no
// RSVP table and no per-member time zone." ALL THREE CLAIMS WERE FALSE, and two
// of them were false when they were written:
//
//	sessions        — scheduled_date + scheduled_time since
//	                  sessions/migrations/004_session_scheduled_time.up.sql
//	RSVP storage    — calendar_event_rsvps + calendar_event_rsvp_tokens +
//	                  calendar_events.collect_rsvps, calendar migration 013,
//	                  and calendar_v2.templ already renders against them
//	member zones    — users.timezone since db/migrations/000001_baseline, with
//	                  a live edit surface at PUT /account/timezone
//	availability    — member_availability + availability_exceptions since
//	                  sessions/migrations/002, minute-accurate and DST-correct
//
// That was a preflight error which propagated into SIGNED HONESTY COPY — the
// exact failure class the honesty states exist to prevent. A `needs backend`
// chip that is wrong is a FABRICATED ABSENCE, and it is worse than no chip: it
// told the operator his most-wanted feature had no foundation when it had
// almost all of one. The retraction, not the fill, is why this type's doc block
// is the first thing the slice rewrote.
//
// WHAT IS ACTUALLY UNBACKED, and it is three controls rather than a panel:
// the propose write (routes_snapshot.txt carries no propose-from-window path),
// the reminder/nudge endpoint (the fan-out fires only on the collect_rsvps
// OFF→ON transition), and a server-side recommender — the last of which this
// slice retires by DERIVING the window arithmetically rather than storing it
// (WG-3). All three are GM-tier; a player receives none of them and therefore
// receives no `needs backend` chip at all
// (decisions/2026-07-27-needs-backend-audience.md).
type BenchRsvp struct {
	Title string
	// Note is the UNFILLED state's one sentence, set only when Filled is false.
	Note string
	// Filled is true once the panel has a roster to print. It is the single
	// gate between "this campaign has entered nothing" and the panel body —
	// never a per-section nil check scattered through the template.
	Filled bool

	// Frame is the head's one statement of what the times mean: "week of
	// 20 Jul 2026 · times in CDT". THE FRAME IS STATED ONCE PER SURFACE
	// (cv4:2235) — no row repeats it. FrameTitle is the full IANA identifier,
	// which rides in `title` on every abbreviation so the product carries one
	// fact at two densities rather than three conventions (§5).
	Frame      string
	FrameTitle string
	// Scope is the head's audience line: the signed string pair at cv4:2238-2239.
	// It is talking about the LANES, which is why a player still receives the
	// full member table beneath it.
	Scope string

	// DayHeads are the seven column labels; each carries its own data-day key.
	DayHeads []BenchRsvpDay
	// Lanes are the per-member availability lanes. OWNER / CO-DM ONLY, and
	// ABSENT rather than empty for anyone else: the payload a player receives
	// does not contain another member's lane data at all (§4).
	Lanes []BenchRsvpLane
	// Density is the anonymous aggregate — everyone's, at every role.
	Density []BenchRsvpDensity
	// Bracket is the derived recommended window's overlay bracket, nil when the
	// window was refused.
	Bracket *BenchRsvpBracket

	// Headline is the side's `Session 41 · today · 3 / 5`.
	Headline string
	// Rec is the derived-window sentence OR its refusal; RecDerived marks the
	// sentence as a computation rather than a stored recommendation, which is
	// what the permanent `derived · not stored` chip states (WG-3).
	Rec        string
	RecDerived bool
	// Why names what the score CANNOT include. It is permanent, not a chip:
	// the grid shows availability only and does not know what is already on the
	// calendar (the W-G honesty ledger's #16).
	Why string
	// Silent names the members who are in the campaign and have not answered.
	// DERIVED, never stored — there is no invitee table (ledger #13).
	Silent string
	// Actions are the Director's two unbacked controls. GM-tier: a player's DOM
	// omits them entirely (WG-8).
	Actions []BenchAction
	// ActionsWhy is the VISIBLE carrier of why those controls are inert (WG-5).
	ActionsWhy string
	SideCap    string

	// SlotLabel is the member table's head. NON-INTERACTIVE in Part A: the
	// mockup draws a `popovertarget` there and zero popovers exist in the
	// calendar-v4 surfaces today — W-F ships the first (§12).
	SlotLabel string
	// Members is EVERY member, at every role. Party-visible by the signed
	// contract (§4) — this is the part v4-bench-player-light.png proves.
	Members []BenchRsvpMember

	// Captions are the panel's foot: the recomputed-counts statement, the
	// numeric-offset zone degradation, and the density's own denominator.
	Captions []string
}

// BenchRsvpDay is one column head plus its ANSWER key.
type BenchRsvpDay struct {
	// Label is the weekday abbreviation the column prints.
	Label string
	// DayKey is the data-day key, in bench.templ's existing namespace
	// (benchTickRule's scheme) — one scheme, never a second (guard B4).
	DayKey string
}

// BenchRsvpLane is one member's availability lane. OWNER / CO-DM ONLY.
type BenchRsvpLane struct {
	// Initials is the lane's label — two characters, because the lane column is
	// 96px and a lane is an at-a-glance shape, not a roster.
	Initials string
	// Axis and Pattern are the identity pair, keyed to the STABLE ROSTER INDEX.
	// OverlayMember.Color is ignored: it is ten hex values with one channel and
	// it dies under grayscale(1). The pattern class is NOT optional (§3).
	Axis    string
	Pattern string
	// Free[i] is whether the member is free at any point on column i.
	Free []bool
	// DayKeys mirrors DayHeads so every dated cell carries its key (guard B4).
	DayKeys []string
}

// BenchRsvpDensity is one column of the anonymous aggregate row.
type BenchRsvpDensity struct {
	// Free is how many members are free at the busiest hour of that column;
	// Total is TotalMembers — the Director and everyone who never answered
	// included, which is why the row is never labelled "of 5 players" (§4).
	Free  int
	Total int
	// DayKey is the column's ANSWER key.
	DayKey string
	// Title is the cell's own denominator, spelled out, so the opacity is never
	// the only carrier of the number.
	Title string
}

// BenchRsvpBracket positions the derived window's `.recbr` over the columns it
// spans. Columns are 1-based grid lines past the 96px label column, which is
// what makes Start/End directly usable as a grid-column range.
type BenchRsvpBracket struct {
	Start int
	End   int
	Label string
}

// BenchRsvpMember is one row of the party-visible member table.
type BenchRsvpMember struct {
	Name string
	// Axis and Pattern are the same identity pair the lane uses, from the same
	// index — so a member reads as the same person in both halves of the panel.
	Axis    string
	Pattern string
	// Role is campaigns.Role.DisplayName(), printed through ONE shim
	// (benchRoleLabel) so the later "role names come from the installed game
	// system" slice is a one-file change.
	Role string
	// IsCoDM renders the third signed `.badge.gm` string, `co-DM` (WG-4).
	IsCoDM bool
	// Host marks the member whose campaign this is — the signed "(host)" note.
	Host bool

	// Zone is the abbreviation the chip prints (CDT / EDT / +0545); ZoneTitle is
	// the full IANA identifier. BOTH EMPTY means the member has not set a zone,
	// which is a first-class state: the row prints the `zone not set` repair and
	// an Ask link, and LocalTime is LITERALLY EMPTY — never `--:--`, never a
	// dash, never a UTC guess (§5).
	Zone      string
	ZoneTitle string
	// LocalTime is the session's start in THIS member's zone.
	LocalTime string
	// NextDay renders the `+1d` badge; Antisocial takes --warn ink below 06:00
	// and at or after 23:00. Both are drawn in every state — they are the two
	// cases where getting it wrong wakes somebody at 5am.
	NextDay    bool
	Antisocial bool

	// Answer is the `.rs` word: "in" | "out" | "maybe" | "—".
	Answer string
	// Tone tints it: "ok" | "bad" | "" (muted).
	Tone string
	// AskHref is the "Ask →" repair's target when the zone is unset.
	AskHref string
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

	// The cross-calendar index, viewer-filtered at the source. It feeds BOTH the
	// NEXT UP index and the RSVP panel's session — one read, one filter, so the
	// two can never disagree about which events this viewer may see.
	upcoming := benchUpcoming(ctx, spine, in.Campaign.ID, viewer)
	data.NextUp = benchNextUp(upcoming, data.IsGM, in.Campaign.ID)
	data.Rsvp = h.benchRsvpResolve(ctx, in, upcoming)

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
	case len(c.Months) == 1:
		return fmt.Sprintf("1 month · %d-day week", blockWeekLen(c))
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

// benchSessionTile is the RSVP tile in its NOT-YET-READING state.
//
// CORRECTED (C-CALV4-RSVP-P8 §2). Wave 1's copy said "session dates and RSVPs
// have no store on Chronicle yet" and titled every inert control "session
// scheduling and RSVP storage do not exist yet". Both were false — see
// BenchRsvp for the four shipped migrations they contradicted. The chip stays
// for exactly as long as this TILE does not read those stores, and the copy now
// names that gap rather than inventing a missing backend.
//
// The signed controls still render, INERT: present so the tile's geometry is
// final, disabled so they are not dead controls that swallow a click (the same
// reading the Block's own layersInvoker ships under). The trio is IMMUTABLE —
// three loose `.btn.xs`, Yes filled, then No, then Maybe, in that order.
func benchSessionTile(isGM bool) BenchTile {
	t := BenchTile{
		Key: "session", Glyph: "◷", Eyebrow: "Session",
		Headline: "Not scheduled here yet",
		Qual:     "no upcoming session on your calendars is collecting RSVPs",
	}
	const why = "this tile does not read the RSVP store yet"
	if isGM {
		// THE CHIP AND THE BUILD-STATUS LINE ARE GM-TIER, and the count oracle
		// caught them not being so: a `needs backend` chip is build status, and
		// build status never renders to a player
		// (decisions/2026-07-27-needs-backend-audience.md). Wave 1 shipped this
		// tile chipped at every role; the audience rule was written afterwards
		// and nothing had re-checked the surfaces it bound. A player now gets
		// the FACT — no session is collecting RSVPs — and no commentary on
		// Chronicle's build state.
		t.NeedsBackend = true
		t.Detail = why
		t.Actions = []BenchAction{{Label: "Nudge", Title: why, NeedsBackend: true}}
		return t
	}
	// The signed RSVP trio is IMMUTABLE: three loose .btn.xs, Yes filled, then
	// No, then Maybe, in that order. Inert here only because this tile does not
	// yet know which event to answer; they carry no chip, because a disabled
	// control whose reason is a build gap may not render to a player at all.
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
	// The qualifier names the LINK, not the state — the headline already said
	// the state, and "In sync / In sync · 1 of 4 linked" reads as a bug.
	// Segments whose data is absent are dropped rather than printed empty.
	t.Qual = strings.Join(benchNonEmpty(p.Transport, p.PushedAgo), " · ")
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

// --- the RSVP panel: the cross-plugin read seam -----------------------------

// BenchScheduleReader is the sessions slice the Bench RSVP panel reads.
//
// Declared HERE and injected by a post-construction setter from
// internal/app/routes.go (house rule 8), exactly as AvailabilityExceptionWriter
// is: the calendar plugin gains no compile-time edge into sessions — which
// internal/wire/plugin_import_guard_test.go forbids outright — and it is
// nil-safe, so a degraded sessions schema costs the panel its body and nothing
// else on the page.
//
// READS ONLY. Availability is written by the scheduler's own surfaces; the one
// thing this panel writes is the viewer's own RSVP, which is the calendar's own
// table and needs no seam at all.
type BenchScheduleReader interface {
	// BenchRoster is the PARTY-VISIBLE half: every member, in the overlay's
	// stable order, with role, co-DM grant and stored zone resolved. Nothing on
	// it is gated — see §4's law.
	BenchRoster(ctx context.Context, campaignID string) ([]BenchRosterMember, error)
	// BenchAvailability is the week overlay. includeDetail IS THE PERMISSION
	// and it is decided by the CALLER from role, in the handler, never by a
	// route: when false the lane data is absent from the returned value, not
	// merely unrendered.
	BenchAvailability(ctx context.Context, campaignID, weekStart, viewerTZ string,
		includeDetail bool) (*BenchAvailability, error)
}

// BenchRosterMember is one campaign member as the panel prints them.
type BenchRosterMember struct {
	UserID string
	Name   string
	// Role is campaigns.Role.DisplayName(): Owner | Scribe | Player.
	Role string
	// IsOwner is the order key and the "(host)" marker, not the label.
	IsOwner bool
	IsCoDM  bool
	// TZ is the stored IANA zone, EMPTY when unset — a state, not a default.
	TZ string
}

// BenchAvailability is the week overlay narrowed to what the panel prints.
type BenchAvailability struct {
	// WeekStart is the Monday the overlay snapped to, YYYY-MM-DD.
	WeekStart string
	// Days are the seven columns.
	Days []BenchAvailabilityDay
	// WithPattern is how many members have ANY saved availability in the week.
	// It is an aggregate — a count with no identity in it — so it is safe at
	// every role, and it is what the derived window's quorum is measured
	// against (WG-3).
	WithPattern int
	// FreeDays is the LANE data: user id → seven booleans. NIL when the viewer
	// is not entitled to it. Absence is in the payload (§4).
	FreeDays map[string][]bool
}

// BenchAvailabilityDay is one column's per-hour aggregate.
type BenchAvailabilityDay struct {
	Date string
	// Free[h] is how many members are free at the top of hour h, 24 entries.
	Free []int
}

// SetScheduleReader wires the sessions availability + roster reads.
func (h *Handler) SetScheduleReader(r BenchScheduleReader) { h.schedule = r }

// SetRSVPReader wires the event-RSVP read the panel's answer column needs. It
// is the SAME service the RSVP routes use — one arithmetic, not a Bench one and
// an API one.
func (h *Handler) SetRSVPReader(r RSVPService) { h.rsvpRead = r }

// --- the RSVP panel ---------------------------------------------------------

// benchRsvpQuorum is the number of members with saved availability below which
// the derived window REFUSES to rank (WG-3). A ranking from two people's data
// is a guess wearing a number, and this panel's whole subject is the difference.
const benchRsvpQuorum = 3

// benchRsvpAntisocialEarly / Late bound the hours that take --warn ink on a
// member's local clock: before 06:00 (cv4:1021) and at or after 23:00 (the W-G
// spec's :143). They are drawn in EVERY state because they are the two cases
// where getting it wrong wakes somebody at 5am.
const (
	benchRsvpAntisocialEarly = 6
	benchRsvpAntisocialLate  = 23
)

// benchRsvpHues are the identity hues, keyed to the STABLE ROSTER INDEX. The
// first five are the signed contract's own owner hues (cv4 --own-kael..--own-rell);
// the last three extend the ramp for campaigns larger than the fixture.
// Defined as tokens in calendar-bench.css so light and dark each get their own
// ramp; this list only names them.
var benchRsvpHues = []string{
	"var(--own-1)", "var(--own-2)", "var(--own-3)", "var(--own-4)",
	"var(--own-5)", "var(--own-6)", "var(--own-7)", "var(--own-8)",
}

// benchRsvpPatterns is the GREYSCALE IDENTITY CHANNEL. Colour is never
// load-bearing alone: a member's swatch carries a locked dash pattern as well as
// a hue, so a viewer who cannot separate the hues can still separate the people.
// This is why OverlayMember.Color is ignored — it is ten hex values with one
// channel, and it dies under grayscale(1) (§3).
var benchRsvpPatterns = []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}

// benchRsvpIdentity resolves the (hue, pattern) pair for roster index i.
//
// Past index 7 the hue REPEATS and the pattern STEPS, so the pair stays unique
// far past any plausible roster: index 8 is hue 1 with pattern 2, not a second
// hue-1/pattern-1 twin. The signed panel's swatch is `swatch ${o.p}` — the
// pattern class is not optional, and mockups/v4-proposed/roles-and-rsvp.html's
// pattern-less swatch is a defect this must not inherit (§3).
func benchRsvpIdentity(i int) (hue, pattern string) {
	if i < 0 {
		i = 0
	}
	n := len(benchRsvpHues)
	return benchRsvpHues[i%n], benchRsvpPatterns[(i+i/n)%n]
}

// benchRoleLabel is THE ONE SHIM for printed role names.
//
// decisions/2026-07-27-calendar-scope-and-roles.md §4 ruled that role display
// names will come from the installed game system ("DM" / "Director") and that
// the slice doing it is not yet scoped. So the literal ships through a single
// lookup rather than as a bare string at each call site, and that later slice is
// a one-file change. The W-G spec's "there is no Director terminology system"
// is stale — this is where it will attach.
func benchRoleLabel(role string) string {
	if strings.TrimSpace(role) == "" {
		return "Player"
	}
	return role
}

// benchRsvpInitials is the lane's 96px label: at most two characters, from the
// member's own name. A lane is an at-a-glance shape, not a roster — the roster
// is the member table underneath, which prints the full name for everyone.
func benchRsvpInitials(name string) string {
	fields := strings.Fields(name)
	switch {
	case len(fields) == 0:
		return "··"
	case len(fields) == 1:
		r := []rune(fields[0])
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[0:2]))
	default:
		a := []rune(fields[0])
		b := []rune(fields[len(fields)-1])
		return strings.ToUpper(string(a[0])) + strings.ToUpper(string(b[0]))
	}
}

// benchRsvpPanel is the panel's unfilled state — the one a campaign with no
// availability, no roster read and no RSVP-collecting session gets.
//
// The note names the REAL gap. Wave 1's note said "none of it is stored on
// Chronicle yet", which was false (BenchRsvp): the stores shipped in calendar
// migration 013, sessions migration 002 and core migration 000001. What is
// actually empty here is the campaign's own data, and a surface that says "the
// feature does not exist" when it means "nobody has entered anything" teaches
// the operator to distrust every other honesty state on the page.
func benchRsvpPanel() BenchRsvp {
	return BenchRsvp{
		Title: "RSVP · Schedule",
		Note: "Nobody in this campaign has saved availability, and no upcoming " +
			"session is collecting RSVPs. The storage for all of it exists — " +
			"this panel fills in from the scheduler and the event RSVP opt-in.",
	}
}

// BenchRsvpSession is the occurrence the panel is about: the soonest upcoming
// event on a real-world calendar with RSVP collection switched on, taken from
// the viewer's OWN already-filtered index (§4's W5a rule — no second calendar
// read, no role branch that resolves calendars itself).
type BenchRsvpSession struct {
	Name string
	// DaysUntil is in that calendar's own days, which for a real-world calendar
	// is real days.
	DaysUntil int
	// Instant is the occurrence's real-world instant, resolved from the stored
	// wall clock in the CALENDAR'S OWN anchor zone — the only defensible anchor,
	// because a session happens once and every member's clock is a conversion of
	// that one instant.
	Instant time.Time
	// Anchored is false when the calendar carries no anchor zone or the event
	// carries no time. THEN NO PER-MEMBER CLOCK IS PRINTED AT ALL: converting
	// from a wall clock with no zone would be a guess presented as a fact, which
	// is the same error the zone-less member's empty clock refuses to make.
	Anchored bool
}

// benchRsvpInput is everything benchRsvpBuild needs, all of it already resolved
// by the handler. Keeping the builder PURE is what lets the count oracle
// reproduce every number on the panel from the same visible set the viewer got.
type benchRsvpInput struct {
	// IsGM is the lane gate and the Director-controls gate. It is the ONLY
	// permission input, and it decides what is in the payload rather than what
	// is hidden in the template.
	IsGM       bool
	ViewerID   string
	CampaignID string

	// Roster is every member, in the overlay's stable order. THE ORDER IS THE
	// IDENTITY KEY: a member answering may not move a single element.
	Roster []BenchRosterMember
	// Avail is the week overlay; nil when the sessions read is unavailable.
	Avail *BenchAvailability
	// Answers is the RAW STORED SET keyed by user id, ex-members included. It is
	// deliberately unfiltered here so the filtering happens once, visibly, in
	// this function, against the roster the panel actually prints.
	Answers map[string]string
	Session *BenchRsvpSession

	// ViewerZone is the IANA zone the panel's own times are stated in.
	// ViewerZoneSource is "member" | "calendar" | "none" and it exists so the
	// frame can say WHOSE zone it is rather than implying it is the viewer's.
	ViewerZone       string
	ViewerZoneSource string
	// WeekLabel is the human date of the overlay week's Monday.
	WeekLabel string
}

// benchRsvpBuild assembles the signed panel.
//
// ── EVERY NUMBER ON THIS SURFACE IS RECOMPUTED HERE (§6) ───────────────────
//
// No stored RSVP aggregate reaches the panel. EventRSVPSummary.Counts is raw
// rows while the named lists drop ex-members, so a stored aggregate printed
// beside a membership-filtered name list is a counts-vs-names disagreement BY
// CONSTRUCTION — and it grows every time somebody leaves a campaign. Killing it
// structurally is cheaper than asserting it, so the tally, the silent list and
// the density denominator are all folded out of `in.Roster` here, and the
// caption says why. A departed member holding a stored answer row therefore
// changes no number and appears in no list.
//
// The GM's and a player's tally are the SAME NUMBER COMPUTED THE SAME WAY: this
// function has one arithmetic, and IsGM gates only what is absent from the
// payload (the lanes and the Director's controls), never how anything is
// counted.
func benchRsvpBuild(in benchRsvpInput) BenchRsvp {
	if len(in.Roster) == 0 {
		return benchRsvpPanel()
	}
	p := BenchRsvp{Title: "RSVP · Schedule", Filled: true}
	p.Frame, p.FrameTitle = benchRsvpFrame(in)
	if in.IsGM {
		p.Scope = "per-member lanes · owner / co-DM only"
	} else {
		p.Scope = "anonymous density only"
	}
	p.DayHeads = benchRsvpDayHeads(in.Avail)

	// The lanes are the ONLY per-member availability on the surface and they are
	// owner / co-DM only. For anyone else in.Avail.FreeDays is nil to begin
	// with — the absence is in the payload, not in this branch.
	if in.IsGM && in.Avail != nil && in.Avail.FreeDays != nil {
		p.Lanes = benchRsvpLanes(in.Roster, in.Avail, p.DayHeads)
	}
	p.Density = benchRsvpDensity(in.Avail, len(in.Roster), p.DayHeads)

	// The derived window: arithmetic over the overlay's own per-hour free
	// counts, refused below quorum. It is NOT a recommender and does not
	// pretend to be one (WG-3).
	p.Rec, p.RecDerived, p.Why, p.Bracket = benchRsvpWindow(in)

	answered, silent := benchRsvpTally(in.Roster, in.Answers)
	p.Headline = benchRsvpHeadline(in.Session, answered, len(in.Roster))
	p.Silent = silent
	p.SideCap = "slot localised per member below"
	if in.IsGM {
		p.Actions = []BenchAction{
			{Label: "Propose", Fill: true, NeedsBackend: true,
				Title: "there is no propose-from-window write path"},
			{Label: "Nudge", NeedsBackend: true,
				Title: "there is no reminder endpoint — RSVP mail fans out only when collection is switched on"},
		}
		// THE VISIBLE CARRIER of why those two are inert (WG-5). `title` is
		// supplementary here, never the only carrier: it is not announced by
		// several screen readers and is unreachable by touch.
		p.ActionsWhy = "Propose and Nudge are inert: Chronicle has no propose-from-window " +
			"write path, and RSVP mail fans out only at the moment collection is switched on."
	}

	p.SlotLabel = benchRsvpSlotLabel(in)
	p.Members = benchRsvpMembers(in)
	p.Captions = benchRsvpCaptions(in, p)
	return p
}

// benchRsvpFrame states what the panel's times mean, ONCE, and says whose zone
// it is (§5). A viewer with no stored zone is told so rather than shown a UTC
// guess wearing their name.
func benchRsvpFrame(in benchRsvpInput) (frame, title string) {
	week := in.WeekLabel
	if week == "" {
		week = "this week"
	} else {
		week = "week of " + week
	}
	abbr, full := benchRsvpZone(in.ViewerZone, in.Session)
	switch in.ViewerZoneSource {
	case "member":
		return week + " · times in " + abbr, full
	case "calendar":
		return week + " · times in the calendar's zone, " + abbr +
			" — you have not set yours", full
	default:
		return week + " · no time zone is set for you or this calendar", ""
	}
}

// benchRsvpZone renders a zone as (abbreviation, full IANA identifier).
//
// THE ABBREVIATION IS RESOLVED AT THE SESSION'S OWN INSTANT, not at "now", so a
// session either side of a DST boundary is labelled with the offset that
// actually applies to it — the same rule, through the same stdlib call, that
// W-B pinned into Mark.Time. Zones with no alphabetic abbreviation degrade to a
// numeric offset (+0545); that residual is stated in a caption, not a chip (§5).
func benchRsvpZone(zone string, s *BenchRsvpSession) (abbr, full string) {
	if strings.TrimSpace(zone) == "" {
		return "", ""
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return "", ""
	}
	at := time.Now()
	if s != nil && s.Anchored {
		at = s.Instant
	}
	return at.In(loc).Format("MST"), zone
}

// benchRsvpDayHeads builds the seven column heads. Every dated node carries a
// data-day key in bench.templ's EXISTING namespace (benchTickRule's scheme) —
// one scheme, never a second (guard B4).
func benchRsvpDayHeads(a *BenchAvailability) []BenchRsvpDay {
	out := make([]BenchRsvpDay, 0, 7)
	if a == nil || len(a.Days) == 0 {
		for _, n := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
			out = append(out, BenchRsvpDay{Label: n})
		}
		return out
	}
	for _, d := range a.Days {
		label := d.Date
		if t, err := time.Parse("2006-01-02", d.Date); err == nil {
			label = t.Format("Mon")
		}
		out = append(out, BenchRsvpDay{Label: label, DayKey: d.Date})
	}
	return out
}

// benchRsvpLanes builds the owner/co-DM availability lanes, in ROSTER ORDER —
// never by availability, tally or answer, because the order is the identity key
// and a member answering may not move a single element (§3).
func benchRsvpLanes(roster []BenchRosterMember, a *BenchAvailability, heads []BenchRsvpDay) []BenchRsvpLane {
	keys := make([]string, len(heads))
	for i, h := range heads {
		keys[i] = h.DayKey
	}
	out := make([]BenchRsvpLane, 0, len(roster))
	for i, m := range roster {
		hue, pattern := benchRsvpIdentity(i)
		free := make([]bool, len(heads))
		for c, v := range a.FreeDays[m.UserID] {
			if c < len(free) {
				free[c] = v
			}
		}
		out = append(out, BenchRsvpLane{
			Initials: benchRsvpInitials(m.Name),
			Axis:     hue,
			Pattern:  pattern,
			Free:     free,
			DayKeys:  keys,
		})
	}
	return out
}

// benchRsvpDensity folds the anonymous aggregate.
//
// THE DENOMINATOR IS TotalMembers — the Director and everyone who never
// answered included — so the row is never labelled "of 5 players" (§4). The
// numerator is the busiest hour of that column, which is the only per-day
// reduction the per-hour aggregate supports without inventing a rule, and it is
// spelled out in each cell's title so the opacity is never the only carrier.
func benchRsvpDensity(a *BenchAvailability, total int, heads []BenchRsvpDay) []BenchRsvpDensity {
	out := make([]BenchRsvpDensity, 0, len(heads))
	for i, h := range heads {
		cell := BenchRsvpDensity{Total: total, DayKey: h.DayKey}
		if a != nil && i < len(a.Days) {
			for _, n := range a.Days[i].Free {
				if n > cell.Free {
					cell.Free = n
				}
			}
		}
		cell.Title = fmt.Sprintf("%s — %d of %d members free at the busiest hour",
			h.Label, cell.Free, cell.Total)
		out = append(out, cell)
	}
	return out
}

// benchRsvpTally recomputes the answered count and the silent list FROM THE
// ROSTER, which is the membership-filtered set the panel prints.
//
// A stored answer belonging to somebody who has left the campaign is simply not
// reachable from here: the loop walks members, not rows. That is the whole
// point — the disagreement is impossible rather than merely tested for (§6).
// "Silent" is likewise DERIVED, not stored: there is no invitee table, so the
// only honest definition is "in the campaign and holding no row" (ledger #13).
func benchRsvpTally(roster []BenchRosterMember, answers map[string]string) (answered int, silent string) {
	var quiet []string
	for _, m := range roster {
		if _, ok := answers[m.UserID]; ok {
			answered++
			continue
		}
		quiet = append(quiet, m.Name)
	}
	switch {
	case len(quiet) == 0:
		return answered, ""
	case len(quiet) <= 3:
		return answered, strings.Join(quiet, ", ") + " silent"
	default:
		return answered, fmt.Sprintf("%s and %d more silent",
			strings.Join(quiet[:3], ", "), len(quiet)-3)
	}
}

// benchRsvpHeadline is the side's `Session 41 · today · 3 / 5`.
func benchRsvpHeadline(s *BenchRsvpSession, answered, total int) string {
	if s == nil {
		return fmt.Sprintf("No session collecting RSVPs · %d in the campaign", total)
	}
	return fmt.Sprintf("%s · %s · %d / %d", s.Name, benchRsvpWhen(s.DaysUntil), answered, total)
}

// benchRsvpWhen renders DaysUntil in words.
func benchRsvpWhen(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", days)
	}
}

// benchRsvpWindow DERIVES the recommended window (WG-3, signed).
//
// THIS IS ARITHMETIC, NOT A RECOMMENDER, and the difference is the reason it is
// allowed to ship. WeekOverlay already carries a per-hour free count per day, so
// "the longest contiguous run of hours at the week's highest free count, and how
// many that is" needs no store, no scoring model and no new table — it is the
// same class as the Almanac's printed arithmetic. Its honesty state is
// therefore the PERMANENT `derived · not stored` chip rather than a temporary
// `needs backend` one, and `Propose` stays inert beside it: the readout is real,
// the action is not, and the gap between them is stated in the caption.
//
// THE QUORUM REFUSAL IS NOT A DEGRADED PATH, IT IS THE POINT. Below three
// members with saved availability the function refuses to rank and says so —
// a ranking from two people's data is a guess wearing a number, and a number is
// exactly what this panel must not invent.
func benchRsvpWindow(in benchRsvpInput) (rec string, derived bool, why string, bracket *BenchRsvpBracket) {
	const cannot = "This ranks availability only — it does not know what is already on the calendar."
	if in.Avail == nil || len(in.Avail.Days) == 0 {
		return "", false, "", nil
	}
	if in.Avail.WithPattern < benchRsvpQuorum {
		return fmt.Sprintf("Not enough saved availability to rank — %d of %d members have entered any.",
			in.Avail.WithPattern, len(in.Roster)), false, "", nil
	}

	// The peak free count anywhere in the week, then the longest contiguous run
	// of hours holding it. Earliest day and earliest hour win ties, so the
	// answer is stable across renders.
	best := 0
	for _, d := range in.Avail.Days {
		for _, n := range d.Free {
			if n > best {
				best = n
			}
		}
	}
	if best == 0 {
		return "Nobody is free at any hour this week.", false, cannot, nil
	}
	bestDay, bestStart, bestLen := -1, 0, 0
	for di, d := range in.Avail.Days {
		run := 0
		for h := 0; h <= len(d.Free); h++ {
			if h < len(d.Free) && d.Free[h] >= best {
				run++
				continue
			}
			if run > bestLen {
				bestDay, bestStart, bestLen = di, h-run, run
			}
			run = 0
		}
	}
	if bestDay < 0 {
		return "Nobody is free at any hour this week.", false, cannot, nil
	}

	label := in.Avail.Days[bestDay].Date
	if t, err := time.Parse("2006-01-02", label); err == nil {
		label = t.Format("Mon")
	}
	abbr, _ := benchRsvpZone(in.ViewerZone, in.Session)
	// The end hour is EXCLUSIVE and printed as the clock time the window closes
	// at, so a one-hour peak reads 19:00–20:00 rather than 19:00–19:00.
	window := fmt.Sprintf("%s %02d:00–%02d:00", label, bestStart, bestStart+bestLen)
	if abbr != "" {
		window += " " + abbr
	}
	rec = fmt.Sprintf("Most free: %s — %d of %d members", window, best, len(in.Roster))
	return rec, true, cannot, &BenchRsvpBracket{
		// Grid lines past the 96px label column: column 1 is the label, so day
		// index d spans lines d+2 to d+3.
		Start: bestDay + 2,
		End:   bestDay + 3,
		Label: "most free",
	}
}

// benchRsvpSlotLabel is the member table's head. NON-INTERACTIVE in Part A —
// the mockup draws a popovertarget there and zero popovers exist in the
// calendar-v4 surfaces today; W-F ships the first (§12).
func benchRsvpSlotLabel(in benchRsvpInput) string {
	if in.Session == nil {
		return "No session collecting RSVPs"
	}
	if !in.Session.Anchored {
		// A session with no anchored instant has no clock to state, and stating
		// one anyway is the exact error the zone-less member's empty clock
		// refuses to make.
		return in.Session.Name + " · no time set"
	}
	abbr, _ := benchRsvpZone(in.ViewerZone, in.Session)
	if in.ViewerZone == "" || abbr == "" {
		return in.Session.Name
	}
	loc, err := time.LoadLocation(in.ViewerZone)
	if err != nil {
		return in.Session.Name
	}
	return fmt.Sprintf("%s · slot %s %s", in.Session.Name,
		in.Session.Instant.In(loc).Format("15:04"), abbr)
}

// benchRsvpMembers builds the PARTY-VISIBLE member table — every member, at
// every role, with their answer, their zone and their own local clock.
//
// This is the half of the panel the signed player render proves
// (v4-bench-player-light.png shows a player receiving Kael, Bryn, Tam, Nissa and
// Rell, their roles, their zone chips, their local clocks, Nissa's +1d, Rell's
// `zone not set` + `Ask →`, and every answer word). It is listed in the
// dispatch's divergence table specifically so nobody "fixes" it toward the
// unsigned spec, which gives a player only their own row (§4).
func benchRsvpMembers(in benchRsvpInput) []BenchRsvpMember {
	out := make([]BenchRsvpMember, 0, len(in.Roster))
	for i, m := range in.Roster {
		hue, pattern := benchRsvpIdentity(i)
		row := BenchRsvpMember{
			Name:    m.Name,
			Axis:    hue,
			Pattern: pattern,
			Role:    benchRoleLabel(m.Role),
			IsCoDM:  m.IsCoDM,
			Host:    m.IsOwner,
		}
		row.Answer, row.Tone = benchRsvpAnswerWord(in.Answers[m.UserID])
		if strings.TrimSpace(m.TZ) == "" {
			// NO ZONE IS A FIRST-CLASS STATE. users.timezone is NULLABLE and
			// every resolver in the product falls back to "UTC", so a clock
			// rendered here would be a guess presented as a fact. The signed
			// pair is the `zone not set` badge plus an Ask link, and the clock
			// is LITERALLY EMPTY — never "--:--", never a dash (§5).
			row.AskHref = benchRsvpAskHref(in.CampaignID)
			out = append(out, row)
			continue
		}
		row.Zone, row.ZoneTitle = benchRsvpZone(m.TZ, in.Session)
		if in.Session != nil && in.Session.Anchored {
			if loc, err := time.LoadLocation(m.TZ); err == nil {
				local := in.Session.Instant.In(loc)
				row.LocalTime = local.Format("15:04")
				row.NextDay = benchRsvpNextDay(in, loc)
				h := local.Hour()
				row.Antisocial = h < benchRsvpAntisocialEarly || h >= benchRsvpAntisocialLate
			}
		}
		out = append(out, row)
	}
	return out
}

// benchRsvpNextDay reports whether the session falls on a LATER calendar date in
// this member's zone than in the panel's own frame — the signed `+1d` badge.
// Drawn in every state, because a member who reads 01:00 without it will be a
// day early or a day late.
func benchRsvpNextDay(in benchRsvpInput, memberLoc *time.Location) bool {
	if in.Session == nil || !in.Session.Anchored || in.ViewerZone == "" {
		return false
	}
	viewerLoc, err := time.LoadLocation(in.ViewerZone)
	if err != nil {
		return false
	}
	return in.Session.Instant.In(memberLoc).Format("2006-01-02") >
		in.Session.Instant.In(viewerLoc).Format("2006-01-02")
}

// benchRsvpAnswerWord maps a stored status onto the signed `.rs` word and its
// tone. An absent answer is an em dash in muted ink — the row still renders,
// because a member who has not answered is the whole reason the panel exists.
func benchRsvpAnswerWord(status string) (word, tone string) {
	switch status {
	case RSVPYes:
		return "in", "ok"
	case RSVPNo:
		return "out", "bad"
	case RSVPMaybe:
		return "maybe", ""
	default:
		return "—", ""
	}
}

// benchRsvpAskHref is the `Ask →` repair's target: the campaign's member list,
// where an owner can see who to prod. It is a LINK to a page that exists rather
// than a control that does nothing, which is why the repair survives at every
// width while the Director's inert controls do not render to a player at all.
func benchRsvpAskHref(campaignID string) string {
	if campaignID == "" {
		return ""
	}
	return "/campaigns/" + campaignID + "/settings/members"
}

// benchRsvpCaptions is the panel's foot. Each line states a fact the numbers
// above cannot state about themselves.
func benchRsvpCaptions(in benchRsvpInput, p BenchRsvp) []string {
	caps := []string{
		// §6's mandated sentence, near enough verbatim: it names the exact
		// disagreement the recomputation prevents.
		"Counts are recomputed from these rows, not from the stored tally, because the " +
			"stored tally still counts people who have left the campaign.",
	}
	if len(p.Density) > 0 {
		caps = append(caps, fmt.Sprintf("The density row counts all %d members of this campaign, "+
			"the Director included — not only the players, and not only the people who answered.",
			p.Density[0].Total))
	}
	// The honest residual of the zone ruling, and it is a CAPTION rather than a
	// chip: abbreviations are real, and the gap is narrower than the spec
	// claimed (§5).
	if benchRsvpAnyNumericZone(p.Members) {
		caps = append(caps, "Zones with no letter abbreviation are shown as a numeric UTC offset; "+
			"the full identifier is on every chip.")
	}
	if in.Session != nil && !in.Session.Anchored {
		caps = append(caps, "This session has no anchored start instant, so no member's local "+
			"clock is shown — a converted time would be a guess.")
	}
	if in.IsGM && p.RecDerived {
		caps = append(caps, "The window above is derived from saved availability at render time and "+
			"is not stored anywhere; Propose cannot act on it yet.")
	}
	return caps
}

// benchRsvpAnyNumericZone reports whether any printed zone degraded to a numeric
// offset, so the caption is stated only when it is true of this render.
func benchRsvpAnyNumericZone(members []BenchRsvpMember) bool {
	for _, m := range members {
		if m.Zone != "" && (m.Zone[0] == '+' || m.Zone[0] == '-') {
			return true
		}
	}
	return false
}

// benchRsvpResolve gathers the panel's inputs from the seams and hands them to
// the pure builder.
//
// ── THE W5a SPLIT STAYS SPLIT (§4) ─────────────────────────────────────────
//
// This function performs NO calendar read of its own. The session it prints is
// taken from `upcoming`, which buildBench already resolved through
// UpcomingAcrossCalendars — viewer-filtered AT THE SOURCE — so there is no
// second calendar resolution here and no role branch that resolves calendars
// itself. app_dashboard.go:96 states in capitals why unifying those paths
// reopens the leak, and the cheapest way to honour it is to add no path at all.
//
// ── includeDetail IS GATED HERE, BY ROLE, NEVER BY ROUTE ───────────────────
//
// in.Role decides what the sessions seam RETURNS, not what the template hides.
// A player's BenchAvailability comes back with FreeDays nil, so their HTML
// cannot contain another member's lane data even by accident.
func (h *Handler) benchRsvpResolve(ctx context.Context, in benchInput, upcoming []BlockUpcoming) BenchRsvp {
	if h.schedule == nil {
		return benchRsvpPanel()
	}
	roster, err := h.schedule.BenchRoster(ctx, in.Campaign.ID)
	if err != nil || len(roster) == 0 {
		if err != nil {
			slog.Warn("bench: rsvp roster read failed",
				slog.String("campaign_id", in.Campaign.ID), slog.Any("error", err))
		}
		return benchRsvpPanel()
	}

	isGM := permissions.CanSeeDmOnly(in.Role)
	out := benchRsvpInput{
		IsGM: isGM, ViewerID: in.UserID, CampaignID: in.Campaign.ID, Roster: roster,
	}

	session, evt, anchorZone := benchRsvpPickSession(upcoming)
	out.Session = session

	// The zone the panel states its own times in: the viewer's own stored zone
	// first, then the calendar's anchor, then nothing — and ViewerZoneSource
	// carries WHICH, so the frame can say whose zone it is instead of implying
	// it belongs to the reader.
	for _, m := range roster {
		if m.UserID == in.UserID && strings.TrimSpace(m.TZ) != "" {
			out.ViewerZone, out.ViewerZoneSource = m.TZ, "member"
			break
		}
	}
	if out.ViewerZone == "" && anchorZone != "" {
		out.ViewerZone, out.ViewerZoneSource = anchorZone, "calendar"
	}
	if out.ViewerZone == "" {
		out.ViewerZoneSource = "none"
	}

	weekStart := benchRsvpWeekStart(session)
	out.WeekLabel = weekStart.Format("2 Jan 2006")
	projectZone := out.ViewerZone
	if projectZone == "" {
		projectZone = "UTC" // the grid has to be drawn in SOME zone; the frame says so.
	}
	if avail, aerr := h.schedule.BenchAvailability(ctx, in.Campaign.ID,
		weekStart.Format("2006-01-02"), projectZone, isGM); aerr == nil {
		out.Avail = avail
	} else {
		slog.Warn("bench: rsvp availability read failed",
			slog.String("campaign_id", in.Campaign.ID), slog.Any("error", aerr))
	}

	if evt != nil && h.rsvpRead != nil {
		answers, rerr := h.rsvpRead.AnswersByUser(ctx, evt, in.UserID, in.Role)
		if rerr != nil {
			slog.Warn("bench: rsvp answers read failed",
				slog.String("event_id", evt.ID), slog.Any("error", rerr))
		}
		out.Answers = answers
	}
	return benchRsvpBuild(out)
}

// benchRsvpPickSession finds the occurrence the panel is about: the soonest row
// of the VIEWER'S OWN index that sits on a real-world calendar and has RSVP
// collection switched on.
//
// Real-world only, deliberately. An in-world date has no instant and no zone,
// and a zone-labelled real-world time on an in-world calendar would contradict
// L15's own rule — real-world time and in-world time can never be confused, and
// this panel is entirely about real clocks in real places.
func benchRsvpPickSession(upcoming []BlockUpcoming) (*BenchRsvpSession, *Event, string) {
	for i := range upcoming {
		u := &upcoming[i]
		if u.Calendar == nil || !u.Calendar.IsRealLife() || !u.Event.CollectRSVPs {
			continue
		}
		evt := u.Event
		s := &BenchRsvpSession{Name: evt.Name, DaysUntil: u.DaysUntil}
		anchor := ""
		if u.Calendar.RealTimeZone != nil {
			anchor = strings.TrimSpace(*u.Calendar.RealTimeZone)
		}
		if anchor != "" && evt.HasTime() {
			if loc, err := time.LoadLocation(anchor); err == nil {
				s.Instant = time.Date(u.Date.Year, time.Month(u.Date.Month), u.Date.Day,
					*evt.StartHour, *evt.StartMinute, 0, 0, loc)
				s.Anchored = true
			}
		}
		return s, &evt, anchor
	}
	return nil, nil, ""
}

// benchRsvpWeekStart is the Monday of the week the panel shows: the session's
// own week when there is a session, otherwise this one. BuildOverlay snaps to
// the Monday itself, so this only has to be a date inside the right week — but
// it is snapped here anyway, because the label printed in the frame has to be
// the same Monday the grid is built from.
func benchRsvpWeekStart(s *BenchRsvpSession) time.Time {
	base := time.Now().UTC()
	if s != nil && s.Anchored {
		base = s.Instant.UTC()
	}
	offset := (int(base.Weekday()) + 6) % 7 // Monday == 0
	return time.Date(base.Year(), base.Month(), base.Day()-offset, 0, 0, 0, 0, time.UTC)
}
