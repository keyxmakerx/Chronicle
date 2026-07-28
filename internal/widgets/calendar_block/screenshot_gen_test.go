// screenshot_gen_test.go — the fidelity evidence generator.
//
// Not a test: a tool that happens to live in a _test.go file so it can reuse the
// fixture and the real templ output rather than re-implementing either. It is
// inert unless BLOCK_SCREENSHOTS names an output directory:
//
//	BLOCK_SCREENSHOTS=/tmp/shots go test ./internal/widgets/calendar_block/ -run Screenshots
//
// Each shot is captioned with the same line the signed stills carry — host
// width, resolved size class, MEASURED column, resulting density — so a
// coordinator gating fidelity can read the arithmetic off the image instead of
// taking the PR's word for it.
package calendar_block

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type shot struct {
	file    string
	title   string
	caption string
	dark    bool
	grey    bool
	w, h    int
	body    func(t *testing.T) string
}

// fxLedgerFull is the signed month with BOTH docked zones switched on — the
// layer set both production hosts pass (entityBlockLayers / benchBlockLayers).
//
// Both NeedsBackend flags are down from wave 2: W-B filled the Ledger and W-E
// filled the Shelf, and a chip beside real content is a lie whichever zone it
// sits in.
func fxLedgerFull(gm bool) BlockData {
	d := fxHarptos(gm)
	d.Layers = LayerState{Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"}}
	d.Ledger = LedgerStub{NeedsBackend: false}
	d.Shelf = ShelfStub{NeedsBackend: false}
	if gm {
		d.Viewer.HiddenCount = 3 // the three dm_only events in the signed month
	}
	return d
}

// shotHostSeq numbers the host boxes on one generated page.
//
// RADIOS SHARING A NAME ARE ONE GROUP DOCUMENT-WIDE. Several shots compose two
// or four Blocks on one page (the size-class ladder, the divergence proof, the
// std player pair), and without per-box isolation only the LAST Block on the
// page keeps a pressed Shelf tab — every earlier one photographs with its zone
// collapsed to a bare strip. The shot would then be evidence of a state the
// product cannot reach.
var shotHostSeq int

// pickDay checks one day's radio in already-rendered markup. There is no JS to
// click with: the DOM state after a click IS a checked radio, so this is the
// selected state exactly, not a simulation of it.
func pickDay(t *testing.T, markup, day string) string {
	t.Helper()
	out := strings.Replace(markup,
		`data-day-pick="`+day+`" name=`, `data-day-pick="`+day+`" checked name=`, 1)
	if out == markup {
		t.Fatalf("no day control for %q in the rendered markup", day)
	}
	return out
}

func TestGenerateScreenshots(t *testing.T) {
	outDir := os.Getenv("BLOCK_SCREENSHOTS")
	if outDir == "" {
		t.Skip("screenshot generator: set BLOCK_SCREENSHOTS=<dir> to run")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("screenshot generator: no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	host := func(width int, d BlockData) string {
		shotHostSeq++
		return fmt.Sprintf(`<div class="shot-host" style="width:%dpx">%s</div>`,
			width, isolateBlockControls(stripLink(render(t, d)), fmt.Sprintf("-s%d", shotHostSeq)))
	}
	// hostPick is host() with one Shelf tab (and optionally one Almanac
	// sub-tab) pressed instead of the server default — the DOM state a click
	// produces, not a simulation of one.
	hostPick := func(width int, d BlockData, tab, sub string) string {
		shotHostSeq++
		m := isolateBlockControls(stripLink(render(t, d)), fmt.Sprintf("-s%d", shotHostSeq))
		if tab != "" {
			m = pickRadio(t, m, "shelfpick", "data-shelf-pick", tab)
		}
		if sub != "" {
			m = pickRadio(t, m, "almpick", "data-alm-pick", sub)
		}
		return fmt.Sprintf(`<div class="shot-host" style="width:%dpx">%s</div>`, width, m)
	}
	arith := func(width int, d BlockData) string {
		tier := SizeClass(width)
		dens := "underline"
		if IsNamedCSS(tier, width, d.Month.WeekLen) {
			dens = "named"
		}
		return fmt.Sprintf("host %dpx → size class <b>%s</b> · %d columns at <b>%.1fpx</b> (model) → <b>%s</b> density",
			width, tier, d.Month.WeekLen, ColWidth(tier, width, d.Month.WeekLen), dens)
	}

	shots := []shot{
		{
			file: "01-block-full-light.png", title: "The Block · full tier · GM · light",
			caption: arith(1232, fxHarptos(true)), w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxHarptos(true)) },
		},
		{
			file: "02-block-full-dark.png", title: "The Block · full tier · GM · dark",
			caption: arith(1232, fxHarptos(true)), dark: true, w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxHarptos(true)) },
		},
		{
			file: "03-block-full-player.png", title: "The Block · full tier · PLAYER · light",
			caption: "permission is ABSENCE — three dm_only events are not in this DOM, and no notch hints that they exist",
			w:       1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxHarptos(false)) },
		},
		{
			file: "04-block-std-390-light.png", title: "The Block · std tier · 390px phone · light",
			caption: arith(358, fxHarptos(true)), w: 390, h: 1000,
			body: func(t *testing.T) string { return host(358, fxHarptos(true)) },
		},
		{
			file: "05-block-std-390-dark.png", title: "The Block · std tier · 390px phone · dark",
			caption: arith(358, fxHarptos(true)), dark: true, w: 390, h: 1000,
			body: func(t *testing.T) string { return host(358, fxHarptos(true)) },
		},
		{
			file: "06-greyscale-proof.png", title: "Greyscale proof · filter: grayscale(1)",
			caption: "every mark must still resolve with the colour removed — by its locked dash pattern and its non-colour token",
			grey:    true, w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxHarptos(true)) },
		},
		{
			file: "07-divergence-1040.png", title: "Divergence proof · one host width, two week lengths",
			caption: "same component, same zones — only the cell interior differs, and it differs for a stated reason",
			w:       1120, h: 1500,
			body: func(t *testing.T) string {
				return `<p class="shot-sub">Harptos · 10 columns — ` + arith(1040, fxHarptos(true)) + `</p>` +
					host(1040, fxHarptos(true)) +
					`<p class="shot-sub">Real world · 7 columns — ` + arith(1040, fxGregorian()) + `</p>` +
					host(1040, fxGregorian())
			},
		},
		{
			file: "08-four-size-classes.png", title: "One Block, four size classes",
			caption: "size class follows HOST width; density follows MEASURED COLUMN width",
			w:       1120, h: 1500,
			body: func(t *testing.T) string {
				var b strings.Builder
				for _, w := range []int{1040, 420, 280, 220} {
					fmt.Fprintf(&b, `<p class="shot-sub">%s</p>%s`, arith(w, fxHarptos(true)), host(w, fxHarptos(true)))
				}
				return b.String()
			},
		},
		{
			file: "10-ledger-full-light.png", title: "The docked Ledger · full tier · GM · light",
			caption: "the panel is ALREADY THERE — 14 rows, one per mark the GM can see, reassembled from the same cells the grid draws",
			w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxLedgerFull(true)) },
		},
		{
			file: "11-ledger-full-dark.png", title: "The docked Ledger · full tier · GM · dark",
			caption: "the same 14 rows on dark; the rule ramp is verified in both directions",
			dark: true, w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxLedgerFull(true)) },
		},
		{
			file: "12-ledger-full-selected.png", title: "ANSWERED · day 5 chosen · full tier · GM",
			caption: "choosing a day repaints a panel that was already on screen. Compare shot 10 ROW FOR ROW, not just corner to corner: the Block's declared height is identical AND each listed row is still the one signed flex line — ordinal · rail · gold rail · glyph · name · chips · meta · right-aligned time. Both readings are measured (TestProbe_LedgerHeightIsInvariantUnderSelection reads computed `display` and day/time overprint in Chromium), because a fixed-height row that has collapsed out of flex is invisible to a height assertion — which is exactly how the first cut of this shot shipped broken. `✕ all` is the reserved head slot, revealed by visibility and never by display.",
			w: 1320, h: 900,
			body: func(t *testing.T) string {
				shotHostSeq++
				m := isolateBlockControls(stripLink(render(t, fxLedgerFull(true))),
					fmt.Sprintf("-s%d", shotHostSeq))
				return fmt.Sprintf(`<div class="shot-host" style="width:1232px">%s</div>`,
					pickDay(t, m, "5"))
			},
		},
		{
			file: "13-ledger-full-player.png", title: "The docked Ledger · full tier · PLAYER · light",
			caption: "PERMISSION IS ABSENCE — three dm_only events are not in this DOM, no gold rail hints that they exist, and no `<N> hidden` chip renders, not even a zero",
			w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxLedgerFull(false)) },
		},
		{
			file: "14-ledger-std-420-light.png", title: "std tier · entity-page host 420px · GM · light",
			caption: "CTS-8's measurement: the filled Ledger stacks BELOW the month with the tab strip above its head, and the Shelf sits under it without collision",
			w: 460, h: 1000,
			body: func(t *testing.T) string { return host(420, fxLedgerFull(true)) },
		},
		{
			file: "15-ledger-std-420-dark.png", title: "std tier · entity-page host 420px · GM · dark",
			caption: "the same reading on dark",
			dark: true, w: 460, h: 1000,
			body: func(t *testing.T) string { return host(420, fxLedgerFull(true)) },
		},
		{
			file: "16-ledger-std-358-light.png", title: "std tier · Bench host 358px · GM · light",
			caption: "the second production host width. Ledger and Shelf headers do not overlap and nothing is clipped",
			w: 400, h: 1000,
			body: func(t *testing.T) string { return host(358, fxLedgerFull(true)) },
		},
		{
			file: "17-ledger-std-358-dark.png", title: "std tier · Bench host 358px · GM · dark",
			caption: "the same reading on dark",
			dark: true, w: 400, h: 1000,
			body: func(t *testing.T) string { return host(358, fxLedgerFull(true)) },
		},
		{
			file: "18-ledger-std-player.png", title: "std tier · 420px and 358px · PLAYER · light",
			caption: "both std host widths for a player: no gold rail, no GM badge, no hidden chip",
			w: 900, h: 1000,
			body: func(t *testing.T) string {
				return `<p class="shot-sub">entity page · 420px</p>` + host(420, fxLedgerFull(false)) +
					`<p class="shot-sub">Bench · 358px</p>` + host(358, fxLedgerFull(false))
			},
		},
		{
			file: "19-ledger-mini-foot.png", title: "The Ledger's smallest form · mini and sub-mini",
			caption: "below 300px the docked zone is display:none and the FOOT is all that is left of it. Entity-hosted and tie-scoped, it states the TIED count and says the word — the mockup prints the month's total beside an Attributes card reading `Ties 4`, which is the tie-count oracle one size down.",
			w: 700, h: 700,
			body: func(t *testing.T) string {
				var b strings.Builder
				scoped := fxLedgerFull(true)
				scoped.Viewer.HostEntity = "ent-hollowmere"
				scoped.Viewer.TieMode = "tied"
				scoped.Viewer.TiedCount, scoped.Viewer.WholeCount = 4, 14
				for _, w := range []int{280, 220} {
					fmt.Fprintf(&b, `<p class="shot-sub">%s</p>%s`, arith(w, scoped), host(w, scoped))
				}
				return b.String()
			},
		},
		{
			file: "20-ledger-owner-greyscale.png", title: "Greyscale proof · the filled Ledger",
			caption: "every Ledger row must still resolve with the colour removed — by its locked dash pattern, its glyph and its gold permission rail",
			grey: true, w: 1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxLedgerFull(true)) },
		},
		{
			file: "21-shelf-full-light.png", title: "The Shelf · full tier · GM · light · the ALMANAC leads",
			caption: "zone D is filled. The Almanac is the server-rendered default because the calendar declares moons (the wave-2 reduction of the signed `SKY_ON() && m.moons`), and Tonight is its own default sub-tab. Four declared bodies, three drawn on the grid — the fourth, Sable, is here at full width, which is the reason the grid's three-moon ceiling is allowed to exist.",
			w: 1320, h: 1000,
			body: func(t *testing.T) string { return host(1232, fxShotAlmanac(t, true)) },
		},
		{
			file: "22-shelf-full-dark.png", title: "The Shelf · full tier · GM · dark",
			caption: "the same zone on dark; the sky is ACHROMATIC in both directions and borrows no event hue",
			dark: true, w: 1320, h: 1000,
			body: func(t *testing.T) string { return host(1232, fxShotAlmanac(t, true)) },
		},
		{
			file: "23-almanac-month-lane.png", title: "The Almanac · Month · one lane per moon, one column per DAY",
			caption: "THE SHAPE THAT CANNOT FIT ON A MONTH GRID, which is why the grid never grows with moon count. Four lanes over thirty columns, each cell carrying its data-day ANSWER key and the per-day illumination as a HEIGHT — the vetoed composite-brightness ribbon relocated as an explicit readout rather than resurrected as a glanceable claim. The footnote's column count is the month's real day count, not the mockup's literal thirty.",
			w: 1320, h: 1000,
			body: func(t *testing.T) string {
				return hostPick(1232, fxShotAlmanac(t, true), shelfTabAlmanac, almTabMonth)
			},
		},
		{
			file: "24-almanac-moons-register.png", title: "The Almanac · Moons · the printed arithmetic",
			caption: "\"the arithmetic is printed so it can be audited — no date in the register was typed by hand\". Period, turns this month and drift, every one computed from the month's REAL day count; and the body past the grid's ceiling says so in words. No epithet: calendar.Moon has no such column and the mockup's \"the great pale moon\" is fixture text.",
			w: 1320, h: 1000,
			body: func(t *testing.T) string {
				return hostPick(1232, fxShotAlmanac(t, true), shelfTabAlmanac, almTabMoons)
			},
		},
		{
			file: "25-shelf-upcoming-and-filters.png", title: "The Shelf · Upcoming, and Filters",
			caption: "Upcoming reuses the LEDGER'S ROW PRIMITIVE VERBATIM — Today, then up to four later days of the same month, from the same viewer-filtered pass the Ledger head counts. Filters ships the TAB and not the engine: nothing in Chronicle backs a filter, so the panel is one `needs backend` chip and no fabricated control.",
			w: 1320, h: 1500,
			body: func(t *testing.T) string {
				return `<p class="shot-sub">Upcoming</p>` +
					hostPick(1232, fxShotAlmanac(t, true), shelfTabUpcoming, "") +
					`<p class="shot-sub">Filters</p>` +
					hostPick(1232, fxShotAlmanac(t, true), shelfTabFilters, "")
			},
		},
		{
			file: "26-shelf-full-player.png", title: "The Shelf · full tier · PLAYER · light",
			caption: "TWO TABS, NOT THREE. World state is not permission-filtered (L23), so the player receives the Almanac in full — but the Filters tab is ABSENT rather than disabled, because a `needs backend` chip never renders to a player and a tab that opens on nothing is worse than either. It is the first per-role difference inside a chrome strip rather than inside content.",
			w: 1320, h: 1000,
			body: func(t *testing.T) string { return host(1232, fxShotAlmanac(t, false)) },
		},
		{
			file: "27-shelf-std-420-light.png", title: "std tier · entity host 420px · GM · light · §12 RE-MEASURE",
			caption: "THE COLLISION GATE, re-taken with BOTH zones full. The signed std Block emits no Shelf at all and puts these tabs in the Ledger head instead, so 132px has never been measured at this tier — filled, the zone wants 166px inside a 520px Block that also holds a month and a docked Ledger. The Shelf is the zone that CAN give (its body already scrolls), so it yields and the Ledger keeps its head and strip intact. Measured at both production host widths.",
			w: 460, h: 1000,
			body: func(t *testing.T) string { return host(420, fxShotAlmanac(t, true)) },
		},
		{
			file: "28-shelf-std-420-dark.png", title: "std tier · entity host 420px · GM · dark",
			caption: "the same reading on dark",
			dark: true, w: 460, h: 1000,
			body: func(t *testing.T) string { return host(420, fxShotAlmanac(t, true)) },
		},
		{
			file: "29-shelf-std-358-light.png", title: "std tier · Bench host 358px · GM · light · §12 RE-MEASURE",
			caption: "the second production host width, both zones full: nothing clipped, nothing overlapping, both zone strips visible",
			w: 400, h: 1000,
			body: func(t *testing.T) string { return host(358, fxShotAlmanac(t, true)) },
		},
		{
			file: "30-shelf-std-358-dark.png", title: "std tier · Bench host 358px · GM · dark",
			caption: "the same reading on dark",
			dark: true, w: 400, h: 1000,
			body: func(t *testing.T) string { return host(358, fxShotAlmanac(t, true)) },
		},
		{
			file: "31-shelf-hidden-and-no-moons.png", title: "The two absences",
			caption: "LEFT: the Bench's real-world Block renders with noShelf — the zone is REMOVED rather than collapsed, or the Block's declared heights stop being invariant, and the nameplate's moons badge withholds its \"all of them are in the Almanac\" tail because this render has no Almanac to point at. RIGHT: a calendar declaring no moons gets no Almanac tab at all — absence, not a disabled control.",
			w: 1320, h: 1200,
			body: func(t *testing.T) string {
				noShelf := fxShotAlmanac(t, true)
				noShelf.Shelf.Hidden = true
				bare := fxShotAlmanac(t, true)
				bare.Month.Almanac, bare.Month.MoonsDeclared = nil, 0
				return `<p class="shot-sub">noShelf — the zone is removed</p>` + host(1232, noShelf) +
					`<p class="shot-sub">no moons declared — no Almanac tab</p>` + host(1232, bare)
			},
		},
		{
			file: "32-shelf-greyscale.png", title: "Greyscale proof · the filled Shelf",
			caption: "the sky is ACHROMATIC by law, so the Almanac loses nothing at all with the colour removed; the Upcoming rows still resolve by their locked dash patterns, glyphs and gold permission rail",
			grey: true, w: 1320, h: 1000,
			body: func(t *testing.T) string { return host(1232, fxShotAlmanac(t, true)) },
		},
		{
			file: "09-fault-and-stubs.png", title: "Honesty states",
			caption: "the fault prints WHERE THE DATE WOULD GO and no date element is emitted. Both docked zones now carry CONTENT rather than the signed `needs backend` chip — W-B filled the Ledger and W-E the Shelf — and the Ledger still LISTS in the fault case, because listing by ordinal day needs no era, no epoch and no reckoning",
			w:       1320, h: 900,
			body: func(t *testing.T) string { return host(1232, fxDwarvenFault()) },
		},
	}

	css := blockCSS(t)
	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			page := shotPage(s, css, s.body(t))
			dir := t.TempDir()
			src := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write page: %v", err)
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, chrome,
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				"--force-device-scale-factor=2",
				fmt.Sprintf("--window-size=%d,%d", s.w, s.h),
				"--virtual-time-budget=4000",
				"--screenshot="+out, "file://"+src,
			)
			if combined, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("chromium screenshot: %v\n%s", err, combined)
			}
			fi, err := os.Stat(out)
			if err != nil || fi.Size() == 0 {
				t.Fatalf("screenshot %s was not written", out)
			}
			t.Logf("wrote %s (%d bytes)", out, fi.Size())
		})
	}
}

var linkRe = regexp.MustCompile(`<link[^>]*>`)

// stripLink removes the AssetURL <link>: file:// cannot resolve /static/, and
// inlining the sheet guarantees the shot is of THIS stylesheet rather than of a
// stale build artefact.
func stripLink(markup string) string { return linkRe.ReplaceAllString(markup, "") }

func shotPage(s shot, css, body string) string {
	cls := ""
	if s.dark {
		cls = ` class="dark"`
	}
	grey := ""
	if s.grey {
		grey = `.shot-body{filter:grayscale(1)}`
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:"Inter",ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		`.shot-wrap{padding:20px}` +
		`h1{font-size:20px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:12px;line-height:1.5;margin:0 0 14px;opacity:.72}` +
		`.shot-sub{font-size:12px;line-height:1.5;margin:14px 0 6px;opacity:.72}` +
		`.shot-host{display:block}` +
		grey +
		css +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + s.title + `</h1><p class="shot-cap">` + s.caption + `</p>` +
		`<div class="shot-body">` + body + `</div>` +
		`</div></body></html>`
}

// fxShotAlmanac is fxLedgerFull with the Almanac register attached, so the
// generated shots photograph the zone the slice actually ships rather than the
// zone a moonless calendar gets.
//
// It reuses the four-moon test fixture deliberately: Sable is the body past the
// grid's ceiling, and the whole point of these shots is that it is visible.
func fxShotAlmanac(t *testing.T, gm bool) BlockData {
	t.Helper()
	d := fxAlmanac(t, gm)
	d.Ledger = LedgerStub{NeedsBackend: false}
	d.Shelf = ShelfStub{NeedsBackend: false}
	if gm {
		d.Viewer.HiddenCount = 3
	}

	// THE SHOTS CARRY A REAL ILLUMINATION CURVE, not the flat test constants.
	// almanac_test.go's fixture holds Illum fixed because its assertions are
	// about strings and thresholds, and a curve there would make them read like
	// arithmetic they are not allowed to do. A SCREENSHOT is fidelity evidence,
	// though, and a flat bar would be evidence of a shape the producer never
	// emits — so the same phase arithmetic the producer runs is run here, in
	// the one place a widget-package file legitimately may: a picture of the
	// data, not a claim about it.
	for i := range d.Month.Almanac {
		m := &d.Month.Almanac[i]
		offset := []float64{-12, 4.25, -3, -41}[i%4]
		m.TurnsThisMonth = 0
		for j := range m.Days {
			day := float64(m.Days[j].Day)
			ph := func(x float64) float64 {
				raw := (x + offset) / m.PeriodDays
				f := raw - math.Floor(raw)
				if f < 0 {
					f++
				}
				return f
			}
			p := ph(day)
			m.Days[j].Illum = (1 - math.Cos(2*math.Pi*p)) / 2
			m.Days[j].Turn = ""
			a, b := ph(day-0.5), ph(day+0.5)
			switch {
			case b < a:
				m.Days[j].Turn = "new"
			case a < 0.5 && b >= 0.5:
				m.Days[j].Turn = "full"
			}
			if m.Days[j].Turn != "" {
				m.TurnsThisMonth++
			}
		}
	}
	return d
}
