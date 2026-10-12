package layouts

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/colour"
)

// AppearanceData is a campaign's Customize page style, resolved for the
// layout. Empty fields are the Classic look and emit nothing, so a campaign
// that never saved Customize renders byte-identical to before.
type AppearanceData struct {
	NavStyle     string
	NavStrength  string
	NavPageName  string
	PageTone     string
	Contrast     string
	BodyFont     string
	HeadingFont  string
	TypeScale    string
	ButtonStyle  string
	Elevation    string
	MotionSpeed  string
	ReduceMotion bool

	// HeaderHeight is "tall" for the taller header; "" is the slim default.
	HeaderHeight string
	// SidebarColour is "ink", "tinted" or "own"; "" is Chronicle's charcoal.
	SidebarColour string
	SidebarOwn    string
	// SidebarCorner is "subtitle" or "banner"; "" is the plain logo and name.
	SidebarCorner   string
	SidebarSubtitle string
	SidebarBanner   string
	// PeekGlow is "own" for a glow colour of its own; "" follows the accent.
	PeekGlow       string
	PeekGlowColour string

	// HoverCard is the hover card look ("plain", "night" or "compact");
	// "" is Paper, which paper.css and hovercard.js use without an attribute.
	HoverCard string

	// SheetStyle is the character sheet style; "" and "modern" add no
	// attribute, so sheets render as before.
	SheetStyle string
}

const keyAppearance ctxKey = "layout_appearance"

// SetAppearance stores the campaign's resolved appearance in the context.
func SetAppearance(ctx context.Context, a *AppearanceData) context.Context {
	return context.WithValue(ctx, keyAppearance, a)
}

// GetAppearance returns the campaign's appearance, or nil for Classic.
func GetAppearance(ctx context.Context) *AppearanceData {
	a, _ := ctx.Value(keyAppearance).(*AppearanceData)
	return a
}

// AppearanceAttrs are the <html> attributes the site's CSS keys its
// Customize styles on (input.css, "Customize: campaign styles").
func AppearanceAttrs(ctx context.Context) templ.Attributes {
	// The viewer's own choices ride on the same element; they are separate
	// attributes (data-view-*), never folded into the campaign's data-cz-*.
	attrs := ViewPrefAttrs(ctx)
	a := GetAppearance(ctx)
	if m := MotionLevel(ctx); m != "full" {
		attrs["data-motion"] = m
	}
	// Site admin is the control room: always dark, red as its accent
	// (input.css, "Site admin: the control room"; base.templ paints it dark
	// before first paint).
	if InSiteAdmin(ctx) {
		attrs["data-site-admin"] = "1"
	}
	if a == nil {
		return attrs
	}
	set := func(k, v string) {
		if v != "" {
			attrs[k] = v
		}
	}
	set("data-cz-nav", a.NavStyle)
	set("data-cz-strength", a.NavStrength)
	set("data-cz-pagename", a.NavPageName)
	set("data-cz-btn", a.ButtonStyle)
	set("data-cz-elev", a.Elevation)
	set("data-cz-scale", a.TypeScale)
	set("data-cz-hover", a.HoverCard)
	if a.SheetStyle != "modern" {
		set("data-cz-sheet", a.SheetStyle)
	}
	if a.HeadingFont != "" && a.HeadingFont != "same" {
		attrs["data-cz-heading"] = a.HeadingFont
	}
	if a.ReduceMotion {
		attrs["data-cz-reduce"] = "1"
	}
	// Sizes the header and the menu's corner by CSS alone, so a Customize
	// save can restyle both by swapping attributes.
	if a.HeaderHeight == "tall" {
		attrs["data-cz-hdr"] = "tall"
	}
	if a.SidebarCorner == "subtitle" || a.SidebarCorner == "banner" {
		attrs["data-cz-corner"] = a.SidebarCorner
	}
	return attrs
}

// MotionLevel is how much this viewer's pages move, written to <html> as
// data-motion when it isn't "full" (absent means full, so the plain look
// adds no attributes): "off" when they picked
// Off in My view, "calm" when they picked Calm or the owner made the campaign
// calm, else "full". A person can lower the owner's level, never raise it.
// The first-paint script in base.templ lowers it to "off" for a device that
// asks for reduced motion.
func MotionLevel(ctx context.Context) string {
	if p := GetViewPrefs(ctx); p != nil {
		switch p.Motion {
		case "off":
			return "off"
		case "calm":
			return "calm"
		}
	}
	if a := GetAppearance(ctx); a != nil && a.ReduceMotion {
		return "calm"
	}
	return "full"
}

// NavCorner is what the menu's top-left corner shows: its kind ("plain",
// "subtitle" or "banner"), the subtitle line and the banner picture. A
// corner whose own content is missing falls back to the plain one rather
// than showing an empty strip.
func NavCorner(ctx context.Context) (kind, subtitle, banner string) {
	a := GetAppearance(ctx)
	if a == nil {
		return "plain", "", ""
	}
	switch a.SidebarCorner {
	case "subtitle":
		if a.SidebarSubtitle != "" {
			return "subtitle", a.SidebarSubtitle, ""
		}
	case "banner":
		if a.SidebarBanner != "" {
			return "banner", "", a.SidebarBanner
		}
	}
	return "plain", "", ""
}

// czTone is one page tone in one theme, in Chronicle's own token names.
type czTone struct{ bg, card, alt, line, line2, text, body, muted string }

// czTones are the page tones from the signed design. Cool is Chronicle's
// own grey scale, so it only matters here when contrast is high.
var czTones = map[string][2]czTone{
	"cool": {
		{"#f9fafb", "#ffffff", "#f3f4f6", "#e5e7eb", "#d1d5db", "#111827", "#374151", "#6b7280"},
		{"#111827", "#1f2937", "#273244", "#374151", "#4b5563", "#f9fafb", "#d1d5db", "#9ca3af"},
	},
	"warm": {
		{"#faf8f5", "#ffffff", "#f4f0ea", "#e8e1d8", "#d6cdc1", "#1c1917", "#44403c", "#78716c"},
		{"#171311", "#231e1b", "#2e2824", "#3d3530", "#534943", "#faf7f2", "#d6d0c8", "#a8a097"},
	},
	"paper": {
		{"#f4eddd", "#fbf6ea", "#ece2cc", "#dccfb3", "#c9b995", "#2a2118", "#43372a", "#75654f"},
		{"#1a1610", "#25201a", "#2f2920", "#43392b", "#5a4d3a", "#f5ecd9", "#dccfb8", "#a89a80"},
	},
}

// tones returns the page tone for theme (0 light, 1 dark) with high
// contrast applied, the same rule the Customize preview uses.
func (a *AppearanceData) tones(theme int) czTone {
	name := a.PageTone
	if name == "" {
		name = "cool"
	}
	t := czTones[name][theme]
	if a.Contrast == "high" {
		t.muted, t.body = t.body, t.text
		t.line, t.line2 = t.line2, colour.Mix(t.line2, t.text, 0.3)
	}
	return t
}

// toneVars writes one theme's tokens. Hints and faint text are mixed toward
// the page so they stay a step quieter than labels on every tone.
func toneVars(t czTone, dark bool) string {
	input := t.card
	borderLight := t.alt
	if dark {
		input, borderLight = t.bg, t.line2
	}
	return fmt.Sprintf("--color-bg-primary:%s;--color-bg-secondary:%s;--color-card-bg:%s;--color-bg-tertiary:%s;"+
		"--color-border:%s;--color-border-light:%s;--color-input-border:%s;--color-input-bg:%s;"+
		"--color-text-primary:%s;--color-text-body:%s;--color-text-secondary:%s;--color-text-muted:%s;--color-text-faint:%s;",
		t.bg, t.card, t.card, t.alt, t.line, borderLight, t.line2, input,
		t.text, t.body, t.muted, colour.Mix(t.muted, t.bg, 0.35), colour.Mix(t.muted, t.bg, 0.6))
}

// czMenu is one menu colour with the shades the menu derives from it.
type czMenu struct{ bg, hover, press, raised, text, text2, text3 string }

// czMenuInk is Ink's own set of words: cooler than Charcoal's, to sit on its
// blue-black. Charcoal's are the stylesheet's defaults, so nothing is emitted
// for it. Must match SIDEBARS in customize_look.js.
var czMenuInk = czMenu{bg: "#0e1424", text: "#cdd5e3", text2: "#8a95aa", text3: "#66728a"}

// menu resolves the menu colour: ok is false for Charcoal, which the
// stylesheet already draws. Tinted and own colours pass through
// colour.MenuDark, so white menu words keep 7:1 whatever was chosen.
func (a *AppearanceData) menu(accent string) (m czMenu, ok bool) {
	switch a.SidebarColour {
	case "ink":
		m = czMenuInk
	case "tinted":
		if accent == "" || !colour.ValidHex(accent) {
			accent = "#6366f1"
		}
		m = czMenu{bg: colour.MenuTinted(accent)}
	case "own":
		if !colour.ValidHex(a.SidebarOwn) {
			return czMenu{}, false
		}
		m = czMenu{bg: colour.MenuDark(a.SidebarOwn)}
	default:
		return czMenu{}, false
	}
	if m.text == "" {
		m.text, m.text2, m.text3 = "#cbd5e1", "#8b93a1", "#5f6776"
	}
	m.hover = colour.Mix(m.bg, "#ffffff", 0.07)
	m.raised = colour.Mix(m.bg, "#ffffff", 0.045)
	m.press = colour.Mix(m.bg, "#000000", 0.25)
	return m, true
}

// menuVars writes the menu's tokens. The stylesheet falls back to
// Charcoal's values wherever one is missing.
func menuVars(m czMenu) string {
	return fmt.Sprintf("--color-sidebar-bg:%s;--cz-sb-hover:%s;--cz-sb-press:%s;--cz-sb-raised:%s;--cz-sb-text:%s;--cz-sb-text-2:%s;--cz-sb-text-3:%s;",
		m.bg, m.hover, m.press, m.raised, m.text, m.text2, m.text3)
}

// czElevation is each elevation's resting and hover shadow, light then
// dark. ACC is replaced by the accent's channels: Flat's outline and
// Ambient's accent-tinted shadows.
var czElevation = map[string]struct {
	lift        int
	rest, hover [2]string
}{
	"flat": {0, [2]string{"none", "none"}, [2]string{"0 0 0 1.5px rgb(ACC / .35)", "0 0 0 1.5px rgb(ACC / .45)"}},
	"dramatic": {4,
		[2]string{"0 2px 6px -1px rgb(0 0 0 / .10), 0 10px 22px -12px rgb(0 0 0 / .28)", "0 2px 6px -1px rgb(0 0 0 / .4), 0 12px 26px -12px rgb(0 0 0 / .7)"},
		[2]string{"0 22px 44px -14px rgb(0 0 0 / .38), 0 6px 14px -6px rgb(0 0 0 / .18)", "0 24px 48px -14px rgb(0 0 0 / .85), 0 6px 14px -6px rgb(0 0 0 / .5)"}},
	"ambient": {3,
		[2]string{"0 1px 2px rgb(16 24 40 / .05), 0 6px 18px -8px rgb(ACC / .28)", "0 1px 2px rgb(0 0 0 / .4), 0 8px 20px -8px rgb(ACC / .32)"},
		[2]string{"0 2px 4px rgb(16 24 40 / .06), 0 18px 36px -12px rgb(ACC / .42), 0 0 0 1px rgb(ACC / .12)", "0 2px 4px rgb(0 0 0 / .45), 0 20px 40px -12px rgb(ACC / .5), 0 0 0 1px rgb(ACC / .2)"}},
}

// czSpeeds retime Chronicle's chrome durations (micro, standard, large) and
// the paper moves (slide-out, page turn).
var czSpeeds = map[string]struct{ micro, std, large, slide, turn int }{
	"snappy":    {80, 130, 180, 240, 380},
	"leisurely": {200, 330, 460, 520, 760},
}

// czHeadingFaces is each heading face's weight, letter-spacing and size
// nudge. Some faces run large or small at the same font size; the nudge
// (matching the Customize example site) makes every face read the same size.
// An empty size means no nudge.
var czHeadingFaces = map[string]struct{ weight, spacing, size string }{
	"cinzel": {"600", ".02em", ".9"}, "marcellus": {"400", ".01em", ""}, "imfell": {"400", "0", "1.06"}, "cormorant": {"600", "0", "1.14"},
	"fraunces": {"600", "-.01em", ""}, "playfair": {"600", "0", ""}, "josefin": {"600", ".01em", "1.02"}, "chakra": {"600", "0", ""},
}

// czFontStacks pairs each face with fallbacks close in shape.
var czFontStacks = map[string]string{
	"inter":        "'Inter', system-ui, -apple-system, sans-serif",
	"sourcesans":   "'Source Sans 3', 'Segoe UI', sans-serif",
	"atkinson":     "'Atkinson Hyperlegible', Verdana, sans-serif",
	"literata":     "'Literata', Georgia, serif",
	"sourceserif":  "'Source Serif 4', Georgia, serif",
	"lora":         "'Lora', Georgia, serif",
	"alegreya":     "'Alegreya', Georgia, serif",
	"merriweather": "'Merriweather', Georgia, serif",
	"cinzel":       "'Cinzel', 'Trajan Pro', Georgia, serif",
	"marcellus":    "'Marcellus', Georgia, serif",
	"imfell":       "'IM Fell English', Georgia, serif",
	"cormorant":    "'Cormorant Garamond', Garamond, Georgia, serif",
	"fraunces":     "'Fraunces', Georgia, serif",
	"playfair":     "'Playfair Display', Georgia, serif",
	"josefin":      "'Josefin Sans', 'Century Gothic', sans-serif",
	"chakra":       "'Chakra Petch', 'Segoe UI', sans-serif",
}

// czFontFace is one @font-face from static/fonts/customize/fonts.json.
type czFontFace struct {
	Family       string `json:"family"`
	Style        string `json:"style"`
	Weight       string `json:"weight"`
	File         string `json:"file"`
	UnicodeRange string `json:"unicodeRange"`
}

var (
	czFontsOnce sync.Once
	czFonts     map[string][]czFontFace
	czFontsFS   fs.FS = os.DirFS("static")
)

// customizeFonts reads the self-hosted font list once. A missing or broken
// list leaves the fallback stacks in charge rather than failing the page.
func customizeFonts() map[string][]czFontFace {
	czFontsOnce.Do(func() {
		b, err := fs.ReadFile(czFontsFS, "fonts/customize/fonts.json")
		if err == nil {
			_ = json.Unmarshal(b, &czFonts)
		}
	})
	return czFonts
}

// fontFaceCSS returns @font-face rules for one face id, served from Chronicle.
func fontFaceCSS(id string) string {
	var b strings.Builder
	for _, f := range customizeFonts()[id] {
		fmt.Fprintf(&b, "@font-face{font-family:'%s';font-style:%s;font-weight:%s;font-display:swap;src:url(%s) format('woff2');unicode-range:%s;}",
			f.Family, f.Style, f.Weight, AssetURL("/static/fonts/customize/"+f.File), f.UnicodeRange)
	}
	return b.String()
}

// AppearanceCSS returns the campaign's Customize tokens: page tone and
// contrast, readable accent shades, fonts, depth and motion timing. The
// values come from validated choices and colours only.
func AppearanceCSS(ctx context.Context) string {
	a := GetAppearance(ctx)
	accent := GetAccentColor(ctx)
	if a == nil {
		a = &AppearanceData{}
	}
	var root, dark, extra strings.Builder

	if a.PageTone != "" && a.PageTone != "cool" || a.Contrast == "high" {
		root.WriteString(toneVars(a.tones(0), false))
		dark.WriteString(toneVars(a.tones(1), true))
	}

	// Readable accent: buttons get a fill their words can sit on, and links
	// a shade that reads on this page tone in each theme.
	// An older separate button colour (Action) keeps its buttons until the
	// owner saves Customize, which folds it into the accent.
	if accent != "" && colour.ValidHex(accent) {
		light, night := a.tones(0), a.tones(1)
		if GetAccentAction(ctx) == "" {
			f := colour.FillFor(accent)
			fmt.Fprintf(&root, "--color-accent-fill:%s;--color-accent-fill-hover:%s;--color-accent-on-fill:%s;",
				f.Fill, colour.Mix(f.Fill, "#000000", 0.12), f.On)
		}
		fmt.Fprintf(&root, "--color-accent-link:%s;", colour.Readable(accent, []string{light.bg, light.card}, 4.5, true).Hex)
		fmt.Fprintf(&dark, "--color-accent-link:%s;", colour.Readable(accent, []string{night.bg, night.card}, 4.5, false).Hex)
	}

	// The menu is dark in both themes, so its tokens go in both blocks: the
	// stylesheet's own dark-theme rule would otherwise out-rank :root.
	if m, ok := a.menu(accent); ok {
		root.WriteString(menuVars(m))
		dark.WriteString(menuVars(m))
	}
	if a.PeekGlow == "own" && colour.ValidHex(a.PeekGlowColour) {
		fmt.Fprintf(&root, "--peek-glow-rgb:%s;", colour.RGBChannels(a.PeekGlowColour))
	}

	if e, ok := czElevation[a.Elevation]; ok {
		acc := "99 102 241"
		if accent != "" && colour.ValidHex(accent) {
			acc = colour.RGBChannels(accent)
		}
		fmt.Fprintf(&root, "--elev-resting:%s;--elev-hover:%s;--cz-lift:%dpx;", strings.ReplaceAll(e.rest[0], "ACC", acc), strings.ReplaceAll(e.hover[0], "ACC", acc), e.lift)
		fmt.Fprintf(&dark, "--elev-resting:%s;--elev-hover:%s;", strings.ReplaceAll(e.rest[1], "ACC", acc), strings.ReplaceAll(e.hover[1], "ACC", acc))
	}

	if s, ok := czSpeeds[a.MotionSpeed]; ok {
		fmt.Fprintf(&root, "--dur-micro:%dms;--dur-standard:%dms;--dur-large:%dms;--dur-slide:%dms;--dur-turn:%dms;", s.micro, s.std, s.large, s.slide, s.turn)
	}

	if stack, ok := czFontStacks[a.BodyFont]; ok && a.BodyFont != "inter" {
		extra.WriteString(fontFaceCSS(a.BodyFont))
		fmt.Fprintf(&root, "--font-campaign:%s;", stack)
	}
	if h, ok := czHeadingFaces[a.HeadingFont]; ok {
		extra.WriteString(fontFaceCSS(a.HeadingFont))
		fmt.Fprintf(&root, "--font-heading:%s;--cz-hw:%s;--cz-hls:%s;", czFontStacks[a.HeadingFont], h.weight, h.spacing)
		if h.size != "" {
			fmt.Fprintf(&root, "--cz-hz:%s;", h.size)
		}
	}

	var out strings.Builder
	out.WriteString(extra.String())
	if root.Len() > 0 {
		out.WriteString(":root{" + root.String() + "}")
	}
	if dark.Len() > 0 {
		out.WriteString(":root.dark{" + dark.String() + "}")
	}
	return out.String()
}

// customizeFontIDs lists every face id that has self-hosted files, sorted,
// for the Customize page's font previews.
func customizeFontIDs() []string {
	ids := make([]string, 0, len(customizeFonts()))
	for id := range customizeFonts() {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CustomizeFontsCSS returns @font-face rules for every Customize face, so
// the Customize page can show each choice in its own type.
func CustomizeFontsCSS() string {
	var b strings.Builder
	for _, id := range customizeFontIDs() {
		b.WriteString(fontFaceCSS(id))
	}
	return b.String()
}
