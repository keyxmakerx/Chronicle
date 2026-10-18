package layouts

import (
	"fmt"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/colour"
)

// czKit is what the server needs to know about one style kit: its faces
// (loaded only when the kit is on) and the page and paper its accent is read
// against in each theme. Everything else about a kit is in style_kits.css.
type czKit struct {
	head, body, label string    // font stacks; label is "" when it follows head
	headFonts         []string  // self-hosted face ids the head and label stacks use
	bodyFonts         []string  // and the body stack
	page, paper       [2]string // light, dark
}

// czKits are the signed style kits other than Material, keyed by their
// data-cz-kit value (campaigns.AppearanceStyleKits). A hand-drawn or display
// face falls back to a plain face of its kind, never a script.
var czKits = map[string]czKit{
	"sharp": {
		head: "'Source Serif 4', Georgia, serif", body: "'Source Serif 4', Georgia, serif",
		label: "'Oswald', 'Arial Narrow', sans-serif", headFonts: []string{"sourceserif", "oswald"}, bodyFonts: []string{"sourceserif"},
		page: [2]string{"#fbfaf7", "#0e0e0e"}, paper: [2]string{"#fbfaf7", "#0e0e0e"},
	},
	"ornate": {
		head: "'Cormorant Garamond', Garamond, Georgia, serif", body: "'EB Garamond', Garamond, Georgia, serif",
		headFonts: []string{"cormorant"}, bodyFonts: []string{"ebgaramond"},
		page: [2]string{"#f6efe2", "#1b1510"}, paper: [2]string{"#fbf5e8", "#241c15"},
	},
	"flowing": {
		head: "'Fraunces', Georgia, serif", body: "'Nunito', 'Segoe UI', system-ui, sans-serif",
		headFonts: []string{"fraunces"}, bodyFonts: []string{"nunito"},
		page: [2]string{"#f1f6f4", "#101918"}, paper: [2]string{"#fbfdfc", "#162221"},
	},
	"lumen": {
		head: "'Marcellus', Georgia, serif", body: "'Jost', 'Century Gothic', system-ui, sans-serif",
		headFonts: []string{"marcellus"}, bodyFonts: []string{"jost"},
		page: [2]string{"#f4f4fa", "#0c0e1c"}, paper: [2]string{"#ffffff", "#141830"},
	},
	"sketchbook": {
		head: "'Kalam', 'Atkinson Hyperlegible', Verdana, sans-serif", body: "'Atkinson Hyperlegible', Verdana, sans-serif",
		headFonts: []string{"kalam"}, bodyFonts: []string{"atkinson"},
		page: [2]string{"#f7f4ec", "#1d2220"}, paper: [2]string{"#fffdf6", "#252b28"},
	},
}

// czDefaultAccent is the colour a campaign has before its owner picks one;
// with it, each kit keeps the colour it was designed with.
const czDefaultAccent = "#6366f1"

// kitCSS writes a style kit's tokens. Faces the owner picked win over the
// kit's, so only the ones left unpicked are set. A colour the owner picked
// replaces the kit's accent: darkened slightly for light pages and lifted
// for dark ones, then stepped in lightness until it reads as link text on
// this kit's page and paper (ui-standard.md, "The campaign's colour in a
// kit").
func kitCSS(root, dark, extra *strings.Builder, a *AppearanceData, k czKit, accent string) {
	_, ownHead := czHeadingFaces[a.HeadingFont]
	_, ownBody := czFontStacks[a.BodyFont]
	ownBody = ownBody && a.BodyFont != "inter"
	if !ownHead {
		fmt.Fprintf(root, "--kit-hf:%s;", k.head)
		if k.label != "" {
			fmt.Fprintf(root, "--kit-lf:%s;", k.label)
		}
		for _, id := range k.headFonts {
			extra.WriteString(fontFaceCSS(id))
		}
	}
	if !ownBody {
		fmt.Fprintf(root, "--kit-bf:%s;", k.body)
		for _, id := range k.bodyFonts {
			if ownHead || !slices.Contains(k.headFonts, id) {
				extra.WriteString(fontFaceCSS(id))
			}
		}
	}

	if !colour.ValidHex(accent) || strings.EqualFold(accent, czDefaultAccent) {
		return
	}
	light := colour.Readable(colour.Mix(accent, "#000000", 0.12), []string{k.page[0], k.paper[0]}, 4.5, true).Hex
	night := colour.Readable(colour.Mix(accent, "#ffffff", 0.45), []string{k.page[1], k.paper[1]}, 4.5, false).Hex
	fmt.Fprintf(root, "--kit-user-acc:%s;", light)
	fmt.Fprintf(dark, "--kit-user-acc:%s;", night)
}
