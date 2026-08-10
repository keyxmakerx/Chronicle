// sky_close_probe_test.go — THE CLOSE, sampled over the frames it claims to
// occupy rather than asserted from a `transition` declaration.
//
// WHY THE SHIPPED GUARDS WERE ALL GREEN. bench_test.go reads the declarations
// and confirms the sky transitions the signed properties over the signed
// durations; sky_measure_probe_test.go samples CLOSED and OPEN and confirms both
// end states are the signed stills'. Neither of them looks at what happens
// BETWEEN two states, and the close happens entirely between them.
//
// THE DEFECT. `.skpane` lives inside `<details>`'s `::details-content`, which
// the UA stylesheet gives `content-visibility: hidden` the instant the `open`
// attribute is removed. `content-visibility: hidden` makes the browser skip the
// subtree's contents, so the box collapses to nothing in a SINGLE FRAME while
// the signed 160ms grid-template-rows transition runs correctly, and invisibly,
// on a subtree that is no longer being rendered. The open envelope is a reveal
// and the close is a cut.
//
// THIS IS WHY EVERY OTHER DISCLOSURE ON THE PAGE ALREADY CARRIES
// `content-visibility` IN ITS TRANSITION LIST (`.cal-bench .disc
// ::details-content`, some 300 lines above the sky's rules in the same sheet).
// The property is on the monopoly guard's BASE allowlist for exactly this
// reason. The sky was authored as a `.skpane` grid-row clip-reveal and never
// picked it up, so it was the one disclosure in the product that closed by
// cutting.
//
// WHY THIS PROBE DOES NOT USE THE PACKAGE'S USUAL --virtual-time-budget
// HARNESS, and this is the load-bearing sentence of the file. Every other probe
// here samples END STATES, and end states survive a frozen animation clock:
// under `--virtual-time-budget` a continuous property simply reads at its final
// value and the probe is right anyway. THIS probe's whole subject is the frames
// in between, and that harness has no clock to advance — measured while writing
// this file, under virtual time a `content-visibility` transition never reaches
// 100% at all, so the pane it holds visible stays visible for 2000ms and a
// CORRECT build reads as a close that never lands. Seeking the transitions by
// hand (`Animation.currentTime`) does not help either: the style flush happens
// at frame time, so the layout never moves. So this one probe drives a REAL
// clock through Playwright, and skips when it cannot.
//
// WHAT IT ASSERTS, in the two halves the defect actually has:
//
//  1. THE CLOSE IS NOT A CUT. Immediately after `open` is removed the pane is
//     still rendered — `::details-content` computes `content-visibility:
//     visible`, and the sky still measures its open height. This is the
//     mechanism, and naming it in the failure message is the difference between
//     "the close is broken" and "here is the one property to add".
//  2. THE CLOSE RENDERS AND THEN LANDS. Intermediate heights exist across the
//     160ms envelope, and the sky is back at its closed height afterwards. The
//     second half matters as much as the first: holding a subtree visible
//     forever would satisfy claim 1 and leave the pane stuck open.
//
// It is deliberately NOT a claim about a specific height at a specific
// millisecond — those are numbers a future timing change should be free to
// move. It should stay green across a re-timing and red across a re-cut.
//
// IT SKIPS HONESTLY under -short, or with no Chromium, or with no Node /
// Playwright to drive one, and a skipped run is NOT a pass.
//
//	go test ./internal/widgets/calendar_block/ -run SkyClose -v
package calendar_block

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
)

// skyCloseReading is one host's close, sampled.
type skyCloseReading struct {
	Host    int    `json:"host"`
	Density string `json:"density"`
	// The two end states, so an intermediate sample can be recognised as one.
	ClosedPx float64 `json:"closedPx"`
	OpenPx   float64 `json:"openPx"`
	// Heights at skyCloseSamplesMs, in order, measured from the frame the
	// `open` attribute was removed.
	Samples []float64 `json:"samples"`
	// The used `content-visibility` of ::details-content, read immediately
	// after `open` is removed and again once the envelope has run.
	CVAtStart string `json:"cvAtStart"`
	CVAtEnd   string `json:"cvAtEnd"`
}

// skyCloseSamplesMs are the offsets sampled after `open` is removed. The signed
// close is 160ms total (24ms of reveal-out, then 136ms of box collapse), so the
// first four land inside it and the last two land after it — the tail is there
// so a probe that measured nothing but the tail could not be mistaken for a
// probe that measured the close.
var skyCloseSamplesMs = []int{16, 50, 84, 118, 260, 500}

// TestSkyCloseProbe_TheCloseRendersFrames is the whole of it.
func TestSkyCloseProbe_TheCloseRendersFrames(t *testing.T) {
	if testing.Short() {
		t.Skip("the sky close probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}
	node, nodePath := findProbePlaywright()
	if node == "" {
		t.Skip("no Node with Playwright resolvable — this probe needs a REAL animation " +
			"clock and the package's virtual-time harness has none. A skipped probe is NOT a pass")
	}

	hosts := []struct {
		name    string
		width   int
		density string
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon"},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal"},
	}

	d := fxSky(t, true)
	var boxes strings.Builder
	for i, h := range hosts {
		markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host cal-bench" id="h%d" style="width:%dpx">%s</div>`,
			i, h.width, markup)
	}

	var sampler strings.Builder
	for i, ms := range skyCloseSamplesMs {
		fmt.Fprintf(&sampler,
			`setTimeout(function(){hosts.forEach(function(root){`+
				`root.__scSamples[%d]=h(root);});},%d);`, i, ms)
	}
	last := skyCloseSamplesMs[len(skyCloseSamplesMs)-1]

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		blockCSS(t) + skyProbeBenchCSS(t) +
		`.probe-host{display:block;margin:24px;max-width:none}` +
		`</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		`var sky=function(root){return root.querySelector('details.skygrow');};` +
		`var h=function(root){return Math.round(sky(root).getBoundingClientRect().height*10)/10;};` +
		`var cv=function(root){return getComputedStyle(sky(root),'::details-content')` +
		`.contentVisibility||'unsupported';};` +
		// CLOSED, then OPEN. The open envelope is 200ms and 600ms is three of
		// them, which is sky_measure_probe's own settling budget.
		`hosts.forEach(function(root){root.__scClosed=h(root);root.__scSamples=[];` +
		`sky(root).setAttribute('open','');});` +
		`setTimeout(function(){` +
		`hosts.forEach(function(root){root.__scOpen=h(root);` +
		`sky(root).removeAttribute('open');` +
		// Read the mechanism in the SAME TASK the attribute was removed in.
		// This is the frame the defect happens in: with the property absent the
		// UA's `content-visibility: hidden` is already in force here.
		`root.__scCVStart=cv(root);});},600);` +
		`setTimeout(function(){` + sampler.String() + `},600);` +
		`setTimeout(function(){` +
		`document.body.setAttribute('data-probe',JSON.stringify(hosts.map(function(root){` +
		`return {host:Math.round(root.getBoundingClientRect().width),` +
		`density:getComputedStyle(sky(root).querySelector('summary.skyhdr'))` +
		`.justifyContent==='flex-end'?'C3 seal':'C1 horizon',` +
		`closedPx:root.__scClosed,openPx:root.__scOpen,samples:root.__scSamples,` +
		`cvAtStart:root.__scCVStart,cvAtEnd:cv(root)};})));},` +
		fmt.Sprint(600+last+200) + `);});</script>` +
		`</body></html>`

	dir := t.TempDir()
	pagePath := filepath.Join(dir, "sky-close.html")
	if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	driverPath := filepath.Join(dir, "driver.mjs")
	if err := os.WriteFile(driverPath, []byte(skyCloseDriver), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe driver: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, driverPath, "file://"+pagePath, chrome)
	cmd.Env = append(os.Environ(), "NODE_PATH="+nodePath)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("playwright driver: %v\n%s", err, stderr)
	}
	var readings []skyCloseReading
	if err := json.Unmarshal(out, &readings); err != nil {
		t.Fatalf("probe payload: %v\n%s", err, out)
	}
	if len(readings) != len(hosts) {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), len(hosts))
	}

	for i, h := range hosts {
		r := readings[i]
		t.Run(h.name, func(t *testing.T) {
			var trace strings.Builder
			for j, ms := range skyCloseSamplesMs {
				if j < len(r.Samples) {
					fmt.Fprintf(&trace, " %dms=%.1f", ms, r.Samples[j])
				}
			}
			t.Logf("host %dpx · %s · closed %.1fpx · open %.1fpx · ::details-content "+
				"content-visibility %q at the start of the close, %q after · close trace:%s",
				r.Host, r.Density, r.ClosedPx, r.OpenPx, r.CVAtStart, r.CVAtEnd, trace.String())

			if r.Density != h.density {
				t.Fatalf("density = %q, want %q", r.Density, h.density)
			}
			if r.OpenPx <= r.ClosedPx+8 {
				t.Fatalf("the sky opened to %.1fpx from a closed %.1fpx — there is no "+
					"reveal to close and every claim below would pass vacuously",
					r.OpenPx, r.ClosedPx)
			}
			if len(r.Samples) != len(skyCloseSamplesMs) {
				t.Fatalf("got %d samples, want %d — the sampler did not run to completion",
					len(r.Samples), len(skyCloseSamplesMs))
			}

			// (1) THE CLOSE IS NOT A CUT — the mechanism, in the frame it fails in.
			if r.CVAtStart == "hidden" {
				t.Errorf("in the very frame `open` was removed, ::details-content already "+
					"computes `content-visibility: hidden` — the browser has stopped "+
					"rendering the subtree the signed 160ms collapse runs on, so the sky "+
					"goes %.1fpx → %.1fpx in ONE frame and the whole close is invisible. "+
					"The fix is one property: transition `content-visibility` with "+
					"`allow-discrete` on ::details-content, exactly as "+
					"`.cal-bench .disc::details-content` in this same sheet already does",
					r.OpenPx, r.ClosedPx)
			}

			// (2) IT RENDERS, AND IT LANDS. An intermediate height is one that is
			// neither end state, with a 1px tolerance at each end so a sub-pixel
			// landing is not counted as motion.
			intermediate := 0
			for _, px := range r.Samples {
				if px > r.ClosedPx+1 && px < r.OpenPx-1 {
					intermediate++
				}
			}
			if intermediate < 2 {
				t.Errorf("the close produced %d intermediate frames across %v ms — the "+
					"signed 160ms envelope is not being rendered. Trace:%s",
					intermediate, skyCloseSamplesMs, trace.String())
			}
			if last := r.Samples[len(r.Samples)-1]; last > r.ClosedPx+1 {
				t.Errorf("%dms after `open` was removed the sky still measures %.1fpx "+
					"against a closed %.1fpx — the close renders but does not LAND, which "+
					"is what holding a subtree visible forever looks like",
					skyCloseSamplesMs[len(skyCloseSamplesMs)-1], last, r.ClosedPx)
			}
			if r.CVAtEnd != "hidden" && r.CVAtEnd != "unsupported" {
				t.Errorf("after the close, ::details-content computes "+
					"`content-visibility: %q` — the discrete transition must END at the "+
					"UA's hidden, or the closed pane is still being rendered", r.CVAtEnd)
			}
		})
	}
}

// skyCloseDriver opens the probe page in a REAL-clock Chromium and prints the
// payload. It exists because this package's `--virtual-time-budget` harness
// cannot advance an animation clock — see this file's header.
const skyCloseDriver = `import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const { chromium } = require('playwright');
const [url, exe] = process.argv.slice(2);
const browser = await chromium.launch({ executablePath: exe, args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 1600, height: 1400 } });
await page.goto(url);
const payload = await page.waitForFunction(
  () => document.body.getAttribute('data-probe'), null, { timeout: 30000 });
process.stdout.write(await payload.jsonValue());
await browser.close();
`

// findProbePlaywright returns a Node binary and the NODE_PATH that makes
// `require('playwright')` resolve from it, or ("", "") when this machine has no
// way to drive a real-clock browser.
func findProbePlaywright() (node, nodePath string) {
	node, err := exec.LookPath("node")
	if err != nil {
		for _, p := range []string{"/opt/node22/bin/node", "/usr/local/bin/node"} {
			if _, statErr := os.Stat(p); statErr == nil {
				node = p
				break
			}
		}
	}
	if node == "" {
		return "", ""
	}
	// The global module root is where a CI image is most likely to have put
	// Playwright; an already-set NODE_PATH and a local node_modules are tried
	// too, so a developer who installed it either way is not skipped.
	roots := []string{os.Getenv("NODE_PATH")}
	if npm, err := exec.LookPath("npm"); err == nil {
		if out, err := exec.Command(npm, "root", "-g").Output(); err == nil {
			roots = append(roots, strings.TrimSpace(string(out)))
		}
	}
	roots = append(roots, filepath.Join("..", "..", "..", "node_modules"))
	for _, root := range roots {
		if root == "" {
			continue
		}
		cmd := exec.Command(node, "-e", "require.resolve('playwright')")
		cmd.Env = append(os.Environ(), "NODE_PATH="+root)
		if err := cmd.Run(); err == nil {
			return node, root
		}
	}
	return "", ""
}
