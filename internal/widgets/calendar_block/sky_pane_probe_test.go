// sky_pane_probe_test.go — the OPEN PANE's two signed-still shapes, measured in
// a real layout engine rather than asserted from markup.
//
// WHY THIS FILE EXISTS. The R2-5 verification found two divergences from
// `mockups/stills/sky-header/sky-open-1440.png` that every shipped guard was
// blind to: the muted sub-head line was absent, and the four-column Tonight row
// had been collapsed into a two-item flex carrying one merged sentence.
// sky_test.go now pins both from the MARKUP — but markup is not what a still
// shows. `<span class="il">33%</span>` is present whether the column lands at a
// shared right edge or ragged, and "four aligned columns" is a CLAIM ABOUT
// GEOMETRY. [SKY-15]'s own rule applies: a count that is argued rather than
// measured is the count failing.
//
// So this probe opens the pane in headless Chromium and reads the boxes:
//   - every `.ph` shares a left edge, every `.il` shares a RIGHT edge, every
//     `.nx` shares a RIGHT edge, and the four run left-to-right in order;
//   - the `.il` column is right-aligned INSIDE its own box, which is the thing
//     tabular percentages need and the thing a bare `<span>` does not give;
//   - the sub-head sits between the head's bottom and the trio's top, in
//     pixels, not in string offsets;
//   - the turn column RETIRES at the seal (the mock's own `.mrowx .nx
//     {display:none}` at C3) rather than wrapping the row.
//
// IT SKIPS HONESTLY when it cannot do that — under -short, or with no Chromium
// present — and a skipped run is NOT a pass, on this package's own precedent.
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// skyPaneRow is one Tonight row's four boxes, in CSS pixels.
type skyPaneRow struct {
	Name     string  `json:"name"`
	NameLeft float64 `json:"nameLeft"`
	PhLeft   float64 `json:"phLeft"`
	IlRight  float64 `json:"ilRight"`
	IlAlign  string  `json:"ilAlign"`
	NxRight  float64 `json:"nxRight"`
	NxShown  bool    `json:"nxShown"`
	NxText   string  `json:"nxText"`
}

// skyPaneReading is one host's open pane.
type skyPaneReading struct {
	Host    int    `json:"host"`
	Density string `json:"density"`
	// The header stack, top to bottom: the bold head, the muted sub-head, the
	// trio. `SubTop` is -1 when no sub-head rendered at all, which is the
	// divergence this probe exists to make impossible to ship again.
	HeadBottom float64      `json:"headBottom"`
	SubTop     float64      `json:"subTop"`
	SubText    string       `json:"subText"`
	TabsTop    float64      `json:"tabsTop"`
	Rows       []skyPaneRow `json:"rows"`
}

const skyPaneProbeScript = `function(root){
  var sky = root.querySelector('details.skygrow');
  var band = sky && sky.querySelector('summary.skyhdr');
  var rect = function(el){ return el ? el.getBoundingClientRect() : null; };
  var r1 = function(v){ return Math.round(v * 10) / 10; };
  var head = rect(sky.querySelector('.skhead'));
  var subEl = sky.querySelector('.skhead + .skynote');
  var sub = rect(subEl);
  var tabs = rect(sky.querySelector('.sktabs'));
  var rows = [].slice.call(sky.querySelectorAll('.sky-tonight .skyrow')).map(function(row){
    var nm = row.querySelector('.nm');
    var ph = row.querySelector('.ph');
    var il = row.querySelector('.il');
    var nx = row.querySelector('.nx');
    var nmr = rect(nm), phr = rect(ph), ilr = rect(il), nxr = rect(nx);
    // display:none lays out at 0x0; that is how the seal's retirement is read.
    var shown = !!nxr && (nxr.width > 0 || nxr.height > 0);
    return {
      name: nm ? nm.textContent.trim() : '',
      nameLeft: nmr ? r1(nmr.left) : -1,
      phLeft:   phr ? r1(phr.left) : -1,
      ilRight:  ilr ? r1(ilr.right) : -1,
      ilAlign:  il ? getComputedStyle(il).textAlign : '',
      nxRight:  shown ? r1(nxr.right) : -1,
      nxShown:  shown,
      nxText:   nx ? nx.textContent.trim() : ''
    };
  });
  return {
    host: Math.round(root.getBoundingClientRect().width),
    density: getComputedStyle(band).justifyContent === 'flex-end' ? 'C3 seal' : 'C1 horizon',
    headBottom: head ? r1(head.bottom) : -1,
    subTop: sub ? r1(sub.top) : -1,
    subText: subEl ? subEl.textContent.trim() : '',
    tabsTop: tabs ? r1(tabs.top) : -1,
    rows: rows
  };
}`

// TestSkyPaneProbe_TheStillsTwoShapes is the geometric half of the fix round.
func TestSkyPaneProbe_TheStillsTwoShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("the sky pane probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}

	hosts := []struct {
		name    string
		width   int
		density string
		wantNx  bool // does the turn column render at this density?
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon", true},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal", false},
	}

	// EACH HOST GETS ITS OWN CalendarSlug, AND THAT IS NOT PROBE COSMETICS.
	// The trio is a radio group whose name is `skyPickGroupName(d)` =
	// sky-<slug>-<hostEntity>. Two renders of the SAME BlockData on one page
	// therefore share one group name, and a document may only have one checked
	// radio per name — so the second copy's `checked` Tonight input silently
	// unchecks the first copy's, the `:has(…:checked)` reveal rule stops
	// matching, and the first host's Tonight panel lays out at 0x0. The first
	// draft of this probe measured exactly that and reported a column of zeroes
	// as a ragged column. In the product the two would differ by construction
	// (that is what the namespace is for); here the fixture has to say so.
	var boxes strings.Builder
	for i, h := range hosts {
		d := fxSky(t, true)
		d.CalendarSlug = fmt.Sprintf("%s-probe-%d", d.CalendarSlug, i)
		// ONE MOON IS RENAMED LONGER, BECAUSE THE SHARED FIXTURE CANNOT TEST
		// THE CLAIM. Alder, Umber, Flint and Sable are all five letters, so
		// they render at the same width and the phase column would start at one
		// edge whether or not the name column has a measure — the guard would
		// pass against a `.nm` with no `inline-size` at all. A name of a
		// different length is what makes "four aligned columns" falsifiable.
		if len(d.Month.Almanac) > 1 {
			d.Month.Almanac[1].Name = "Umberlight-the-Second"
		}
		markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host cal-bench" id="h%d" style="width:%dpx">%s</div>`,
			i, h.width, markup)
	}

	// BOTH sheets, for the same reason the count probe needs both: the pane's
	// markup is the Block's and its rules are the Bench's ([SKY-2]).
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		blockCSS(t) + skyProbeBenchCSS(t) +
		`.probe-host{display:block;margin:24px;max-width:none}` +
		`</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + skyPaneProbeScript + `;` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		`hosts.forEach(function(root){` +
		`root.querySelector('details.skygrow').setAttribute('open','');});` +
		// Sampled AFTER the carve-out's 200ms envelope, three envelopes out, for
		// the reason the count probe states: a synchronous read measures the
		// probe's own impatience rather than the settled pane.
		`setTimeout(function(){` +
		`document.body.setAttribute('data-probe', JSON.stringify(hosts.map(read)));},600);});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "skypane.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1400", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload — the page script did not run")
	}
	var readings []skyPaneReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(hosts) {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), len(hosts))
	}

	for i, h := range hosts {
		r := readings[i]
		t.Run(h.name, func(t *testing.T) {
			t.Logf("host %dpx · %s · head bottom %.1f → sub-head top %.1f → trio top %.1f · "+
				"sub-head %q · %d Tonight rows · turn column %v",
				r.Host, r.Density, r.HeadBottom, r.SubTop, r.TabsTop, r.SubText,
				len(r.Rows), r.NxShown())

			if r.Density != h.density {
				t.Fatalf("density = %q, want %q — this probe's subject is the density's "+
					"own row shape, so a host that resolved the wrong way measures nothing",
					r.Density, h.density)
			}

			// ── D1: THE MUTED SUB-HEAD, IN PIXELS ────────────────────────────
			// Between the head's bottom and the trio's top. The build this
			// replaces had no such element at all (SubTop would read -1), and
			// the differently-worded line it shipped instead sat BELOW the
			// trio — which a string-order assertion catches but a "is it
			// present" assertion does not.
			if r.SubTop < 0 {
				t.Error("no muted sub-head rendered under the head — the signed still " +
					"(sky-open-1440.png) puts one there and its absence is the " +
					"divergence this probe exists to catch")
			} else {
				if r.SubTop < r.HeadBottom {
					t.Errorf("the sub-head's top (%.1f) is above the head's bottom (%.1f) — "+
						"it is part of the header STACK, not an overlay", r.SubTop, r.HeadBottom)
				}
				if r.TabsTop > 0 && r.SubTop >= r.TabsTop {
					t.Errorf("the sub-head's top (%.1f) is at or below the trio's top (%.1f) — "+
						"a muted line under the tabs is a FOOTNOTE, which is what shipped "+
						"and what the still does not show", r.SubTop, r.TabsTop)
				}
				// The gap is the tight one, not the footnote's 8px.
				if gap := r.SubTop - r.HeadBottom; gap > 6 {
					t.Errorf("the sub-head sits %.1fpx below the head — that is the "+
						"footnote spacing, and it reads as a third block rather than as "+
						"part of one header stack", gap)
				}
			}

			// ── D2: FOUR ALIGNED COLUMNS, IN PIXELS ──────────────────────────
			if len(r.Rows) < 2 {
				t.Fatalf("only %d Tonight rows — alignment is a claim about a COLUMN and "+
					"one row cannot carry it", len(r.Rows))
			}
			first := r.Rows[0]
			for _, row := range r.Rows[1:] {
				if !closeTo(row.NameLeft, first.NameLeft) {
					t.Errorf("%s's name starts at %.1f, %s's at %.1f — not a column",
						row.Name, row.NameLeft, first.Name, first.NameLeft)
				}
				if !closeTo(row.PhLeft, first.PhLeft) {
					t.Errorf("%s's phase word starts at %.1f, %s's at %.1f — the stills "+
						"show the phase words starting at one edge, which is what the "+
						"fixed name measure buys", row.Name, row.PhLeft, first.Name, first.PhLeft)
				}
				if !closeTo(row.IlRight, first.IlRight) {
					t.Errorf("%s's percentage ends at %.1f, %s's at %.1f — the illumination "+
						"column is RIGHT-aligned in the still so the digits read down",
						row.Name, row.IlRight, first.Name, first.IlRight)
				}
				if row.NxShown && first.NxShown && !closeTo(row.NxRight, first.NxRight) {
					t.Errorf("%s's turn ends at %.1f, %s's at %.1f — the turn column is "+
						"parked at the far right, not trailing the sentence",
						row.Name, row.NxRight, first.Name, first.NxRight)
				}
			}

			// LEFT TO RIGHT, IN THE STILL'S ORDER: name, phase, percentage,
			// turn. The build this replaces printed the percentage FIRST, inside
			// a merged sentence — same facts, wrong order, and no probe or guard
			// would have noticed.
			if first.PhLeft <= first.NameLeft {
				t.Errorf("the phase word (%.1f) is not right of the name (%.1f)",
					first.PhLeft, first.NameLeft)
			}
			if first.IlRight <= first.PhLeft {
				t.Errorf("the percentage (ends %.1f) is not right of the phase word (starts %.1f) — "+
					"the still reads NAME, phase, percentage, turn", first.IlRight, first.PhLeft)
			}
			// The percentage is right-aligned INSIDE its own box. A fixed-width
			// box with default alignment is a ragged column wearing a measure.
			if first.IlAlign != "end" && first.IlAlign != "right" {
				t.Errorf("the illumination column's text-align resolved to %q — tabular "+
					"digits in a fixed box still read ragged without it", first.IlAlign)
			}

			// ── THE SEAL RETIRES THE TURN COLUMN ─────────────────────────────
			for _, row := range r.Rows {
				if row.NxShown != h.wantNx {
					t.Errorf("%s's turn column shown = %v at %s, want %v — the mock retires "+
						"it at the seal (cv4-sky:1017) rather than wrapping the row, and "+
						"the sub-head above still names the soonest turn",
						row.Name, row.NxShown, h.density, h.wantNx)
				}
			}
			if h.wantNx {
				// The turn states a DISTANCE with its unit, which is the wording
				// the merged sentence lost ("next full 10" carried no "days").
				if !strings.Contains(first.NxText, "day") {
					t.Errorf("%s's turn reads %q — the still states a distance in days",
						first.Name, first.NxText)
				}
			}
		})
	}
}

// NxShown reports whether this host rendered any turn column at all, for the
// log line. It is a summary of the rows, not an assertion.
func (r skyPaneReading) NxShown() bool {
	for _, row := range r.Rows {
		if row.NxShown {
			return true
		}
	}
	return false
}

// closeTo compares two laid-out edges. Sub-pixel layout means two boxes that
// share an edge can differ in the last tenth; a half-pixel tolerance accepts
// that and still reds a genuinely ragged column, which is tens of pixels.
func closeTo(a, b float64) bool { return math.Abs(a-b) <= 0.5 }
