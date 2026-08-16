// default_categories_ink_test.go — the six colours every new calendar is seeded
// with, measured through the surfaces that actually render them.
//
// WHY THIS NEEDED A BROWSER AND NOT A LIST OF HEX STRINGS. `DefaultEventCategories`
// declares sRGB; the grid does not draw sRGB. Below 84px of column it draws
// `oklch(from var(--axis) 0.36 calc(c * 0.62) h)` — the category's HUE at a
// pinned ink lightness — and two hex values that look nothing alike can land on
// the same ink. Amber `#f59e0b` and the GM-gold token look different in a
// swatch and painted 28/255 apart as runes, which at 9x12px is one colour.
//
// WHY IT MATTERS MORE THAN IT SOUNDS. C-CALV4-TILES §9.2 made the rune's SHAPE
// decorative — the glyph comes from the mark's position in the day, not its
// type — so at narrow density COLOUR IS THE ONLY CHANNEL that tells one event
// type from another. And a GM-only day strikes every one of its runes gold, so
// gold is a seventh thing competing in the same space.
//
// These are the DEFAULTS, so this is what an operator sees before they have
// configured anything. An operator's own palette is their business; the one
// Chronicle ships is Chronicle's.
//
// Skips honestly with no Chromium; a skipped run is NOT a pass.
//
//	go test ./internal/plugins/calendar/ -run DefaultCategories -v
package calendar

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

// dciFloor is how far apart two inks must sit, worst single channel in 0-255.
//
// Calibrated against what the defect measured (28 for the collision, 29 for the
// worst pair) and what the fix measures (75 and 43). 35 sits above both defects
// and below both fixes, so it has real room on each side rather than being
// pinned to today's number.
const dciFloor = 35

// dciInk is one painted colour.
type dciInk struct {
	Name string  `json:"name"`
	R    float64 `json:"r"`
	G    float64 `json:"g"`
	B    float64 `json:"b"`
}

func (a dciInk) dist(b dciInk) float64 {
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

// TestDefaultCategories_NoneCollidesWithTheGMGold is the guard for the ruling.
func TestDefaultCategories_NoneCollidesWithTheGMGold(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	cats := DefaultEventCategories()
	if len(cats) < 2 {
		t.Fatalf("DefaultEventCategories returned %d entries — this test would prove nothing", len(cats))
	}

	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			inks := dciPaint(t, chrome, theme, cats)
			gold := inks[len(inks)-1] // the GM permission ink, appended last
			cat := inks[:len(inks)-1]

			// 1. NONE of them is the GM gold.
			for _, c := range cat {
				d := c.dist(gold)
				t.Logf("%-9s rgb(%3.0f,%3.0f,%3.0f)  vs GM-gold rgb(%3.0f,%3.0f,%3.0f) = %3.0f/255",
					c.Name, c.R, c.G, c.B, gold.R, gold.G, gold.B, d)
				if d < dciFloor {
					t.Errorf("the shipped %q default inks %.0f/255 from the GM-ONLY gold, under the "+
						"%d floor. Below 84px of column a GM-only day strikes every rune gold and "+
						"the glyph carries no type, so a %q and a GM-only day become the same mark. "+
						"Gold's meaning is load-bearing across the product; the CATEGORY is what "+
						"moves", c.Name, d, dciFloor, c.Name)
				}
			}

			// 2. …and none of them is EACH OTHER. Fixing one collision by
			//    walking a colour into a neighbour is a lateral move, and it is
			//    the obvious way to get this wrong.
			for i := range cat {
				for j := i + 1; j < len(cat); j++ {
					if d := cat[i].dist(cat[j]); d < dciFloor {
						t.Errorf("the shipped %q and %q defaults ink %.0f/255 apart, under the %d "+
							"floor — at narrow density colour is the ONLY thing separating two "+
							"event types", cat[i].Name, cat[j].Name, d, dciFloor)
					}
				}
			}
		})
	}
}

// dciPaint renders each category's colour through the SHIPPED rune-ink rule,
// plus the GM-gold ink appended last, and reads the painted sRGB back.
//
// THE RECIPE IS NEVER RE-TYPED. The swatches are real `.ulseg` elements inside a
// real `.cal-block-host .cell`, styled by the real stylesheet, so `--runeink` is
// whatever the sheet says today — a copy of the expression in this file would
// keep passing after somebody changed the sheet. The GM gold is produced the
// same way, by putting a `.dogear` in the cell so the `:has()` rule fires,
// rather than by restating the gold expression.
func dciPaint(t *testing.T, chrome, theme string, cats []EventCategoryInput) []dciInk {
	t.Helper()
	root := probeRepoRoot(t)
	css, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-block.css"))
	if err != nil {
		t.Fatalf("read calendar-block.css: %v", err)
	}

	var boxes strings.Builder
	for _, c := range cats {
		fmt.Fprintf(&boxes,
			`<div class="cal-block-host"><div class="cell"><div class="cunder"><div class="ul">`+
				`<span class="ulseg p1 dci" data-name="%s" style="--axis:%s"></span>`+
				`</div></div></div></div>`,
			html.EscapeString(c.Name), html.EscapeString(c.Color))
	}
	// The GM ink, produced by the shipped `:has(> .dogear)` rule rather than by
	// restating its expression. --axis is irrelevant on this one: the gold rule
	// overrides --runeink outright, and that is the point of measuring it here.
	boxes.WriteString(`<div class="cal-block-host"><div class="cell"><div class="cunder"><div class="ul">` +
		`<span class="ulseg p1 dci" data-name="GM gold" style="--axis:oklch(0.55 0.17 258)"></span>` +
		`</div></div><span class="dogear"></span></div></div>`)

	open, closeTag := "", ""
	if theme == "dark" {
		open, closeTag = `<div class="dark">`, `</div>`
	}
	const reader = `function(){
      var out=[];var els=document.querySelectorAll('.ulseg.dci');
      for(var i=0;i<els.length;i++){var e=els[i];
        var cx=document.createElement('canvas').getContext('2d');
        cx.fillStyle=getComputedStyle(e).backgroundColor;cx.fillRect(0,0,1,1);
        var d=cx.getImageData(0,0,1,1).data;
        out.push({name:e.getAttribute('data-name'),r:d[0],g:d[1],b:d[2]});}
      return out;}`

	page := `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0}` +
		string(css) + `</style></head><body>` + open + boxes.String() + closeTag +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`document.body.setAttribute('data-probe', JSON.stringify((` + reader + `)()));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "dci-"+theme+".html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=600,800", "--virtual-time-budget=4000", "--dump-dom", "file://"+path,
	).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := regexp.MustCompile(`data-probe="([^"]*)"`).FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var inks []dciInk
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &inks); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(inks) != len(cats)+1 {
		t.Fatalf("probe returned %d inks for %d categories plus gold", len(inks), len(cats))
	}
	// A swatch that painted nothing would make every comparison below a
	// comparison of two absences, which reads as perfect agreement.
	for _, k := range inks {
		if k.R+k.G+k.B == 0 {
			t.Fatalf("%q inked to transparent black — --runeink did not resolve, so every "+
				"distance in this file would be measuring nothing against nothing", k.Name)
		}
	}
	// …and the GM arm must actually be GOLD rather than the axis it was handed,
	// or the collision test is comparing a blue rune with a blue rune.
	gold, first := inks[len(inks)-1], inks[0]
	if gold.dist(first) < 1 {
		t.Fatal("the GM-gold swatch is identical to a category's — the `:has(> .dogear)` rule " +
			"did not fire, so this file would be measuring the wrong thing entirely")
	}
	return inks
}
