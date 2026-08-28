// Package systems — operator_diag_campaign.go is the CAMPAIGN half of the
// operator diagnostic catalog: "why does MY campaign look like this?"
//
// WHY IT EXISTS. The host.* family answers WHICH CODE IS RUNNING. On
// 2026-08-11 an operator reported three things about their phone — no RSVP on
// the calendar, no real moon on the real calendar, and a skybox that was still
// there — and answering them took a five-lane source investigation. The
// catalog scored 0 for 3, because every diagnostic in it reads process state,
// on-disk or embedded bytes, or entity rows, and NOT ONE reads `campaign_addons`,
// `campaigns.dashboard_layout` / `sidebar_config`, the `calendars` /
// `calendar_moons` tables, the route table, or any render decision. The one
// reachable move was `host.deploy-check data-bench-rsvp`, which would have
// answered "✓ found in the executable" — true, and the wrong answer.
//
// The blind spot is THREE axes, not one:
//
//   - campaign CONFIG (addons, layouts, sidebar),
//   - per-VIEWER state (`block_layers`, `bench_sections` — both
//     `(user_id, campaign_id)`-grained, both defaulting to something other
//     than nothing), and
//   - PRODUCER RULES ([SKY-1], benchClassify) that exist only as Go comments.
//
// THE MIRROR, STATED UP FRONT BECAUSE IT IS THIS FILE'S ONE REAL RISK.
// `benchClassify`, `resolveBenchSections`, the [SKY-1] seat arguments and the
// Almanac gate are all UNEXPORTED inside `internal/plugins/calendar`, and
// `internal/systems` must not import a plugin package. So `calendar.render`
// re-derives them here from the same inputs — it is a MIRROR of the producer,
// not a call into it. A mirror that silently goes stale would be a second
// opinion that disagrees with the page precisely while somebody is using it to
// decide what the page did, which is the worst available failure. Two things
// hold it honest:
//
//  1. Every mirrored rule carries its source file and the exact line it copies,
//     and `operator_diag_campaign_mirror_test.go` reads that source and fails
//     when the line changes.
//  2. The render trace SAYS it is a mirror, in its own output, every time.
//
// DEGRADE LOUDLY. An unwired provider prints "provider not wired"; a read that
// failed prints the error. A plausible-looking empty answer is the one thing
// none of these may ever produce — "no moons" and "nobody could read the moons
// table" must never render the same.
package systems

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ── the injected window ─────────────────────────────────────────────────────

// DiagCalendar is one calendar as the Bench's own loaders return it.
//
// MoonRowsStored and MoonsRendered are DELIBERATELY TWO FIELDS. Since
// 2026-08-11 the Block spine synthesizes THE Moon for a real-life calendar
// with no authored moons (`moon_fallback.go`), so "how many moons does this
// calendar declare" and "how many moons does the grid draw" are different
// numbers, and the gap between them is exactly what a GM asking "where is my
// moon" needs to see. Collapsing them into one count would hide the answer.
type DiagCalendar struct {
	ID   string
	Name string
	Mode string // "fantasy" | "reallife"

	IsDefault      bool
	TracksRealTime bool
	RealTimeZone   string
	Visibility     string

	Months    int
	Weekdays  int
	Seasons   int
	Eras      int
	Cycles    int
	Festivals int

	// MoonRowsStored is the `calendar_moons` row count (the calendar service's
	// own eager load, which applies no fallback). Empty when the provider could
	// not read it — see StoredCountNote.
	MoonRowsStored  int
	StoredCountNote string

	// MoonsRendered is len(cal.Moons) as the Block spine hands it to the
	// renderer, i.e. AFTER the real-Moon fallback.
	MoonsRendered int
	MoonNames     []string

	// SynthesizedMoon reports that MoonsRendered includes a body with no
	// database row.
	SynthesizedMoon bool
}

// ViewerFacts is who a render trace was run for.
type ViewerFacts struct {
	UserID     string
	Supplied   bool // false when the arg carried no `:userId`
	Found      bool // a campaign_members row resolved
	MemberRole int  // the raw membership role (RequireRole compares this)
	DmGranted  bool
	Role       int // the VISIBILITY role — cc.VisibilityRole(): RoleOwner when DmGranted
	Note       string
}

// CampaignCalendarFacts is everything `calendar.render` and `calendar.config`
// read. One provider call serves both: the expensive part is the calendar list
// plus its sub-resource counts, and both diagnostics need it.
type CampaignCalendarFacts struct {
	Found        bool
	CampaignID   string
	CampaignName string

	// AddonEnabled is nil when the addons service could not be read. Every
	// calendar route rides RequireAddon(addonSvc, "calendar"), so `false` here
	// means the whole feature is unreachable and no other line below matters.
	AddonEnabled *bool
	AddonNote    string

	// SpineInstalled is calendar.BlockSpine() != nil. A nil spine renders NO
	// Block at all — every calendar falls back to a subordinate row.
	SpineInstalled bool

	Viewer ViewerFacts

	// ListVia names which of the Bench's two list calls this viewer gets
	// (buildBench keeps them separate to preserve the W5a visibility split).
	ListVia string
	ListErr string

	// All is every calendar in the campaign, hydrated, in the Bench's own
	// order. Visible is the same list filtered to what the viewer may see, and
	// is nil when no viewer was supplied.
	All     []DiagCalendar
	Visible []DiagCalendar

	// ActiveID is the viewer's `calendar_active` pointer (empty when none).
	ActiveID   string
	ActiveNote string

	// DefaultID is what GetCalendar(campaignID) resolves to — the campaign
	// default, which is what the syncapi and the Foundry module are served.
	DefaultID   string
	DefaultNote string

	// SectionsStored is the viewer's stored CLOSED set for the four Bench
	// disclosures; SectionsNeverChosen is the nil case, which is NOT the same
	// as an empty list ([BR2-4]).
	SectionsStored      []string
	SectionsNeverChosen bool
	SectionsNote        string

	// LayersStored is the viewer's stored Block layer set, same nil-vs-empty
	// discipline.
	LayersStored      []string
	LayersNeverChosen bool
	LayersNote        string

	Notes []string
}

// RouteFact is one row of the LIVE Echo route table.
//
// Handler is Echo's own handler name (the runtime function name), which is what
// makes `campaign.surfaces` a measurement rather than an assertion: the surface
// map below declares which handler should serve each path, and a disagreement
// is printed.
type RouteFact struct {
	Method  string
	Path    string
	Handler string
}

// SidebarItemFact is one item from `campaigns.sidebar_config`.
type SidebarItemFact struct {
	Type    string
	Slug    string
	Label   string
	URL     string
	Visible bool
}

// CampaignSurfaceFacts is what `campaign.surfaces` reads.
type CampaignSurfaceFacts struct {
	Found        bool
	CampaignID   string
	CampaignName string

	Routes     []RouteFact
	RoutesNote string

	CalendarAddonEnabled *bool
	AddonNote            string

	// SidebarCalendarPath is the path the sidebar's Calendar item links to,
	// read from the SAME map the sidebar renders from (layouts' addon URL map)
	// rather than re-typed here.
	SidebarCalendarPath string
	SidebarItems        []SidebarItemFact
	SidebarNote         string

	Notes []string
}

// AddonFact is one row of `campaign_addons` joined to `addons`.
type AddonFact struct {
	Slug      string
	Name      string
	Status    string
	Enabled   bool
	Installed bool
}

// LayoutFact is one placed layout and the block TYPES in it.
//
// Blocks carries the types in placement order WITH duplicates, because "two
// skybox blocks" and "one skybox block" are different findings.
type LayoutFact struct {
	Surface  string // "dashboard_layout" | "owner_dashboard_layout" | "entity type <name> — page template" | …
	Stored   bool   // the column is non-NULL and parsed
	ParseErr string
	Blocks   []string
}

// CampaignConfigFacts is what `campaign.config` reads.
type CampaignConfigFacts struct {
	Found        bool
	CampaignID   string
	CampaignName string

	Addons     []AddonFact
	AddonsNote string

	Layouts     []LayoutFact
	LayoutsNote string

	// DefaultDashboardBlocks is DefaultDashboardLayout()'s own block types,
	// printed so "not in any default" is a comparison the reader can see rather
	// than a claim they have to take on trust.
	DefaultDashboardBlocks []string

	Notes []string
}

// CampaignDiagProvider is the injected read-only window into per-campaign
// state. Implemented by the app layer (dependency inversion — systems must not
// import the calendar / campaigns / addons plugins), wired once at startup by
// SetCampaignDiagProvider, exactly as SetInstalledPackagesProvider does.
type CampaignDiagProvider interface {
	CalendarFacts(ctx context.Context, campaignID, userID string) (CampaignCalendarFacts, error)
	SurfaceFacts(ctx context.Context, campaignID string) (CampaignSurfaceFacts, error)
	ConfigFacts(ctx context.Context, campaignID string) (CampaignConfigFacts, error)
}

var campaignDiagProvider CampaignDiagProvider

// SetCampaignDiagProvider wires the per-campaign read window for the
// calendar.* / campaign.* diagnostics.
func SetCampaignDiagProvider(p CampaignDiagProvider) { campaignDiagProvider = p }

// ── catalog entries ─────────────────────────────────────────────────────────

// calendarRenderDiagnostic is rank 1 of the 2026-08-11 proposals. Four
// independent causes currently render as one identical unfilled Bench, and
// three investigation lanes independently ended on "I cannot distinguish them".
func calendarRenderDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "calendar.render",
		Title:   "Why does the calendar page look like this for THIS viewer?",
		Desc:    "The v4 Bench's render trace: which calendar is Primary vs real-world and by which rule, whether each Block carries a sky, how many moons each draws and whether the Almanac register was built, the four disclosure sections and WHY each is open or closed, and the viewer's role. Run this before concluding a feature is missing — `host.deploy-check` can only say the code shipped.",
		ArgHint: "<campaignId>[:<userId>]",
		Run:     renderCalendarRender,
	}
}

// calendarConfigDiagnostic is rank 2: no viewer, one question — "is the data
// there at all?". Nothing in the catalog counted moon rows before this.
func calendarConfigDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "calendar.config",
		Title:   "What do this campaign's calendars actually contain?",
		Desc:    "One row per calendar — id, name, mode, is_default, tracks_real_time — with counts of months / weekdays / MOONS / seasons / eras, plus which calendar each surface resolves to. THE check for 'I don't see the moon': it prints stored moon rows and rendered moons separately, so a synthesized real Moon is never mistaken for a row you can edit.",
		ArgHint: "<campaignId>[:<calId>]",
		Run:     renderCalendarConfig,
	}
}

// campaignSurfacesDiagnostic is rank 3. Both calendar surfaces are Templ
// compiled into one binary, so neither appears in host.assets or host.embedded
// and nothing anywhere exposed the route table.
func campaignSurfacesDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "campaign.surfaces",
		Title:   "Which calendar page does a URL actually render? (live route table)",
		Desc:    "Every user-facing calendar route this campaign exposes, read from the LIVE Echo table with its real handler, flagged CURRENT / LEGACY / REDIRECT, plus where the sidebar's Calendar item links and any hand-authored sidebar link. THE answer to 'which calendar am I looking at?' and to 'the deploy landed but I still see the old thing'.",
		ArgHint: "<campaignId>",
		Run:     renderCampaignSurfaces,
	}
}

// campaignConfigDiagnostic is rank 4, and the ONLY way to establish whether a
// legacy `skybox` block was hand-placed: it is in no default layout and no
// migration seeds it.
func campaignConfigDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "campaign.config",
		Title:   "Enabled addons + the blocks this campaign has PLACED",
		Desc:    "Which addons are enabled (a disabled `calendar` makes every calendar route vanish), and the block TYPES placed in dashboard_layout / owner_dashboard_layout and on each entity template. THE check for 'the skybox is still there': a `skybox` block is in no default layout and no migration seeds it, so it can only be operator-placed.",
		ArgHint: "<campaignId>",
		Run:     renderCampaignConfig,
	}
}

// ── the mirrored producer rules ─────────────────────────────────────────────
//
// Each of these copies ONE rule out of internal/plugins/calendar. Every one
// names the file and the line it copies, and operator_diag_campaign_mirror_test.go
// reads that source and fails when the copied line changes.

// benchSeat is one classified seat on the Bench.
type benchSeat struct {
	Seat   string // "PRIMARY" | "REAL-WORLD" | "ROW"
	Cal    DiagCalendar
	Clause string // which rule selected it, in the producer's own terms
}

// mirrorBenchClassify mirrors benchClassify (bench.go:1390-1428).
//
// PRIMARY is the calendar a reader means when they say "the campaign
// calendar": the campaign default, else the viewer's active one, else the
// first in-world calendar, else the default at all, else the first row.
// REAL-WORLD is the first real-life calendar that is not already the primary.
// EVERYTHING ELSE IS A ROW.
//
// Reproduced rather than called because it is unexported and systems may not
// import the plugin. The CLAUSE STRINGS are the reason this is worth
// mirroring at all: the producer decides silently, and "which clause fired"
// is the fact that separates "signed behaviour, adding a moon will not help"
// from "adding one moon fixes it".
func mirrorBenchClassify(cals []DiagCalendar, activeID string) []benchSeat {
	if len(cals) == 0 {
		return nil
	}
	pick := func(match func(DiagCalendar) bool) int {
		for i := range cals {
			if match(cals[i]) {
				return i
			}
		}
		return -1
	}
	inWorld := func(c DiagCalendar) bool { return !isRealLifeMode(c.Mode) }

	primary, clause := pick(func(c DiagCalendar) bool { return c.IsDefault && inWorld(c) }),
		"campaign default AND in-world (`IsDefault && inWorld`)"
	if primary < 0 && activeID != "" {
		if primary = pick(func(c DiagCalendar) bool { return c.ID == activeID && inWorld(c) }); primary >= 0 {
			clause = "the viewer's ACTIVE calendar, and in-world (`c.ID == activeID && inWorld`)"
		}
	}
	if primary < 0 {
		if primary = pick(inWorld); primary >= 0 {
			clause = "the first in-world calendar (no default, no active)"
		}
	}
	if primary < 0 {
		if primary = pick(func(c DiagCalendar) bool { return c.IsDefault }); primary >= 0 {
			clause = "the campaign default — THIS CAMPAIGN HAS NOTHING BUT REAL-WORLD CALENDARS, so the real-world one is promoted to Primary and DOES get a sky"
		}
	}
	if primary < 0 {
		primary, clause = 0, "the first calendar in the list (nothing else matched)"
	}

	realWorld := pick(func(c DiagCalendar) bool {
		return isRealLifeMode(c.Mode) && c.ID != cals[primary].ID
	})

	out := []benchSeat{{Seat: seatPrimary, Cal: cals[primary], Clause: clause}}
	if realWorld >= 0 {
		out = append(out, benchSeat{
			Seat:   seatRealWorld,
			Cal:    cals[realWorld],
			Clause: "the first real-life calendar that is not the Primary",
		})
	}
	for i := range cals {
		if i == primary || i == realWorld {
			continue
		}
		out = append(out, benchSeat{Seat: seatRow, Cal: cals[i], Clause: "everything else is a subordinate ROW — no Block, no sky, no moons"})
	}
	return out
}

const (
	seatPrimary   = "PRIMARY"
	seatRealWorld = "REAL-WORLD"
	seatRow       = "ROW"
)

// isRealLifeMode mirrors Calendar.IsRealLife (model.go) — `Mode == "reallife"`.
func isRealLifeMode(mode string) bool { return mode == "reallife" }

// mirrorSeatRender mirrors the two [SKY-1] seat calls in buildBench:
//
//	bench.go:1129  h.benchBlock(…, primary,   …, false, true,  …)  → noShelf=false, sky=true
//	bench.go:1143  h.benchBlock(…, realWorld, …, true,  false, …)  → noShelf=true,  sky=false
//
// ONE SKY PER SURFACE, on the Primary Block and nowhere else. This is signed
// behaviour, not an oversight — a fix that seats a sky on the real-world Block
// would AMEND [SKY-1].
func mirrorSeatRender(seat string) (skyOn, shelfHidden bool) {
	switch seat {
	case seatPrimary:
		return true, false
	case seatRealWorld:
		return false, true
	default:
		return false, false // a ROW renders no Block at all
	}
}

// mirrorAlmanacBuilt mirrors the Almanac gate (block_geometry.go:773):
//
//	if (!in.ShelfHidden || !in.SkyHidden) && len(cal.Moons) > 0 {
//
// with SkyHidden = !SkyOn (block_projection.go:166). TWO READERS, NAMED
// SEPARATELY ([SKY-7]): either the Shelf or the sky header asking is enough.
func mirrorAlmanacBuilt(skyOn, shelfHidden bool, moonsRendered int) bool {
	return (!shelfHidden || skyOn) && moonsRendered > 0
}

// benchSectionKeysMirror mirrors bench_sections.go:41 — the CLOSED registry of
// collapsible Bench sections, in the page's contract order.
var benchSectionKeysMirror = []string{"ribbon", "rsvp", "nextup", "rows"}

// benchSectionLabels names each key for a reader who has never seen the page.
// `rsvp` is the one the 2026-08-11 operator met: its summary line reads
// "Session & availability", and the ONLY link to /schedule in the entire
// product sits inside it (bench.templ:715).
var benchSectionLabels = map[string]string{
	"ribbon": "the whole ribbon (session tile, next-up, sync pill, attention rows)",
	"rsvp":   `"Session & availability" — the RSVP panel, and the only link to /schedule in the product`,
	"nextup": "the NEXT UP cross-calendar index",
	"rows":   "the subordinate-calendar row grid",
}

// mirrorResolveBenchSections mirrors resolveBenchSections (bench_sections.go:68-81).
//
//	stored == nil          never chosen   → all four CLOSED  ([BR2-4] SIGNED)
//	stored == []string{}   closed nothing → all four OPEN
//	stored == [rsvp rows]  → those two closed, the other two open
//
// The nil case is the whole point: "the operator never touched it" and "the
// operator closed nothing" are different states that the page renders
// oppositely, and a diagnostic that printed only the resolved booleans would
// lose the distinction that explains the complaint.
func mirrorResolveBenchSections(stored []string, neverChosen bool) map[string]bool {
	closed := make(map[string]bool, len(benchSectionKeysMirror))
	if neverChosen {
		for _, k := range benchSectionKeysMirror {
			closed[k] = true
		}
		return closed
	}
	for _, k := range stored {
		for _, known := range benchSectionKeysMirror {
			if k == known {
				closed[k] = true
			}
		}
	}
	return closed
}

// ── calendar.render ─────────────────────────────────────────────────────────

// renderCalendarRender prints the Bench's render trace for one viewer.
// Arg: "<campaignId>[:<userId>]".
func renderCalendarRender(arg string) string {
	var b strings.Builder
	b.WriteString("## calendar.render\n\n")
	campaignID, userID := splitArgOpt2(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>[:<userId>]` — run `campaigns.list` for the id. Without a user id this traces the OWNER path, which is not what a player sees._\n")
		return b.String()
	}
	if campaignDiagProvider == nil {
		b.WriteString(providerNotWired)
		return b.String()
	}
	f, err := campaignDiagProvider.CalendarFacts(context.Background(), campaignID, userID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !f.Found {
		fmt.Fprintf(&b, "_No campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}

	fmt.Fprintf(&b, "campaign **%s** (`%s`) — the v4 Bench at `/campaigns/%s/apps/calendar`\n\n", f.CampaignName, f.CampaignID, f.CampaignID)
	writeMirrorWarning(&b)

	writeRenderViewer(&b, f)
	if !writeRenderGate(&b, f) {
		return b.String()
	}
	writeRenderSeats(&b, f)
	writeRenderSections(&b, f)
	writeRenderLayers(&b, f)
	writeNotes(&b, f.Notes)
	return b.String()
}

// providerNotWired is the ONE degraded string these diagnostics may print. It
// says "nobody is answering", never "the answer is empty".
const providerNotWired = "_Campaign provider not wired — the app layer did not inject it at startup, so NOTHING WAS READ. This is NOT an empty result: it is not \"this campaign has no calendars / no addons / no blocks\", it is \"nobody was asked\". Fix the wiring in RegisterRoutes before drawing any conclusion._\n"

// writeMirrorWarning states the mirror in the output, every time. It is the
// second of the two things that keep the mirror honest (the first is the
// source-pin test); a reader who trusts this trace absolutely, and is wrong,
// is a worse outcome than one who checks.
func writeMirrorWarning(b *strings.Builder) {
	b.WriteString("> **This trace RE-DERIVES the producer's rules; it does not call it.** `benchClassify`, `resolveBenchSections`, the `[SKY-1]` sky seat and the Almanac gate are unexported inside the calendar plugin and this package may not import it. Each mirrored rule below names the source line it copies, and a CI test reads that source and fails if the line moves. If a claim here contradicts the page, trust the page and report the mirror.\n\n")
}

// writeRenderViewer prints WHO the trace is for. Role is printed before any
// gate, because three of the Bench's branches are role floors and a trace that
// buried the role would make its own conclusions unreadable.
func writeRenderViewer(b *strings.Builder, f CampaignCalendarFacts) {
	b.WriteString("### 1. The viewer\n\n")
	if !f.Viewer.Supplied {
		b.WriteString("- **No user id supplied**, so this traces the OWNER path (`ListCalendars`, every calendar visible, every owner-only control present). A player's page can differ in what it LISTS as well as in what it renders. Re-run as `<campaignId>:<userId>` to trace a real viewer.\n\n")
		return
	}
	fmt.Fprintf(b, "- user `%s` — ", f.Viewer.UserID)
	if !f.Viewer.Found {
		b.WriteString("**NOT A MEMBER of this campaign** (no `campaign_members` row). Anonymous and non-member visitors are RoleNone, which is below every role floor on the page.\n")
		if f.Viewer.Note != "" {
			fmt.Fprintf(b, "- %s\n", f.Viewer.Note)
		}
		b.WriteString("\n")
		return
	}
	fmt.Fprintf(b, "membership role **%d** (%s)\n", f.Viewer.MemberRole, roleName(f.Viewer.MemberRole))
	fmt.Fprintf(b, "- DM grant: **%t** → visibility role **%d** (%s). `cc.VisibilityRole()` returns RoleOwner for a DM-grantee; `RequireRole` compares the MEMBERSHIP role. The page uses both, for different gates.\n",
		f.Viewer.DmGranted, f.Viewer.Role, roleName(f.Viewer.Role))
	fmt.Fprintf(b, "- calendar list read via **%s**\n", fallback(f.ListVia, "unknown"))
	if f.ListErr != "" {
		fmt.Fprintf(b, "- **list FAILED**: %s — the Bench renders its LoadError card and nothing else.\n", f.ListErr)
	}
	if f.Viewer.Note != "" {
		fmt.Fprintf(b, "- %s\n", f.Viewer.Note)
	}
	b.WriteString("\n")
}

// writeRenderGate prints the two conditions under which NOTHING below applies,
// and reports whether the trace should continue.
func writeRenderGate(b *strings.Builder, f CampaignCalendarFacts) bool {
	b.WriteString("### 2. Can this page render at all?\n\n")
	ok := true
	switch {
	case f.AddonEnabled == nil:
		fmt.Fprintf(b, "- calendar addon: **UNKNOWN** — %s\n", fallback(f.AddonNote, "the addons service could not be read"))
	case !*f.AddonEnabled:
		b.WriteString("- calendar addon: **DISABLED for this campaign**. Every calendar route rides `addons.RequireAddon(addonSvc, \"calendar\")`, so the whole feature — Bench, schedule, settings, the V2 shell — is unreachable. Nothing below this line is what the operator is looking at. Enable it in the campaign's addon settings first.\n\n")
		ok = false
	default:
		b.WriteString("- calendar addon: **enabled**\n")
	}
	if !ok {
		return false
	}
	if !f.SpineInstalled {
		b.WriteString("- Block spine: **NOT INSTALLED** (`calendar.BlockSpine()` is nil — the plugin is degraded). Every calendar falls back to a subordinate ROW: no Block, no sky, no moons, on any seat. That is the whole explanation for an empty-looking page.\n")
	} else {
		b.WriteString("- Block spine: installed\n")
	}
	visible, caveat := viewerCalendars(f)
	fmt.Fprintf(b, "- calendars: **%d** in the campaign, **%d** on this viewer's own list\n", len(f.All), len(visible))
	if caveat != "" {
		fmt.Fprintf(b, "- ⚠ %s\n", caveat)
	}
	if len(visible) == 0 {
		b.WriteString("\n_This viewer sees NO calendars, so the Bench renders its empty state. If the campaign has calendars, they are hidden by per-calendar visibility (`calendars.visibility` / `visibility_rules`) — run `calendar.config` to see them all._\n")
		return false
	}
	b.WriteString("\n")
	return true
}

// viewerCalendars returns the list the classification must run over, plus a
// caveat when that list is not really the viewer's.
//
// The ambiguity it removes is worth naming: a nil Visible can mean "no viewer
// was supplied" or "the viewer's own list could not be read". Those look
// identical in the struct and must NOT look identical in the output — silently
// classifying the full campaign list for a player would report seats they
// cannot see, which is a confident wrong answer about the exact thing they
// asked about.
func viewerCalendars(f CampaignCalendarFacts) (cals []DiagCalendar, caveat string) {
	if !f.Viewer.Supplied {
		return f.All, ""
	}
	if f.Visible != nil {
		return f.Visible, ""
	}
	return f.All, "**this viewer's own calendar list was not resolved**, so the seats below are classified over the FULL campaign list and may include calendars they cannot see. Treat the classification as an upper bound."
}

// writeRenderSeats prints the classification and what each seat renders.
func writeRenderSeats(b *strings.Builder, f CampaignCalendarFacts) {
	visible, _ := viewerCalendars(f)
	b.WriteString("### 3. Which calendar took which seat, and what each seat renders\n\n")
	if f.ActiveNote != "" {
		fmt.Fprintf(b, "_active-calendar read: %s_\n\n", f.ActiveNote)
	}
	fmt.Fprintf(b, "viewer's active calendar: %s\n\n", codeOr(f.ActiveID, "none stored"))

	for _, s := range mirrorBenchClassify(visible, f.ActiveID) {
		fmt.Fprintf(b, "#### %s — **%s** (`%s`)\n", s.Seat, s.Cal.Name, s.Cal.ID)
		fmt.Fprintf(b, "- selected by: %s\n", s.Clause)
		fmt.Fprintf(b, "- mode `%s`%s%s\n", s.Cal.Mode,
			boolSuffix(s.Cal.IsDefault, " · campaign default"),
			boolSuffix(s.Cal.TracksRealTime, " · tracks_real_time"))

		if s.Seat == seatRow {
			b.WriteString("- renders as a ROW: a one-line entry with a date label. **No Block, so no sky, no moon discs and no Almanac** — this is not a fault, it is what a subordinate calendar is on this page.\n\n")
			continue
		}
		if !f.SpineInstalled {
			b.WriteString("- the spine is not installed, so this seat FALLS BACK TO A ROW.\n\n")
			continue
		}

		skyOn, shelfHidden := mirrorSeatRender(s.Seat)
		fmt.Fprintf(b, "- SkyOn: **%t**, ShelfHidden: **%t** — `[SKY-1]` SIGNED seats the sky on the PRIMARY Block only.\n", skyOn, shelfHidden)
		if s.Seat == seatRealWorld {
			b.WriteString("  - So the real-world Block on this page has **no sky band at all**, by design. Adding a moon to it changes nothing visible here. That is the answer to \"I don't see the real moon on the real calendar\" whenever a campaign has BOTH an in-world and a real-world calendar.\n")
		}
		writeSeatMoons(b, s.Cal, skyOn, shelfHidden)
		b.WriteString("\n")
	}
}

// writeSeatMoons prints the moon facts for one seated Block: what the calendar
// stores, what the Block draws, what the grid's cap allows, and whether the
// Almanac register — the only surface that carries the fourth body — was built.
func writeSeatMoons(b *strings.Builder, c DiagCalendar, skyOn, shelfHidden bool) {
	if c.StoredCountNote != "" {
		fmt.Fprintf(b, "- stored moon rows: **unknown** — %s\n", c.StoredCountNote)
	} else {
		fmt.Fprintf(b, "- stored `calendar_moons` rows: **%d**\n", c.MoonRowsStored)
	}
	fmt.Fprintf(b, "- moons the Block receives: **%d**%s\n", c.MoonsRendered, moonNameSuffix(c.MoonNames))
	if c.SynthesizedMoon {
		b.WriteString("  - one of these has **no database row**: a real-life calendar with no authored moons is given THE Moon, phase computed from the real synodic cycle. Adding any moon row REPLACES it — that is deliberate, the operator's declaration wins whole.\n")
	}
	if c.MoonsRendered > benchMoonCapMirror {
		fmt.Fprintf(b, "  - the month grid draws at most **%d** discs per day; %d declared bodies means %d is drawn in the grid nowhere and appears only in the Almanac.\n",
			benchMoonCapMirror, c.MoonsRendered, c.MoonsRendered-benchMoonCapMirror)
	}
	built := mirrorAlmanacBuilt(skyOn, shelfHidden, c.MoonsRendered)
	fmt.Fprintf(b, "- Almanac register built: **%t** — gate is `(!ShelfHidden || SkyOn) && moons > 0` = `(!%t || %t) && %d > 0`\n", built, shelfHidden, skyOn, c.MoonsRendered)
	if !built && c.MoonsRendered == 0 {
		b.WriteString("  - no moons, so nothing to register. `calendar.config` prints every calendar's moon count.\n")
	}
	if !built && c.MoonsRendered > 0 {
		b.WriteString("  - the calendar HAS moons and the register was still not built, because neither reader asked for it on this seat.\n")
	}
}

// benchMoonCapMirror mirrors benchMoonCap (bench.go:60) — the Bench passes 3,
// matching the renderer's own ceiling and the governing render's three discs.
const benchMoonCapMirror = 3

// writeRenderSections prints the four disclosures and, for each, WHY.
func writeRenderSections(b *strings.Builder, f CampaignCalendarFacts) {
	b.WriteString("### 4. The four collapsible sections — open or closed, and why\n\n")
	if f.SectionsNote != "" {
		fmt.Fprintf(b, "_%s_\n\n", f.SectionsNote)
	}
	switch {
	case !f.Viewer.Supplied:
		b.WriteString("_No user id supplied. The disclosure state is per-(user, campaign) and cannot be traced without one._\n\n")
		return
	case f.SectionsNeverChosen:
		b.WriteString("**Provenance: NO STORED ROW — this viewer has never opened or closed anything.** `resolveBenchSections(nil)` marks all four CLOSED, at every width ([BR2-4] SIGNED, Option A). A server-rendered `open` attribute cannot vary by viewport, so one default had to be chosen and closed is it.\n\n")
	default:
		fmt.Fprintf(b, "**Provenance: a stored row** — closed set = %s. An EMPTY stored list is a real choice (\"closed nothing\") and renders all four OPEN; it is not the same as no row.\n\n", codeList(f.SectionsStored))
	}
	closed := mirrorResolveBenchSections(f.SectionsStored, f.SectionsNeverChosen)
	for _, k := range benchSectionKeysMirror {
		state := "OPEN"
		if closed[k] {
			state = "CLOSED"
		}
		fmt.Fprintf(b, "- `%s` — **%s** · %s\n", k, state, benchSectionLabels[k])
	}
	if closed["rsvp"] {
		b.WriteString("\n**The `rsvp` section is closed, and that is worth saying plainly.** The RSVP panel IS on the page and its controls DO meet the phone tap floor; it is behind a chevron whose summary says \"Session & availability\", and the only link to `/schedule` in the entire product is inside it. A viewer who has never opened it will report that RSVP is not built into the calendar — and they are also right for a second, separate reason: the calendar widget carries zero RSVP markup by design, and `/schedule` mounts no calendar. Opening the chevron does not merge them.\n")
	}
	b.WriteString("\n")
}

// writeRenderLayers prints the per-viewer Block layer set, which has the same
// nil-vs-empty discipline and the same ability to explain a "missing" feature.
func writeRenderLayers(b *strings.Builder, f CampaignCalendarFacts) {
	b.WriteString("### 5. The viewer's Block layer set\n\n")
	if f.LayersNote != "" {
		fmt.Fprintf(b, "_%s_\n\n", f.LayersNote)
	}
	if !f.Viewer.Supplied {
		b.WriteString("_No user id supplied; the layer set is per-(user, campaign)._\n\n")
		return
	}
	if f.LayersNeverChosen {
		b.WriteString("- **no stored row** — the host's seed renders. The Bench seeds five layers (era bands, week gutter, moons, docked Ledger, Shelf).\n\n")
		return
	}
	fmt.Fprintf(b, "- stored: %s — this OVERRIDES the host seed. A viewer who turned `moons` off has no discs and no Almanac, and nothing on the page says so.\n\n", codeList(f.LayersStored))
}

// ── calendar.config ─────────────────────────────────────────────────────────

// renderCalendarConfig prints per-calendar contents. Arg: "<campaignId>[:<calId>]".
func renderCalendarConfig(arg string) string {
	var b strings.Builder
	b.WriteString("## calendar.config\n\n")
	campaignID, calID := splitArgOpt2(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>[:<calId>]` — run `campaigns.list` for the id._\n")
		return b.String()
	}
	if campaignDiagProvider == nil {
		b.WriteString(providerNotWired)
		return b.String()
	}
	f, err := campaignDiagProvider.CalendarFacts(context.Background(), campaignID, "")
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !f.Found {
		fmt.Fprintf(&b, "_No campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "campaign **%s** (`%s`)\n\n", f.CampaignName, f.CampaignID)
	if f.AddonEnabled != nil && !*f.AddonEnabled {
		b.WriteString("> **The `calendar` addon is DISABLED for this campaign**, so none of the data below is reachable from any page. Run `campaign.config` for the addon list.\n\n")
	}
	if f.ListErr != "" {
		fmt.Fprintf(&b, "- **calendar list FAILED**: %s\n", f.ListErr)
		return b.String()
	}
	if len(f.All) == 0 {
		b.WriteString("_This campaign has NO calendars._ The Bench renders its empty state and offers the builder wizard at `/calendars/new`.\n")
		return b.String()
	}

	shown := 0
	for _, c := range f.All {
		if calID != "" && c.ID != calID {
			continue
		}
		shown++
		writeCalendarConfigRow(&b, c)
	}
	if shown == 0 {
		fmt.Fprintf(&b, "_No calendar `%s` in this campaign. Re-run without the `:calId` to list them all._\n", calID)
		return b.String()
	}

	writeCalendarResolution(&b, f)
	writeNotes(&b, f.Notes)
	return b.String()
}

// writeCalendarConfigRow prints one calendar's identity and sub-resource counts.
func writeCalendarConfigRow(b *strings.Builder, c DiagCalendar) {
	fmt.Fprintf(b, "### %s — `%s`\n", c.Name, c.ID)
	fmt.Fprintf(b, "- mode **%s** · is_default **%t** · tracks_real_time **%t**%s · visibility `%s`\n",
		c.Mode, c.IsDefault, c.TracksRealTime, tzSuffix(c.RealTimeZone), fallback(c.Visibility, "everyone"))
	fmt.Fprintf(b, "- months **%d** · weekdays **%d** · seasons **%d** · eras **%d** · cycles **%d** · festivals **%d**\n",
		c.Months, c.Weekdays, c.Seasons, c.Eras, c.Cycles, c.Festivals)

	if c.StoredCountNote != "" {
		fmt.Fprintf(b, "- **moons: stored count UNKNOWN** — %s\n", c.StoredCountNote)
	} else {
		fmt.Fprintf(b, "- **moons: %d stored row(s)", c.MoonRowsStored)
		if c.MoonsRendered != c.MoonRowsStored {
			fmt.Fprintf(b, ", %d rendered", c.MoonsRendered)
		}
		fmt.Fprintf(b, "**%s\n", moonNameSuffix(c.MoonNames))
	}
	switch {
	case c.SynthesizedMoon:
		b.WriteString("  - the rendered moon has **no database row**: a real-life calendar with no authored moons is given THE Moon, its phase computed from the real synodic cycle. There is nothing to edit in Settings → Moons, and adding a row REPLACES it.\n")
	case c.MoonRowsStored == 0 && !isRealLifeMode(c.Mode):
		b.WriteString("  - an in-world calendar declares its own sky and is seeded with none: `seedDefaults` seeds months, weekdays and event categories and deliberately never calls `SetMoons`. Add moons at Settings → Moons (Owner only).\n")
	case c.MoonRowsStored == 0:
		b.WriteString("  - no rows AND no synthesized body — check the months count above: the real-Moon fallback needs a month list to place its anchor date.\n")
	}
	if c.Months == 0 {
		b.WriteString("- ⚠ **zero months** — this calendar cannot resolve a date at all, and every surface that prints one will show a fault instead.\n")
	}
	b.WriteString("\n")
}

// writeCalendarResolution answers "which calendar does each surface pick?",
// which is the half of the question that a per-calendar listing cannot.
func writeCalendarResolution(b *strings.Builder, f CampaignCalendarFacts) {
	b.WriteString("### Which calendar each surface resolves to\n\n")
	if f.DefaultNote != "" {
		fmt.Fprintf(b, "- campaign default: **unknown** — %s\n", f.DefaultNote)
	} else {
		fmt.Fprintf(b, "- **campaign default** (`GetCalendar`) → %s — this is what the Foundry sync API and every default-calendar surface are served.\n", codeOr(f.DefaultID, "none"))
	}
	if seats := mirrorBenchClassify(f.All, ""); len(seats) > 0 {
		fmt.Fprintf(b, "- **the v4 Bench's PRIMARY** → `%s` (%s) — selected by: %s\n", seats[0].Cal.ID, seats[0].Cal.Name, seats[0].Clause)
		for _, s := range seats {
			if s.Seat == seatRealWorld {
				fmt.Fprintf(b, "- **the Bench's real-world Block** → `%s` (%s) — renders with NO sky ([SKY-1]).\n", s.Cal.ID, s.Cal.Name)
			}
		}
		b.WriteString("  - computed with NO viewer, so the \"viewer's active calendar\" clause could not fire. Run `calendar.render <campaignId>:<userId>` for a real viewer's classification.\n")
	}
	b.WriteString("- **the V2 shell** and the sidebar's per-user switcher resolve the viewer's own `calendar_active` row, then the campaign default. That is per-viewer; `calendar.render` prints it.\n\n")
}

// ── campaign.surfaces ───────────────────────────────────────────────────────

// surfaceRow is one declared user-facing calendar route.
//
// WHY A DECLARED MAP AT ALL, when the route table is live. The table knows
// paths and handler names; it does not know which of them is the CURRENT door
// and which is a preserved legacy one, and that is the entire question. So the
// map is authored — and then CHECKED against the live table, in both
// directions: a declared route that is not registered is printed as missing,
// and a registered calendar route that is not declared is printed as
// unclassified. Neither direction is silent.
type surfaceRow struct {
	Path    string
	Handler string // the Echo handler name this path should resolve to
	Surface string
	Status  string
	Note    string
}

const (
	statusCurrent  = "CURRENT"
	statusLegacy   = "LEGACY-PRESERVED"
	statusRedirect = "LEGACY-REDIRECT"
)

// calendarSurfaceMap is the declared map, in the order a reader should meet it:
// the doors that are live first, then the ones that only forward, then the
// preserved legacy pages.
func calendarSurfaceMap() []surfaceRow {
	return []surfaceRow{
		{"/campaigns/:id/apps/calendar", "AppDashboard", "v4 Bench", statusCurrent,
			"THE calendar page. The sidebar's Calendar item points here and every legacy door redirects here."},
		{"/campaigns/:id/schedule", "SchedulePage", "Schedule (availability matrix)", statusCurrent,
			"Player+. Mounts NO calendar — the availability half of \"session & availability\" lives here, adjacent to the calendar and never integrated with it."},
		{"/campaigns/:id/calendars/new", "ShowBuilder", "Builder wizard", statusCurrent,
			"Owner only. Re-pointed at the wizard by [WZ-13]; the old three-card chooser is retired."},
		{"/campaigns/:id/calendars/builder", "ShowBuilder", "Builder wizard", statusCurrent, "Owner only."},
		{"/campaigns/:id/calendars/:calId/settings", "ShowSettings", "Calendar settings editor", statusCurrent,
			"Owner only. The Moons tab sits OUTSIDE the `!IsRealLife()` guard, so it works on a real-world calendar too."},

		{"/campaigns/:id/calendar", "legacyRedirect", "→ v4 Bench", statusRedirect,
			"301. The oldest bookmark in the product; it has pointed at V1, then V2, now the Bench."},
		{"/campaigns/:id/calendars", "Index", "→ v4 Bench (or the 0-calendar setup branch)", statusRedirect, ""},
		{"/campaigns/:id/calendars/:calId", "RedirectShowV2", "→ v4 Bench", statusRedirect, ""},
		{"/campaigns/:id/calendars/:calId/week", "RedirectWeekV2", "→ v4 Bench", statusRedirect,
			"The week segment has no v4 destination yet, so it lands on the month Bench."},
		{"/campaigns/:id/calendars/:calId/day", "RedirectDayV2", "→ v4 Bench", statusRedirect,
			"Same as week: no v4 day surface yet."},

		{"/campaigns/:id/calendars/:calId/embed", "EmbedCalendar", "V1 embed page", statusLegacy, "PRESERVED: no v4 embed exists."},
		{"/campaigns/:id/calendars/embed", "EmbedCalendar", "V1 embed page (default calendar)", statusLegacy, ""},
		{"/campaigns/:id/calendars/:calId/timeline", "ShowTimeline", "V1 timeline", statusLegacy, "PRESERVED: the V2 timeline is a deferred arc."},
	}
}

// handlerSurfaces classifies the routes this file DISCOVERS rather than
// declares: the surface is keyed on the handler Echo reports, and the path is
// taken from the running router.
//
// WHY DISCOVERY RATHER THAN A DECLARED ROW, for the frozen shell in particular.
// `[VS-2]` SIGNED sunset the old shell as a CLICKABLE destination — it stays
// reachable by URL and by nothing else — and `TestSunset_NoLiveDoorRemains`
// enforces that by walking `internal/` for the shell's path prefix and failing
// on any hit that is not a signed exemption. Writing those paths into this map
// would have added four, and the honest options were to amend a signed
// exemption list or to change the construction.
//
// Changing the construction is better on its own terms. A declared path is a
// claim this repo makes about a page; a discovered one is a fact read off the
// router. So these rows print the LIVE path with an authored explanation
// attached to the handler, and if the shell is ever genuinely removed,
// `campaign.surfaces` stops mentioning it the moment the route goes — with no
// edit here, and no stale row claiming a page that no longer exists.
func handlerSurfaces() map[string]surfaceRow {
	return map[string]surfaceRow{
		"ShowV2": {Surface: "the frozen V2 calendar shell", Status: statusLegacy,
			Note: "NO LIVE LINK REACHES THIS ([VS-2] SIGNED — sunset as a destination, preserved as a URL). It is reachable only by an old bookmark or a hand-authored sidebar link, which is why the sidebar section below is worth reading."},
		"ShowV2SubresourceSettings": {Surface: "the V2 sub-resource settings namespace", Status: statusLegacy,
			Note: "Shares the shell's prefix but is not a view, and SURVIVES the shell ([VS-7] SIGNED)."},
	}
}

// renderCampaignSurfaces prints the live route table against the declared map.
func renderCampaignSurfaces(arg string) string {
	var b strings.Builder
	b.WriteString("## campaign.surfaces\n\n")
	campaignID := strings.TrimSpace(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>` (run `campaigns.list` first)._\n")
		return b.String()
	}
	if campaignDiagProvider == nil {
		b.WriteString(providerNotWired)
		return b.String()
	}
	f, err := campaignDiagProvider.SurfaceFacts(context.Background(), campaignID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !f.Found {
		fmt.Fprintf(&b, "_No campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "campaign **%s** (`%s`)\n\n", f.CampaignName, f.CampaignID)

	writeSurfaceGate(&b, f)
	live := writeSurfaceTable(&b, f)
	writeSurfaceUnclassified(&b, f, live)
	writeSurfaceSidebar(&b, f)
	writeNotes(&b, f.Notes)
	return b.String()
}

// writeSurfaceGate prints the addon gate that can remove every row below.
func writeSurfaceGate(b *strings.Builder, f CampaignSurfaceFacts) {
	switch {
	case f.CalendarAddonEnabled == nil:
		fmt.Fprintf(b, "> calendar addon: **UNKNOWN** — %s\n\n", fallback(f.AddonNote, "the addons service could not be read"))
	case !*f.CalendarAddonEnabled:
		b.WriteString("> **The `calendar` addon is DISABLED for this campaign.** Every route below is registered in the binary and rides `addons.RequireAddon(addonSvc, \"calendar\")`, so all of them answer as if the feature did not exist. A registered route is not a reachable one.\n\n")
	default:
		b.WriteString("> calendar addon: **enabled** — the routes below are reachable (subject to their own role floors).\n\n")
	}
}

// writeSurfaceTable prints the declared map checked against the live table, and
// returns the set of live paths it consumed.
func writeSurfaceTable(b *strings.Builder, f CampaignSurfaceFacts) map[string]bool {
	b.WriteString("### Calendar routes — declared surface vs the LIVE route table\n\n")
	if f.RoutesNote != "" {
		fmt.Fprintf(b, "_%s_\n\n", f.RoutesNote)
	}
	// The live table, indexed by path for GET only: these are the doors a
	// person can type into a phone.
	liveByPath := map[string]RouteFact{}
	for _, r := range f.Routes {
		if r.Method == "GET" {
			liveByPath[r.Path] = r
		}
	}
	consumed := map[string]bool{}

	// One line per route, with a second line ONLY when something is wrong or
	// when the row carries a note. A route inventory that spends four lines on
	// every healthy row buries the one unhealthy row it exists to surface.
	for _, row := range calendarSurfaceMap() {
		live, ok := liveByPath[row.Path]
		consumed[row.Path] = true
		fmt.Fprintf(b, "- **%s** `%s` → %s", row.Status, row.Path, row.Surface)
		switch {
		case len(f.Routes) == 0:
			b.WriteString(" · registration **NOT CHECKED** (the route table could not be read — see the note above)\n")
		case !ok:
			b.WriteString("\n  - ⚠ **NOT REGISTERED IN THIS BUILD.** The map expects this route and the binary does not have it. Either the route moved or this map is stale — do not conclude the surface exists.\n")
		case row.Handler != "" && !handlerMatches(live.Handler, row.Handler):
			fmt.Fprintf(b, "\n  - ⚠ registered, but the LIVE handler is `%s` where this map expected `%s`. **Trust the live handler.**\n", live.Handler, row.Handler)
		default:
			fmt.Fprintf(b, " · `%s`\n", shortHandler(live.Handler))
		}
		if row.Note != "" {
			fmt.Fprintf(b, "  - %s\n", row.Note)
		}
	}
	b.WriteString("\n")
	return consumed
}

// handlerMatches compares a declared handler name to Echo's live one. Echo
// records the runtime function name, and a METHOD VALUE (which every plugin
// handler is) carries a `-fm` suffix — so both spellings must match, or every
// healthy row would report a disagreement.
func handlerMatches(live, want string) bool {
	return strings.HasSuffix(live, "."+want) || strings.HasSuffix(live, "."+want+"-fm")
}

// shortHandler trims Echo's fully-qualified runtime name to the readable tail.
// The full name is printed only on a DISAGREEMENT, where the package matters.
func shortHandler(live string) string {
	s := strings.TrimSuffix(live, "-fm")
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// writeSurfaceUnclassified prints campaign-scoped calendar GETs the map does
// not know about. A new page that nobody classified is exactly the thing this
// diagnostic must not hide.
func writeSurfaceUnclassified(b *strings.Builder, f CampaignSurfaceFacts, consumed map[string]bool) {
	byHandler := handlerSurfaces()
	var known, extra []string
	for _, r := range f.Routes {
		if r.Method != "GET" || consumed[r.Path] {
			continue
		}
		if !strings.Contains(r.Path, "/calendar") && !strings.HasSuffix(r.Path, "/schedule") {
			continue
		}
		if row, ok := byHandler[shortHandler(r.Handler)]; ok {
			line := fmt.Sprintf("- **%s** `%s` → %s · `%s`", row.Status, r.Path, row.Surface, shortHandler(r.Handler))
			if row.Note != "" {
				line += "\n  - " + row.Note
			}
			known = append(known, line)
			continue
		}
		extra = append(extra, fmt.Sprintf("- `%s` → `%s`", r.Path, r.Handler))
	}

	if len(known) > 0 {
		sort.Strings(known)
		b.WriteString("### Calendar routes DISCOVERED from the live table\n\n")
		b.WriteString("These are classified by the handler the router reports rather than by a path written here, so the path below is read off the running server. A surface that has genuinely been removed disappears from this section on its own.\n\n")
		b.WriteString(strings.Join(known, "\n") + "\n\n")
	}
	if len(extra) == 0 {
		return
	}
	sort.Strings(extra)
	b.WriteString("### Registered calendar GETs nothing classifies\n\n")
	b.WriteString("Each of these is a live door whose surface nobody has declared. Most are JSON/fragment endpoints rather than pages; a new PAGE appearing here means the map above needs an entry.\n\n")
	b.WriteString(strings.Join(extra, "\n") + "\n\n")
}

// writeSurfaceSidebar prints where the nav actually points.
func writeSurfaceSidebar(b *strings.Builder, f CampaignSurfaceFacts) {
	b.WriteString("### Where the sidebar's Calendar item links\n\n")
	if f.SidebarCalendarPath == "" {
		b.WriteString("- **unknown** — the addon URL map could not be read.\n")
	} else {
		fmt.Fprintf(b, "- `/campaigns/%s%s` — read from the same map the sidebar renders from. There is no phone-specific navigation: the hamburger opens the same `<aside>`.\n", f.CampaignID, f.SidebarCalendarPath)
	}
	if f.SidebarNote != "" {
		fmt.Fprintf(b, "- _%s_\n", f.SidebarNote)
	}
	var links []SidebarItemFact
	for _, it := range f.SidebarItems {
		if it.Type == "link" {
			links = append(links, it)
		}
	}
	if len(links) == 0 {
		b.WriteString("- no hand-authored `type:\"link\"` items in `campaigns.sidebar_config`.\n\n")
		return
	}
	b.WriteString("- hand-authored sidebar links (these can point at ANY url, including a retired surface):\n")
	for _, it := range links {
		fmt.Fprintf(b, "  - %s → `%s` (visible: %t)\n", fallback(it.Label, "(unlabelled)"), fallback(it.URL, "(no url)"), it.Visible)
	}
	b.WriteString("\n")
}

// ── campaign.config ─────────────────────────────────────────────────────────

// renderCampaignConfig prints enabled addons and every placed block type.
func renderCampaignConfig(arg string) string {
	var b strings.Builder
	b.WriteString("## campaign.config\n\n")
	campaignID := strings.TrimSpace(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>` (run `campaigns.list` first)._\n")
		return b.String()
	}
	if campaignDiagProvider == nil {
		b.WriteString(providerNotWired)
		return b.String()
	}
	f, err := campaignDiagProvider.ConfigFacts(context.Background(), campaignID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !f.Found {
		fmt.Fprintf(&b, "_No campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "campaign **%s** (`%s`)\n\n", f.CampaignName, f.CampaignID)

	writeConfigAddons(&b, f)
	writeConfigLayouts(&b, f)
	writeNotes(&b, f.Notes)
	return b.String()
}

// writeConfigAddons prints `campaign_addons`. A disabled addon is a feature
// that has VANISHED rather than broken, and nothing else in the catalog reads
// this table.
func writeConfigAddons(b *strings.Builder, f CampaignConfigFacts) {
	b.WriteString("### Addons\n\n")
	// A read failure and a genuinely empty table both arrive as len(Addons)==0.
	// They must not print the same sentence: "every addon-gated feature is off"
	// is a finding, and asserting it over a read nobody completed is the exact
	// class of confident wrong answer this file exists to remove.
	if f.AddonsNote != "" {
		fmt.Fprintf(b, "_%s_\n\nNothing is known about this campaign's addons — this is NOT \"no addons are enabled\".\n\n", f.AddonsNote)
		return
	}
	if len(f.Addons) == 0 {
		b.WriteString("_No addon rows for this campaign._ Every addon-gated feature is therefore off; that is a real state, not a read failure (a read failure prints its reason instead).\n\n")
		return
	}
	for _, a := range f.Addons {
		mark := "✗ disabled"
		if a.Enabled {
			mark = "✓ enabled"
		}
		fmt.Fprintf(b, "- %s `%s` — %s", mark, a.Slug, fallback(a.Name, "(unnamed)"))
		if a.Status != "" {
			fmt.Fprintf(b, " · status `%s`", a.Status)
		}
		if !a.Installed {
			b.WriteString(" · **backing code NOT present in this build**")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nA disabled `calendar` addon removes every calendar route at the middleware, so the whole feature reads as absent rather than broken. `campaign.surfaces` lists the routes it gates.\n\n")
}

const (
	// blockTypeCalendar is a LAYOUT BLOCK TYPE, not a reference to the calendar
	// plugin. The two spell the same word, which is why this is a named const
	// rather than a literal: tools/check-plugin-isolation.sh (T-B2) forbids a
	// plugin slug spelled outside the owning plugin, and internal/systems may
	// not import a plugin to borrow calendar.PluginSlug the way the app-layer
	// adapter does.
	//
	// The block type arrives here as DATA — the app layer reads it out of a
	// stored layout — so nothing is being coupled: this map only decides which
	// of the types already on the page get an explanatory note. Declared under
	// the guard's const-registry amendment (R4-S26-A), which exempts a bare
	// const-assignment line and nothing else in the file, so a future
	// `"calendar"` written anywhere else here still fails.
	blockTypeCalendar = "calendar"
)

// writeConfigLayouts prints the block types placed on every layout surface.
func writeConfigLayouts(b *strings.Builder, f CampaignConfigFacts) {
	b.WriteString("### Placed blocks\n\n")
	if f.LayoutsNote != "" {
		fmt.Fprintf(b, "_%s_\n\n", f.LayoutsNote)
	}
	if len(f.DefaultDashboardBlocks) > 0 {
		fmt.Fprintf(b, "For comparison, `DefaultDashboardLayout()` is %s — **no sky, worldstate or calendar block is in any default, and no migration seeds one.** Anything of that kind below was placed by hand.\n\n", codeList(f.DefaultDashboardBlocks))
	}
	if len(f.Layouts) == 0 {
		b.WriteString("_No layout surfaces read._\n\n")
		return
	}

	interesting := map[string]string{
		// The class name of the v4 sky band is deliberately NOT written here.
		// It is a signed carve-out that lives in exactly two files, guarded by
		// a repository-wide walk, and a diagnostic naming it would be a third
		// consumer of a signature that names ONE band on ONE Block. The
		// distinction the reader needs is between the two THINGS, and it can
		// be drawn without the selector.
		// CALV5-PLACEHOLDER: the four calendar-family descriptions below are
		// rebuild-era truth. The world-state pipeline (cal-almanac.js) was
		// deleted with the v4 calendar; every one of these placements renders
		// the "being rebuilt" notice today. The bindings themselves are kept
		// on purpose (Sweep skips unknown types), so a placed block is a fact
		// worth reporting — what it renders is not what it used to.
		"skybox":            "the LEGACY skybox widget placement. Its canvas/particle engine and the world-state pipeline behind it were deleted in the CALV5 clean slate, so this placement renders the calendar-rebuilding notice today; the binding is kept so V5 can reclaim the seat. (Historically this was the surface that rendered the synthesized real Moon — distinct from the v4 sky band on the Bench, which was server-rendered with no JavaScript.)",
		"entity_worldstate": "the world-state band placement — its pipeline was deleted in the CALV5 clean slate; renders the rebuilding notice until V5.",
		"entity_calendar":   "a calendar Block embedded on an entity page — renders the rebuilding notice until V5.",
		"calendar_full":     "a full calendar block — renders the rebuilding notice until V5.",
		blockTypeCalendar:   "a calendar block — renders the rebuilding notice until V5.",
	}

	for _, l := range f.Layouts {
		fmt.Fprintf(b, "#### %s\n", l.Surface)
		switch {
		case l.ParseErr != "":
			fmt.Fprintf(b, "- **could not be parsed**: %s — the page falls back to its hardcoded default.\n\n", l.ParseErr)
			continue
		case !l.Stored:
			b.WriteString("- not customised (column is NULL) — the hardcoded default renders.\n\n")
			continue
		case len(l.Blocks) == 0:
			b.WriteString("- stored but contains **no blocks**.\n\n")
			continue
		}
		for _, t := range countedInOrder(l.Blocks) {
			fmt.Fprintf(b, "- `%s`", t.name)
			if t.count > 1 {
				fmt.Fprintf(b, " ×%d", t.count)
			}
			if note, ok := interesting[t.name]; ok {
				fmt.Fprintf(b, " — **%s**", note)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
}

// ── small shared helpers ────────────────────────────────────────────────────

// splitArgOpt2 splits "<a>[:<b>]" — b is "" when the colon is absent. Unlike
// splitArg2 this does NOT require the second part, because three of the four
// diagnostics here have an optional tail.
func splitArgOpt2(arg string) (a, bb string) {
	parts := strings.SplitN(strings.TrimSpace(arg), ":", 2)
	a = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		bb = strings.TrimSpace(parts[1])
	}
	return a, bb
}

// roleName maps a campaign role number to its name. Numbers alone are unusable
// in a pasted result: "role 2" tells the reader nothing about which floors it
// clears.
func roleName(role int) string {
	switch {
	case role >= 3:
		return "Owner"
	case role == 2:
		return "Scribe"
	case role == 1:
		return "Player"
	case role == 0:
		return "None — not a member"
	default:
		return "unknown"
	}
}

// writeNotes prints whatever the provider could not read. Never omitted when
// non-empty: a partial answer that does not say it is partial is the failure
// this whole catalog exists to prevent.
func writeNotes(b *strings.Builder, notes []string) {
	if len(notes) == 0 {
		return
	}
	b.WriteString("### Reads that did not succeed\n\n")
	for _, n := range notes {
		fmt.Fprintf(b, "- %s\n", n)
	}
	b.WriteString("\nNothing above is evidence about the parts these cover.\n")
}

func codeOr(s, empty string) string {
	if strings.TrimSpace(s) == "" {
		return "_" + empty + "_"
	}
	return "`" + s + "`"
}

func codeList(xs []string) string {
	if len(xs) == 0 {
		return "_(empty)_"
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, "`"+x+"`")
	}
	return strings.Join(out, ", ")
}

func boolSuffix(v bool, s string) string {
	if v {
		return s
	}
	return ""
}

func tzSuffix(tz string) string {
	if strings.TrimSpace(tz) == "" {
		return ""
	}
	return " (" + tz + ")"
}

func moonNameSuffix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return " — " + codeList(names)
}

// countedInOrder collapses duplicates while preserving first-appearance order,
// so a layout with two skybox blocks reports "×2" rather than two lines or one
// silently deduplicated one.
type namedCount struct {
	name  string
	count int
}

func countedInOrder(xs []string) []namedCount {
	var out []namedCount
	idx := map[string]int{}
	for _, x := range xs {
		if i, ok := idx[x]; ok {
			out[i].count++
			continue
		}
		idx[x] = len(out)
		out = append(out, namedCount{name: x, count: 1})
	}
	return out
}
