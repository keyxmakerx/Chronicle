// moonshots_gen_test.go — C-CALV4-MOONS' fidelity evidence (MN-G15).
//
// NOT A TEST: a tool that lives in a _test.go file so it can drive the REAL
// templ output and the REAL shipped stylesheets rather than a mockup or a
// re-implementation. It is inert unless MOON_SHOTS names an output directory:
//
//	MOON_SHOTS=/tmp/moons go test ./internal/widgets/calendar_block/ -run MoonShots
//
// EVERY STATE IS REACHED BY OPERATING THE PRODUCT, never by forcing a class.
// The panel is opened by dispatching a click on the node
// `document.elementFromPoint()` resolves at the disc cluster's painted centre;
// the tabs are switched by clicking their labels. A screenshot of a state the
// product cannot actually reach is evidence of nothing, which is the failure
// screenshot_gen_test.go's own `openSheet` note records for the layer sheet.
//
// AND THE FOLD IS A CLIP, NOT A STILL. This arc learned twice that a still of a
// transition is a still of nothing: the frames below are captured by SEEKING the
// live CSSTransition objects to a known millisecond and photographing each one,
// then encoded — with the standard library, because this environment has no
// ffmpeg — into an animated GIF whose every frame is separately decodable. The
// PNGs are written beside it for exactly that reason.
package calendar_block

import (
	"context"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// moonShot is one captured state: a name, the script that reaches it, and the
// flags the browser needs to be in the right mode.
type moonShot struct {
	file  string
	title string
	// reach is a JS statement run before the shot. "" means the resting state.
	reach string
	// flags are extra Chromium switches (reduced motion, so far).
	flags []string
	// bench seats the Bench's stylesheet too, which is where the sky band's
	// rules live ([SKY-2]) even though its markup is the Block's.
	bench bool
	// moonless renders the layer switched off.
	moonless bool
	w, h     int
}

// moonShotReach is the shared preamble every reach script gets: the handles,
// and the hit-test helper that makes "operating the product" mean it.
const moonShotReach = `
  var host = document.querySelector('.probe-host');
  var row = host.querySelector('.wk');
  var cluster = row && row.querySelector('.phctl');
  var panel = row && row.querySelector('.mpan');
  function tapCentre(el){
    if (!el) return null;
    var r = el.getBoundingClientRect();
    if (r.width <= 0 || r.height <= 0) return null;
    var hit = document.elementFromPoint(r.left + r.width/2, r.top + r.height/2);
    if (hit) hit.click();
    return hit;
  }
  function tab(which){
    var l = host.querySelector('.mpbtn[data-mp-btn="' + which + '"]');
    return tapCentre(l);
  }
  function seek(ms){
    if (!panel) return;
    panel.getAnimations().forEach(function(a){ try { a.currentTime = ms; } catch(e){} });
    void panel.offsetHeight;
  }
`

var moonShots = []moonShot{
	{file: "01-resting", title: "resting — three discs, static, no +N",
		w: 1232, h: 620},
	{file: "02-hover", title: "pointed at — colour only, nothing grows ([MN-6])",
		reach: `var c=cluster.getBoundingClientRect();
			cluster.dispatchEvent(new MouseEvent('mouseover',{bubbles:true,clientX:c.left+c.width/2,clientY:c.top+c.height/2}));`,
		w: 1232, h: 620},
	{file: "03-panel-graph", title: "panel open, Graph tab — v4-moons-graph-only.png's shape",
		reach: `tapCentre(cluster); seek(1000);`, w: 1232, h: 720},
	// THE SEEK COMES BEFORE THE TAB, and that ordering is a measured product
	// fact rather than a tidy-up. Mid-fold the panel is clipped to
	// `inset(0 0 100%)`, and clip-path clips HIT TESTING as well as paint — so a
	// click aimed at a tab during the first frames falls straight through to the
	// day cell beneath it. The first capture of this shot did exactly that: it
	// photographed the Graph tab with the Ledger filtered to day 12, because the
	// tab click had landed on a cell. Reaching the state means letting the fold
	// finish first, which is what a person does.
	{file: "04-panel-details", title: "panel open, Details tab — uncapped, four bodies, no epithet",
		reach: `tapCentre(cluster); seek(1000); tab('details'); seek(1000);`, w: 1232, h: 720},
	{file: "05-band-closed", title: "the sky band, closed — the discs are scenery",
		bench: true, w: 1232, h: 620},
	{file: "06-band-open", title: "the sky band, open at its caret (MN-G2)",
		bench: true,
		reach: `var s=host.querySelector('details.skygrow');
			var k=s.querySelector('.skcaret'); tapCentre(k);`,
		w: 1232, h: 720},
	{file: "07-reduced-motion", title: "prefers-reduced-motion — instant AND complete",
		reach: `tapCentre(cluster);`, flags: []string{"--force-prefers-reduced-motion"},
		w: 1232, h: 720},
	{file: "08-moonless", title: "moons layer off — nothing at all (v4-bare-no-moons.png)",
		moonless: true, w: 1232, h: 620},
}

// TestMoonShots writes the evidence. Inert without MOON_SHOTS.
func TestMoonShots(t *testing.T) {
	out := os.Getenv("MOON_SHOTS")
	if out == "" {
		t.Skip("set MOON_SHOTS=<dir> to write C-CALV4-MOONS' fidelity evidence")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, s := range moonShots {
		t.Run(s.file, func(t *testing.T) {
			page := moonShotPage(t, s)
			dir := t.TempDir()
			path := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
				t.Fatalf("write: %v", err)
			}
			dst := filepath.Join(out, s.file+".png")
			if err := moonShotCapture(chrome, path, dst, s.w, s.h, s.flags); err != nil {
				t.Fatalf("capture %s: %v", s.file, err)
			}
			fi, err := os.Stat(dst)
			if err != nil || fi.Size() == 0 {
				t.Fatalf("%s was not written", dst)
			}
			t.Logf("%s — %s (%d bytes)", s.file+".png", s.title, fi.Size())
		})
	}

	// ── THE CLIP ────────────────────────────────────────────────────────────
	// Ten frames across the 200ms open, each seeked to a known millisecond and
	// photographed, then encoded to an animated GIF. A still of a transition is
	// a still of nothing; this is the transition.
	marks := []int{0, 20, 40, 60, 80, 100, 120, 150, 180, 200}
	frames := make([]string, 0, len(marks))
	for i, ms := range marks {
		s := moonShot{
			file:  fmt.Sprintf("clip-%02d-%03dms", i, ms),
			reach: fmt.Sprintf("tapCentre(cluster); seek(%d);", ms),
			w:     1232, h: 720,
		}
		page := moonShotPage(t, s)
		dir := t.TempDir()
		path := filepath.Join(dir, "shot.html")
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
			t.Fatalf("write: %v", err)
		}
		dst := filepath.Join(out, s.file+".png")
		if err := moonShotCapture(chrome, path, dst, s.w, s.h, nil); err != nil {
			t.Fatalf("capture frame %dms: %v", ms, err)
		}
		frames = append(frames, dst)
	}
	clip := filepath.Join(out, "panel-opening.gif")
	if err := moonShotGIF(frames, clip); err != nil {
		t.Fatalf("encode clip: %v", err)
	}
	fi, err := os.Stat(clip)
	if err != nil {
		t.Fatalf("clip not written: %v", err)
	}
	t.Logf("panel-opening.gif — %d frames across the 200ms fold (%d bytes), "+
		"and every frame is beside it as a decodable PNG", len(frames), fi.Size())
}

// moonShotPage builds the evidence page around the REAL rendered Block.
func moonShotPage(t *testing.T, s moonShot) string {
	t.Helper()
	d := fxAlmanac(t, true)
	if s.bench {
		d = fxSky(t, true)
	}
	if s.moonless {
		on := make([]string, 0, len(d.Layers.Enabled))
		for _, k := range d.Layers.Enabled {
			if k != "moons" {
				on = append(on, k)
			}
		}
		d.Layers.Enabled = on
	}
	markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
	css := blockCSS(t)
	class := "probe-host"
	if s.bench {
		css += skyProbeBenchCSS(t)
		class += " cal-bench"
	}
	reach := ""
	if s.reach != "" {
		reach = `<script>document.addEventListener('DOMContentLoaded',function(){` +
			`setTimeout(function(){` + moonShotReach + s.reach + `},150);});</script>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#f6f7f9;font-family:system-ui,sans-serif}` + css +
		`.cap{font:600 11px/1.5 ui-monospace,monospace;color:#556;padding:10px 12px 0}` +
		`.probe-host{display:block;margin:10px 12px;max-width:none;width:1200px}` +
		`</style></head><body>` +
		`<div class="cap">` + htmlEscapeShot(s.title) + `</div>` +
		`<div class="` + class + `">` + markup + `</div>` + reach +
		`</body></html>`
}

func htmlEscapeShot(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// moonShotCapture drives one headless screenshot.
func moonShotCapture(chrome, page, dst string, w, h int, flags []string) error {
	args := append([]string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"--force-device-scale-factor=1",
		"--virtual-time-budget=4000",
		"--screenshot=" + dst,
	}, flags...)
	args = append(args, "file://"+page)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, chrome, args...).Run()
}

// moonShotGIF encodes the captured frames into an animated GIF.
//
// PURE STANDARD LIBRARY, because this environment has no ffmpeg and a clip that
// cannot be produced here is a clip that will not be produced. Each frame is
// dithered into the Plan9 palette, which is a lossy step and is why the PNGs
// stay on disk: the GIF is for watching, the PNGs are the measurements.
func moonShotGIF(frames []string, dst string) error {
	g := &gif.GIF{}
	for _, f := range frames {
		fh, err := os.Open(f) //nolint:gosec // paths this function just wrote
		if err != nil {
			return err
		}
		src, err := png.Decode(fh)
		_ = fh.Close()
		if err != nil {
			return fmt.Errorf("decode %s: %w", f, err)
		}
		b := src.Bounds()
		p := image.NewPaletted(b, palette.Plan9)
		draw.FloydSteinberg.Draw(p, b, src, image.Point{})
		g.Image = append(g.Image, p)
		// 6/100s a frame: ten frames read as the 200ms fold slowed enough to
		// see, which is what a reviewer needs from it.
		g.Delay = append(g.Delay, 6)
	}
	if len(g.Image) == 0 {
		return fmt.Errorf("no frames")
	}
	out, err := os.Create(dst) //nolint:gosec // caller-named artefact path
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	return gif.EncodeAll(out, g)
}
