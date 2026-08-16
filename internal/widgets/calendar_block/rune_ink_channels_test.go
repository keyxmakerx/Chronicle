// rune_ink_channels_test.go — the rune ink must not throw away a channel the
// operator's own data carries.
//
// WHY THIS FILE EXISTS. §9.2 made COLOUR the only thing that tells one event
// type from another below 84px of column: the glyph is chosen by position in
// the day, not by type, so hue is carrying the whole load. The first build of
// that ink wrote `oklch(from var(--axis) 0.36 0.10 h)` — a CONSTANT chroma, so
// only the HUE survived. That reads as a small thing and is not:
//
//	`--axis` IS NOT ONE OF SIX CURATED TOKENS. block_projection.go stamps
//	EventCategories[].Color verbatim, and calendar_settings.templ edits it with
//	a bare `<input type="color">`. It is any sRGB value an operator picks.
//
// Three measured consequences, each of them the tool losing a distinction its
// own data had:
//
//	· two types separated by SATURATION collapse onto one ink. This sheet's own
//	  palette does it — --ev-social (c 0.17) and --ev-session (c 0.04) share
//	  hue 255–258 and are told apart by chroma ALONE. Forced to a constant they
//	  painted 6/255 apart in both themes, which at 9×12px is one colour.
//	· an ACHROMATIC category acquires a hue it does not have. OKLCH gives grey
//	  a powerless h, so #888888 inked rgb(235,130,125) on dark — a grey category
//	  painting a RED rune, 3/255 from a vivid red one.
//	· the six SHIPPED defaults (model.go DefaultEventCategories) lost most of
//	  their separation: min pairwise 80/255 raw → 32/255 light, 48/255 dark.
//
// The fix scales chroma instead of replacing it. This file is the guard, and it
// is deliberately NOT a string match on `calc(c * 0.62)`: the recipe is allowed
// to be re-tuned, and what must not change is that TWO COLOURS THE OPERATOR CAN
// TELL APART STILL INK APART. So it measures the painted result.
//
// IT DRIVES A REAL BROWSER because there is no other way. Relative colour
// syntax is resolved by the engine, `getComputedStyle` hands back
// `oklch(0.36 0.1 70.07)` — a computed value in a wide-gamut space, not the
// sRGB bytes the screen gets — and reading that answers a different question
// than the one asked. Skips honestly under -short or with no Chromium; a
// skipped run is NOT a pass. Registered in tools/check-browser-probes.sh.
//
//	go test ./internal/widgets/calendar_block/ -run RuneInkChannels -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rkSubject is one axis colour pushed through the shipped recipe.
type rkSubject struct {
	// Name is what a failure calls it.
	Name string
	// CSS is the value stamped as `--axis`, exactly as a producer would.
	CSS string
	// Achromatic marks a colour with no hue to preserve. The ink must come
	// back achromatic too — inventing a hue for grey is the fabricated figure
	// this package refuses everywhere else.
	Achromatic bool
}

// rkSubjects is the adversarial set: the SHIPPED defaults (what every new
// calendar actually gets), the sheet's own chroma-separated pair, and the two
// achromatic picks a bare colour input permits.
//
// THE SHIPPED DEFAULTS ARE HERE ON PURPOSE. A probe that only exercised the
// curated --ev-* tokens would be testing the palette this file already trusts;
// the defect was about values that arrive from the database.
func rkSubjects() []rkSubject {
	return []rkSubject{
		{Name: "Holiday #f59e0b", CSS: "#f59e0b"},
		{Name: "Battle #ef4444", CSS: "#ef4444"},
		{Name: "Quest #8b5cf6", CSS: "#8b5cf6"},
		{Name: "Birthday #ec4899", CSS: "#ec4899"},
		{Name: "Festival #10b981", CSS: "#10b981"},
		{Name: "Travel #3b82f6", CSS: "#3b82f6"},
		{Name: "ev-social (c .17)", CSS: "oklch(0.55 0.17 258)"},
		{Name: "ev-session (c .04)", CSS: "oklch(0.55 0.04 255)"},
		{Name: "grey #888888", CSS: "#888888", Achromatic: true},
		{Name: "near-grey #8a8f8a", CSS: "#8a8f8a", Achromatic: true},
	}
}

// rkReading is one subject's painted ink, in sRGB, per theme.
type rkReading struct {
	Name string  `json:"name"`
	CSS  string  `json:"css"`
	R    float64 `json:"r"`
	G    float64 `json:"g"`
	B    float64 `json:"b"`
}

// rkChromaPairMin is the floor for two inks that the operator separated BY
// CHROMA ALONE. Set against what the defect measured (6/255 — one colour at
// letterform size) and what the fix measures (41 light / 64 dark), so it has
// real room on both sides rather than being pinned to today's number.
const rkChromaPairMin = 20

// rkAchromaticMax is how far apart an achromatic ink's own channels may sit
// before it has acquired a hue. 6/255 tolerates the engine's rounding through
// OKLCH and nothing else; the defect measured 110/255 on the same subject.
const rkAchromaticMax = 6

// TestRuneInkChannels_ChromaSurvivesTheDeepening is the regression guard for the
// collapse described in this file's header.
func TestRuneInkChannels_ChromaSurvivesTheDeepening(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}

	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			got := rkRun(t, chrome, theme)

			byName := map[string]rkReading{}
			for _, r := range got {
				byName[r.Name] = r
			}

			// ── 1. THE CHROMA-SEPARATED PAIR ─────────────────────────────
			a, b := byName["ev-social (c .17)"], byName["ev-session (c .04)"]
			d := rkWorst(a, b)
			t.Logf("chroma-separated pair: %s → rgb(%.0f,%.0f,%.0f) vs %s → rgb(%.0f,%.0f,%.0f) — %.0f/255 apart",
				a.CSS, a.R, a.G, a.B, b.CSS, b.R, b.G, b.B, d)
			if d < rkChromaPairMin {
				t.Errorf("--ev-social and --ev-session ink %.0f/255 apart, under the %d floor. "+
					"These two share a hue (258 vs 255) and are told apart BY CHROMA ALONE. "+
					"An ink recipe that writes a constant `c` keeps only the hue, so the two "+
					"become one colour — and below 84px of column colour is the ONLY channel "+
					"left telling event types apart, because §9.2 chose the glyph by position "+
					"in the day rather than by type. Scale chroma, do not replace it",
					d, rkChromaPairMin)
			}

			// ── 2. GREY MUST STAY GREY ───────────────────────────────────
			//
			// OKLCH gives an achromatic colour a POWERLESS hue. Read it back
			// out and multiply it by a constant chroma and the engine happily
			// paints a saturated colour that was never in the data.
			for _, s := range rkSubjects() {
				if !s.Achromatic {
					continue
				}
				r := byName[s.Name]
				spread := rkSpread(r)
				t.Logf("achromatic subject %s → rgb(%.0f,%.0f,%.0f) · channel spread %.0f/255",
					s.Name, r.R, r.G, r.B, spread)
				if spread > rkAchromaticMax {
					t.Errorf("%s inks rgb(%.0f,%.0f,%.0f) — its channels sit %.0f/255 apart, over "+
						"the %d ceiling, so a category with NO hue has been painted with one. "+
						"OKLCH's hue is powerless for grey; a constant chroma promotes that "+
						"meaningless angle into a real colour and the grid shows an event type "+
						"a shade the operator never chose",
						s.Name, r.R, r.G, r.B, spread, rkAchromaticMax)
				}
			}

			// ── 3. THE SHIPPED DEFAULTS STAY TELLABLE APART ──────────────
			//
			// Reported, not gated on a floor: the six defaults include two
			// that differ mainly in LIGHTNESS (Holiday amber / Battle red),
			// and normalising lightness is what makes a rune an ink at all —
			// so their convergence is a cost of the design, not a defect.
			// The census is logged so a future regression shows up as a
			// number moving rather than as nothing at all.
			var worstPair string
			min := 1e9
			defs := rkSubjects()[:6]
			for i := range defs {
				for j := i + 1; j < len(defs); j++ {
					if v := rkWorst(byName[defs[i].Name], byName[defs[j].Name]); v < min {
						min, worstPair = v, defs[i].Name+" / "+defs[j].Name
					}
				}
			}
			t.Logf("shipped DefaultEventCategories: closest pair as ink is %s at %.0f/255", worstPair, min)
			if min == 0 {
				t.Errorf("two of the six shipped default categories ink IDENTICALLY (%s). Every "+
					"new calendar is seeded with these, so this is what an operator sees before "+
					"they have configured anything", worstPair)
			}
		})
	}
}

// rkChipTintMin is the floor for two NAMED chips of different event types. The
// defect measured 4/255 (three pale pinks, indistinguishable); the fix measures
// 12–18. 10 leaves room on both sides without pinning today's number.
const rkChipTintMin = 10

// TestRuneInkChannels_TheChipTintCarriesItsOwnHue is the same defect one
// surface over, and it PREDATES this pass — it is at HEAD.
//
// `.chip`'s fill was `color-mix(in oklch, var(--surface-card) 90%, var(--axis))`,
// and `color-mix(in oklch, …)` INTERPOLATES THE HUE ANGLE. The light theme's
// --surface-card is `oklch(1 0 0)`: white written with an EXPLICIT hue of zero.
// So the mix dragged every result 10% of the way from 0° toward the type's hue
// and landed in the reds — a blue event, a red event and a green event all
// painted pale PINK, 4/255 apart, on the largest colour surface a wide cell has.
// Dark was the same failure mirrored through the card's own hue of 265.
//
// It was invisible while the cell had a white ground; the gapped tile and the
// era tint gave the chip a coloured surround and the pink stopped hiding.
//
// WHY IT IS TESTED HERE rather than filed: §9.2's ruling is the operator's own
// words — "just have the colors determine what kind of event" — so a chip fill
// showing the wrong hue and carrying no separation between types contradicts
// the thing the grid was just rebuilt around.
func TestRuneInkChannels_TheChipTintCarriesItsOwnHue(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			got := rkChipRun(t, chrome, theme)
			for i := range got {
				for j := i + 1; j < len(got); j++ {
					d := rkWorst(got[i], got[j])
					t.Logf("chip %s rgb(%.0f,%.0f,%.0f) vs %s rgb(%.0f,%.0f,%.0f) — %.0f/255",
						got[i].Name, got[i].R, got[i].G, got[i].B,
						got[j].Name, got[j].R, got[j].G, got[j].B, d)
					if d < rkChipTintMin {
						t.Errorf("the %s and %s chips fill %.0f/255 apart, under the %d floor. "+
							"If the mix is back in OKLCH, the hue ANGLE is being interpolated "+
							"and the light theme's `oklch(1 0 0)` card drags every type toward "+
							"0° — three different event types painting the same pale pink. "+
							"sRGB has no hue channel to wrap",
							got[i].Name, got[j].Name, d, rkChipTintMin)
					}
				}
			}
		})
	}
}

// rkChipRun paints the SHIPPED `.chip` rule with three well-separated axis hues
// and reads back what it fills with.
func rkChipRun(t *testing.T, chrome, theme string) []rkReading {
	t.Helper()
	css := blockCSS(t)
	subjects := []rkSubject{
		{Name: "blue", CSS: "oklch(0.55 0.17 258)"},
		{Name: "red", CSS: "oklch(0.60 0.19 25)"},
		{Name: "green", CSS: "oklch(0.65 0.13 145)"},
	}
	var boxes strings.Builder
	for _, s := range subjects {
		fmt.Fprintf(&boxes,
			`<div class="cal-block-host"><span class="chip rk" data-name="%s" style="--axis:%s">x</span></div>`,
			html.EscapeString(s.Name), html.EscapeString(s.CSS))
	}
	open, closeTag := "", ""
	if theme == "dark" {
		open, closeTag = `<div class="dark">`, `</div>`
	}
	const reader = `function(){
      var out=[];var els=document.querySelectorAll('.chip.rk');
      for(var i=0;i<els.length;i++){var e=els[i];
        var cx=document.createElement('canvas').getContext('2d');
        cx.fillStyle=getComputedStyle(e).backgroundColor;cx.fillRect(0,0,1,1);
        var d=cx.getImageData(0,0,1,1).data;
        out.push({name:e.getAttribute('data-name'),css:'',r:d[0],g:d[1],b:d[2]});}
      return out;}`
	page := `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0}` +
		css + `</style></head><body>` + open + boxes.String() + closeTag +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`document.body.setAttribute('data-probe', JSON.stringify((` + reader + `)()));});</script>` +
		`</body></html>`
	path := filepath.Join(t.TempDir(), "chiptint-"+theme+".html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=600,400", "--virtual-time-budget=4000", "--dump-dom", "file://"+path,
	).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var got []rkReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &got); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(got) != len(subjects) {
		t.Fatalf("probe returned %d chips for %d subjects", len(got), len(subjects))
	}
	return got
}

// rkWorst is the largest single-channel distance between two inks, in 0–255 —
// the same measure every other colour claim in this package is made in.
func rkWorst(a, b rkReading) float64 {
	d := func(x, y float64) float64 {
		if x > y {
			return x - y
		}
		return y - x
	}
	m := d(a.R, b.R)
	if v := d(a.G, b.G); v > m {
		m = v
	}
	if v := d(a.B, b.B); v > m {
		m = v
	}
	return m
}

// rkSpread is how far one ink's own channels sit apart — 0 for a true grey.
func rkSpread(r rkReading) float64 {
	hi, lo := r.R, r.R
	for _, v := range []float64{r.G, r.B} {
		if v > hi {
			hi = v
		}
		if v < lo {
			lo = v
		}
	}
	return hi - lo
}

// rkRun paints every subject through THE SHIPPED RULE and reads the result back.
//
// THE RECIPE IS NEVER RE-TYPED HERE. The swatches are real `.ulseg` elements
// inside a real `.cal-block-host .cell`, so `--runeink` is whatever the
// stylesheet says today; a copy of the expression in this file would keep
// passing after someone changed the sheet. The ink is resolved through a canvas
// rather than a parser of our own, for the reason rpSwatch gives — the sheet
// speaks relative colour syntax and a second implementation of colour is a
// second thing to be wrong in.
func rkRun(t *testing.T, chrome, theme string) []rkReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i, s := range rkSubjects() {
		fmt.Fprintf(&boxes,
			`<div class="cal-block-host"><div class="cell"><div class="cunder"><div class="ul">`+
				`<span class="ulseg p1 rk" data-name="%s" style="--axis:%s"></span>`+
				`</div></div></div></div>`,
			html.EscapeString(s.Name), html.EscapeString(s.CSS))
		_ = i
	}

	wrapOpen, wrapClose := "", ""
	if theme == "dark" {
		wrapOpen, wrapClose = `<div class="dark">`, `</div>`
	}

	// The reader stamps each ink onto a canvas and reads the bytes back. Note
	// it reads `background-color` and NOT a custom property: --runeink is a
	// token, and a token is exactly the thing that comes back unresolved.
	const reader = `function(){
      var out=[];
      var els=document.querySelectorAll('.ulseg.rk');
      for(var i=0;i<els.length;i++){
        var e=els[i];
        var cx=document.createElement('canvas').getContext('2d');
        cx.fillStyle=getComputedStyle(e).backgroundColor;
        cx.fillRect(0,0,1,1);
        var d=cx.getImageData(0,0,1,1).data;
        out.push({name:e.getAttribute('data-name'),css:e.style.getPropertyValue('--axis'),
                  r:d[0],g:d[1],b:d[2]});
      }
      return out;
    }`

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#888}` +
		css + `</style></head><body>` + wrapOpen + boxes.String() + wrapClose +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`document.body.setAttribute('data-probe', JSON.stringify((` + reader + `)()));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "runeink-"+theme+".html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=800,1200", "--virtual-time-budget=4000",
		"--dump-dom", "file://"+path,
	).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var got []rkReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &got); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(got) != len(rkSubjects()) {
		t.Fatalf("probe returned %d inks for %d subjects", len(got), len(rkSubjects()))
	}
	// A SUBJECT THAT PAINTED NOTHING IS NOT A PASS. Transparent black is what
	// an unresolved --runeink looks like, and comparing two of those would
	// report perfect agreement between two colours that never rendered.
	for _, r := range got {
		if r.R+r.G+r.B == 0 {
			t.Fatalf("%s inked to transparent black — --runeink did not resolve, so every "+
				"comparison in this file would be measuring nothing against nothing", r.Name)
		}
	}
	return got
}
