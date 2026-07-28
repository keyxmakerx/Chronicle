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

// fxLedgerFull is the signed month with the docked Ledger switched on — the
// layer set both production hosts pass (entityBlockLayers / benchBlockLayers).
func fxLedgerFull(gm bool) BlockData {
	d := fxHarptos(gm)
	d.Layers = LayerState{Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"}}
	d.Ledger = LedgerStub{NeedsBackend: false}
	d.Shelf = ShelfStub{NeedsBackend: true}
	if gm {
		d.Viewer.HiddenCount = 3 // the three dm_only events in the signed month
	}
	return d
}

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
		return fmt.Sprintf(`<div class="shot-host" style="width:%dpx">%s</div>`,
			width, stripLink(render(t, d)))
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
			caption: "choosing a day repaints a panel that was already on screen. The Block's DECLARED HEIGHT IS IDENTICAL to shot 10 — measured, not asserted (TestProbe_LedgerHeightIsInvariantUnderSelection). `✕ all` is the reserved head slot, revealed by visibility and never by display.",
			w: 1320, h: 900,
			body: func(t *testing.T) string {
				return fmt.Sprintf(`<div class="shot-host" style="width:1232px">%s</div>`,
					pickDay(t, stripLink(render(t, fxLedgerFull(true))), "5"))
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
			file: "09-fault-and-stubs.png", title: "Honesty states",
			caption: "the fault prints WHERE THE DATE WOULD GO and no date element is emitted; the Ledger and Shelf zones are docked at their real size with the signed `needs backend` chip",
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
