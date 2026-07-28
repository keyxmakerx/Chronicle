package calendar_block

// shelf_probe_test.go — Zone D measured in a real engine.
//
// WHY A BROWSER. Every claim here is about a box: the strip's 34px, the
// scroller's 132px, and whether the filled Shelf and the filled Ledger fit in a
// std Block together. Those are questions about flexbox, container queries and
// a max-height interacting, and no string assertion over the sheet can answer
// one — which is precisely the lesson C-CALV4-LEDGER-P6 §4.5 wrote down: a
// FIXED-SIZE BOX HIDES THE FAILURE OF WHAT IS INSIDE IT, so an invariant of the
// form "this surface keeps its shape" must be measured on the surface's own
// computed style or on the relative geometry of its children.
//
// It reuses the Ledger probe's harness — findProbeChromium, probePayloadRe and
// the DOMContentLoaded/data-probe idiom — rather than authoring a second one. A
// new FILE is not a fork.

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// controlAttrRe matches the three attributes that carry a Block's CSS-only
// control identifiers. It requires a leading space so `data-event-id="ev-1"`
// cannot match on its own `-id=` tail.
var controlAttrRe = regexp.MustCompile(`(\s)(name|id|for)="([^"]*)"`)

// isolateBlockControls suffixes every radio group name and every control id in
// one rendered Block.
//
// RADIOS SHARING A NAME ARE ONE GROUP DOCUMENT-WIDE. Put two Blocks on one
// probe page and only the LAST `checked` in each group survives — so every
// earlier box measures with no tab pressed, its panels all display:none, and
// its Shelf collapses to a bare 34px strip. That is not a product state; it is
// a harness artefact, and it is the same one C-CALV4-LEDGER-P6 §4.1 hit with
// the day pick and fixed for that ONE group.
//
// W-E added two more groups (the Shelf tabs and the Almanac sub-tabs), so the
// isolation is generalised here to every control the Block emits. Without it a
// std-tier collision probe would measure a Shelf with an empty body — the
// smallest it can ever be — and report no collision however tall the real one
// grew, which is a gate that has stopped gating.
func isolateBlockControls(markup, suffix string) string {
	return controlAttrRe.ReplaceAllString(markup, `$1$2="$3`+suffix+`"`)
}

// pickRadio moves `checked` within ONE named radio group in already-rendered
// markup, leaving every other group alone.
//
// It is the DOM state a click produces, not a simulation of one — the same
// claim pickDay makes for the day group. It rewrites whole <input> tags rather
// than splicing on an attribute PAIR (` checked name=`), because templ emits
// `checked` last and a pair-splice silently matches nothing: the probe then
// measures the server default four times over and passes vacuously. That is
// how the first cut of this probe reported the Almanac panel for every box,
// including the two that were supposed to be showing Upcoming and Filters.
func pickRadio(t *testing.T, markup, class, attr, key string) string {
	t.Helper()
	re := regexp.MustCompile(`<input type="radio" class="` + class + `"[^>]*>`)
	want := attr + `="` + key + `"`
	hit := false
	out := re.ReplaceAllStringFunc(markup, func(tag string) string {
		tag = strings.Replace(tag, " checked", "", 1)
		if strings.Contains(tag, want) {
			hit = true
			return strings.TrimSuffix(tag, ">") + " checked>"
		}
		return tag
	})
	if !hit {
		t.Fatalf("no .%s control carrying %s — the probe would measure the server default "+
			"and pass vacuously", class, want)
	}
	return out
}

type shelfProbeReading struct {
	Width int `json:"width"`

	BlockH  float64 `json:"blockH"`
	ScrollH float64 `json:"scrollH"`
	ClientH float64 `json:"clientH"`

	ShelfTop float64 `json:"shelfTop"`
	ShelfBot float64 `json:"shelfBot"`
	StripH   float64 `json:"stripH"`
	BodyH    float64 `json:"bodyH"`
	BodyMax  string  `json:"bodyMax"`
	BodyOvf  string  `json:"bodyOvf"`

	// PaneShown is how many Shelf panels compute a display other than none.
	// Exactly one may.
	PaneShown int    `json:"paneShown"`
	PaneKey   string `json:"paneKey"`
	// PaneBot is the bottom of the visible panel's own border box. A panel
	// taller than the scroller legitimately reports a rect past it — that is
	// what a scrolled child does — so this is EVIDENCE, logged, and the
	// bounding claim is made on the scroller's own client/scroll heights.
	PaneBot     float64 `json:"paneBot"`
	BodyScrollH float64 `json:"bodyScrollH"`
	BodyClientH float64 `json:"bodyClientH"`

	// StripScrollW / StripClientW measure whether the strip's controls fit.
	// They may overflow — the strip scrolls — but the LAST control must never
	// be drawn past the zone's own right edge with no way to reach it.
	StripScrollW float64 `json:"stripScrollW"`
	StripClientW float64 `json:"stripClientW"`
	StripOvfX    string  `json:"stripOvfX"`
	LastTabRight float64 `json:"lastTabRight"`
	StripRight   float64 `json:"stripRight"`

	LedgerBot float64 `json:"ledgerBot"`
	RowsBot   float64 `json:"rowsBot"`
	TabCount  int     `json:"tabCount"`
}

const shelfProbeScript = `function(hostEl){
  var q=function(s){return hostEl.querySelector(s)};
  var box=function(e){return e?e.getBoundingClientRect():null};
  var blk=q('.block'), sh=q('.shelf'), st=q('.shelf .st'), sp=q('.sp2');
  var bb=box(blk), sb=box(sh), stb=box(st), spb=box(sp);
  var shown=0, key='', paneBot=0;
  [].slice.call(hostEl.querySelectorAll('.spane')).forEach(function(p){
    if (getComputedStyle(p).display !== 'none'){
      shown++; key=p.getAttribute('data-spane'); paneBot=p.getBoundingClientRect().bottom;
    }
  });
  var led=box(q('.ledger')), rows=box(q('.lrows'));
  var ctrls=[].slice.call(hostEl.querySelectorAll('.st .stab, .st .almbtn'));
  var lastR = ctrls.length ? ctrls[ctrls.length-1].getBoundingClientRect().right : 0;
  return {
    width: Math.round(hostEl.getBoundingClientRect().width),
    blockH: blk?+bb.height.toFixed(1):0,
    scrollH: blk?blk.scrollHeight:0, clientH: blk?blk.clientHeight:0,
    shelfTop: sb?+sb.top.toFixed(1):0, shelfBot: sb?+sb.bottom.toFixed(1):0,
    stripH: stb?+stb.height.toFixed(1):0,
    bodyH: spb?+spb.height.toFixed(1):0,
    bodyMax: sp?getComputedStyle(sp).maxHeight:'', bodyOvf: sp?getComputedStyle(sp).overflowY:'',
    bodyScrollH: sp?sp.scrollHeight:0, bodyClientH: sp?sp.clientHeight:0,
    paneShown: shown, paneKey: key, paneBot: +paneBot.toFixed(1),
    stripScrollW: st?st.scrollWidth:0, stripClientW: st?st.clientWidth:0,
    stripOvfX: st?getComputedStyle(st).overflowX:'',
    lastTabRight: +lastR.toFixed(1), stripRight: stb?+stb.right.toFixed(1):0,
    ledgerBot: led?+led.bottom.toFixed(1):0, rowsBot: rows?+rows.bottom.toFixed(1):0,
    tabCount: hostEl.querySelectorAll('.stab').length
  };
}`

func runShelfProbe(t *testing.T, chrome string, boxes, styles []string) []shelfProbeReading {
	t.Helper()
	css := blockCSS(t)
	var body strings.Builder
	for i, markup := range boxes {
		fmt.Fprintf(&body, `<div class="probe-host" id="sh%d" style="%s">%s</div>`, i, styles[i], markup)
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + body.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + shelfProbeScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "shelf-probe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-gpu",
		"--hide-scrollbars", "--window-size=1600,1600", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []shelfProbeReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	return readings
}

// TestProbe_ShelfGeometryIsInvariant measures the two numbers the Block's
// declared heights rest on, and the one thing the CSS-only tab mechanism can
// silently get wrong.
//
// `.st` is 34px and `.sp2` is 132px WITH ITS OWN SCROLLER (dispatch §1): "the
// body scrolls; the zone does not grow". A panel taller than the scroller —
// the Almanac's Month lane is, by construction — must scroll inside it and must
// not push the zone.
//
// EXACTLY ONE PANEL MAY BE SHOWN. Zero means the radio group lost its checked
// option and the zone renders as a bare strip; two means a reveal rule stopped
// naming its own panel. Both are invisible to every string assertion in the
// package, because the markup is identical in all three cases.
func TestProbe_ShelfGeometryIsInvariant(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	// The production host layer set, with the Almanac register filled: the
	// tallest Shelf the product can render.
	gm := fxAlmanac(t, true)
	player := fxAlmanac(t, false)

	type box struct {
		label string
		width int
		d     BlockData
		pick  string // "" leaves the server default pressed
		sub   string // "" leaves Tonight, the signed sub-tab default
	}
	// THE MONTH SUB-TAB IS THE TALLEST PANEL THE ZONE CAN RENDER — one lane per
	// declared moon plus the ruler and the footnote — so it is the case that
	// proves the scroller rather than the case that happens to fit.
	cases := []box{
		{"full 1232 GM · default (Almanac · Tonight)", 1232, gm, "", ""},
		{"full 1232 GM · Almanac · Month", 1232, gm, shelfTabAlmanac, almTabMonth},
		{"full 1232 GM · Almanac · Moons", 1232, gm, shelfTabAlmanac, almTabMoons},
		{"full 1232 GM · Upcoming", 1232, gm, shelfTabUpcoming, ""},
		{"full 1232 GM · Filters", 1232, gm, shelfTabFilters, ""},
		{"full 1232 PLAYER", 1232, player, "", ""},
		{"std 420 GM", 420, gm, "", ""},
		{"std 358 GM", 358, gm, "", ""},
	}

	var boxes, styles []string
	for i, c := range cases {
		markup := isolateBlockControls(stripLink(render(t, c.d)), fmt.Sprintf("-sp%d", i))
		if c.pick != "" {
			markup = pickRadio(t, markup, "shelfpick", "data-shelf-pick", c.pick)
		}
		if c.sub != "" {
			markup = pickRadio(t, markup, "almpick", "data-alm-pick", c.sub)
		}
		boxes = append(boxes, markup)
		styles = append(styles, fmt.Sprintf("width:%dpx", c.width))
	}

	readings := runShelfProbe(t, chrome, boxes, styles)
	if len(readings) != len(cases) {
		t.Fatalf("probe returned %d readings for %d boxes", len(readings), len(cases))
	}

	for i, c := range cases {
		r := readings[i]
		t.Logf("%s — block %.1fpx (content %.0f in %.0f) · shelf %.1f→%.1f · strip %.1fpx · "+
			"body %.1fpx (%0.f of %0.f scrolling, max %s, overflow-y %s) · panel %q shown=%d · tabs %d",
			c.label, r.BlockH, r.ScrollH, r.ClientH, r.ShelfTop, r.ShelfBot,
			r.StripH, r.BodyH, r.BodyClientH, r.BodyScrollH, r.BodyMax, r.BodyOvf,
			r.PaneKey, r.PaneShown, r.TabCount)
		t.Logf("    strip: %.0fpx of controls in %.0fpx (overflow-x %s) · last control right "+
			"%.1f vs strip right %.1f", r.StripScrollW, r.StripClientW, r.StripOvfX,
			r.LastTabRight, r.StripRight)

		if r.PaneShown != 1 {
			t.Errorf("%s: %d Shelf panels are shown, want exactly 1 — zero means the radio "+
				"group lost its checked option and the zone is a bare strip; two means a "+
				"reveal rule stopped naming its own panel. Neither is visible in the markup.",
				c.label, r.PaneShown)
		}
		if d := r.StripH - 34; d > 0.5 || d < -0.5 {
			t.Errorf("%s: the strip is %.1fpx, want 34 — the Block's declared heights depend "+
				"on it", c.label, r.StripH)
		}
		if r.BodyMax != "132px" {
			t.Errorf("%s: the body's max-height is %q, want the signed 132px at every tier",
				c.label, r.BodyMax)
		}
		if r.BodyOvf != "auto" && r.BodyOvf != "scroll" {
			t.Errorf("%s: the body's overflow-y is %q — the body SCROLLS; the zone does not "+
				"grow", c.label, r.BodyOvf)
		}
		if r.BodyH > 132.5 {
			t.Errorf("%s: the body measured %.1fpx, past its own 132px ceiling — a panel is "+
				"pushing the zone instead of scrolling inside it", c.label, r.BodyH)
		}
		// A panel TALLER than the scroller is the normal state — the Almanac's
		// Month lane is, by construction — and its rect legitimately extends
		// past the scroller because that is what a scrolled child does. What
		// must NOT happen is the overflow reaching the Block: if the zone
		// bounded its own content, the Block's own scrollHeight equals its
		// clientHeight and nothing is clipped.
		if r.BodyScrollH <= r.BodyClientH && r.PaneKey == "almanac" && r.BodyH > 131 {
			t.Errorf("%s: the Almanac panel fits its 132px scroller exactly (%0.f in %0.f) — "+
				"either the lane stopped rendering or the scroller stopped scrolling",
				c.label, r.BodyScrollH, r.BodyClientH)
		}
		if r.ScrollH-r.ClientH > 1 {
			t.Errorf("%s: %.0fpx of content in a %.0fpx Block — the component is clipping a "+
				"zone it declared room for", c.label, r.ScrollH, r.ClientH)
		}
		// THE STRIP MAY OVERFLOW; IT MAY NOT CLIP. Six controls do not fit
		// 358px of Bench host, and measured there the last sub-tab was drawn
		// past the zone's own right edge — not hidden, not wrapped, just gone.
		// A control a viewer cannot reach is worse than one that is absent,
		// because nothing says it is there.
		if r.StripScrollW-r.StripClientW > 1 && r.StripOvfX != "auto" && r.StripOvfX != "scroll" {
			t.Errorf("%s: the strip holds %.0fpx of controls in %.0fpx and its overflow-x is "+
				"%q — the last tab is drawn past the zone's edge with no way to reach it",
				c.label, r.StripScrollW, r.StripClientW, r.StripOvfX)
		}
	}

	// [S10] measured rather than asserted: the GM's strip carries three tabs
	// and the player's two. It is the first per-role difference inside a CHROME
	// STRIP, so it is worth reading off a real box rather than off a substring.
	if readings[0].TabCount != 3 {
		t.Errorf("the GM's Shelf has %d tabs, want 3 (Upcoming · Filters · Almanac)", readings[0].TabCount)
	}
	if readings[5].TabCount != 2 {
		t.Errorf("a player's Shelf has %d tabs, want 2 — Filters is ABSENT, not disabled",
			readings[3].TabCount)
	}
}

// TestProbe_StdTierFilledShelfDoesNotCollideWithTheFilledLedger is the §12
// RE-MEASURE, and it is a STOP-AND-FLAG gate ([S11], SIGNED).
//
// entity_calendar_block_test.go:374-375 and bench.go:513-521 record that at std
// an extra needzone row collides the Ledger and Shelf headers — the measurement
// that booked `moongraph` and `horizon` out of both host layer sets. It was
// taken against STUBS. C-CALV4-LEDGER-P6 re-took it with the Ledger full and
// answered a real collision inside the Block's own std geometry. This slice
// replaces the OTHER stub, so it is taken a third time, at both production host
// widths, with BOTH zones full and the Almanac's tallest panel selected.
//
// IF THIS FAILS THE ANSWER IS NOT TO DROP A HOST LAYER KEY. Dropping a key from
// entityBlockLayers() or benchBlockLayers() is a HOST decision and belongs to
// that host's owner; a Block that quietly stops docking a zone is the failure
// the layer registry exists to make visible.
func TestProbe_StdTierFilledShelfDoesNotCollideWithTheFilledLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	gm := fxAlmanac(t, true)
	widths := []int{420, 358}
	var boxes, styles []string
	var labels []string
	for i, w := range widths {
		for j, tab := range []string{shelfTabAlmanac, shelfTabUpcoming} {
			markup := isolateBlockControls(stripLink(render(t, gm)), fmt.Sprintf("-c%d%d", i, j))
			markup = pickRadio(t, markup, "shelfpick", "data-shelf-pick", tab)
			if tab == shelfTabAlmanac {
				// The tallest panel, at the tier with the least room for it.
				markup = pickRadio(t, markup, "almpick", "data-alm-pick", almTabMonth)
			}
			boxes = append(boxes, markup)
			styles = append(styles, fmt.Sprintf("width:%dpx", w))
			labels = append(labels, fmt.Sprintf("std %dpx · %s", w, tab))
		}
	}

	for i, r := range runShelfProbe(t, chrome, boxes, styles) {
		t.Logf("%s — block %.1fpx (content %.0f in %.0f) · ledger ends %.1f · rows end %.1f · "+
			"shelf %.1f→%.1f · body %.1fpx · panel %q",
			labels[i], r.BlockH, r.ScrollH, r.ClientH, r.LedgerBot, r.RowsBot,
			r.ShelfTop, r.ShelfBot, r.BodyH, r.PaneKey)

		// CONTENT overlap, not box overlap: the two zone boxes abut exactly by
		// construction, so comparing their rects can never catch anything.
		if r.ShelfTop > 0 && r.RowsBot-r.ShelfTop > 1 {
			t.Errorf("%s: the Ledger's rows box ends at %.1f and the Shelf starts at %.1f — "+
				"the filled Ledger is ON TOP of the filled Shelf. STOP AND FLAG ([S11]): the "+
				"answer is a host-layer or std-geometry decision for the coordinator, never a "+
				"silent key drop from entityBlockLayers()/benchBlockLayers().",
				labels[i], r.RowsBot, r.ShelfTop)
		}
		if r.LedgerBot-r.ShelfTop > 1 {
			t.Errorf("%s: the Ledger zone's box (ends %.1f) overlaps the Shelf's (starts %.1f)",
				labels[i], r.LedgerBot, r.ShelfTop)
		}
		if r.ScrollH-r.ClientH > 1 {
			t.Errorf("%s: %.0fpx of content in a %.0fpx Block — a zone is being clipped",
				labels[i], r.ScrollH, r.ClientH)
		}
		if r.PaneShown != 1 {
			t.Errorf("%s: %d panels shown, want 1", labels[i], r.PaneShown)
		}
	}
}
