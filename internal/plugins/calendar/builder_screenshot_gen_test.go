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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
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

// builderSimpleCalendarExport is a REAL Simple Calendar v1 export of Harptos,
// assembled from the shipped sc* structs so it cannot drift from the parser,
// and it exists because the signed `importer` still is the POST-DROP state.
//
// The mockup's importer sheet shows "DETECTED · HARPTOS-OF-IMIX.JSON — SIMPLE
// CALENDAR EXPORT" and the eight-row mapping table beneath it, ending on the
// blocking `eras · add before create` row — which §1 calls the importer's own
// honesty mechanism. A generator with no file to drop photographs the pre-drop
// state instead, so the one sheet the mapping table lives on evidenced
// everything EXCEPT the mapping table.
//
// Simple Calendar genuinely carries no eras (parseSimpleCalendarInner never
// populates result.Eras), so this payload dead-ends on the era gate for a real
// reason and the blocking row is a fact rather than a staging.
func builderSimpleCalendarExport(t *testing.T, d *builderDraft) []byte {
	t.Helper()
	cal := scCalendar{
		Name:     d.Name,
		LeapYear: scLeapYear{Rule: "custom", CustomMod: d.LeapEvery},
		Time:     scTime{HoursInDay: 24, MinutesInHour: 60, SecondsInMinute: 60},
		Year:     scYear{NumericRepresentation: d.Year, Postfix: " " + d.EpochName},
	}
	for i, m := range d.Months {
		cal.Months = append(cal.Months, scMonth{
			Name: m.Name, NumericRepresentation: i + 1,
			NumberOfDays: m.Days, NumberOfLeapYearDays: m.Days + m.LeapDays,
			Intercalary: m.Intercalary,
		})
	}
	for i, w := range d.Weekdays {
		cal.Weekdays = append(cal.Weekdays, scWeekday{Name: w, NumericRepresentation: i + 1})
	}
	for _, m := range d.Moons {
		cal.Moons = append(cal.Moons, scMoon{Name: m.Name, CycleLength: m.Period})
	}
	for _, se := range d.Seasons {
		cal.Seasons = append(cal.Seasons, scSeason{
			Name: se.Name, StartingMonth: se.StartMonth - 1, StartingDay: se.StartDay - 1,
			Color: se.Color,
		})
	}
	body, err := json.Marshal(scData{Calendar: cal})
	if err != nil {
		t.Fatalf("marshal the Simple Calendar export: %v", err)
	}
	return body
}

// builderRenderShot renders one shot key's markup, including the importer's
// post-drop state, which is what the signed importer still actually draws.
func builderRenderShot(t *testing.T, key string) string {
	t.Helper()
	d, step, imp, pv := builderShotState(t, key)
	data := builderView("camp-1", "tok", d, step, imp, pv)
	if key == "importer" {
		// Mirrors BuilderPreviewAPI's upload branch exactly: the file is
		// parsed, BECOMES the draft, and only then is the mapping read off it.
		res, err := DetectAndParse(builderSimpleCalendarExport(t, d))
		if err != nil {
			t.Fatalf("the importer fixture must parse through the SHIPPED parser: %v", err)
		}
		data = builderView("camp-1", "tok", builderDraftFromImport(res), step, imp, 0)
		data.Detected = fmt.Sprintf("harptos-of-imix.json — %s", builderFormatLabel(res.Format))
		data.Mapping = builderMappingFor(res, data.Draft)
	}
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
	return builderHarnessWith(t, title, caption, body, dark, reduced, builderStillSettle)
}

// builderStillSettle is the SETTLE the still generator applies, and the reason
// a still gate is worth anything.
//
// A frame taken while the register is mid-flight is not a still of the surface,
// it is a still of a moment. The verifier reproduced the whole set and found six
// of forty-two differing from the committed evidence for exactly that reason —
// same content, caught mid-pv-swap or mid-w-in — and a gate whose frames are
// non-deterministic is a weak artefact even when it happens to be green.
//
// So the stills are captured at REST. The block below is Chronicle's own global
// reduced-motion guard verbatim (static/css/input.css), minus the media query:
// every animation finishes in 0.01ms, so every frame is the resting state and
// two runs produce CONTENT-IDENTICAL images. It suppresses nothing about layout,
// colour or content, because all four keyframes end at the element's natural
// state — which builder_css_contract_test.go pins by keeping them to opacity,
// transform and clip-path.
//
// "CONTENT-IDENTICAL", NOT "BYTE-FOR-BYTE", and the difference was measured
// rather than hedged: regenerating the full set reproduces 39 of 42 files byte
// for byte, and three — `start--mobile-dark`, `step-structure--mobile-dark`,
// `step-week--light` — differ by a few hundred to a few thousand bytes, every
// differing pixel inside the generator's OWN caption line and every one of them
// glyph antialiasing. The settle fixed the real non-determinism (frames caught
// mid-pv-swap or mid-w-in); text rasterisation is not something a stylesheet
// controls, and claiming a byte guarantee the harness cannot give is how a
// gate's other claims start being read as approximate.
//
// THE MOTION IS NOT GATED BY THESE. It is gated by the five clips
// (TestGenerateBuilderClips), which is the split §12.1 asks for: "a PNG of a
// transition is a PNG of nothing".
const builderStillSettle = `*,*::before,*::after{animation-duration:0.01ms !important;` +
	`animation-delay:0ms !important;animation-iteration-count:1 !important;` +
	`transition-duration:0.01ms !important;scroll-behavior:auto !important}`

// builderHarnessWith is builderHarness with the trailing stylesheet named, so
// the clip generator can pass its own isolation block instead of the settle.
func builderHarnessWith(t *testing.T, title, caption, body string, dark, reduced bool, extra string) string {
	t.Helper()
	cls := ""
	if dark {
		cls = ` class="dark"`
	}
	reduce := extra
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

// builderShotCaption prints the arithmetic the coordinator reads off the image,
// FOR THE WIDTH THE IMAGE WAS ACTUALLY PHOTOGRAPHED AT.
//
// The narrow captions used to print the 390px chain under a 500px photograph:
// title said "500px window", caption said "viewport 390px · preview host
// ≈336px · 10 columns at ≈33.6px", and at the width really in the picture the
// chain is 500 − 32 − 2 − 20 = 446px and ≈44.6px a column. The tier conclusion
// survived — both are std — but §12.2's whole point is that the arithmetic is
// read OFF THE IMAGE, and on eighteen of the forty-two images the arithmetic
// described a different width than the pixels. A caption that is right about
// the conclusion and wrong about the numbers teaches a reader to stop checking.
//
// So the caption states the photographed width, and the 390px readings ride a
// SEPARATE probe line (builderProbeCaption) that names where they come from —
// TestBuilderProbe_NarrowLaneHoldsItsGate, which takes them inside a frame
// where the viewport really is 390, because headless Chromium will not give a
// window narrower than 500.
//
// The chain is stated once and re-derived here so a caption can never disagree
// with the sheet: viewport → 16px gutter each side → 1px shell border each side
// → 10px panel padding each side → preview host. sizeClass ≥300 is std, so the
// month renders at 390, at 500 and at 1280 the same way.
func builderShotCaption(key string, vw int, d *builderDraft) string {
	host, tier := builderPreviewHost(vw)
	col := 0.0
	if len(d.Weekdays) > 0 {
		col = float64(host) / float64(len(d.Weekdays))
	}
	return fmt.Sprintf(
		"station <b>%s</b> · viewport %dpx · preview host ≈%dpx → size class <b>%s</b> · "+
			"%d columns at ≈%.1fpx · the preview is the SHIPPED Block, not a second renderer",
		key, vw, host, tier, len(d.Weekdays), col)
}

// builderProbeCaption is the second caption line the narrow stills carry: the
// 390px arithmetic, labelled as the PROBE's reading rather than the photograph's.
func builderProbeCaption(d *builderDraft) string {
	host, tier := builderPreviewHost(390)
	col := 0.0
	if len(d.Weekdays) > 0 {
		col = float64(host) / float64(len(d.Weekdays))
	}
	return fmt.Sprintf(
		"<br>the 390px readings, MEASURED IN A FRAME by TestBuilderProbe_NarrowLaneHoldsItsGate "+
			"(headless Chromium will not give a window under %dpx): viewport 390px · "+
			"preview host ≈%dpx → size class <b>%s</b> · %d columns at ≈%.1fpx — same tier, "+
			"same treatment",
		builderMinHeadlessWindow, host, tier, len(d.Weekdays), col)
}

// builderPreviewHost derives the preview column's width and its size class from
// the viewport, for both lanes. One derivation, so the two caption lines and the
// probe cannot drift apart.
func builderPreviewHost(vw int) (int, string) {
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
	return host, tier
}

// builderMaxShotHeight caps a full-page capture. Nothing in the wizard is
// anywhere near it; it exists so a future station that accidentally grows an
// unbounded list cannot ask Chromium for a gigapixel PNG.
const builderMaxShotHeight = 6000

// builderMeasureHeight loads the page once and reads the document's own
// scrollHeight, so the capture that follows is the WHOLE page rather than the
// first window-height of it. Returns 0 when the reading cannot be taken, and
// the caller then keeps its declared height — a shot is better cropped than
// missing.
func builderMeasureHeight(t *testing.T, chrome, page, dir string, w int) int {
	t.Helper()
	probe := strings.Replace(page, "</body>",
		`<style>#probe{display:none}</style><pre id="probe"></pre><script>`+
			`addEventListener('load',function(){var w=document.querySelector('.shot-wrap');`+
			`var h=w?w.getBoundingClientRect().bottom+16:document.body.scrollHeight;`+
			`document.getElementById('probe').textContent=String(Math.ceil(h));});`+
			`</script></body>`, 1)
	src := filepath.Join(dir, "measure.html")
	if err := os.WriteFile(src, []byte(probe), 0o644); err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		// THE MEASURE RUNS AT THE CAPTURE'S OWN SCALE AND A TALL WINDOW.
		// Measured at a short viewport the reading came back ~40px under, and
		// the narrow captures lost their foot line to it — sticky offsets and
		// the device scale both move layout, so the instrument has to match the
		// photograph. The margin the caller adds is the belt on top of that.
		fmt.Sprintf("--window-size=%d,%d", w, builderMaxShotHeight),
		"--force-device-scale-factor=2",
		"--virtual-time-budget=4000", "--dump-dom", "file://"+src).Output()
	if err != nil {
		return 0
	}
	m := builderProbeRe.FindSubmatch(out)
	if m == nil {
		return 0
	}
	h := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(m[1])), "%d", &h); err != nil {
		return 0
	}
	return h
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
						return builderRenderShot(t, k)
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
					return builderShotCaption(k, builderMinHeadlessWindow, d) +
						builderProbeCaption(d)
				}(key),
				body: func(k string) func(*testing.T) string {
					return func(t *testing.T) string {
						return builderRenderShot(t, k)
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
					return builderRenderShot(t, k)
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
			// FULL PAGE, MEASURED FIRST. Chromium's --screenshot captures the
			// WINDOW, so a fixed height silently crops — the narrow set was
			// cut at 1600 CSS px and sliced the preview's divergence note
			// through "the moon…", losing the stats block, the Year-length
			// line and the foot line below the fold. The signed mobile stills
			// are full-page, so a gate below the fold was not a gate at all.
			// One extra load reads the document's own scrollHeight, and the
			// capture uses it.
			shotH := s.h
			if m := builderMeasureHeight(t, chrome, page, dir, s.w); m > 0 {
				shotH = m + 64
			}
			if shotH < s.h {
				shotH = s.h
			}
			if shotH > builderMaxShotHeight {
				shotH = builderMaxShotHeight
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, chrome,
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				"--force-device-scale-factor=2",
				fmt.Sprintf("--window-size=%d,%d", s.w, shotH),
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

// --- THE CLIPS — gate item §12.1(ii), and why they are not optional ----------
//
// [WZ-16] SIGNED requires "a clip per shipped motion pass, named against
// motion-policy §3", and §12.2 requires this file to state HOW they were
// captured, "because a clip with no provenance is not evidence". The reason is
// in §12.1's own words: the deliverable is partly motion and "a PNG of a
// transition is a PNG of nothing". A getAnimations() count — which
// TestBuilderProbe_TheMonthDidNotMove takes, and which is excellent — answers
// gate item (iv), not (ii). Dropping a signed gate item is a coordinator call.
//
// ── HOW THEY ARE CAPTURED, stated because the gate asks ───────────────────
//
// Chromium cannot record video, so the clip is BUILT rather than recorded, and
// that is a strength here: each frame is a separate headless load at a stated
// VIRTUAL time (--virtual-time-budget=N), so frame N is the register at exactly
// N milliseconds and two runs produce the same film. There is no wall clock and
// no dropped frame anywhere in the pipeline.
//
//	for N in 0,25,…,400ms:  chromium --virtual-time-budget=N --screenshot
//	each PNG -> image/jpeg (stdlib) -> ffmpeg -f image2pipe -c:v mjpeg
//	-> libvpx/webm at 40fps, so 1 frame = 25ms = the film's own time base
//
// The JPEG hop is not cosmetic: the ffmpeg this environment ships is Playwright's
// minimal build, which has a PNG *encoder* but no PNG *decoder* (it decodes
// mjpeg and nothing else). MJPEG is the only bridge available and it is lossy
// only in the JPEG sense — geometry, timing and colour placement are untouched.
//
// ── WHY EACH CLIP ISOLATES ONE PASS ───────────────────────────────────────
//
// A station change fires passes 1, 2 and 3 together, which is what the surface
// really does and what the mockup's three clips show. But the gate is a clip
// PER PASS, and three passes overlapping in one film cannot evidence any one of
// them. So each clip suppresses the other four passes at the harness level
// (animation:none) and lets one run. The composite is not lost: the register is
// the sum of these five and the stills show its resting state.
//
// Inert unless BUILDER_CLIPS names an output directory.

// builderPass is one shipped motion pass, its §3 pattern, and the selector that
// carries it — which is also how the other four are suppressed.
type builderPass struct {
	// n is the pass number in §5.2's table.
	n int
	// key names the file: pass<N>-<keyframe>--<§3 pattern>.
	key string
	// pattern is the motion-policy §3 vocabulary word this pass belongs to.
	pattern string
	// why is the caption burned into the clip's own page.
	why string
	// sel is the pass's prelude, verbatim from calendar-builder.css.
	sel string
	// station is the shot key the clip is filmed on.
	station string
}

// builderPasses is §5.2's table, in order, with its §3 mapping.
//
// ONE MAPPING NOTE, STATED RATHER THAN SMOOTHED OVER: §3's four patterns pair a
// meaning with a duration — enter is --t-base, exit is --t-fast. Passes 1, 3
// and 5 are ARRIVALS taken at --t-fast, which is exit's duration. That is not a
// mis-mapping, it is §5.2's design: pass 1 is "information never waits", and
// passes 3 and 5 are latches, which are state changes rather than travel and
// are named as ATTENTION here because that is what they say — "you are here",
// "this one is chosen". The pattern word names the JOB; the token names the
// duration; the two are allowed to disagree and the disagreement is the design.
var builderPasses = []builderPass{
	{1, "pass1-w-in-fine", "enter", "the station names itself first — 2px, --t-fast, delay 0",
		".cal-builder .wz-panel .wz-ph,.cal-builder .wz-panel .wz-ps", "step-moons"},
	{2, "pass2-w-in", "enter", "the station's content arrives in reading order — 3px, --t-base, on the --m-step ladder capped at --m-cap",
		".cal-builder .wz-panel .wz-frow,.cal-builder .wz-panel .wz-pgal," +
			".cal-builder .wz-panel .wz-igrid,.cal-builder .wz-panel .wz-note," +
			".cal-builder .wz-panel .wz-sum,.cal-builder .wz-panel .wz-chipswrap," +
			".cal-builder .wz-panel .wz-impdoor,.cal-builder .wz-panel .wz-dropz," +
			".cal-builder .wz-panel .wz-maptable,.cal-builder .wz-panel .wz-stepper.wz-big",
		"step-structure"},
	{3, "pass3-m-latch-rail", "attention", "\"you are here\" latches shut centre-to-corners — canon A6 forbids a travelling indicator",
		".cal-builder .wz-step.wz-cur::after", "step-moons"},
	{4, "pass4-pv-swap", "enter", "the preview is REPLACED, not rearranged — opacity only, no transform, no ladder; the month's interior never moves",
		".cal-builder .wz-pv", "step-structure"},
	{5, "pass5-m-latch-preset", "attention", "selection is identity, not elevation — the ring latches, the shadow does not change",
		".cal-builder .wz-pcard.wz-sel::after", "presets"},
}

// builderFFmpeg finds the encoder. Playwright ships one beside its browsers.
func builderFFmpeg() string {
	if p := os.Getenv("FFMPEG_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	for _, pattern := range []string{
		"/opt/pw-browsers/ffmpeg-*/ffmpeg-linux",
		filepath.Join(os.Getenv("HOME"), ".cache/ms-playwright/ffmpeg-*/ffmpeg-linux"),
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}

// builderIsolate suppresses every pass except `keep`, so one clip films one
// pass. It is `animation: none`, which leaves the element at its natural rest —
// the same reason the still settle is safe.
func builderIsolate(keep int) string {
	var sb strings.Builder
	for _, p := range builderPasses {
		if p.n == keep {
			continue
		}
		sb.WriteString(p.sel)
		sb.WriteString("{animation:none !important}")
	}
	return sb.String()
}

// builderClipFrames is the ladder of virtual times, in milliseconds. It runs
// past --t-long (400ms) so every clip ends at rest, and its step is the film's
// frame duration.
const (
	builderClipStep  = 25
	builderClipEnd   = 450
	builderClipFPS   = 1000 / builderClipStep
	builderClipW     = 1280
	builderClipH     = 860
	builderClipScale = 1
)

// TestGenerateBuilderClips writes one webm per shipped motion pass.
func TestGenerateBuilderClips(t *testing.T) {
	outDir := os.Getenv("BUILDER_CLIPS")
	if outDir == "" {
		t.Skip("clip generator: set BUILDER_CLIPS=<dir> to run")
	}
	chrome := builderShotChromium()
	if chrome == "" {
		t.Skip("clip generator: no Chromium binary found (set CHROMIUM_BIN)")
	}
	ff := builderFFmpeg()
	if ff == "" {
		t.Skip("clip generator: no ffmpeg found (set FFMPEG_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	for _, p := range builderPasses {
		t.Run(p.key, func(t *testing.T) {
			d, step, imp, pv := builderShotState(t, p.station)
			body := builderRenderShell(t, d, step, imp, pv)
			title := fmt.Sprintf("pass %d · %s · %s", p.n, p.key, p.pattern)
			caption := fmt.Sprintf("%s · station <b>%s</b> · motion-policy §3 pattern <b>%s</b> · "+
				"the other four passes are suppressed so this film is of ONE pass · "+
				"one frame = %dms of VIRTUAL time, so the film is reproducible",
				p.why, p.station, p.pattern, builderClipStep)
			page := builderHarnessWith(t, title, caption, body, false, false, builderIsolate(p.n))

			dir := t.TempDir()
			var film []byte
			for at := 0; at <= builderClipEnd; at += builderClipStep {
				src := filepath.Join(dir, fmt.Sprintf("f%04d.html", at))
				if err := os.WriteFile(src, []byte(builderSeekTo(page, at)), 0o644); err != nil {
					t.Fatalf("write frame page: %v", err)
				}
				shot := filepath.Join(dir, fmt.Sprintf("f%04d.png", at))
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				cmd := exec.CommandContext(ctx, chrome,
					"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
					fmt.Sprintf("--force-device-scale-factor=%d", builderClipScale),
					fmt.Sprintf("--window-size=%d,%d", builderClipW, builderClipH),
					"--virtual-time-budget=3000",
					"--screenshot="+shot, "file://"+src)
				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("frame at %dms: %v\n%s", at, err, out)
				}
				jpg, err := builderPNGToJPEG(shot)
				if err != nil {
					t.Fatalf("frame at %dms: %v", at, err)
				}
				film = append(film, jpg...)
			}

			out := filepath.Join(outDir, fmt.Sprintf("wizard--%s--%s.webm", p.key, p.pattern))
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			enc := exec.CommandContext(ctx, ff, "-y",
				"-f", "image2pipe", "-framerate", fmt.Sprint(builderClipFPS),
				"-c:v", "mjpeg", "-i", "pipe:",
				"-c:v", "libvpx", "-b:v", "3M", "-crf", "18", out)
			enc.Stdin = bytes.NewReader(film)
			if combined, err := enc.CombinedOutput(); err != nil {
				t.Fatalf("encode %s: %v\n%s", out, err, combined)
			}
			fi, err := os.Stat(out)
			if err != nil || fi.Size() == 0 {
				t.Fatalf("clip %s was not written", out)
			}
			t.Logf("wrote %s (%d bytes, %d frames at %dfps)",
				out, fi.Size(), (builderClipEnd/builderClipStep)+1, builderClipFPS)
		})
	}
}

// builderSeekTo returns the clip page with every animation PAUSED at exactly
// `at` milliseconds, which is what makes a built film reproducible.
//
// The first attempt sampled --virtual-time-budget instead, and it was jittery:
// virtual time also covers load and first style resolution, so the animation's
// start moved a frame or two between budgets and four sampled positions came
// back identical. The Web Animations API answers the question directly — pause
// every running animation and set its currentTime — and currentTime INCLUDES
// the delay phase, so pass 2's --m-step ladder films correctly: at t=0 a row
// four steps down the ladder is still in its `both` fill start state, which is
// exactly what the eye sees.
//
// The script is test-harness JS on a file:// page. It reaches no product code
// and ships nowhere near the widget package.
func builderSeekTo(page string, at int) string {
	return strings.Replace(page, "</body>",
		fmt.Sprintf(`<script>addEventListener('load',function(){`+
			`document.getAnimations().forEach(function(a){a.pause();a.currentTime=%d;});`+
			`});</script></body>`, at), 1)
}

// builderPNGToJPEG re-encodes one captured frame, because the ffmpeg available
// here decodes mjpeg and nothing else (see the header). Quality 92 — high
// enough that the 2px and 3px travels the register is made of survive it.
func builderPNGToJPEG(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // a path this test just wrote
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// TestBuilderClips_CoverEveryShippedPass runs everywhere and is the assertion
// the clip generator itself cannot make: the roster of clips is exactly the
// roster of passes in the stylesheet, so a sixth pass cannot ship unfilmed and
// a retired one cannot leave a stale film behind.
func TestBuilderClips_CoverEveryShippedPass(t *testing.T) {
	css := builderCSSRaw(t)
	if n := len(builderPasses); n != 5 {
		t.Fatalf("§5.2 tabulates FIVE passes and no sixth; the roster has %d", n)
	}
	seen := map[string]bool{}
	for _, p := range builderPasses {
		if seen[p.key] {
			t.Errorf("two clips would share the file name %q", p.key)
		}
		seen[p.key] = true
		// The pass's prelude must be in the sheet, or the clip films nothing.
		for _, sel := range strings.Split(p.sel, ",") {
			want := strings.TrimSpace(sel)
			if !strings.Contains(strings.Join(strings.Fields(css), " "),
				strings.Join(strings.Fields(want), " ")) {
				t.Errorf("pass %d names %q, which is not a prelude in the shipped sheet",
					p.n, want)
			}
		}
		switch p.pattern {
		case "enter", "exit", "attention", "ambient":
		default:
			t.Errorf("pass %d's pattern %q is not one of motion-policy §3's four",
				p.n, p.pattern)
		}
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
	return builderRunProbeFlags(t, chrome, page)
}

// builderRunProbeFlags is builderRunProbe with extra Chromium switches, so a
// probe can ask the engine for a different USER PREFERENCE rather than
// simulating one in CSS. `--force-prefers-reduced-motion` is the only caller
// today, and simulating reduced motion by injecting a media block would prove
// the injection worked, not that the shipped guard does.
func builderRunProbeFlags(t *testing.T, chrome, page string, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "probe.html")
	if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := append([]string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1200", "--virtual-time-budget=6000",
	}, extra...)
	args = append(args, "--dump-dom", "file://"+src)
	cmd := exec.CommandContext(ctx, chrome, args...)
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

	page := builderHarnessWith(t, "month-did-not-move", "preset swap", before+script, false, false, "")
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

// TestBuilderCaptions_DescribeThePhotographedWidth pins the fix for a caption
// that was arithmetic about a width the image is not.
//
// The narrow set is captured at a 500px window (headless Chromium will not give
// a narrower one) and its captions printed the 390px chain, so eighteen of the
// forty-two images stated a host width and a per-column measure for a different
// viewport than the pixels. §12.2's premise is that the coordinator reads the
// arithmetic OFF THE IMAGE; a caption that is right about the tier and wrong
// about the numbers teaches a reader to stop checking.
//
// This runs everywhere, needs no browser, and asserts both halves: the
// photograph's own numbers, and the 390px readings present and LABELLED as the
// probe's rather than the photograph's.
func TestBuilderCaptions_DescribeThePhotographedWidth(t *testing.T) {
	d, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatal(err)
	}
	shot := builderShotCaption("presets", builderMinHeadlessWindow, d)
	probe := builderProbeCaption(d)

	host, tier := builderPreviewHost(builderMinHeadlessWindow)
	if host != 446 || tier != "std" {
		t.Fatalf("the 500px chain is 500 − 32 − 2 − 20 = 446 → std; got %d → %s", host, tier)
	}
	for _, want := range []string{
		fmt.Sprintf("viewport %dpx", builderMinHeadlessWindow),
		fmt.Sprintf("preview host ≈%dpx", host),
		fmt.Sprintf("columns at ≈%.1fpx", float64(host)/float64(len(d.Weekdays))),
	} {
		if !strings.Contains(shot, want) {
			t.Errorf("the narrow caption must state the PHOTOGRAPHED width's arithmetic; "+
				"missing %q in %q", want, shot)
		}
	}
	if strings.Contains(shot, "viewport 390px") {
		t.Error("the photograph's own caption line states 390px — that is the probe's " +
			"reading and it belongs on the probe line, not on the picture's arithmetic")
	}
	// And the 390 numbers are still there, attributed.
	pHost, pTier := builderPreviewHost(390)
	for _, want := range []string{"viewport 390px",
		fmt.Sprintf("preview host ≈%dpx", pHost),
		"<b>" + pTier + "</b>",
		"TestBuilderProbe_NarrowLaneHoldsItsGate"} {
		if !strings.Contains(probe, want) {
			t.Errorf("the probe line must carry the 390px readings and say where they "+
				"come from; missing %q in %q", want, probe)
		}
	}
	if pTier != tier {
		t.Errorf("390 resolves to %s and 500 to %s — the narrow stills are only evidence "+
			"for 390 because both are the same tier", pTier, tier)
	}
}

// TestBuilderProbe_TheLadderActuallyRuns measures §5.2 pass 2's delay ladder in
// a real engine, in both motion directions.
//
// THE REASON THIS TEST EXISTS. The ladder rule was on disk and dead: declared
// before pass 2's `animation:` shorthand, at equal specificity, so the
// shorthand reset animation-delay to 0s and every row arrived together. Every
// static guard passed — the rule was present, in the right prelude, composing
// the right tokens — and the clip evidence was read as showing a stagger when
// it showed --t-base plus encoder residue. A rule that is present is not a
// mechanism that runs, and only the engine can tell the two apart.
//
// WHAT IS ASSERTED, all four of them the bound [WZ-8] signed:
//
//	(1) the ladder is not flat — the delays actually differ;
//	(2) they are non-decreasing in DOM order, which is reading order, and
//	    each step is one --m-step (≈33.3ms) per --m-i;
//	(3) nothing waits longer than --m-cap steps, ≈132ms, so no pass exceeds
//	    ~282ms with --t-base's 150 on top;
//	(4) the station's own title and subtitle (pass 1) still start at 0 —
//	    information never waits, even now that the ladder is live.
//
// And under `--force-prefers-reduced-motion` the engine reports no animation at
// all on the same nodes: instant, complete, in final position.
func TestBuilderProbe_TheLadderActuallyRuns(t *testing.T) {
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
	// Station 1 (structure) is the deepest ladder the wizard ever draws: twelve
	// month rows plus the intercalary rows, so the cap is genuinely exercised.
	body := builderRenderShell(t, harptos, 1, false, 0)

	script := `<pre id="probe"></pre><script>
(function(){
  function ms(v){
    v = (v || '').trim();
    if (!v) return 0;
    if (v.slice(-2) === 'ms') return parseFloat(v);
    if (v.slice(-1) === 's') return parseFloat(v) * 1000;
    return parseFloat(v) || 0;
  }
  var panel = document.querySelector('.wz-panel');
  var rows = [].slice.call(panel.querySelectorAll('[style*="--m-i"]'));
  var read = rows.map(function(n){
    var cs = getComputedStyle(n);
    return {
      cls: (n.className || '').split(' ')[0],
      i: parseInt(cs.getPropertyValue('--m-i'), 10),
      delay: ms(cs.animationDelay),
      name: cs.animationName,
      dur: ms(cs.animationDuration)
    };
  });
  var head = [].slice.call(panel.querySelectorAll('.wz-ph, .wz-ps')).map(function(n){
    var cs = getComputedStyle(n);
    return { cls: (n.className || '').split(' ')[0], delay: ms(cs.animationDelay),
             name: cs.animationName };
  });
  document.getElementById('probe').textContent =
    JSON.stringify({ rows: read, head: head, animations: document.getAnimations().length });
})();
</script>`

	page := builderHarnessWith(t, "the-ladder-actually-runs",
		"§5.2 pass 2 · computed animation-delay per --m-i", body+script, false, false, "")

	type node struct {
		Cls   string  `json:"cls"`
		I     int     `json:"i"`
		Delay float64 `json:"delay"`
		Name  string  `json:"name"`
		Dur   float64 `json:"dur"`
	}
	var got struct {
		Rows       []node `json:"rows"`
		Head       []node `json:"head"`
		Animations int    `json:"animations"`
	}
	if err := json.Unmarshal([]byte(builderRunProbe(t, chrome, page)), &got); err != nil {
		t.Fatalf("probe reading: %v", err)
	}
	if len(got.Rows) < 4 {
		t.Fatalf("the structure station laddered only %d nodes — the probe cannot see a "+
			"ladder that is not drawn", len(got.Rows))
	}

	// --m-step is ≈33.3ms (--t-fast/3) and --m-cap is 4, so the ceiling is
	// ≈133ms. The tolerance is one millisecond of the engine's own rounding,
	// not a slack allowance.
	const step, cap0, tol = 100.0 / 3.0, 4, 1.0
	ceiling := step*cap0 + tol

	var flat = true
	var prev float64 = -1
	for _, r := range got.Rows {
		want := step * float64(min(r.I, cap0))
		if diff := r.Delay - want; diff > tol || diff < -tol {
			t.Errorf("%s carries --m-i:%d and computes animation-delay %.1fms; the signed "+
				"ladder is min(--m-i, --m-cap) * --m-step = %.1fms. A delay of 0 on every row "+
				"means the `animation:` shorthand reset it — declare the ladder AFTER pass 2 "+
				"(coordinator ruling R1)", r.Cls, r.I, r.Delay, want)
		}
		if r.Delay > ceiling {
			t.Errorf("%s waits %.1fms — [WZ-8] caps the ladder at --m-cap steps, ≈%.1fms, so "+
				"that no pass exceeds ~282ms", r.Cls, r.Delay, ceiling)
		}
		if r.Delay > 0 {
			flat = false
		}
		if r.Delay+tol < prev {
			t.Errorf("%s waits %.1fms after a row that waited %.1fms — the ladder runs in "+
				"READING ORDER and never goes backwards", r.Cls, r.Delay, prev)
		}
		prev = r.Delay
	}
	if flat {
		t.Fatalf("STOP-AND-FLAG: every laddered row computes animation-delay 0. The rule is "+
			"on disk and does nothing, which is the state the second fix round shipped and "+
			"the evidence mis-read as a stagger. Rows seen: %+v", got.Rows)
	}
	for _, h := range got.Head {
		if h.Delay != 0 {
			t.Errorf("%s waits %.1fms — pass 1 is delay 0 BY DESIGN: information never waits, "+
				"the station names itself while its rows are still on the ladder", h.Cls, h.Delay)
		}
	}
	t.Logf("ladder measured in Chromium: %d laddered rows, delays %s; pass 1 head pair at 0ms",
		len(got.Rows), builderDelayList(got.Rows[:min(8, len(got.Rows))]))

	// ── THE OTHER DIRECTION, asked of the engine rather than simulated ───────
	reduced := builderRunProbeFlags(t, chrome, page, "--force-prefers-reduced-motion")
	var quiet struct {
		Rows       []node `json:"rows"`
		Animations int    `json:"animations"`
	}
	if err := json.Unmarshal([]byte(reduced), &quiet); err != nil {
		t.Fatalf("reduced-motion probe reading: %v", err)
	}
	if quiet.Animations != 0 {
		t.Errorf("under --force-prefers-reduced-motion the engine reports %d running "+
			"animation(s); the whole register lives inside %s and must be silent",
			quiet.Animations, builderBudget.guard)
	}
	for _, r := range quiet.Rows {
		if r.Name != "none" || r.Delay != 0 {
			t.Errorf("under reduced motion %s still has animation-name %q with delay %.1fms — "+
				"the rows are simply THERE, instantly and completely", r.Cls, r.Name, r.Delay)
		}
	}
	t.Logf("under --force-prefers-reduced-motion: %d animations, %d rows all animation-name "+
		"none at 0ms delay", quiet.Animations, len(quiet.Rows))
}

// builderDelayList renders the measured ladder for the log line.
func builderDelayList[T any](rows []T) string {
	b, _ := json.Marshal(rows)
	return string(b)
}

// TestBuilderProbe_NarrowLaneHoldsItsGate is [WZ-14]'s gate, measured at the
// same widths every other v4 surface was gated on.
//
// "past the edge" is a visible, non-fixed element whose border box crosses the
// viewport's right edge by more than 0.6px; a "target" is a visible enabled
// interactive element measured against 24px on BOTH axes; a "clipped" control
// is a text input whose own value does not fit inside it.
//
// ── IT WALKS ALL ELEVEN SHEETS, AND THAT IS THE POINT ──────────────────────
//
// It used to walk four — presets, structure, review, importer — and the report
// stated the 24px floor as if it held everywhere. The 0×26 moon-name control
// lived on step-moons, which the probe never visited, which is exactly why that
// bug shipped. A gate scoped to a subset states a fact about the subset; the
// claim was about the surface. So the roster IS builderShotKeys, and the two
// holes that let a zero-width control through are closed with it:
//
//	the 24px check skipped anything measuring 0 wide (`r.width > 0 &&`), so
//	the very worst case — a control with no width at all — was the one case
//	it exempted; and
//
//	nothing measured whether a control's VALUE fits inside it, so a field
//	clipping "Reckoning of Wards" to "Reckonir" mid-word read as passing.
func TestBuilderProbe_NarrowLaneHoldsItsGate(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode); run without -short")
	}
	chrome := builderShotChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	// ONE BROWSER PER WIDTH. Resizing a wrapper inside one 1600px window would
	// measure a fake right edge and would never fire a media query, so the
	// reading would be of a layout the product never has. The viewport IS the
	// instrument.
	// 390 is measured INSIDE A FRAME (see builderFrameHarness); everything from
	// 500 up is measured in a real window, because that is the narrowest window
	// this headless build will give.
	widths := []int{390, 500, 640, 768, 1024, 1280, 1440}

	for _, key := range builderShotKeys {
		t.Run(key, func(t *testing.T) {
			sd, step, imp, pv := builderShotState(t, key)
			body := builderRenderShell(t, sd, step, imp, pv)
			script := `<pre id="probe"></pre><script>
(function(){
  var w = document.documentElement.clientWidth;
  var over = 0, small = 0, clipped = 0, worst = null, worstClip = null;
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
      // NO 'r.width > 0' GUARD. A control measuring zero wide is the WORST
      // case of an under-floor target, not an exempt one — that guard is why
      // a 0x26 moon-name input passed this probe on the station it lived on.
      if (r.width < 24 || r.height < 24) {
        small++;
        if (!worst) worst = (el.name || el.className || el.tagName) + ' ' +
          r.width.toFixed(1) + 'x' + r.height.toFixed(1);
      }
      // AND A CONTROL MUST SHOW WHAT IT HOLDS. scrollWidth past clientWidth is
      // an input whose own value is cut off inside it: "Reckoning of Wards"
      // rendering as "Reckonir" is a printed lie about authored data, and no
      // width measurement catches it.
      if (el.matches('input[type=text]') && el.scrollWidth > el.clientWidth + 1) {
        clipped++;
        if (!worstClip) worstClip = (el.name || el.tagName) + ' value=' +
          JSON.stringify(el.value) + ' needs ' + el.scrollWidth +
          'px, has ' + el.clientWidth + 'px';
      }
    }
  });
  var reading = JSON.stringify({
    w: w, scrollW: document.documentElement.scrollWidth,
    clientW: document.documentElement.clientWidth,
    past: over, under24: small, clipped: clipped,
    worst: worst, worstClip: worstClip});
  var slot = document.getElementById('probe');
  if (slot) slot.textContent = reading;
  // When this page is the instrument's FRAME, the reading rides up to the
  // document --dump-dom actually captures.
  if (window.parent !== window) window.parent.postMessage(reading, '*');
})();
</script>`
			page := builderHarnessWith(t, "narrow lane", key, body+script, false, false, "")

			for _, w := range widths {
				raw := builderRunProbeAt(t, chrome, page, w, 1400)
				var r struct {
					W         int     `json:"w"`
					ScrollW   int     `json:"scrollW"`
					ClientW   int     `json:"clientW"`
					Past      int     `json:"past"`
					Under24   int     `json:"under24"`
					Clipped   int     `json:"clipped"`
					Worst     *string `json:"worst"`
					WorstClip *string `json:"worstClip"`
				}
				if err := json.Unmarshal([]byte(raw), &r); err != nil {
					t.Fatalf("probe reading %q: %v", raw, err)
				}
				worst := ""
				if r.Worst != nil {
					worst = " · worst: " + *r.Worst
				}
				drag := r.ScrollW - r.ClientW
				clip := ""
				if r.WorstClip != nil {
					clip = " · worst clip: " + *r.WorstClip
				}
				t.Logf("%s @%dpx: scrollWidth %d vs clientWidth %d (%dpx of sideways drag) · "+
					"%d past the edge · %d target(s) under 24px · %d clipped value(s)%s%s",
					key, w, r.ScrollW, r.ClientW, drag, r.Past, r.Under24, r.Clipped,
					worst, clip)

				if drag > 0 {
					t.Errorf("%s @%dpx: %dpx of horizontal scroll — the gate is measured ZERO "+
						"across 390–1440", key, w, drag)
				}
				if r.Past != 0 {
					t.Errorf("%s @%dpx: %d element(s) past the viewport's right edge",
						key, w, r.Past)
				}
				if r.Under24 != 0 {
					t.Errorf("%s @%dpx: %d interactive target(s) under the 24px floor%s",
						key, w, r.Under24, worst)
				}
				if r.Clipped != 0 {
					t.Errorf("%s @%dpx: %d control(s) cannot show their own value%s",
						key, w, r.Clipped, clip)
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
			if key != "fault" && !strings.Contains(html, "cal-block-host") {
				t.Error("the shipped Block must render in every station's preview")
			}
			if key == "fault" {
				// ── THE COMPOSITION [WZ-15] ITEM 5 RATIFIES AS DRAWN, AND THIS
				// ASSERTION USED TO CONTRADICT IT.
				//
				// It demanded "Cannot resolve a date" on this sheet, which is
				// what the build printed: a warn rail replacing the anchor bar
				// while the Block's own Nameplate, a hundred-odd pixels below,
				// printed "Hammer 1, RoW 1523". The date resolves. The signed
				// still keeps the ANCHOR ("TODAY RESOLVES TO …") beside the
				// warn-ink Year length and puts the fault INSIDE the grid, and
				// that two-honesty-states-at-once composition is the ruling.
				//
				// INVERTED, NOT DELETED: the sheet must still say the thing is
				// broken — in the two places that know it — and it must no
				// longer say the thing that is not.
				if !strings.Contains(html, "today resolves to") {
					t.Error("the anchor must state what today resolves to — today's month " +
						"is intact and the anchor's question is about today ([WZ-15] item 5)")
				}
				if strings.Contains(html, "Cannot resolve a date") {
					t.Error("today RESOLVES on this sheet — a headline claiming otherwise is " +
						"the claim-about-the-model falsehood [WZ-3] was signed to remove")
				}
				if !strings.Contains(html, "Nothing to draw — Ches declares 0 days") ||
					!strings.Contains(html, "The year cannot be walked past 30 Alturiak.") {
					t.Error("the fault belongs where the grid would be, in the signed copy (§6.2)")
				}
				if strings.Contains(html, "cal-block-host") {
					t.Error("the Block must not be asked to draw a month with no days — an " +
						"empty grid shape is the placeholder §6.2 refuses")
				}
				if !strings.Contains(html, "unresolvable while") {
					t.Error("the Year-length stat must print in warn ink beside it — the " +
						"two-honesty-states composition is RATIFIED as drawn ([WZ-15] item 5)")
				}
			}
		})
	}
}
