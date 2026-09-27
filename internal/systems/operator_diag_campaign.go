// Package systems — operator_diag_campaign.go is the CAMPAIGN half of the
// operator diagnostic catalog: "why does MY campaign look like this?" — as
// opposed to the host.* family, which answers WHICH CODE IS RUNNING.
//
// Two diagnostics live here, both reading campaign CONFIG: `campaign.surfaces`
// (which live route serves a URL, and where the sidebar points) and
// `campaign.config` (enabled addons and every block a campaign has placed).
//
// DEGRADE LOUDLY. An unwired provider prints "provider not wired"; a read
// that failed prints the error. A plausible-looking empty answer is the one
// thing neither of these may ever produce.
package systems

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ── the injected window ─────────────────────────────────────────────────────

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
// import the campaigns / addons / entities plugins), wired once at startup by
// SetCampaignDiagProvider, exactly as SetInstalledPackagesProvider does.
type CampaignDiagProvider interface {
	SurfaceFacts(ctx context.Context, campaignID string) (CampaignSurfaceFacts, error)
	ConfigFacts(ctx context.Context, campaignID string) (CampaignConfigFacts, error)
}

var campaignDiagProvider CampaignDiagProvider

// SetCampaignDiagProvider wires the per-campaign read window for the
// campaign.* diagnostics.
func SetCampaignDiagProvider(p CampaignDiagProvider) { campaignDiagProvider = p }

// ── catalog entries ─────────────────────────────────────────────────────────

// campaignSurfacesDiagnostic exposes the live route table: Templ pages are
// compiled into the binary, so they never appear in host.assets or
// host.embedded.
func campaignSurfacesDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "campaign.surfaces",
		Title:   "Which calendar page does a URL actually render? (live route table)",
		Desc:    "Every user-facing calendar route this campaign exposes, read from the LIVE Echo table with its real handler, flagged CURRENT / LEGACY / REDIRECT, plus where the sidebar's Calendar item links and any hand-authored sidebar link. THE answer to 'which calendar am I looking at?' and to 'the deploy landed but I still see the old thing'.",
		ArgHint: "<campaignId>",
		Run:     renderCampaignSurfaces,
	}
}

// campaignConfigDiagnostic shows enabled addons and the block types a
// campaign has placed, to establish whether a block was hand-placed vs seeded
// by a default layout or migration.
func campaignConfigDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "campaign.config",
		Title:   "Enabled addons + the blocks this campaign has PLACED",
		Desc:    "Which addons are enabled, and the block TYPES placed in dashboard_layout / owner_dashboard_layout and on each entity template. THE check for 'the skybox is still there': a `skybox` block is in no default layout and no migration seeds it, so it can only be operator-placed.",
		ArgHint: "<campaignId>",
		Run:     renderCampaignConfig,
	}
}

// providerNotWired is the ONE degraded string these diagnostics may print. It
// says "nobody is answering", never "the answer is empty".
const providerNotWired = "_Campaign provider not wired — the app layer did not inject it at startup, so NOTHING WAS READ. This is NOT an empty result: it is not \"this campaign has no addons / no blocks\", it is \"nobody was asked\". Fix the wiring in RegisterRoutes before drawing any conclusion._\n"

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

// statusCurrent is the only status calendarSurfaceMap uses today — its
// LEGACY-PRESERVED / LEGACY-REDIRECT siblings went with the routes they
// described. CALV5-PLACEHOLDER: re-add them if V5's calendar routes need a
// legacy or redirect row again. Still accurate as of calendar-v5 seams
// (#778): that pass restored the plugin's service, JSON API and a
// dashboard-embed fragment, not a calendar PAGE, so the three rows below are
// unchanged. TODO(#778)
const statusCurrent = "CURRENT"

// calendarSurfaceMap is the declared map.
//
// The v4 Bench, builder wizard, settings editor, V1 legacy pages and every
// redirect between them were deleted with the pre-V5 calendar plugin; these
// three GETs are what is left. /calendars is the calendars list page, and
// /apps/calendar and /calendar redirect (302) to it. They stay redirects,
// not linked directly from the sidebar or dashboard, because the
// plugin-isolation guard won't let a literal "calendars" path live in
// app.templ's addonURLMap (see internal/app/routes.go).
//
// Handler is left "" on all three: for /apps/calendar and /calendar the
// live handler is one anonymous closure registered in
// internal/app/routes.go, not a stable named method, so pinning its runtime
// name here would be pinning a Go compiler detail; /calendars' handler
// (calendar.Handler.Index) IS stable, but is left unpinned too so all three
// rows are checked the same way. An empty Handler skips the disagreement
// check and only confirms the path is registered — see writeSurfaceTable.
func calendarSurfaceMap() []surfaceRow {
	return []surfaceRow{
		{"/campaigns/:id/apps/calendar", "", "calendar (redirect)", statusCurrent,
			"Redirects (302) to `/campaigns/:id/calendars` — not a notice, not the v4 Bench. The sidebar's Calendar item points here."},
		{"/campaigns/:id/calendar", "", "calendar (redirect)", statusCurrent,
			"Same redirect as `/apps/calendar` — the oldest bookmark in the product."},
		{"/campaigns/:id/calendars", "", "calendar (list page)", statusCurrent,
			"The calendars list page — not a notice, not a redirect; each calendar opens from here to its own page at `/campaigns/:id/calendars/:calid/view`. Unlike the two rows above, this one IS gated on the calendar addon (RequireAddon): a disabled addon 404s here instead of redirecting."},
	}
}

// handlerSurfaces classifies routes this file DISCOVERS by handler rather
// than declares by path (see writeSurfaceUnclassified). Empty: the one
// route this used to cover, the frozen V2 calendar shell, was deleted
// outright with the pre-V5 plugin — not merely unreachable, gone — so there
// is no live handler left to discover. CALV5-PLACEHOLDER: if V5 preserves an
// old page under a stable handler name the way the V2 shell once was, its
// entry belongs here rather than in calendarSurfaceMap. TODO(#778)
func handlerSurfaces() map[string]surfaceRow {
	return map[string]surfaceRow{}
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

// writeSurfaceGate prints the calendar addon's enabled state.
//
// Before the rebuild this state gated every calendar route. Now
// /apps/calendar and /calendar ride RequireCampaignAccess only and redirect
// regardless of it, while /calendars and the calendar JSON API sit in
// calendar.RegisterRoutes' group, which gates on addons.RequireAddon: a
// disabled addon 404s there. The dashboard and category "Upcoming Events"
// cards check the same addon state (and the calendar plugin's health) before
// calling that API, so a disabled addon shows their quiet "Calendar isn't
// enabled" state rather than a request that never swaps in.
func writeSurfaceGate(b *strings.Builder, f CampaignSurfaceFacts) {
	switch {
	case f.CalendarAddonEnabled == nil:
		fmt.Fprintf(b, "> calendar addon: **UNKNOWN** — %s\n\n", fallback(f.AddonNote, "the addons service could not be read"))
	case !*f.CalendarAddonEnabled:
		b.WriteString("> calendar addon: **disabled** for this campaign. `/apps/calendar` and `/calendar` redirect either way; `/calendars` itself 404s while disabled, as does the calendar JSON API.\n\n")
	default:
		b.WriteString("> calendar addon: **enabled**. `/calendars` and the calendar JSON API are reachable; `/apps/calendar` and `/calendar` would redirect there regardless of this setting.\n\n")
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
	b.WriteString("\nA disabled `calendar` addon gates the calendar pages and the calendar plugin's JSON API (404), while `/apps/calendar` and `/calendar` still redirect to the list page either way; see `campaign.surfaces`. The dashboard/category \"Upcoming Events\" cards also check it: they the dashboard/category \"Upcoming Events\" cards check the same addon-enabled state (and the plugin's own health) before calling that API, and degrade to a quiet \"Calendar isn't enabled\" state instead of a stuck spinner — see `calendar.stats` for that plugin's own counts and migration state.\n\n")
}

const (
	// blockTypeCalendar is a LAYOUT BLOCK TYPE, not a reference to the calendar
	// plugin — named as a const rather than a literal because
	// tools/check-plugin-isolation.sh (T-B2) forbids a plugin slug spelled
	// outside the owning plugin, and internal/systems may not import a plugin
	// to borrow calendar.PluginSlug. The value arrives here as DATA read out
	// of a stored layout; this const only decides which placed block gets an
	// explanatory note.
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
		// CALV5-PLACEHOLDER: "skybox", "entity_worldstate" and
		// "entity_calendar" describe rebuild-era behavior for OTHER
		// placements (the skybox widget-binding pipeline and the
		// entity_calendar/entity_worldstate widget-type bindings) that are
		// out of scope for this pass — see #778's own priority list. Left
		// as-is; do not assume they are still accurate without checking
		// their own restoration status first. TODO(#778)
		"skybox":            "the sky pane placement (issue #763) — renders the real sky (moons/weather/events for the resolved default calendar). (Historically this was the surface that rendered the synthesized real Moon — distinct from the v4 sky band on the Bench, which was server-rendered with no JavaScript.)",
		"entity_worldstate": "the world-state band placement — its pipeline was deleted in the CALV5 clean slate; renders the rebuilding notice until it is rebuilt (#778).",
		"entity_calendar":   "a calendar Block embedded on an entity page — renders the rebuilding notice until it is rebuilt (#778).",
		// calendar_full/calendar_preview now render the real "Upcoming
		// Events" card (calendar-v5 seams, #778) via the calendar plugin's
		// own /calendars/upcoming embed fragment, not the rebuilding notice.
		"calendar_full":    "a full calendar block — now renders real upcoming-events data via the calendar plugin's embed fragment (calendar-v5 seams, #778), not the rebuilding notice.",
		"calendar_preview": "an upcoming-events preview block — renders real data via the calendar plugin's embed fragment (calendar-v5 seams, #778), not the rebuilding notice.",
		blockTypeCalendar:  "a calendar block — renders the rebuilding notice until it is rebuilt (#778).",
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
