// availability_probe_test.go — THE STRIP PAINTS ZERO PIXELS ON CURRENT DATA.
//
// C-CALV4-TILES §9.5 ships the availability strip complete and DORMANT: the CSS
// is whole, and nothing may reach the screen from it until a future dispatch
// supplies windows. availability_strip_test.go proves the three gates exist in
// the sheet and the markup. This proves the CONSEQUENCE, which is the only claim
// the operator can check: a full render of the current fixtures is
// PIXEL-IDENTICAL with and without the strip.
//
// WHY THIS AND NOT A COMPUTED-STYLE READING. getComputedStyle answers what the
// CASCADE resolved, never what the compositor DREW — the finding that let a
// stylesheet regression pass tile edge probes on a grid painting nothing
// (tile_paint_probe_test.go's header). "It paints nothing" is a statement about
// pixels and has to be measured as one.
//
// IT CARRIES ITS OWN SENTINEL. A comparator that always returns "identical"
// would report perfect dormancy on a strip painting scarlet across every cell,
// so each host is ALSO shot with the strip's own box deliberately filled, and
// the run fails if that comparison does not differ. The dormancy reading only
// means something once the comparison has been shown able to fail.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT a
// pass. Registered in tools/check-browser-probes.sh.
//
//	go test ./internal/widgets/calendar_block/ -run AvailabilityProbe -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// apCase is one host: a width, a theme, and the density that width produces.
type apCase struct {
	label   string
	width   int
	dark    bool
	density string
}

// apCounts is what the DOM reports alongside the pixels: the subjects that must
// be there (slots) and the ones that must not (lanes, rsvp days).
//
// `Days` counts BOTH dated surfaces — the week grid's cells and the intercalary
// festival rows — because both reserve a slot and the reservation is
// unconditional across both. Counting `.cell` alone reports 30 against 31 slots
// on the Harptos fixture and reads as an off-by-one in the reservation, which is
// exactly backwards: a festival day is a day, people are available or not on it,
// and an intercalary row the wiring could not paint would be a hole in the
// surface at precisely the days a group is most likely to be planning around.
type apCounts struct {
	Days      int `json:"days"`
	Slots     int `json:"slots"`
	Lanes     int `json:"lanes"`
	RSVPCells int `json:"rsvpCells"`
}

const apCountScript = `function(host){
  return {
    days:      host.querySelectorAll('.cell[data-day], .interc[data-day]').length,
    slots:     host.querySelectorAll('.avail').length,
    lanes:     host.querySelectorAll('.lane').length,
    rsvpCells: host.querySelectorAll('.rsvp').length
  };
}`

// apPage builds one host's page. extraCSS is appended AFTER the sheet, so a
// sentinel can outrank it without editing the file under test.
func apPage(t *testing.T, c apCase, extraCSS string) string {
	t.Helper()
	d := fxAlmanac(t, true)
	markup := cpLinkRe.ReplaceAllString(render(t, d), "")
	open, closeTag := "", ""
	if c.dark {
		open, closeTag = `<div class="dark">`, `</div>`
	}
	// MAGENTA GROUND, tile_paint_probe_test.go's convention: if the Block ever
	// fails to paint its own ground, a neutral page would supply a plausible
	// grey and both shots would agree on the harness instead of the product.
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#ff00ff}` +
		`.probe-host{display:block;margin:20px}` +
		blockCSS(t) + extraCSS + `</style></head><body>` +
		fmt.Sprintf(`%s<div class="probe-host" style="width:%dpx">%s</div>%s`,
			open, c.width, markup, closeTag) +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + apCountScript + `;` +
		`document.body.setAttribute('data-probe', JSON.stringify(` +
		`read(document.querySelector('.probe-host'))));});</script>` +
		`</body></html>`
}

// apShoot renders one page and returns its screenshot. The DOM counts are read
// in a second invocation over the SAME file, because --dump-dom and --screenshot
// cannot both be asked of one run; the page has no scripted animation and no
// clock, so the two agree.
func apShoot(t *testing.T, chrome string, c apCase, extraCSS string, wantCounts bool) (image.Image, apCounts) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "avail.html")
	if err := os.WriteFile(path, []byte(apPage(t, c, extraCSS)), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1400", "--virtual-time-budget=6000",
		"--force-device-scale-factor=1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var counts apCounts
	if wantCounts {
		dom, err := exec.CommandContext(ctx, chrome, append(append([]string{}, args...),
			"--dump-dom", "file://"+path)...).Output()
		if err != nil {
			t.Fatalf("chromium (dom pass): %v", err)
		}
		m := probePayloadRe.FindStringSubmatch(string(dom))
		if m == nil {
			t.Fatal("no probe payload in the rendered DOM — the page script did not run")
		}
		if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &counts); err != nil {
			t.Fatalf("probe payload: %v", err)
		}
	}

	shot := filepath.Join(dir, "avail.png")
	if _, err := exec.CommandContext(ctx, chrome, append(append([]string{}, args...),
		"--screenshot="+shot, "file://"+path)...).Output(); err != nil {
		t.Fatalf("chromium (screenshot pass): %v", err)
	}
	f, err := os.Open(shot) //nolint:gosec // test artefact in t.TempDir()
	if err != nil {
		t.Fatalf("open screenshot: %v", err)
	}
	defer f.Close() //nolint:errcheck // test artefact
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	return img, counts
}

// apDiff counts differing pixels and names the first one, so a failure says
// WHERE rather than only THAT.
func apDiff(t *testing.T, a, b image.Image) (n int, firstX, firstY int) {
	t.Helper()
	ab, bb := a.Bounds(), b.Bounds()
	if ab != bb {
		t.Fatalf("the two shots are different sizes (%v vs %v) — the strip changed the "+
			"page's layout, which it must not: it is absolutely positioned inside a cell",
			ab, bb)
	}
	firstX, firstY = -1, -1
	for y := ab.Min.Y; y < ab.Max.Y; y++ {
		for x := ab.Min.X; x < ab.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				n++
				if firstX < 0 {
					firstX, firstY = x, y
				}
			}
		}
	}
	return n, firstX, firstY
}

// TestAvailabilityProbe_TheStripPaintsNothingOnCurrentData.
//
// THE MEASUREMENT: for each host, the Block as shipped against the Block with
// every availability surface forced out of the render. Zero differing pixels is
// the claim §9.5 makes, and it is the whole reason this change is safe to land
// with no producer, no field and no data.
//
// THE SENTINEL IS RUN FIRST at each host, for the reason stated in the file
// header: a comparison that cannot fail proves nothing about the one that
// passes.
func TestAvailabilityProbe_TheStripPaintsNothingOnCurrentData(t *testing.T) {
	// THE GATE IS INLINE, NOT IN A HELPER: tools/check-browser-probes.sh takes
	// its census by looking for a Chromium finder INSIDE each top-level Test
	// function's body.
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}

	// The strip's surfaces, removed. `!important` and `display: none` together,
	// because the sentinel below proves this override reaches the same region
	// the strip occupies.
	const stripOff = `
.cal-block-host .avail { display: none !important }
.cal-block-host .lane  { display: none !important }`
	// The same region, deliberately painted. A strip that DID reach the screen
	// would look like this, and the comparison must be able to see it.
	const stripLoud = `
.cal-block-host .avail { background: #ff0000 !important }`

	for _, c := range []apCase{
		{"light · 366px host · underline density", 366, false, "underline"},
		{"dark · 366px host · underline density", 366, true, "underline"},
		{"light · 1400px host · named density", 1400, false, "named"},
	} {
		t.Run(c.label, func(t *testing.T) {
			shipped, counts := apShoot(t, chrome, c, "", true)

			// ── THE SUBJECTS ARE REALLY THERE ────────────────────────────
			if counts.Days == 0 || counts.Slots == 0 {
				t.Fatalf("%d dated day surfaces and %d availability slots — with no subject "+
					"every pixel comparison below is a comparison of two identical blanks",
					counts.Days, counts.Slots)
			}
			if counts.Slots != counts.Days {
				t.Errorf("%d slots for %d dated day surfaces — the reservation is "+
					"unconditional BY DESIGN, across the week grid AND the intercalary rows, "+
					"or the cell's interior reflows from day to day",
					counts.Slots, counts.Days)
			}
			if counts.Lanes != 0 || counts.RSVPCells != 0 {
				t.Errorf("%d lanes and %d `rsvp` cells reached a real render. DayCell has no "+
					"availability field and block_projection.go fetches none, so any lane on "+
					"screen was invented — §8.6's fantasy-date blocker is unresolved and this "+
					"package does not draw fabricated figures",
					counts.Lanes, counts.RSVPCells)
			}

			// ── THE SENTINEL: the comparison CAN see a painted strip ─────
			loud, _ := apShoot(t, chrome, c, stripLoud, false)
			sn, _, _ := apDiff(t, shipped, loud)
			if sn == 0 {
				t.Fatalf("filling the strip's own box changed NOTHING on screen, so the " +
					"dormancy reading below is worthless: the comparison cannot see the " +
					"region the strip occupies. Fix the probe before trusting it")
			}

			// ── THE CLAIM ────────────────────────────────────────────────
			off, _ := apShoot(t, chrome, c, stripOff, false)
			n, fx, fy := apDiff(t, shipped, off)
			b := shipped.Bounds()
			t.Logf("%s · %d dated day surfaces · %d slots · %d lanes · shipped vs "+
				"strip-removed: %d/%d differing pixels · sentinel (strip filled): %d "+
				"differing pixels",
				c.density, counts.Days, counts.Slots, counts.Lanes,
				n, b.Dx()*b.Dy(), sn)
			if n != 0 {
				t.Errorf("removing the availability strip changed %d pixels, the first at "+
					"(%d,%d). §9.5: it paints NOTHING until data arrives — a strip that "+
					"reaches the screen with no windows behind it is a fabricated figure, "+
					"and the operator would read it as a feature somebody built", n, fx, fy)
			}
		})
	}
}
