// bench_gmconsole_test.go — C-CALV4-GM-CONSOLE.
//
// THE FINDING THIS SLICE CLOSES. The GM world-state console — thirteen control
// families that drive weather, world-events, mood and the clock — had exactly
// one mount in the product, `calendar_v2.templ:152`, on the page
// `C-CALV4-V2SUNSET` [VS-1] (SIGNED) has committed to deleting. The backend
// (`PUT /calendar/world-state`) survives that sunset untouched; the DOOR did
// not. This is the rehousing.
//
// WHAT THIS FILE PINS, IN THE ORDER THAT MATTERS:
//
//  1. EVERY CONTROL RENDERS, ENUMERATED. A rehousing that quietly drops a verb
//     is the whole risk of a rehousing, and the enumeration is written as a
//     table of (handle, what it writes) so a reader can check the list against
//     the endpoint rather than against a promise.
//  2. THE PERMISSION GATE IS ABSENCE, AT THREE FLOORS. Capability / Owner /
//     dm_only, each asserted on a viewer who holds the others.
//  3. THE MOUNT CARRIES ITS OWN ENDPOINT. This is the trap that would have made
//     the whole surface silently dead: the driver used to read its endpoint off
//     a `[data-cal-v2-root]` that does not exist here, and would have returned.
//  4. THE DRIVER IS ACTUALLY MOUNTED, through the plugin body-script registry
//     and not a page `<script src>` — which htmx DELETES out of every boosted
//     swap, giving a surface that works on a direct load and dies through the
//     sidebar, in production only.
//  5. THE ONE CONTROL THAT DID NOT MOVE (Pause) IS ABSENT ON PURPOSE, asserted
//     so that its absence reads as a decision rather than as an oversight.
package calendar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// benchConsoleFx renders a Bench for one audience row, feeding the console the
// same benchInput production feeds it.
//
// The three inputs are the three independent facts the console's three gates
// read, passed as themselves rather than derived from one "role" number:
// CanControlWorldState is `cc.CanControlWorldState()` (Owner OR DM-grantee),
// IsOwner is `cc.MemberRole >= RoleOwner`, and CanAuthorDmOnly is the
// orthogonal co-DM capability. Collapsing any two makes the co-DM row
// untestable, and the co-DM row is where every floor in this file differs.
func benchConsoleFx(t *testing.T, isOwner, canControl, canDmOnly bool) string {
	t.Helper()
	data := benchFxData(true, isOwner)
	cals := benchFxAll()
	primary, _, _ := benchClassify(cals, "cal-harptos")
	if primary == nil {
		t.Fatal("fixture lost its primary calendar")
	}
	data.GMConsole = benchGMConsole(primary, benchInput{
		Campaign:             &campaigns.Campaign{ID: "camp-1", Name: "Imix"},
		IsOwner:              isOwner,
		CanControlWorldState: canControl,
		CanAuthorDmOnly:      canDmOnly,
		CSRFToken:            "fx-csrf",
	})
	return renderBench(t, data)
}

// --- 1. every control renders ------------------------------------------------

// benchConsoleControls is THE INVENTORY: every control the V2 console offers,
// with the wire it writes on. It is a table rather than a list of strings so
// the reader can audit "does this handle still reach that payload" without
// leaving the file.
//
// PAUSE IS NOT IN IT, and its absence is asserted separately and argued below.
var benchConsoleControls = []struct {
	handle string
	writes string
}{
	{"data-gm-advance", "PUT {advance:{days,hours,minutes}} — five verbs share the handle"},
	{"data-gm-set-time", "PUT {time:{hour,minute}}"},
	{"data-gm-set-date", "PUT {time:{year,month,day}}"},
	{"data-gm-weather-tile", "PUT {weather:\"<id>\"} on the current date"},
	{"data-gm-event-tile", "PUT {triggerEvent:{type,name,start_hour,duration_hours,dm_only}}"},
	{"data-gm-event-dmonly", "flips triggerEvent.dm_only; re-checked server-side"},
	{"data-gm-active-events", "the container the driver builds the per-event × clears into"},
	{"data-gm-events-clear", "PUT {clearEvents:true} — the stuck-meteor off switch"},
	{"data-gm-mood", "PUT {moodTint:{color,intensity}} — eight presets"},
	{"data-gm-mood-custom", "PUT {moodTint:{color,intensity}} from the colour picker"},
	{"data-gm-mood-intensity-slider", "re-commits the active wash at a new strength"},
	{"data-gm-mood-clear", "PUT {moodTint:{color:null,intensity:0}}"},
	{"data-gm-reset", "PUT {weather:'clear',clearEvents:true,moodTint:{color:null,intensity:0}}"},
	{"data-gm-panel-open", "chrome: opens the pull-out"},
	{"data-gm-panel-toggle", "chrome: folds it back into the tab"},
	{"data-gm-sheet-open", "chrome: the four section buttons"},
	{"data-gm-sheet-close", "chrome: per-sheet ✕"},
	{"data-gm-clock", "chrome: the in-world clock readout"},
	{"data-gm-weather-search", "chrome: the weather filter"},
	{"data-gm-event-search", "chrome: the world-event filter"},
}

func TestBenchGMConsole_EveryControlIsRehoused(t *testing.T) {
	html := benchConsoleFx(t, true, true, true)
	for _, c := range benchConsoleControls {
		if !strings.Contains(html, c.handle) {
			t.Errorf("%s is missing from the Bench console — it writes: %s. "+
				"A rehousing that drops a control is the failure this test exists for",
				c.handle, c.writes)
		}
	}
}

// The two catalogs arrive WHOLE. A console that offered eleven of the
// forty-nine weather conditions would pass every handle assertion above.
func TestBenchGMConsole_BothCatalogsAreComplete(t *testing.T) {
	html := benchConsoleFx(t, true, true, true)
	if got, want := strings.Count(html, "data-gm-weather-tile"), len(gmWeatherTypes()); got != want {
		t.Errorf("weather tiles = %d, want %d (the whole gmWeatherTypes catalog)", got, want)
	}
	if got, want := strings.Count(html, "data-gm-event-tile"), len(gmCelestialTypes()); got != want {
		t.Errorf("world-event tiles = %d, want %d (the whole gmCelestialTypes catalog)", got, want)
	}
	// The ids, not just the count: a catalog that shipped forty-nine copies of
	// "clear" would satisfy the counts.
	for _, w := range []string{"clear", "thunderstorm", "blood-rain", "fireflies"} {
		if !strings.Contains(html, fmt.Sprintf(`data-gm-id="%s"`, w)) {
			t.Errorf("weather id %q is not on a tile", w)
		}
	}
	for _, e := range []string{"eclipse-solar", "meteor-shower", "blood-moon", "plague"} {
		if !strings.Contains(html, fmt.Sprintf(`data-gm-id="%s"`, e)) {
			t.Errorf("world-event id %q is not on a tile", e)
		}
	}
}

// ONE CONSOLE PER PAGE. The Bench stacks up to TWO Blocks, and a console per
// Block would be two tabs, two listeners and an unanswerable question about
// which calendar the GM just changed the weather on.
func TestBenchGMConsole_ExactlyOnePerPage(t *testing.T) {
	html := benchConsoleFx(t, true, true, true)
	// `class="gm-console"` with its closing quote is the ROOT and only the root:
	// every other node in the console is `gm-console__something`, so a bare
	// Count on `data-gm-panel` would also collect -open, -toggle and -card.
	if got := strings.Count(html, `class="gm-console"`); got != 1 {
		t.Fatalf("the console root appears %d times, want exactly 1 — the Bench renders two "+
			"Blocks and the console is a page-level singleton", got)
	}
	if got := strings.Count(html, "data-gm-panel-card"); got != 1 {
		t.Fatalf("data-gm-panel-card appears %d times, want exactly 1", got)
	}
}

// --- 2. permission is absence, at three floors -------------------------------

func TestBenchGMConsole_AudienceMatrixDOM(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		isOwner, canControl, canDmOnly   bool
		wantConsole, wantSetters, wantDM bool
	}{
		// Owner: everything.
		{"owner", true, true, true, true, true, true},
		// Co-DM grantee: the capability WITHOUT Owner. Steps, triggers, sets
		// weather and mood — and does NOT get the absolute setters, because
		// this page's shipped floor for "set the world's date" is Owner-only
		// ([GR-SIGN-A](b)) and two controls a foot apart may not disagree.
		{"co-DM grantee", false, true, true, true, false, true},
		// A capability holder WITHOUT the dm_only grant: the console, no switch.
		{"capability without dm_only", false, true, false, true, false, false},
		// Below the capability: NOTHING, whatever else they hold. An Owner-flagged
		// fixture with the capability withheld is the strongest form of this row —
		// it proves the gate is the capability and not the role.
		{"scribe", false, false, false, false, false, false},
		{"player", false, false, false, false, false, false},
		{"owner without the capability (impossible in prod, decisive here)", true, false, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := benchConsoleFx(t, tc.isOwner, tc.canControl, tc.canDmOnly)
			if got := strings.Contains(html, "data-gm-panel"); got != tc.wantConsole {
				t.Errorf("console present = %v, want %v — permission is ABSENCE, so a "+
					"viewer below the floor gets no element at all", got, tc.wantConsole)
			}
			for _, setter := range []string{"data-gm-set-time", "data-gm-set-date"} {
				if got := strings.Contains(html, setter); got != tc.wantSetters {
					t.Errorf("%s present = %v, want %v", setter, got, tc.wantSetters)
				}
			}
			if got := strings.Contains(html, "data-gm-event-dmonly"); got != tc.wantDM {
				t.Errorf("dm_only switch present = %v, want %v — it is a SECOND, "+
					"distinct capability and PutWorldState re-checks it server-side",
					got, tc.wantDM)
			}
		})
	}
}

// A NON-HOLDER RECEIVES NO ROUTE STRING AND NO TOKEN, not merely no marker.
// A hidden control is a permission model made of CSS; a leaked endpoint plus a
// leaked CSRF token is worse than a visible button.
func TestBenchGMConsole_PlayerGetsNothingAtAll(t *testing.T) {
	html := renderBench(t, benchFxData(false, false))
	for _, forbidden := range []string{
		"data-gm-panel", "gm-console", "calendar/world-state",
		"data-gm-url", "data-gm-csrf", "fx-csrf",
		"data-gm-weather-tile", "data-gm-event-tile", "data-gm-mood",
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("a player's page carries %q — the console's gate is server-side "+
				"absence, not a CSS hide", forbidden)
		}
	}
}

// THE TWO SET-DATE CONTROLS ON THIS PAGE ANSWER THE SAME QUESTION.
//
// The V2 console gates Set date on CanControlWorldState; the Bench's own
// date-verb row gates it on the stricter Owner floor. Rehousing put both on ONE
// page, so a co-DM would otherwise see two Set-date controls a foot apart, one
// of which works. This asserts the resolution rather than trusting the prose:
// whatever the verb row decides about Set date, the console decides too.
func TestBenchGMConsole_SetDateFloorAgreesWithTheVerbRow(t *testing.T) {
	for _, tc := range []struct {
		name              string
		isOwner, canCtrl  bool
		wantSetOnBothOrNo bool
	}{
		{"owner", true, true, true},
		{"co-DM grantee", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBench(t, benchVerbFxData(t, true, tc.isOwner, tc.canCtrl))
			row := strings.Contains(html, "data-bench-date-set")
			console := strings.Contains(html, "data-gm-set-date")
			if row != console {
				t.Fatalf("the verb row offers Set date = %v and the console offers it = %v "+
					"— one page, one answer. See bench_gmconsole.go's floor ruling", row, console)
			}
			if row != tc.wantSetOnBothOrNo {
				t.Fatalf("Set date present = %v, want %v", row, tc.wantSetOnBothOrNo)
			}
			// The STEP verbs are the other half of the asymmetry and must NOT
			// follow the Owner floor: a co-DM steps.
			if !strings.Contains(html, "data-gm-advance") {
				t.Error("the console's step verbs are gone for a capability holder — " +
					"[GR-SIGN-A](b) is 'a co-DM steps and does not set', not 'neither'")
			}
		})
	}
}

// --- 3. the mount carries its own endpoint -----------------------------------

// THE TRAP, ASSERTED. gm_panel.js read campaignID / calendarID / CSRF off
// `[data-cal-v2-root]` and returned when it was absent — and that attribute
// exists on calendar_v2.templ and nowhere else. Mounted on the Bench unchanged,
// the console would have rendered, looked perfect, and done nothing.
func TestBenchGMConsole_MountCarriesItsOwnEndpointAndCalendar(t *testing.T) {
	html := benchConsoleFx(t, true, true, true)
	if strings.Contains(html, "data-cal-v2-root") {
		t.Error("the Bench carries [data-cal-v2-root] — that is the V2 shell's page root, " +
			"and depending on it here is the silent-death the mount attributes replace")
	}
	// The endpoint AND the calendar, pinned together: `?calendarId=` is not
	// decoration. The PUT's fallback resolves the viewer's ACTIVE calendar, and
	// the only UI that can change which calendar that is (POST /calendar/v2/
	// switch) dies with the shell — so a console that leaned on the fallback
	// would drive an unchangeable row for the rest of the campaign's life.
	want := `data-gm-url="/campaigns/camp-1/calendar/world-state?calendarId=cal-harptos"`
	if !strings.Contains(html, want) {
		t.Errorf("the console mount does not carry %s", want)
	}
	if !strings.Contains(html, `data-gm-csrf="fx-csrf"`) {
		t.Error("the console mount does not carry its CSRF token; every write would be refused")
	}
	// STANDALONE: the fact that there is no sky engine on this page, declared
	// rather than sniffed for. Without it the driver would hand the seed to an
	// absent engine and fall through to a full page reload per weather tap.
	if !strings.Contains(html, "data-gm-standalone") {
		t.Error("the console mount does not declare data-gm-standalone — the driver would " +
			"then expect cal-almanac.js, which [SKY-13] refused on this page")
	}
}

// The driver's side of the same contract, read from the shipped file. A test
// that only checked the markup would stay green if the driver stopped reading
// it.
func TestBenchGMConsole_DriverReadsTheMount(t *testing.T) {
	js := readRepoFile(t, "internal/plugins/calendar/static/js/gm_panel.js")
	for _, want := range []string{
		`panel.getAttribute('data-gm-url')`,
		`panel.getAttribute('data-gm-csrf')`,
		`panel.getAttribute('data-gm-standalone')`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("gm_panel.js does not read %s — the Bench console would render and "+
				"do nothing", want)
		}
	}
	// AND THE V2 FALLBACK SURVIVES. The shell is not deleted yet and its console
	// must keep working; the page root is now the fallback, not the requirement.
	if !strings.Contains(js, "[data-cal-v2-root]") {
		t.Error("gm_panel.js lost its [data-cal-v2-root] fallback — the V2 console goes " +
			"dead, and C-CALV4-V2SUNSET has not run yet")
	}
}

// --- 4. the driver is actually mounted ---------------------------------------

// THE HTMX SCRIPT TRAP, ASSERTED FROM BOTH ENDS. `boot.js` sets
// `htmx.config.allowScriptTags = false`, at which setting htmx does not merely
// decline to execute a <script> in a swapped fragment — it DELETES it. Every
// sidebar link is `hx-boost` + `hx-select="#main-content"`, and the App layout
// puts `{children...}` inside that element. So a `<script src>` in bench.templ
// wires on a direct load and silently does not through the sidebar: the
// stylesheets in the same region survive, so the page looks perfect and the
// console simply never responds.
func TestBenchGMConsole_DriverRidesTheBodyScriptRegistry(t *testing.T) {
	routes := readRepoFile(t, "internal/app/routes.go")
	if !strings.Contains(routes, `js/gm_panel.js"`) {
		t.Fatal("gm_panel.js is not in pluginBodyScripts — the Bench console has no driver " +
			"on any navigation path")
	}
	// The tag name is split so this assertion does not match itself, and the
	// COMMENT LINES ARE STRIPPED FIRST: both files explain this very trap in
	// prose that quotes the tag, and a scanner that read the explanation as the
	// defect would be a guard that can only be satisfied by deleting the
	// warning.
	tag := "<script" + " src"
	for name, rel := range map[string]string{
		"bench.templ":           "internal/plugins/calendar/bench.templ",
		"bench_gmconsole.templ": "internal/plugins/calendar/bench_gmconsole.templ",
	} {
		src := benchStripTemplComments(readRepoFile(t, rel))
		if strings.Contains(src, tag) {
			t.Errorf("%s carries a page-side script tag — it sits inside #main-content, "+
				"which every boosted navigation swaps, and htmx deletes script tags out of "+
				"a swapped fragment. tools/check-page-scripts.sh refuses it for that reason", name)
		}
	}
	// The console also ships NO stylesheet link of its own: its rules live in
	// calendar-bench.css, which bench.templ already links, so the motion
	// register's guard reads them (see the sheet's §GM CONSOLE header).
	consoleTempl := benchStripTemplComments(
		readRepoFile(t, "internal/plugins/calendar/bench_gmconsole.templ"))
	if strings.Contains(consoleTempl, `rel="stylesheet"`) {
		t.Error("bench_gmconsole.templ links a stylesheet — a second sheet would keep " +
			"TestBenchCSS_NoMotionAtAll green while defeating it, which is the laundering " +
			"[DC-6] refuses by name")
	}
}

// benchStripTemplComments blanks whole-line `//` comments. Both templ files
// scanned above document the htmx script trap by quoting the tag it is about,
// so the prose has to be removed before the markup can be judged.
func benchStripTemplComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "//") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// --- 5. the one control that did not move ------------------------------------

// PAUSE IS ABSENT ON PURPOSE, AND THIS IS WHERE THAT IS WRITTEN DOWN.
//
// It is the console's only CLIENT-ONLY verb: it calls
// window.__calTimeControl.setPaused / window.CalParticleEngine.setPaused, both
// defined by static/js/cal-almanac.js — the sky engine, which [SKY-13] refused
// on the Bench BY ARGUMENT ("the sky header is server markup, custom properties
// and a native <details> — no canvas, no rAF, NO JAVASCRIPT"). Rendered here it
// would press, flip its own aria-pressed, and freeze nothing. There is no
// atmosphere to pause on a surface with no atmosphere.
//
// BRINGING THE ENGINE TO THE BENCH WOULD REVERSE A SIGNED DECISION and is an
// operator signature, not a dev call. Until someone takes it, the honest render
// is no button.
func TestBenchGMConsole_PauseIsAbsentBecauseTheEngineIs(t *testing.T) {
	html := benchConsoleFx(t, true, true, true)
	for _, forbidden := range []string{"data-gm-pause", "data-gm-pause-label", "Pause atmosphere"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("%q is on the Bench console — Pause drives the sky engine, and "+
				"[SKY-13] refused the engine on this page. A control that presses and "+
				"does nothing is worse than an absent one", forbidden)
		}
	}
	// …and the engine really is absent, so the reason above is a measurement
	// rather than a claim.
	if strings.Contains(html, "cal-almanac") {
		t.Error("the Bench loads cal-almanac.js — [SKY-13] refused it, and if that has " +
			"been reversed then Pause should come back and this test should be re-pointed")
	}
}

// --- 6. the sheet defines what the markup names ------------------------------

// The #568 gap, reaching one surface further: every DOM assertion above stays
// green while the stylesheet drops what the markup depends on. Two of these
// class names exist in NO markup in this repo at all — the driver builds the
// active-event chips in JavaScript — so a rename would silently unstyle them
// with nothing to grep for.
func TestBenchGMConsoleCSS_DefinesWhatTheMarkupNames(t *testing.T) {
	// EXACT SELECTOR MEMBERSHIP, NOT strings.Contains. A substring test passes
	// on an ALIAS: delete `.cal-bench .gm-console__active-x { … }` and the
	// `:hover` rule below it still contains the string, so the base rule can be
	// removed with the guard staying green. Measured — that is exactly what
	// happened when this test was first written.
	have := map[string]bool{}
	for _, prelude := range cssSelectors(benchCommentRe.ReplaceAllString(benchCSS(t), " ")) {
		for _, part := range strings.Split(prelude, ",") {
			have[strings.TrimSpace(part)] = true
		}
	}
	for _, want := range []string{
		".cal-bench .gm-console", ".cal-bench .gm-console__pill",
		".cal-bench .gm-console__card", ".cal-bench .gm-console__strip",
		".cal-bench .gm-console__verb", ".cal-bench .gm-console__section",
		".cal-bench .gm-console__sheet", ".cal-bench .gm-console__sheet-body",
		".cal-bench .gm-console__catalog", ".cal-bench .gm-console__tile",
		".cal-bench .gm-console__input", ".cal-bench .gm-console__btn",
		".cal-bench .gm-console__swatch", ".cal-bench .gm-console__check",
		// Driver-built, therefore ungreppable from markup:
		".cal-bench .gm-console__active-chip", ".cal-bench .gm-console__active-x",
		// The two states the driver writes and the CSS must answer.
		`.cal-bench .gm-console[data-gm-collapsed="true"] .gm-console__card`,
		`.cal-bench .gm-console__tile[data-gm-filtered="true"]`,
		`.cal-bench .gm-console__tile[data-gm-active="true"]`,
	} {
		if !have[want] {
			t.Errorf("calendar-bench.css declares no rule whose selector IS %q — a "+
				"`:hover` or descendant variant is not the base rule", want)
		}
	}
}

// THE CONSOLE ADDS NO MOTION, and this says so about the console specifically
// rather than leaving it to the sheet-wide guard.
//
// TestBenchCSS_NoMotionAtAll already forbids a transition outside the single
// reduced-motion wrapper, so this cannot be the only thing standing — that is
// the point. It is here because the V2 console's sheet is FULL of transitions
// (slide, hover lift, the mutation fade), and the obvious next move for a hand
// porting more of gm_panel.css is to bring one along. The register's third
// carve-out is an operator signature; this fails first and names it.
func TestBenchGMConsoleCSS_AddsNoMotionToTheRegister(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	for _, rule := range benchCSSRules(code) {
		if !strings.Contains(rule[0], "gm-console") {
			continue
		}
		for _, bad := range []string{"transition", "animation", "translate:"} {
			if strings.Contains(rule[1], bad) {
				t.Errorf("`%s` declares %q — the GM console ships motion-free on the Bench, "+
					"and a third carve-out on the disclosure register is an OPERATOR "+
					"SIGNATURE rather than a tuning. See the sheet's §GM CONSOLE header",
					rule[0], bad)
			}
		}
	}
}
