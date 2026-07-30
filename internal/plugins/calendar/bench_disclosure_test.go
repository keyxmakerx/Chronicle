// bench_disclosure_test.go — the four Bench disclosures, their defaults, their
// summary lines and the two traps around them (C-CALV4-BENCH-R2 slice R2-1,
// [BR2-1] / [BR2-3] / [BR2-4] SIGNED).
//
// THE CLAIM UNDER TEST, in one sentence: a closed section is not a hidden one.
// Every assertion below is either "it collapses" or "and it still tells the
// truth while collapsed", because the second half is the only thing that makes
// the first half acceptable (the register's clause 5).
package calendar

import (
	"regexp"
	"strings"
	"testing"
)

// --- the primitive ----------------------------------------------------------

// [BR2-1] SIGNED: native <details>/<summary>. Not a div+button pair, not a
// hidden checkbox with :has(). The markers are asserted, not the copy.
func TestBenchDisclosure_FourNativeDetailsWithTheSignedMarkers(t *testing.T) {
	for _, role := range []struct {
		name string
		gm   bool
	}{{"gm", true}, {"player", false}} {
		html := renderBench(t, benchFxData(role.gm, role.gm))
		for _, key := range benchSectionKeys {
			if !strings.Contains(html, `data-bench-disc="`+key+`"`) {
				t.Errorf("%s: no <details data-bench-disc=%q> on the page", role.name, key)
			}
			if !strings.Contains(html, `data-disc-pick="`+key+`"`) {
				t.Errorf("%s: no <summary data-disc-pick=%q> — guard B3 wants the CONTROL to end in -pick", role.name, key)
			}
		}
		if n := strings.Count(html, "<details"); n != len(benchSectionKeys) {
			t.Errorf("%s: %d <details> elements, want exactly %d — a fifth disclosure is a product decision, not a refactor",
				role.name, n, len(benchSectionKeys))
		}
		if n := strings.Count(html, "<summary"); n != len(benchSectionKeys) {
			t.Errorf("%s: %d <summary> elements, want exactly %d", role.name, n, len(benchSectionKeys))
		}
	}
}

// [BR2-4] SIGNED Option A: CLOSED at every width, on a first visit. The render
// is width-independent, which is precisely the ADR-048 §13 bound this ruling
// obeys rather than fights — so ONE render proves it at every width.
func TestBenchDisclosure_FreshViewerMeetsFourClosedSections(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	for _, key := range benchSectionKeys {
		if benchDiscIsOpen(t, html, key) {
			t.Errorf("a fresh viewer (nil stored set) met %q OPEN; the ruled default is closed at every width", key)
		}
	}
}

// ” — "I closed nothing" — is a real state and it must render four OPEN
// sections, or the store's whole nil-vs-empty discipline buys nothing.
func TestBenchDisclosure_StoredEmptySetOpensAllFour(t *testing.T) {
	d := benchFxData(true, true)
	d.SectionsClosed = []string{}
	html := renderBench(t, d)
	for _, key := range benchSectionKeys {
		if !benchDiscIsOpen(t, html, key) {
			t.Errorf("%q rendered CLOSED for a viewer whose stored set is '' — that value means they opened everything on purpose", key)
		}
	}
}

func TestBenchDisclosure_StoredSetClosesExactlyThoseSections(t *testing.T) {
	d := benchFxData(true, true)
	d.SectionsClosed = []string{"rsvp", "rows"}
	html := renderBench(t, d)
	for key, wantOpen := range map[string]bool{
		"ribbon": true, "rsvp": false, "nextup": true, "rows": false,
	} {
		open := benchDiscIsOpen(t, html, key)
		if open != wantOpen {
			t.Errorf("section %q open = %v, want %v", key, open, wantOpen)
		}
	}
}

// --- the wire ---------------------------------------------------------------

// The flip rides on the <details> ITSELF, because `toggle` does not bubble.
// hx-vals is a STATIC JSON literal: hx-vals='js:…' is dead under
// htmx.config.allowEval = false (static/js/boot.js:168) and §12 forbids it.
func TestBenchDisclosure_PostsTheFlipFromTheDetailsElement(t *testing.T) {
	d := benchFxData(true, true)
	d.SectionsPersistURL = "/campaigns/camp-1/calendar/prefs"
	html := renderBench(t, d)
	for _, key := range benchSectionKeys {
		el := benchDisclosureElement(t, html, key)
		for _, want := range []string{
			`hx-post="/campaigns/camp-1/calendar/prefs"`,
			`hx-trigger="toggle"`,
			`hx-swap="none"`,
			`hx-vals="{&#34;section&#34;:&#34;` + key + `&#34;}"`,
		} {
			if !strings.Contains(el, want) {
				t.Errorf("section %q: the <details> open tag is missing %s\ngot: %s", key, want, el)
			}
		}
	}
	if strings.Contains(html, "hx-vals='js:") || strings.Contains(html, `hx-vals="js:`) {
		t.Error("hx-vals='js:…' is DEAD under allowEval=false and is forbidden by §12")
	}
}

// No persistence endpoint (anonymous viewer, or a Bench outside a campaign) →
// no hx-* at all, rather than a dead control. The disclosure still opens and
// closes natively; it simply forgets. That degradation is free and it is the
// whole argument for <details>.
func TestBenchDisclosure_NoEndpointEmitsNoDeadControl(t *testing.T) {
	d := benchFxData(true, true)
	d.SectionsPersistURL = ""
	html := renderBench(t, d)
	for _, key := range benchSectionKeys {
		el := benchDisclosureElement(t, html, key)
		if strings.Contains(el, "hx-post") {
			t.Errorf("section %q emitted hx-post with no endpoint to post to", key)
		}
	}
	// …and it is still a working disclosure.
	if n := strings.Count(html, "<summary"); n != 4 {
		t.Errorf("%d summaries, want 4 — the control must survive losing its store", n)
	}
}

// --- what never collapses ---------------------------------------------------

// [BR2-3] SIGNED, and this is the absolute half of it. The subject of the page
// may not be inside a disclosure: "5-6 blocks of data before you get to the
// calendar" is not answered by a calendar you have to open.
func TestBenchDisclosure_TheBlocksAndTheChromeNeverCollapse(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	for _, marker := range []string{
		`class="phead"`, `data-bench-stack`, `data-bench-caption`,
	} {
		i := strings.Index(html, marker)
		if i < 0 {
			t.Fatalf("bench DOM is missing %q", marker)
		}
		if benchInsideDisclosure(html, i) {
			t.Errorf("%s is inside a <details> — [BR2-3] SIGNED: the two Blocks, .phead, .sechead and .caption never collapse", marker)
		}
	}
	// The "The bench" .sechead labels the stack; a label that hides while its
	// subject stays is worse than no label.
	if i := strings.Index(html, "The bench"); i < 0 {
		t.Fatal(`the "The bench" sechead is gone`)
	} else if benchInsideDisclosure(html, i) {
		t.Error(`the "The bench" .sechead is inside a <details>`)
	}
}

// --- THE TRAP ---------------------------------------------------------------

// The owner sort control hx-GETs #cal-dash-grid and swaps it
// (app_dashboard.templ:112). If the <details> were rendered INSIDE that id, a
// sort would destroy and recreate the disclosure — the section would snap shut
// or open on every sort and the store would desynchronise from the DOM.
// The <details> must therefore WRAP #cal-dash-grid, with .sortrow inside the
// disclosure BODY.
func TestBenchDisclosure_RowsWrapsTheSwapTargetFromOutside(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))

	disc := strings.Index(html, `data-bench-disc="rows"`)
	grid := strings.Index(html, `id="cal-dash-grid"`)
	if disc < 0 || grid < 0 {
		t.Fatalf("missing markers: disc=%d grid=%d", disc, grid)
	}
	if disc > grid {
		t.Fatal("the rows <details> opens AFTER #cal-dash-grid — it is a descendant, and a sort swap would destroy it")
	}
	// Ancestry, not merely ordering: no </details> may close between the two.
	if strings.Contains(html[disc:grid], "</details>") {
		t.Error("a </details> closes between the rows disclosure and #cal-dash-grid — the swap target is NOT inside the disclosure")
	}
	// And the sort control is inside the disclosure body, so it travels with the
	// swap exactly as it did before this slice.
	if sort := strings.Index(html, `data-cal-dashboard-sort`); sort < 0 {
		t.Fatal("the owner sort control vanished")
	} else if sort < grid {
		t.Error("the sort control moved OUTSIDE #cal-dash-grid; it must stay in the swapped fragment")
	}
}

// The sort control's own swap fragment is a WAVE-0 surface and this slice does
// not touch it. Proof: what the fragment renders is still exactly
// #cal-dash-grid and carries no disclosure of its own, so a swap replaces the
// disclosure's CONTENTS and never the disclosure.
func TestBenchDisclosure_TheSwapFragmentCarriesNoDisclosure(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	grid := benchGridSection(t, html)
	if strings.Contains(grid, "<details") {
		t.Error("the swapped fragment contains a <details>; a sort would then replace the disclosure with itself and reset its state")
	}
}

// --- guard B4 ---------------------------------------------------------------

// A <details> does not remove its content from the DOM, but the guard is about
// PROOF rather than about belief: the nextup rows carry data-row AND data-day,
// and they still do with the section CLOSED.
func TestBenchDisclosure_ClosedNextUpStillPairsRowAndDay(t *testing.T) {
	d := benchFxData(true, true)
	d.SectionsClosed = nil // the ruled default: closed
	html := renderBench(t, d)
	rows := regexp.MustCompile(`data-row="([^"]*)"[^>]*data-day="([^"]*)"`).FindAllStringSubmatch(html, -1)
	if len(rows) == 0 {
		t.Fatal("a closed nextup emitted no data-row/data-day pairs at all — <details> is not display:none and its content stays in the DOM")
	}
	for _, m := range rows {
		if m[1] != m[2] || m[1] == "" {
			t.Errorf("data-row=%q / data-day=%q must be the same non-empty ANSWER key (guard B4)", m[1], m[2])
		}
	}
}

// --- the summary lines ------------------------------------------------------

// Clause 5: never a bare chevron. Every summary carries a title AND a real
// line, in every viewer state this fixture set can reach.
func TestBenchDisclosure_EverySummaryCarriesATitleAndALine(t *testing.T) {
	for _, fx := range []struct {
		name string
		data BenchData
	}{
		{"gm-unfilled", benchFxData(true, true)},
		{"player-unfilled", benchFxData(false, false)},
		{"gm-filled", benchFxDataRsvp(true, true)},
		{"player-filled", benchFxDataRsvp(false, false)},
	} {
		html := renderBench(t, fx.data)
		for _, key := range benchSectionKeys {
			label, line := benchSummaryParts(t, html, key)
			if strings.TrimSpace(label) == "" {
				t.Errorf("%s/%s: the summary has no title", fx.name, key)
			}
			if strings.TrimSpace(line) == "" {
				t.Errorf("%s/%s: the summary line is EMPTY — clause 5 says a real line "+
					"(count, next item, or title), never a bare chevron", fx.name, key)
			}
		}
	}
}

// §4.2 item 1: a player's ribbon summary is built from the payload a PLAYER
// received — three tiles, and it never names a GM tile.
func TestBenchDisclosure_PlayerRibbonSummaryNamesNoGMTile(t *testing.T) {
	player := renderBench(t, benchFxData(false, false))
	_, line := benchSummaryParts(t, player, "ribbon")
	for _, gmWord := range []string{"attention", "sync", "horizon", "item"} {
		if strings.Contains(strings.ToLower(line), gmWord) {
			t.Errorf("a player's ribbon summary says %q: %q — the three GM tiles are ABSENT from their payload, so naming one invents a fact", gmWord, line)
		}
	}
	// A GM's does carry the attention half, so the assertion above is proving a
	// gate rather than an accident of copy.
	gm := renderBench(t, benchFxData(true, true))
	if _, gmLine := benchSummaryParts(t, gm, "ribbon"); !strings.Contains(strings.ToLower(gmLine), "attention") {
		t.Errorf("a GM's ribbon summary omits the attention count: %q — then the player assertion above proves nothing", gmLine)
	}
}

// §4.2 item 2: a player's rsvp summary is built from the panel a player
// receives. No lane exists in their payload at all, so a summary that counted
// free slots would be inventing a fact from data they were deliberately not
// sent.
func TestBenchDisclosure_PlayerRsvpSummaryUsesNoLaneData(t *testing.T) {
	d := benchFxDataRsvp(false, false)
	if len(d.Rsvp.Lanes) != 0 {
		t.Fatalf("fixture invariant broken: a player received %d lanes", len(d.Rsvp.Lanes))
	}
	_, line := benchSummaryParts(t, renderBench(t, d), "rsvp")
	if strings.TrimSpace(line) == "" {
		t.Fatal("a player's rsvp summary is empty")
	}
	// The GM's line and the player's line come from the SAME field
	// (Rsvp.Headline / Rsvp.Note), which is already audience-shaped by
	// benchRsvpBuild. The proof is that no lane-derived word can appear.
	for _, laneWord := range []string{"free", "slot", "lane", "density"} {
		if strings.Contains(strings.ToLower(line), laneWord) {
			t.Errorf("a player's rsvp summary says %q: %q", laneWord, line)
		}
	}
}

// §4.2 item 3: a summary line is a SENTENCE, not a status surface.
// `.badge.need` is GM-tier and is not diluted (WG-5 / RSVP-P8B §7); a summary
// is not one of its homes.
func TestBenchDisclosure_NoSummaryCarriesANeedsBackendChip(t *testing.T) {
	for _, data := range []BenchData{
		benchFxData(true, true), benchFxData(false, false),
		benchFxDataRsvp(true, true), benchFxDataRsvp(false, false),
	} {
		html := renderBench(t, data)
		for _, key := range benchSectionKeys {
			el := benchSummaryElement(t, html, key)
			if strings.Contains(el, "badge") {
				t.Errorf("the %q summary carries a badge: %s", key, el)
			}
			if strings.Contains(strings.ToLower(el), "needs backend") {
				t.Errorf("the %q summary says \"needs backend\"", key)
			}
		}
	}
}

// The rows summary's setup half is GM-ONLY, exactly as benchSectionSubtitle
// already gates it: a player is not told that a calendar they cannot fix is
// broken.
func TestBenchDisclosure_RowsSummarySetupHalfIsGMOnly(t *testing.T) {
	_, gm := benchSummaryParts(t, renderBench(t, benchFxData(true, true)), "rows")
	if !strings.Contains(gm, "needs setup") {
		t.Errorf("the GM's rows summary omits the setup half: %q — the fixture has a faulted calendar", gm)
	}
	_, player := benchSummaryParts(t, renderBench(t, benchFxData(false, false)), "rows")
	if strings.Contains(player, "needs setup") {
		t.Errorf("a player's rows summary says %q; a player never receives the warnrow either", player)
	}
}

// --- no JavaScript ----------------------------------------------------------

// §12: hx-* attributes are markup and are the whole budget. The one <script>
// on the page is the pre-existing owner-only calendar_permissions.js link.
func TestBenchDisclosure_AddsNoScript(t *testing.T) {
	// Scoped to the Bench SURFACE, not to the page: the app shell brings its own
	// script tags and counting those would be measuring the layout, not this
	// slice. The surface is everything from .cal-bench to the permissions modal
	// the owner branch appends after it.
	owner := benchSurfaceHTML(t, renderBench(t, benchFxData(true, true)))
	player := benchSurfaceHTML(t, renderBench(t, benchFxData(false, false)))
	for name, html := range map[string]string{"owner": owner, "player": player} {
		if n := strings.Count(html, "<script"); n != 0 {
			t.Errorf("%s: %d <script> tags inside the Bench surface, want 0 — hx-* attributes are markup and are the whole budget (§12)", name, n)
		}
		for _, banned := range []string{"onclick=", "onchange=", "ontoggle=", "hx-on"} {
			if strings.Contains(html, banned) {
				t.Errorf("%s: inline handler %q appeared on the Bench", name, banned)
			}
		}
	}
	// The one script the page is allowed to link is the PRE-EXISTING owner-only
	// permissions driver, outside the surface and untouched by this slice.
	full := renderBench(t, benchFxData(true, true))
	if !strings.Contains(full, "calendar_permissions.js") {
		t.Error("the pre-existing owner-only permissions driver link vanished")
	}
	if strings.Contains(benchSurfaceHTML(t, full), "calendar_permissions.js") {
		t.Error("the permissions driver moved INSIDE the Bench surface")
	}
}

// --- helpers ----------------------------------------------------------------

// benchDisclosureElement returns the OPEN TAG of one <details>. The index is
// checked before it is used as a bound: a bare strings.Index slice bound PANICS
// on a rename instead of failing cleanly (COMMON §3).
func benchDisclosureElement(t *testing.T, html, key string) string {
	t.Helper()
	marker := `data-bench-disc="` + key + `"`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("no disclosure %q in the rendered bench", key)
	}
	start := strings.LastIndex(html[:i], "<details")
	if start < 0 {
		t.Fatalf("disclosure %q is not on a <details> element", key)
	}
	end := strings.Index(html[start:], ">")
	if end < 0 {
		t.Fatalf("disclosure %q has an unterminated open tag", key)
	}
	return html[start : start+end+1]
}

// benchSummaryElement returns one <summary>…</summary>, inner markup included.
func benchSummaryElement(t *testing.T, html, key string) string {
	t.Helper()
	marker := `data-disc-pick="` + key + `"`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("no summary %q in the rendered bench", key)
	}
	start := strings.LastIndex(html[:i], "<summary")
	if start < 0 {
		t.Fatalf("the %q control is not on a <summary> element", key)
	}
	rest := html[start:]
	end := strings.Index(rest, "</summary>")
	if end < 0 {
		t.Fatalf("summary %q is unterminated", key)
	}
	return rest[:end+len("</summary>")]
}

var benchSpanRe = regexp.MustCompile(`<span class="(cap|c)"[^>]*>([^<]*)</span>`)

// benchSummaryParts returns (title, line) — the .cap and the .c of one summary.
func benchSummaryParts(t *testing.T, html, key string) (string, string) {
	t.Helper()
	el := benchSummaryElement(t, html, key)
	var label, line string
	for _, m := range benchSpanRe.FindAllStringSubmatch(el, -1) {
		switch m[1] {
		case "cap":
			label = m[2]
		case "c":
			line = m[2]
		}
	}
	return label, line
}

// benchDiscIsOpen answers whether one disclosure rendered with the `open`
// attribute. Templ emits it positionally among the other attributes, so the
// check is on the OPEN TAG rather than on a literal prefix — a literal would
// pass vacuously the day templ reorders anything.
func benchDiscIsOpen(t *testing.T, html, key string) bool {
	t.Helper()
	return regexp.MustCompile(`[\s]open[\s>]`).MatchString(benchDisclosureElement(t, html, key))
}

// benchSurfaceHTML returns the .cal-bench wrapper's contents — everything this
// slice is responsible for, and nothing the app shell brings.
func benchSurfaceHTML(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `class="cal-bench"`)
	if start < 0 {
		t.Fatal("rendered page has no .cal-bench wrapper")
	}
	rest := html[start:]
	end := strings.Index(rest, "data-bench-caption")
	if end < 0 {
		return rest
	}
	tail := rest[end:]
	if close := strings.Index(tail, "</div>"); close >= 0 {
		return rest[:end+close]
	}
	return rest
}

// benchInsideDisclosure answers whether byte offset i sits inside a <details>.
// It counts opens and closes to the left, which is exact for this page because
// disclosures never nest ([BR2-3]: four sections, no sub-sections).
func benchInsideDisclosure(html string, i int) bool {
	head := html[:i]
	return strings.Count(head, "<details") > strings.Count(head, "</details>")
}
