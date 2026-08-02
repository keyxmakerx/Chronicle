// builder_screenshot_gen_test.go — THE FIDELITY EVIDENCE, and the two
// measurements that a still cannot make.
//
// This file is the sibling of
// internal/widgets/calendar_block/screenshot_gen_test.go and deliberately
// extends that pattern rather than inventing a second evidence mechanism. The
// generator is inert unless BUILDER_SCREENSHOTS names an output directory:
//
//	BUILDER_SCREENSHOTS=/tmp/shots go test ./internal/plugins/calendar/ -run BuilderScreenshots
//
// Every shot is captioned with its station, theme, viewport and the preview
// host's resolved size class, so a coordinator gating fidelity reads the
// arithmetic off the image instead of taking this PR's word for it.
//
// ── WHY THIS WAVE CANNOT GATE ON STILLS ALONE ─────────────────────────────
//
// The deliverable is partly MOTION, and a PNG of a transition is a PNG of
// nothing. So two probes live here beside the generator, and both SKIP HONESTLY
// rather than passing when they cannot run — under -short (which is CI's mode)
// or with no Chromium present, with a skip message that names what was missing:
//
//	TestBuilderProbe_TheMonthDidNotMove — canon A8 L-M2, reproduced as the
//	    document.getAnimations() measurement mkgallery.py recorded: across a
//	    PRESET SWAP, the count of animations running inside the month grid must
//	    be ZERO. A non-zero count is a STOP-AND-FLAG, not a tuning.
//	TestBuilderProbe_NarrowLaneHoldsItsGate — measured zero horizontal scroll
//	    from 390 to 1440 and a 24px hit-area floor, the standard every other v4
//	    surface was gated on ([WZ-14] SIGNED AS MODIFIED).
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// builderShotChromium finds a Chromium the same way the Block's probe does.
func builderShotChromium() string {
	if p := os.Getenv("CHROMIUM_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, pattern := range []string{
		"/opt/pw-browsers/chromium-*/chrome-linux/chrome",
		"/opt/pw-browsers/chromium_headless_shell-*/chrome-linux/headless_shell",
		filepath.Join(os.Getenv("HOME"), ".cache/ms-playwright/chromium-*/chrome-linux/chrome"),
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}

func builderSheet(t *testing.T) string {
	t.Helper()
	return builderCSSRaw(t)
}

// builderBlockSheet is the Block's own stylesheet, inlined into the harness so
// the preview renders as it really does. It is READ, never written: this wave
// touches no file under internal/widgets/calendar_block/.
func builderBlockSheet(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(builderRepoRoot(t), "static", "css", "calendar-block.css"))
	if err != nil {
		t.Fatalf("read calendar-block.css: %v", err)
	}
	return string(body)
}

func builderRepoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(filepath.Dir(builderCSSPath(t))))
}

// builderRenderShell renders one wizard state as the shell fragment, which is
// what a station change actually swaps in.
func builderRenderShell(t *testing.T, d *builderDraft, step int, importer bool, pvMonth int) string {
	t.Helper()
	data := builderView("camp-1", "tok", d, step, importer, pvMonth)
	var sb strings.Builder
	if err := BuilderShellFragment(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render shell: %v", err)
	}
	return sb.String()
}

var builderLinkRe = regexp.MustCompile(`<link[^>]*>`)

// builderStripLink removes the AssetURL <link>: file:// cannot resolve /static/,
// and inlining guarantees the shot is of THESE stylesheets rather than of a
// stale build artefact.
func builderStripLink(markup string) string { return builderLinkRe.ReplaceAllString(markup, "") }

// builderHarness wraps rendered markup in a page carrying both sheets.
//
// reduced is the `prefers-reduced-motion: reduce` still's switch. It reproduces
// CHRONICLE'S OWN global guard verbatim (static/css/input.css:31-47) rather than
// the mockup's token-zeroing block, because Chronicle's is what actually ships
// and [WZ-9] refused the port.
func builderHarness(t *testing.T, title, caption, body string, dark, reduced bool) string {
	t.Helper()
	cls := ""
	if dark {
		cls = ` class="dark"`
	}
	reduce := ""
	if reduced {
		reduce = `@media (prefers-reduced-motion: reduce){*,*::before,*::after{` +
			`animation-duration:0.01ms !important;animation-iteration-count:1 !important;` +
			`transition-duration:0.01ms !important;scroll-behavior:auto !important}}`
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:"Inter",ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		`.shot-wrap{padding:16px}` +
		`h1{font-size:18px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:11px;line-height:1.5;margin:0 0 12px;opacity:.72}` +
		builderBlockSheet(t) + builderSheet(t) + reduce +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + title + `</h1><p class="shot-cap">` + caption + `</p>` +
		`<div class="cal-builder" data-cal-builder>` + builderStripLink(body) + `</div>` +
		`</div></body></html>`
}

// builderFrameHarness renders `page` inside an iframe of exactly innerW CSS
// pixels.
//
// HEADLESS CHROMIUM CLAMPS ITS WINDOW TO 500px WIDE — measured, not assumed:
// --window-size=390 yields clientWidth 500 in every headless mode this build
// offers. A 390px reading taken in the window would therefore be a 500px
// reading wearing a 390px label, which is worse than no reading. An iframe's
// viewport is its own, and media queries inside it resolve against it, so the
// frame is the instrument. The repo's calendar_v2 mobile probe uses the same
// technique for the same reason.
func builderFrameHarness(t *testing.T, dir, page string, innerW, innerH int) string {
	t.Helper()
	inner := filepath.Join(dir, "inner.html")
	if err := os.WriteFile(inner, []byte(page), 0o644); err != nil {
		t.Fatalf("write inner page: %v", err)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb}` +
		`iframe{border:0;display:block;width:` + fmt.Sprint(innerW) + `px;height:` +
		fmt.Sprint(innerH) + `px}</style></head><body>` +
		`<pre id="probe"></pre><iframe src="inner.html"></iframe>` +
		`<script>addEventListener('message',function(e){` +
		`document.getElementById('probe').textContent = e.data;});</script>` +
		`</body></html>`
}

// builderShot is one captioned still.
type builderShot struct {
	file    string
	title   string
	caption string
	dark    bool
	reduced bool
	w, h    int
	// frame, when non-zero, renders the page inside an iframe of exactly that
	// many CSS pixels. See builderFrameHarness: headless Chromium will not give
	// a window narrower than 500px, so 390 is only reachable in a frame.
	frame int
	body  func(t *testing.T) string
}

// builderShotStates is the state behind each shot key, and it is the mockup's
// own SECTIONS table: one independent state per station so the sheets stay
// comparable, plus the fault sheet, which is Structure with a month emptied and
// the preview parked on it.
func builderShotState(t *testing.T, key string) (*builderDraft, int, bool, int) {
	t.Helper()
	d, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	switch key {
	case "presets", "start":
		return d, 0, false, 0
	case "step-structure":
		return d, 1, false, 0
	case "step-week":
		return d, 2, false, 0
	case "step-intercalary":
		return d, 3, false, 0
	case "step-leap":
		return d, 4, false, 0
	case "step-moons":
		return d, 5, false, 0
	case "step-seasons":
		return d, 6, false, 0
	case "step-eras":
		return d, 7, false, 0
	case "step-review":
		return d, 8, false, 0
	case "importer":
		return d, 0, true, 0
	case "fault":
		// THE FAULT SHEET, exactly as the mockup composes it: Structure with the
		// third month emptied and the preview parked on it. It is a deliberate
		// TWO-HONESTY-STATES-AT-ONCE composition and it is RATIFIED as drawn
		// ([WZ-15] item 5) — the anchor bar names the fault while the Year-length
		// stat prints its own warn ink, because the two are answering different
		// questions.
		for i := range d.Months {
			if !d.Months[i].Intercalary && d.Months[i].Name == "Ches" {
				d.Months[i].Days = 0
				return d, 1, false, i
			}
		}
	case "rail-sticky":
		return d, 5, false, 0
	}
	return d, 0, false, 0
}

// builderShotKeys is the mockup's own shot-key roster.
var builderShotKeys = []string{
	"presets", "step-structure", "step-week", "step-intercalary", "step-leap",
	"step-moons", "step-seasons", "step-eras", "step-review", "importer", "fault",
}

// builderMobileKeys is the 390px roster the signed mobile pass drew.
var builderMobileKeys = []string{
	"start", "presets", "step-structure", "step-moons", "step-eras",
	"step-review", "importer", "fault", "rail-sticky",
}

// builderShotCaption prints the arithmetic the coordinator reads off the image.
//
// The preview host chain at 390 is stated once and re-derived here so a caption
// can never disagree with the sheet: 390 host → 16px gutter each side → 358
// page → 1px shell border each side → 356 → 10px panel padding each side → 336
// preview host, and sizeClass(336) is std — the signed threshold is 300, so the
// month renders exactly as it does at 1280.
func builderShotCaption(key string, vw int, d *builderDraft) string {
	host := vw - 32 - 2 - 20
	if vw >= 1101 {
		// desktop: 1280 page − 186 rail − 442 panel, then the column's padding
		host = vw - 186 - 442 - 32
	}
	tier := "submini"
	switch {
	case host >= 900:
		tier = "full"
	case host >= 300:
		tier = "std"
	case host >= 240:
		tier = "mini"
	}
	col := 0.0
	if len(d.Weekdays) > 0 {
		col = float64(host) / float64(len(d.Weekdays))
	}
	return fmt.Sprintf(
		"station <b>%s</b> · viewport %dpx · preview host ≈%dpx → size class <b>%s</b> · "+
			"%d columns at ≈%.1fpx · the preview is the SHIPPED Block, not a second renderer",
		key, vw, host, tier, len(d.Weekdays), col)
}

// TestGenerateBuilderScreenshots writes the fidelity set.
func TestGenerateBuilderScreenshots(t *testing.T) {
	outDir := os.Getenv("BUILDER_SCREENSHOTS")
	if outDir == "" {
		t.Skip("screenshot generator: set BUILDER_SCREENSHOTS=<dir> to run")
	}
	chrome := builderShotChromium()
	if chrome == "" {
		t.Skip("screenshot generator: no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	var shots []builderShot
	for _, key := range builderShotKeys {
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			shots = append(shots, builderShot{
				file:  fmt.Sprintf("builder-wizard--%s--%s.png", key, theme),
				title: fmt.Sprintf("The builder wizard · %s · %s", key, theme),
				dark:  dark, w: 1440, h: 1100,
				caption: func(k string) string {
					d, _, _, _ := builderShotState(t, k)
					return builderShotCaption(k, 1280, d)
				}(key),
				body: func(k string) func(*testing.T) string {
					return func(t *testing.T) string {
						d, step, imp, pv := builderShotState(t, k)
						return builderRenderShell(t, d, step, imp, pv)
					}
				}(key),
			})
		}
	}
	for _, key := range builderMobileKeys {
		for _, dark := range []bool{false, true} {
			theme := "dark"
			if !dark {
				theme = "light"
			}
			shots = append(shots, builderShot{
				file: fmt.Sprintf("builder-wizard--%s--mobile-%s.png", key, theme),
				// CAPTURED AT 500px, NOT 390, AND THE TITLE SAYS SO. Headless
				// Chromium clamps its window to 500px wide (measured: every
				// headless mode this build offers returns clientWidth 500 for
				// --window-size=390), and a screenshot taken inside an iframe
				// renders at the frame's width but crops to the outer document's
				// box. 500 is INSIDE the same ≤640 narrow branch, so these show
				// the real narrow treatment — the strip rail, the stacked
				// preview, the full-width footer. The 390 measurements are in
				// TestBuilderProbe_NarrowLaneHoldsItsGate, which takes them
				// inside a frame where the viewport really is 390.
				title: fmt.Sprintf("The builder wizard · %s · narrow (500px window; the 390px readings are in the probe) · %s", key, theme),
				dark:  dark, w: builderMinHeadlessWindow, h: 1600,
				caption: func(k string) string {
					d, _, _, _ := builderShotState(t, k)
					return builderShotCaption(k, 390, d)
				}(key),
				body: func(k string) func(*testing.T) string {
					return func(t *testing.T) string {
						d, step, imp, pv := builderShotState(t, k)
						return builderRenderShell(t, d, step, imp, pv)
					}
				}(key),
			})
		}
	}
	// THE REDUCED-MOTION STILL, at Start and at one bent station. The claim it
	// evidences is BEHAVIOURAL: with reduced motion on, every station change,
	// preset pick and preview repaint lands INSTANTLY, CORRECTLY, with no layout
	// shift and no lost content. A still can only show the second half — that
	// the resting state is complete and legible with zero motion — and that is
	// exactly what these two are for.
	for _, key := range []string{"presets", "step-moons"} {
		shots = append(shots, builderShot{
			file:  fmt.Sprintf("builder-wizard--%s--reduced-motion.png", key),
			title: fmt.Sprintf("The builder wizard · %s · prefers-reduced-motion: reduce", key),
			caption: "under Chronicle's OWN global guard (input.css, outside every cascade layer) " +
				"the whole register is instant and complete — the still-state loses nothing",
			reduced: true, w: 1440, h: 1100,
			body: func(k string) func(*testing.T) string {
				return func(t *testing.T) string {
					d, step, imp, pv := builderShotState(t, k)
					return builderRenderShell(t, d, step, imp, pv)
				}
			}(key),
		})
	}

	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			dir := t.TempDir()
			page := builderHarness(t, s.title, s.caption, s.body(t), s.dark, s.reduced)
			if s.frame > 0 {
				page = builderFrameHarness(t, dir, page, s.frame, s.h-40)
			}
			src := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write page: %v", err)
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

// --- the two measurements a still cannot make ---------------------------------

var builderProbeRe = regexp.MustCompile(`(?s)<pre id="probe">(.*?)</pre>`)

// builderMinHeadlessWindow is the narrowest window this headless build will
// give, MEASURED rather than assumed: --window-size=390 yields clientWidth 500
// in every headless mode it offers. Narrower readings go through an iframe.
const builderMinHeadlessWindow = 500

// builderRunProbe loads a page in Chromium, lets its in-page script write a JSON
// reading into #probe, dumps the DOM and returns the JSON.
func builderRunProbe(t *testing.T, chrome, page string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "probe.html")
	if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1200", "--virtual-time-budget=6000",
		"--dump-dom", "file://"+src)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium probe: %v", err)
	}
	m := builderProbeRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("the probe wrote no reading; DOM was %d bytes", len(out))
	}
	return strings.NewReplacer("&quot;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">").
		Replace(string(m[1]))
}

// builderRunProbeAt is builderRunProbe at a stated viewport size.
func builderRunProbeAt(t *testing.T, chrome, page string, w, h int) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "probe.html")
	winW := w
	if w < builderMinHeadlessWindow {
		// Below the clamp, the frame is the instrument and the reading comes
		// back by postMessage.
		if err := os.WriteFile(src, []byte(builderFrameHarness(t, dir, page, w, h)), 0o644); err != nil {
			t.Fatalf("write probe page: %v", err)
		}
		winW = builderMinHeadlessWindow + 60
	} else if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu",
		fmt.Sprintf("--window-size=%d,%d", winW, h),
		"--virtual-time-budget=6000", "--dump-dom", "file://"+src)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium probe at %dpx: %v", w, err)
	}
	m := builderProbeRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("the probe wrote no reading at %dpx; DOM was %d bytes", w, len(out))
	}
	return strings.NewReplacer("&quot;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">").
		Replace(string(m[1]))
}

// TestBuilderProbe_TheMonthDidNotMove is canon A8 L-M2, measured.
//
// "A layer that changes the MONTH's geometry applies instantly and silently."
// The wizard's design ASKS for a preview that cross-fades and the Block sits
// inside it, so this is the single most likely place the Block's austerity is
// violated. §9's leak assertions catch it mechanically in the stylesheet; this
// catches it in a real engine, on the real trigger.
//
// THE TRIGGER IS REPRODUCED, NOT SIMULATED: a preset swap is an HTMX outerHTML
// replacement of #wz-shell, so the probe performs exactly that replacement and
// then counts document.getAnimations() whose target is inside the Block's host.
// A NON-ZERO COUNT IS A STOP-AND-FLAG.
func TestBuilderProbe_TheMonthDidNotMove(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode); run without -short")
	}
	chrome := builderShotChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	harptos, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatal(err)
	}
	elven, err := builderPresetDraft("elven")
	if err != nil {
		t.Fatal(err)
	}
	before := builderRenderShell(t, harptos, 0, false, 0)
	after := builderRenderShell(t, elven, 0, false, 0)

	// The replacement shell rides a <template>, which parses it into inert DOM
	// exactly as an HTMX swap would, without it rendering twice.
	script := `<pre id="probe"></pre><template id="next">` + builderStripLink(after) +
		`</template><script>
(function(){
  function inGrid(a){
    var t = a.effect && a.effect.target;
    if (!t || !t.closest) return false;
    // ANYTHING INSIDE THE BLOCK'S OWN SCOPING ROOT counts as "inside the month".
    return !!t.closest('[class*="cal-block-host"]');
  }
  function readNow(){
    var all = document.getAnimations();
    var inside = all.filter(inGrid);
    var wrapper = all.filter(function(a){
      var t = a.effect && a.effect.target;
      return t && t.classList && t.classList.contains('wz-pv');
    });
    document.getElementById('probe').textContent = JSON.stringify({
      total: all.length,
      insideGrid: inside.length,
      previewWrapper: wrapper.length,
      names: all.map(function(a){ return a.animationName || ''; })
    });
  }
  var shell = document.getElementById('wz-shell');
  var tpl = document.getElementById('next');
  var repl = tpl.content.querySelector('#wz-shell');
  if (!repl) {
    document.getElementById('probe').textContent =
      JSON.stringify({error:'the replacement shell did not parse'});
    return;
  }
  shell.replaceWith(document.importNode(repl, true));
  // Force a style + layout flush so any insertion animation is REGISTERED, then
  // read synchronously. A rAF callback does not reliably fire before
  // --dump-dom captures, and a probe that silently measured nothing would be
  // worse than no probe: it would read as a pass.
  void document.body.offsetHeight;
  readNow();
  // Read again a tick later and let the later reading win, so an animation that
  // only registers after the frame is still counted.
  setTimeout(readNow, 50);
})();
</script>`

	page := builderHarness(t, "month-did-not-move", "preset swap", before+script, false, false)
	raw := builderRunProbe(t, chrome, page)

	var got struct {
		Total          int      `json:"total"`
		InsideGrid     int      `json:"insideGrid"`
		PreviewWrapper int      `json:"previewWrapper"`
		Names          []string `json:"names"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("probe reading %q: %v", raw, err)
	}

	t.Logf("across a preset swap: %d animations running in total, %d INSIDE THE GRID, "+
		"%d on the preview's own wrapper; names=%v",
		got.Total, got.InsideGrid, got.PreviewWrapper, got.Names)

	if got.InsideGrid != 0 {
		t.Fatalf("STOP-AND-FLAG: %d animation(s) are running inside the month grid across "+
			"a preset swap. Canon A8 L-M2 requires a geometry change to apply INSTANTLY "+
			"AND SILENTLY; the cross-fade belongs to the wizard's own wrapper and the "+
			"cells, numerals, today pill, moon marks, weekday headers and era bands must "+
			"repaint in place. Names seen: %v", got.InsideGrid, got.Names)
	}
	for _, n := range got.Names {
		if !builderBudget.keyframes[n] && n != "" {
			t.Errorf("an animation named %q ran, and it is outside the four-keyframe budget", n)
		}
	}
}

// TestBuilderProbe_NarrowLaneHoldsItsGate is [WZ-14]'s gate, measured at the
// same widths every other v4 surface was gated on.
//
// "past the edge" is a visible, non-fixed element whose border box crosses the
// viewport's right edge by more than 0.6px; a "target" is a visible enabled
// interactive element measured against 24px on BOTH axes.
func TestBuilderProbe_NarrowLaneHoldsItsGate(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode); run without -short")
	}
	chrome := builderShotChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	d, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatal(err)
	}

	// ONE BROWSER PER WIDTH. Resizing a wrapper inside one 1600px window would
	// measure a fake right edge and would never fire a media query, so the
	// reading would be of a layout the product never has. The viewport IS the
	// instrument.
	// 390 is measured INSIDE A FRAME (see builderFrameHarness); everything from
	// 500 up is measured in a real window, because that is the narrowest window
	// this headless build will give.
	widths := []int{390, 500, 640, 768, 1024, 1280, 1440}

	for _, station := range []struct {
		key  string
		step int
		imp  bool
	}{{"presets", 0, false}, {"step-structure", 1, false},
		{"step-review", 8, false}, {"importer", 0, true}} {

		t.Run(station.key, func(t *testing.T) {
			body := builderRenderShell(t, d, station.step, station.imp, 0)
			script := `<pre id="probe"></pre><script>
(function(){
  var w = document.documentElement.clientWidth;
  var over = 0, small = 0, worst = null;
  // AN ELEMENT INSIDE A HORIZONTAL SCROLLER IS NOT OVERFLOW, IT IS CONTENT.
  // The rail becomes a sticky horizontal strip at the narrow breakpoint —
  // nine stations of track inside a 336px port — and the whole point of that
  // treatment is that the STRIP scrolls and the DOCUMENT does not. Counting
  // its off-screen buttons as overflow would fail the surface for doing
  // exactly what it was designed to do.
  function inScroller(el){
    for (var p = el.parentElement; p; p = p.parentElement) {
      var ox = getComputedStyle(p).overflowX;
      if (ox === 'auto' || ox === 'scroll') return true;
      if (p.classList && p.classList.contains('cal-builder')) return false;
    }
    return false;
  }
  document.querySelectorAll('.cal-builder *').forEach(function(el){
    var cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || cs.position === 'fixed') return;
    var r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) return;
    if (r.right > w + 0.6 && !inScroller(el)) {
      over++;
      if (!worst) worst = 'OVERFLOW ' + (el.className || el.tagName) +
        ' right=' + r.right.toFixed(1) + ' w=' + r.width.toFixed(1);
    }
    if (el.matches('button,a[href],input,select,textarea') && !el.disabled) {
      if (r.width > 0 && (r.width < 24 || r.height < 24)) {
        small++;
        if (!worst) worst = (el.className || el.tagName) + ' ' +
          r.width.toFixed(1) + 'x' + r.height.toFixed(1);
      }
    }
  });
  var reading = JSON.stringify({
    w: w, scrollW: document.documentElement.scrollWidth,
    clientW: document.documentElement.clientWidth,
    past: over, under24: small, worst: worst});
  var slot = document.getElementById('probe');
  if (slot) slot.textContent = reading;
  // When this page is the instrument's FRAME, the reading rides up to the
  // document --dump-dom actually captures.
  if (window.parent !== window) window.parent.postMessage(reading, '*');
})();
</script>`
			page := builderHarness(t, "narrow lane", station.key, body+script, false, false)

			for _, w := range widths {
				raw := builderRunProbeAt(t, chrome, page, w, 1400)
				var r struct {
					W       int     `json:"w"`
					ScrollW int     `json:"scrollW"`
					ClientW int     `json:"clientW"`
					Past    int     `json:"past"`
					Under24 int     `json:"under24"`
					Worst   *string `json:"worst"`
				}
				if err := json.Unmarshal([]byte(raw), &r); err != nil {
					t.Fatalf("probe reading %q: %v", raw, err)
				}
				worst := ""
				if r.Worst != nil {
					worst = " · worst: " + *r.Worst
				}
				drag := r.ScrollW - r.ClientW
				t.Logf("%s @%dpx: scrollWidth %d vs clientWidth %d (%dpx of sideways drag) · "+
					"%d past the edge · %d target(s) under 24px%s",
					station.key, w, r.ScrollW, r.ClientW, drag, r.Past, r.Under24, worst)

				if drag > 0 {
					t.Errorf("%s @%dpx: %dpx of horizontal scroll — the gate is measured ZERO "+
						"across 390–1440", station.key, w, drag)
				}
				if r.Past != 0 {
					t.Errorf("%s @%dpx: %d element(s) past the viewport's right edge",
						station.key, w, r.Past)
				}
				if r.Under24 != 0 {
					t.Errorf("%s @%dpx: %d interactive target(s) under the 24px floor%s",
						station.key, w, r.Under24, worst)
				}
			}
		})
	}
}

// TestBuilderPage_IsWholeInOneRender is the cheap sanity the two browser probes
// cannot be relied on for, because both SKIP under -short and CI runs -short.
// It runs everywhere and asserts that every station renders at all.
func TestBuilderPage_IsWholeInOneRender(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Imix"}}
	for _, key := range append(append([]string{}, builderShotKeys...), "rail-sticky") {
		t.Run(key, func(t *testing.T) {
			d, step, imp, pv := builderShotState(t, key)
			data := builderView("camp-1", "tok", d, step, imp, pv)
			var sb strings.Builder
			if err := BuilderPage(cc, data).Render(context.Background(), &sb); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := sb.String()
			if !strings.Contains(html, `id="wz-shell"`) || !strings.Contains(html, `id="wz-live"`) {
				t.Error("the shell and the preview must both be present in every station")
			}
			if !strings.Contains(html, "cal-block-host") {
				t.Error("the shipped Block must render in every station's preview")
			}
			if key == "fault" {
				if !strings.Contains(html, "Cannot resolve a date") {
					t.Error("the fault sheet must draw the fault where the date would go")
				}
				if !strings.Contains(html, "unresolvable while") {
					t.Error("the Year-length stat must print in warn ink beside it — the " +
						"two-honesty-states composition is RATIFIED as drawn ([WZ-15] item 5)")
				}
			}
		})
	}
}
