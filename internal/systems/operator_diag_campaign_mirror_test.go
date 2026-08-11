package systems

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// operator_diag_campaign_mirror_test.go is the guard on the one real risk in
// operator_diag_campaign.go: `calendar.render` RE-DERIVES four rules that live,
// unexported, inside internal/plugins/calendar, because this package may not
// import a plugin.
//
// A mirror is only defensible if going stale is LOUD. Without this test the
// failure mode is silent and specific: the producer's rule changes, the trace
// keeps printing the old answer, and it prints it with the same confidence —
// to somebody who is using it to decide what their page did. That is strictly
// worse than having no trace, which is why the mirror is guarded rather than
// merely commented.
//
// The guard is a SOURCE READ, not a behavioural test, and that bound is worth
// stating: it proves the copied line still exists, not that the mirror
// reproduces it correctly. Reproduction is covered by the table tests in
// operator_diag_campaign_test.go. Together they cover "the rule did not move"
// and "we copied it right"; neither covers the other.

// calendarSourceDir is where the mirrored rules live, relative to this package.
const calendarSourceDir = "../plugins/calendar"

// mirroredRule is one line of producer source the diagnostic copies.
type mirroredRule struct {
	File string
	// Want is the source text, whitespace-normalised. It must be distinctive
	// enough that an unrelated edit cannot satisfy it by accident.
	Want string
	// Why says what breaks in the diagnostic when this line moves — the message
	// a failing run needs in order to be actionable rather than merely red.
	Why string
}

// mirroredRules is the complete pin set. Every entry corresponds to a named
// mirror function or constant in operator_diag_campaign.go.
func mirroredRules() []mirroredRule {
	return []mirroredRule{
		// --- benchClassify → mirrorBenchClassify -------------------------------
		{"bench.go", `inWorld := func(c *Calendar) bool { return !c.IsRealLife() }`,
			"mirrorBenchClassify's in-world predicate"},
		{"bench.go", `primary = pick(func(c *Calendar) bool { return c.IsDefault && inWorld(c) })`,
			"mirrorBenchClassify clause 1 — campaign default AND in-world"},
		{"bench.go", `primary = pick(func(c *Calendar) bool { return c.ID == activeID && inWorld(c) })`,
			"mirrorBenchClassify clause 2 — the viewer's active calendar"},
		{"bench.go", `primary = pick(inWorld)`,
			"mirrorBenchClassify clause 3 — the first in-world calendar"},
		{"bench.go", `primary = pick(func(c *Calendar) bool { return c.IsDefault })`,
			"mirrorBenchClassify clause 4 — the real-world-only campaign, where the real-world calendar IS promoted to Primary and DOES get a sky"},
		{"bench.go", `realWorld = pick(func(c *Calendar) bool { return c.IsRealLife() && c.ID != primary.ID })`,
			"mirrorBenchClassify's real-world seat"},

		// --- the [SKY-1] seat arguments → mirrorSeatRender ---------------------
		//
		// Argument ORDER is load-bearing here: benchBlock's signature is
		// (…, noShelf bool, sky bool, …), so `false, true` is the Primary's
		// "shelf shown, sky on" and `true, false` is the real-world Block's
		// "shelf hidden, no sky". Pinning the whole call rather than the two
		// booleans is what makes a signature change fail this test too.
		{"bench.go", `if b := h.benchBlock(ctx, spine, primary, viewer, activeID, false, true, layerPrefs, in.View); b != nil {`,
			"mirrorSeatRender(PRIMARY) — SkyOn=true, ShelfHidden=false ([SKY-1])"},
		{"bench.go", `if b := h.benchBlock(ctx, spine, realWorld, viewer, activeID, true, false, layerPrefs, BlockDate{}); b != nil {`,
			"mirrorSeatRender(REAL-WORLD) — SkyOn=false, ShelfHidden=true ([SKY-1]). If this ever becomes `false, true`, [SKY-1] has been AMENDED and the trace's central claim about the real-world Block is wrong"},

		// --- the Almanac gate → mirrorAlmanacBuilt -----------------------------
		{"block_geometry.go", `if (!in.ShelfHidden || !in.SkyHidden) && len(cal.Moons) > 0 {`,
			"mirrorAlmanacBuilt's gate"},
		{"block_projection.go", `SkyHidden: !in.SkyOn,`,
			"mirrorAlmanacBuilt reads SkyOn where the producer reads SkyHidden; this is the line that makes them the same fact"},

		// --- resolveBenchSections → mirrorResolveBenchSections -----------------
		{"bench_sections.go", `var benchSectionKeys = []string{"ribbon", "rsvp", "nextup", "rows"}`,
			"benchSectionKeysMirror — the closed registry of collapsible sections"},
		{"bench_sections.go", `if stored == nil {`,
			"mirrorResolveBenchSections' nil branch — [BR2-4]'s closed-by-default, which is the provenance the trace reports"},

		// --- benchMoonCap → benchMoonCapMirror ---------------------------------
		{"bench.go", `const benchMoonCap = 3`,
			"benchMoonCapMirror — the grid's three-disc ceiling, which is what makes the Almanac's fourth body worth mentioning"},
	}
}

// collapseWS reduces every whitespace run to one space so the pins survive
// gofmt's alignment decisions (which is what `SkyHidden:   !in.SkyOn,` is) but
// not an actual edit to the code.
var wsRun = regexp.MustCompile(`[ \t]+`)

func collapseWS(s string) string { return wsRun.ReplaceAllString(s, " ") }

// TestMirroredProducerRulesStillMatchTheirSource fails when a rule the render
// trace copies has changed in the calendar plugin.
func TestMirroredProducerRulesStillMatchTheirSource(t *testing.T) {
	cache := map[string]string{}
	read := func(name string) string {
		if s, ok := cache[name]; ok {
			return s
		}
		b, err := os.ReadFile(filepath.Join(calendarSourceDir, name))
		if err != nil {
			t.Fatalf("could not read %s: %v\n"+
				"The render trace mirrors rules from this file. If the calendar plugin moved, this guard must move with it — do NOT delete it, because the mirror it protects is still shipping.", name, err)
		}
		cache[name] = collapseWS(string(b))
		return cache[name]
	}

	rules := mirroredRules()
	if len(rules) == 0 {
		t.Fatal("no mirrored rules pinned — this test would pass vacuously")
	}
	for _, r := range rules {
		if !strings.Contains(read(r.File), collapseWS(r.Want)) {
			t.Errorf("MIRROR IS STALE.\n  source: internal/plugins/calendar/%s\n  expected line: %s\n  it protects: %s\n\n"+
				"calendar.render re-derives this rule in internal/systems/operator_diag_campaign.go because it is unexported and systems may not import the plugin. "+
				"The line has changed, so the trace is now printing a rule the product no longer follows. Re-read the producer, update the mirror, then update this pin.",
				r.File, r.Want, r.Why)
		}
	}
}

// getRouteRe pulls the literal path out of a `pub.GET("…"` / `cg.GET("…"` /
// `e.GET("…"` registration. Non-literal paths are invisible to it, which is the
// same limitation internal/wire's own route oracle documents — and the calendar
// plugin registers /schedule as a literal specifically so that oracle can see it.
var getRouteRe = regexp.MustCompile(`(?m)\b(?:pub|cg|e)\.GET\(\s*"([^"]+)"`)

// TestDeclaredSurfacePathsAreRegisteredInTheCalendarPlugin checks the authored
// half of campaign.surfaces against the plugin's own route file.
//
// The diagnostic ALSO checks itself against the live Echo table at run time,
// which is stronger — but only where a server is running. This is the CI half:
// a map entry naming a route the plugin does not register is a claim that
// something exists when it does not, and that is the one error this family of
// diagnostics is built to stop making.
func TestDeclaredSurfacePathsAreRegisteredInTheCalendarPlugin(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(calendarSourceDir, "routes.go"))
	if err != nil {
		t.Fatalf("could not read the calendar plugin's routes.go: %v", err)
	}
	registered := map[string]bool{}
	for _, m := range getRouteRe.FindAllStringSubmatch(string(src), -1) {
		registered[m[1]] = true
	}
	if len(registered) == 0 {
		t.Fatal("found no GET registrations in the calendar plugin's routes.go — this test would pass vacuously")
	}

	// Every declared surface sits under the campaign group, so the registered
	// literal is the path with that prefix removed.
	const groupPrefix = "/campaigns/:id"
	for _, row := range calendarSurfaceMap() {
		tail := strings.TrimPrefix(row.Path, groupPrefix)
		if tail == row.Path {
			t.Errorf("%s: every declared surface must sit under %q", row.Path, groupPrefix)
			continue
		}
		if !registered[tail] {
			t.Errorf("campaign.surfaces declares %q but the calendar plugin registers no GET %q.\n"+
				"The map would tell an operator this page exists. Either the route moved (update the map) or it was retired (remove the row).",
				row.Path, tail)
		}
	}
}
