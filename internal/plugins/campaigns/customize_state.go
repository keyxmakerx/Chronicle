package campaigns

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// customizeDraft is the Customize page's starting draft, in the shape the
// page's script edits: what is saved now, with every unset choice filled
// from the Classic look so the page never has to guess a default. Save sends
// an AppearanceInput back; the script maps between the two shapes.
type customizeDraft struct {
	Look  string `json:"look"`
	Brand struct {
		Name     string `json:"name"`
		Logo     string `json:"logo"`
		Welcome  string `json:"welcome"`
		Backdrop string `json:"backdrop"`
	} `json:"brand"`
	Header struct {
		Bg      string       `json:"bg"`     // "solid", "gradient", "moving", "sky" or "image".
		Height  string       `json:"height"` // "slim" or "tall".
		Solid   string       `json:"solid"`  // A colour, or "page" for the header that matches the page.
		From    string       `json:"from"`
		To      string       `json:"to"`
		Dir     string       `json:"dir"` // "r", "br" or "b".
		Image   string       `json:"image"`
		Scrim   string       `json:"scrim"`
		Widgets []string     `json:"widgets"`
		Links   []TopbarLink `json:"links"`
		Text    string       `json:"text"`
	} `json:"header"`
	Nav struct {
		Style    string `json:"style"`
		Strength string `json:"strength"`
		PageName string `json:"pageName"`
	} `json:"nav"`
	Colours struct {
		Accent   string  `json:"accent"`
		S1       *string `json:"s1"` // null follows the chrome accent.
		S2       *string `json:"s2"`
		Page     string  `json:"page"`
		Contrast string  `json:"contrast"`
		Sidebar  string  `json:"sidebar"` // Menu colour: "charcoal", "ink", "tinted" or "own".
	} `json:"colours"`
	Sidebar struct {
		Own        string `json:"own"` // Used while the menu colour is "own"; a starting colour otherwise.
		Corner     string `json:"corner"`
		Subtitle   string `json:"subtitle"`
		Banner     string `json:"banner"`
		Glow       string `json:"glow"`
		GlowColour string `json:"glowColour"` // Used while the glow is "own"; a starting colour otherwise.
	} `json:"sidebar"`
	Type struct {
		Body    string `json:"body"`
		Heading string `json:"heading"`
		Scale   string `json:"scale"`
	} `json:"type"`
	Buttons struct {
		Style string `json:"style"`
	} `json:"buttons"`
	Motion struct {
		Elevation string `json:"elevation"`
		Speed     string `json:"speed"`
		ReduceAll bool   `json:"reduceAll"`
	} `json:"motion"`
	Hover struct {
		Look string `json:"look"`
	} `json:"hover"`
	Sheet struct {
		Style string `json:"style"`
	} `json:"sheet"`
}

// customizeState is everything the Customize page starts from.
type customizeState struct {
	Campaign string            `json:"campaign"` // Campaign name, for the example site and messages.
	Draft    customizeDraft    `json:"draft"`
	Pictures map[string]string `json:"pictures"` // Stored picture name -> URL the page can show.
}

// Chronicle's own default chrome accent, as the Classic look shows it.
const classicAccent = "#6366f1"

// Starting colours the editor offers before the owner picks their own menu
// or glow colour; never stored unless that choice is selected and saved.
const (
	startMenuOwn = "#1a2a22"
	startGlowOwn = "#3b9fb5"
)

// legacyBodyFonts maps the older font_family choice to the nearest text
// face, so a campaign that picked one sees it selected rather than reset.
var legacyBodyFonts = map[string]string{
	"serif":        "sourceserif",
	"georgia":      "literata",
	"merriweather": "merriweather",
	"sans-serif":   "inter",
	"monospace":    "inter",
}

// orDefault returns v, or the first allowed value (the Classic default)
// when v is empty or not one of allowed.
func orDefault(v string, allowed []string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return allowed[0]
}

// optColour turns an optional stored colour into the draft's nullable form.
func optColour(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// buildCustomizeState reads the saved settings into the page's draft.
func buildCustomizeState(ctx context.Context, cc *CampaignContext) customizeState {
	s := cc.Campaign.ParseSettings()
	var a Appearance
	if s.Appearance != nil {
		a = *s.Appearance
	}
	st := customizeState{Campaign: cc.Campaign.Name, Pictures: map[string]string{}}
	d := &st.Draft
	picture := func(name string) string {
		if name != "" {
			st.Pictures[name] = layouts.MediaURL(ctx, name)
		}
		return name
	}

	d.Look = orDefault(a.Look, AppearanceLooks)
	d.Brand.Name = s.BrandName
	d.Brand.Logo = picture(s.BrandLogo)
	d.Brand.Welcome = s.WelcomeMessage
	if cc.Campaign.BackdropPath != nil {
		d.Brand.Backdrop = picture(*cc.Campaign.BackdropPath)
	}

	// Header: the Classic header matches the page; the gradient colours
	// keep the Classic pair until the owner picks a gradient.
	h := &d.Header
	h.Bg, h.Solid, h.From, h.To, h.Dir, h.Scrim = "solid", "page", "#0f172a", "#1e2a5a", "r", "medium"
	if ts := s.TopbarStyle; ts != nil {
		switch ts.Mode {
		case "solid":
			if isValidHexColor(ts.Color) {
				h.Solid = ts.Color
			}
		case "gradient", "moving":
			if isValidHexColor(ts.GradientFrom) && isValidHexColor(ts.GradientTo) {
				h.Bg, h.From, h.To = ts.Mode, ts.GradientFrom, ts.GradientTo
				h.Dir = map[string]string{"to-br": "br", "to-b": "b"}[ts.GradientDir]
				if h.Dir == "" {
					h.Dir = "r"
				}
			}
		case "sky":
			h.Bg = "sky"
		case "image":
			if ts.ImagePath != "" {
				h.Bg = "image"
				h.Image = picture(ts.ImagePath)
				h.Scrim = orDefault(ts.Scrim, AppearanceScrims)
			}
		}
	}
	h.Height = orDefault(a.HeaderHeight, AppearanceHeights)
	h.Widgets = []string{}
	h.Links = []TopbarLink{}
	if tc := s.TopbarContent; tc != nil {
		h.Widgets = append(h.Widgets, (&layouts.TopbarContentData{Mode: tc.Mode, Widgets: tc.Widgets}).TopbarWidgets()...)
		h.Links = append(h.Links, tc.Links...)
		h.Text = tc.Quote
	}

	d.Nav.Style = orDefault(a.NavStyle, AppearanceNavStyles)
	d.Nav.Strength = orDefault(a.NavStrength, AppearanceNavStrengths)
	d.Nav.PageName = orDefault(a.NavPageName, AppearanceNavPageNames)

	d.Colours.Accent = classicAccent
	if isValidHexColor(s.AccentColor) {
		d.Colours.Accent = s.AccentColor
	}
	// Apps used to have their own slot; Surface A now carries that colour.
	d.Colours.S1 = optColour(s.AccentSurface1)
	if d.Colours.S1 == nil {
		d.Colours.S1 = optColour(s.AccentApp)
	}
	d.Colours.S2 = optColour(s.AccentSurface2)
	d.Colours.Page = orDefault(a.PageTone, AppearancePageTones)
	d.Colours.Contrast = orDefault(a.Contrast, AppearanceContrasts)

	d.Colours.Sidebar = orDefault(a.SidebarColour, AppearanceSidebars)
	sb := &d.Sidebar
	sb.Own = startMenuOwn
	if isValidHexColor(a.SidebarOwn) {
		sb.Own = a.SidebarOwn
	}
	sb.Corner = orDefault(a.SidebarCorner, AppearanceCorners)
	sb.Subtitle = a.SidebarSubtitle
	sb.Banner = picture(a.SidebarBanner)
	sb.Glow = orDefault(a.PeekGlow, AppearanceGlows)
	sb.GlowColour = startGlowOwn
	if isValidHexColor(a.PeekGlowColour) {
		sb.GlowColour = a.PeekGlowColour
	}

	d.Type.Body = a.BodyFont
	if d.Type.Body == "" {
		d.Type.Body = legacyBodyFonts[s.FontFamily]
	}
	d.Type.Body = orDefault(d.Type.Body, AppearanceBodyFonts)
	d.Type.Heading = orDefault(a.HeadingFont, AppearanceHeadingFonts)
	d.Type.Scale = orDefault(a.TypeScale, AppearanceTypeScales)
	d.Buttons.Style = orDefault(a.ButtonStyle, AppearanceButtonStyles)
	d.Motion.Elevation = orDefault(a.Elevation, AppearanceElevations)
	d.Motion.Speed = orDefault(a.MotionSpeed, AppearanceSpeeds)
	d.Motion.ReduceAll = a.ReduceMotion
	d.Hover.Look = orDefault(a.HoverCard, AppearanceHoverCards)
	d.Sheet.Style = orDefault(a.SheetStyle, AppearanceSheetStyles)
	return st
}

// customizeStateJSON is the data-state attribute the Customize page's
// script starts from.
func customizeStateJSON(ctx context.Context, cc *CampaignContext) string {
	b, err := json.Marshal(buildCustomizeState(ctx, cc))
	if err != nil {
		return "{}"
	}
	return string(b)
}
