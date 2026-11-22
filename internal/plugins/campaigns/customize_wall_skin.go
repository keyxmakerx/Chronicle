// customize_wall_skin.go: the campaign's look as the wall's miniatures wear
// it. Each picture is drawn from custom properties on its own element, so the
// wall needs none of the campaign's page CSS and a draft shows before it is
// saved.

package campaigns

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// wallSkin is what the miniatures share.
type wallSkin struct {
	Campaign string
	Initial  string
	// Style is the custom properties, as a style attribute value built only
	// from validated colours and fixed font stacks.
	Style templ.SafeCSS
	// Nav is the names the miniature menu lists under Categories.
	Nav []string
}

// wallTones are the page tones' colours, light then dark, in the order the
// Look editor's TONES table has them: page, card, ink, text, muted, line.
var wallTones = map[string][2][6]string{
	"cool":  {{"#f9fafb", "#ffffff", "#111827", "#374151", "#6b7280", "#e5e7eb"}, {"#111827", "#1f2937", "#f9fafb", "#d1d5db", "#9ca3af", "#374151"}},
	"warm":  {{"#faf8f5", "#ffffff", "#1c1917", "#44403c", "#78716c", "#e8e1d8"}, {"#171311", "#231e1b", "#faf7f2", "#d6d0c8", "#a8a097", "#3d3530"}},
	"paper": {{"#f4eddd", "#fbf6ea", "#2a2118", "#43372a", "#75654f", "#dccfb3"}, {"#1a1610", "#25201a", "#f5ecd9", "#dccfb8", "#a89a80", "#43392b"}},
}

// wallBodyStacks and wallHeadStacks are the faces' CSS stacks, as the Look
// editor lists them.
var wallBodyStacks = map[string]string{
	"inter": "'Inter', system-ui, sans-serif", "sourcesans": "'Source Sans 3', 'Segoe UI', sans-serif",
	"atkinson": "'Atkinson Hyperlegible', Verdana, sans-serif", "literata": "'Literata', Georgia, serif",
	"sourceserif": "'Source Serif 4', Georgia, serif", "lora": "'Lora', Georgia, serif",
	"alegreya": "'Alegreya', Georgia, serif", "merriweather": "'Merriweather', Georgia, serif",
}

var wallHeadStacks = map[string]string{
	"cinzel": "'Cinzel', Georgia, serif", "marcellus": "'Marcellus', Georgia, serif",
	"imfell": "'IM Fell English', Georgia, serif", "cormorant": "'Cormorant Garamond', Georgia, serif",
	"fraunces": "'Fraunces', Georgia, serif", "playfair": "'Playfair Display', Georgia, serif",
	"josefin": "'Josefin Sans', sans-serif", "chakra": "'Chakra Petch', sans-serif",
}

// wallRadius is the corner each button style gives.
var wallRadius = map[string]string{"lift": "8px", "press": "6px", "glow": "999px", "ink": "3px", "keystone": "12px"}

func hexOr(v, dflt string) string {
	if hexColour.MatchString(v) {
		return v
	}
	return dflt
}

func hexRGB(h string) (r, g, b int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) < 6 {
		return 0, 0, 0
	}
	p := func(s string) int { n, _ := strconv.ParseInt(s, 16, 32); return int(n) }
	return p(h[0:2]), p(h[2:4]), p(h[4:6])
}

// mixHex blends a toward b by t (0..1).
func mixHex(a, b string, t float64) string {
	ar, ag, ab := hexRGB(a)
	br, bg, bb := hexRGB(b)
	m := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", m(ar, br), m(ag, bg), m(ab, bb))
}

// onColour is white or near-black, whichever reads on bg.
func onColour(bg string) string {
	r, g, b := hexRGB(bg)
	if (299*r+587*g+114*b)/1000 > 150 {
		return "#111827"
	}
	return "#ffffff"
}

// skinOf dresses the miniatures in the look: its accent, menu, header, page
// tone, faces and button corners. A colour that is not a plain hex falls back
// to Classic's, so a hand-edited draft cannot put anything but a colour into
// the style attribute.
func skinOf(d customizeDraft, name string, cats []CustomizeCategory) wallSkin {
	acc := hexOr(d.Colours.Accent, classicAccent)
	menu, menuText := "#1a1c23", "#cfd3dc"
	switch d.Colours.Sidebar {
	case "ink":
		menu = "#0e1424"
	case "tinted":
		menu = mixHex(acc, "#0b0b12", 0.72)
	case "own":
		menu = hexOr(d.Sidebar.Own, startMenuOwn)
	}
	tone, ok := wallTones[d.Colours.Page]
	if !ok {
		tone = wallTones["cool"]
	}

	var sb strings.Builder
	w := func(k, v string) { sb.WriteString(k + ":" + v + ";") }
	w("--x-acc", acc)
	w("--x-sb", menu)
	w("--x-sbt", menuText)
	hd, hdt, hdd, hddt := "", "", "", ""
	switch d.Header.Bg {
	case "gradient", "moving":
		dir := map[string]string{"r": "to right", "br": "to bottom right", "b": "to bottom"}[d.Header.Dir]
		if dir == "" {
			dir = "to right"
		}
		g := fmt.Sprintf("linear-gradient(%s,%s,%s)", dir, hexOr(d.Header.From, "#0f172a"), hexOr(d.Header.To, "#1e2a5a"))
		hd, hdt, hdd, hddt = g, "#ffffff", g, "#ffffff"
	case "sky", "image":
		g := "linear-gradient(to right,#1e3a5f,#4b83b4 60%,#9fc9ea)"
		hd, hdt, hdd, hddt = g, "#ffffff", g, "#ffffff"
	default:
		if c := d.Header.Solid; hexColour.MatchString(c) {
			hd, hdt, hdd, hddt = c, onColour(c), c, onColour(c)
		}
	}
	for i, mode := range []string{"l", "d"} {
		t := tone[i]
		w("--x"+mode+"-page", t[0])
		w("--x"+mode+"-card", t[1])
		w("--x"+mode+"-ink", t[2])
		w("--x"+mode+"-txt", t[3])
		w("--x"+mode+"-mut", t[4])
		w("--x"+mode+"-line", t[5])
		bg, fg := hd, hdt
		if i == 1 {
			bg, fg = hdd, hddt
		}
		if bg == "" {
			bg, fg = t[1], t[3]
		}
		w("--x"+mode+"-hd", bg)
		w("--x"+mode+"-hdt", fg)
	}
	body := wallBodyStacks[d.Type.Body]
	if body == "" {
		body = wallBodyStacks["inter"]
	}
	head := wallHeadStacks[d.Type.Heading]
	if head == "" {
		head = body
	}
	w("--x-fb", body)
	w("--x-fh", head)
	if d.Type.Heading == "cinzel" {
		w("--x-tt", "uppercase")
	} else {
		w("--x-tt", "none")
	}
	if r := wallRadius[d.Buttons.Style]; r != "" {
		w("--x-br", r)
	}

	skin := wallSkin{Campaign: name, Initial: "C", Style: templ.SafeCSS(sb.String())}
	if r := []rune(strings.TrimSpace(name)); len(r) > 0 {
		skin.Initial = strings.ToUpper(string(r[0]))
	}
	for _, c := range cats {
		if c.ParentID == nil && len(skin.Nav) < 4 {
			skin.Nav = append(skin.Nav, c.NamePlural)
		}
	}
	return skin
}
