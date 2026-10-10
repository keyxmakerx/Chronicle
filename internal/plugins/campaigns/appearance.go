package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/colour"
)

// Appearance holds the Customize page's style choices that have no older
// home in CampaignSettings. Every empty field means the Classic look, so a
// campaign that never opens Customize renders exactly as Chronicle ships.
type Appearance struct {
	Look         string `json:"look,omitempty"`          // Look the owner started from; Custom is derived, never stored.
	NavStyle     string `json:"nav_style,omitempty"`     // Menu highlight on the current page.
	NavStrength  string `json:"nav_strength,omitempty"`  // "calm" or "lively".
	NavPageName  string `json:"nav_page_name,omitempty"` // "row" (shown beside its menu entry) or "hidden".
	PageTone     string `json:"page_tone,omitempty"`     // "cool", "warm" or "paper".
	Contrast     string `json:"contrast,omitempty"`      // "standard" or "high".
	BodyFont     string `json:"body_font,omitempty"`     // Text face id (AppearanceBodyFonts).
	HeadingFont  string `json:"heading_font,omitempty"`  // Heading face id (AppearanceHeadingFonts); "same" follows the text.
	TypeScale    string `json:"type_scale,omitempty"`    // "compact", "standard" or "roomy".
	ButtonStyle  string `json:"button_style,omitempty"`  // "lift", "press", "glow", "ink" or "keystone".
	Elevation    string `json:"elevation,omitempty"`     // "flat", "standard", "dramatic" or "ambient".
	MotionSpeed  string `json:"motion_speed,omitempty"`  // "snappy", "standard" or "leisurely".
	ReduceMotion bool   `json:"reduce_motion,omitempty"` // Calmer motion for every member of the campaign.

	SidebarColour   string `json:"sidebar_colour,omitempty"`   // Menu colour: "charcoal" (default), "ink", "tinted" or "own".
	SidebarOwn      string `json:"sidebar_own,omitempty"`      // The owner's menu colour, used when SidebarColour is "own".
	SidebarCorner   string `json:"sidebar_corner,omitempty"`   // Menu's top-left corner: "plain", "subtitle" or "banner".
	SidebarSubtitle string `json:"sidebar_subtitle,omitempty"` // The subtitle line under the name (corner "subtitle").
	SidebarBanner   string `json:"sidebar_banner,omitempty"`   // Banner picture behind the name (corner "banner").
	PeekGlow        string `json:"peek_glow,omitempty"`        // Hidden-menu edge glow: "accent" (default) or "own".
	PeekGlowColour  string `json:"peek_glow_colour,omitempty"` // The glow's own colour, used when PeekGlow is "own".
	HeaderHeight    string `json:"header_height,omitempty"`    // "slim" (default) or "tall".
	HoverCard       string `json:"hover_card,omitempty"`       // Hover card look: "paper" (default), "plain", "night" or "compact".
	SheetStyle      string `json:"sheet_style,omitempty"`      // Character sheet style (AppearanceSheetStyles); "modern" is the default.
}

// The allowed values of each Appearance choice. The first entry is the
// Classic default; the save stores it as empty so defaults stay implicit.
var (
	AppearanceLooks        = []string{"classic", "parchment", "midnight", "forest", "ember", "frost", "arcane", "starship"}
	AppearanceNavStyles    = []string{"ring", "comet", "breathe", "tide", "rail", "tab", "edge", "icon"}
	AppearanceNavStrengths = []string{"calm", "lively"}
	AppearanceNavPageNames = []string{"row", "hidden"}
	AppearancePageTones    = []string{"cool", "warm", "paper"}
	AppearanceContrasts    = []string{"standard", "high"}
	AppearanceBodyFonts    = []string{"inter", "sourcesans", "atkinson", "literata", "sourceserif", "lora", "alegreya", "merriweather"}
	AppearanceHeadingFonts = []string{"same", "cinzel", "marcellus", "imfell", "cormorant", "fraunces", "playfair", "josefin", "chakra"}
	AppearanceTypeScales   = []string{"standard", "compact", "roomy"}
	AppearanceButtonStyles = []string{"lift", "press", "glow", "ink", "keystone"}
	AppearanceElevations   = []string{"standard", "flat", "dramatic", "ambient"}
	AppearanceSpeeds       = []string{"standard", "snappy", "leisurely"}
	AppearanceScrims       = []string{"medium", "light", "strong"}
	AppearanceWidgets      = []string{"links", "text", "note", "search", "date", "weather", "moon", "era", "session"}
	AppearanceSidebars     = []string{"charcoal", "ink", "tinted", "own"}
	AppearanceCorners      = []string{"plain", "subtitle", "banner"}
	AppearanceGlows        = []string{"accent", "own"}
	AppearanceHeights      = []string{"slim", "tall"}
	AppearanceHoverCards   = []string{"paper", "plain", "night", "compact"}
	// AppearanceSheetStyles are the character sheet styles sheet_styles.css
	// knows, keyed by the data-cz-sheet value; "modern" is the unstyled default.
	AppearanceSheetStyles = []string{"modern", "parchment", "ledger", "journal", "vellum", "night", "deck", "pencil", "starship", "neon", "runes", "brass"}
)

// Text limits the Customize page shows. Brand name and welcome keep their
// older server limits (40, 500) so a longer message saved before is never
// refused on an unrelated save.
const (
	appearanceWelcomeMax = 500
	appearanceTextMax    = 200
	// appearanceWidgetMax is how many widgets the header holds: past four the
	// centre of the bar has no room on a laptop.
	appearanceWidgetMax = 4
	// appearanceSubtitleMax keeps the menu's subtitle to one short line.
	appearanceSubtitleMax = 40
)

// AppearanceInput is one Save from the Customize page: the whole draft, so
// every setting lands in a single write and nothing saved by half.
type AppearanceInput struct {
	Look    string                `json:"look"`
	Brand   AppearanceBrandInput  `json:"brand"`
	Header  AppearanceHeaderInput `json:"header"`
	Colours AppearanceColourInput `json:"colours"`
	Nav     struct {
		Style    string `json:"style"`
		Strength string `json:"strength"`
		PageName string `json:"pageName"`
	} `json:"nav"`
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
	Sidebar AppearanceSidebarInput `json:"sidebar"`
	Hover   struct {
		Look string `json:"look"`
	} `json:"hover"`
	Sheet struct {
		Style string `json:"style"`
	} `json:"sheet"`
}

// AppearanceSidebarInput is the Sidebar section. Colour is the menu colour
// choice; Own, GlowColour and Banner only matter while the choice that uses
// them is selected, and are dropped otherwise so nothing stale is stored.
type AppearanceSidebarInput struct {
	Colour     string `json:"colour"`
	Own        string `json:"own"`
	Corner     string `json:"corner"`
	Subtitle   string `json:"subtitle"`
	Banner     string `json:"banner"`
	Glow       string `json:"glow"`
	GlowColour string `json:"glowColour"`
}

// AppearanceBrandInput is the Brand section. Pictures are media filenames
// from the Customize picture upload; empty removes the picture.
type AppearanceBrandInput struct {
	Name     string `json:"name"`
	Logo     string `json:"logo"`
	Welcome  string `json:"welcome"`
	Backdrop string `json:"backdrop"`
}

// AppearanceHeaderInput is the Header section. Bg "" is the default header
// that matches the page.
type AppearanceHeaderInput struct {
	Bg      string       `json:"bg"`
	Height  string       `json:"height"`
	Color   string       `json:"color"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Dir     string       `json:"dir"`
	Image   string       `json:"image"`
	Scrim   string       `json:"scrim"`
	Widgets []string     `json:"widgets"`
	Links   []TopbarLink `json:"links"`
	Text    string       `json:"text"`
}

// AppearanceColourInput is the Colours section. S1 and S2 empty follow the
// chrome accent.
type AppearanceColourInput struct {
	Accent   string `json:"accent"`
	S1       string `json:"s1"`
	S2       string `json:"s2"`
	Page     string `json:"page"`
	Contrast string `json:"contrast"`
}

// AppearancePictures lists the picture filenames one Save writes, so the
// handler can check each belongs to this campaign before the service runs.
func (in *AppearanceInput) AppearancePictures() []string {
	var out []string
	for _, p := range []string{in.Brand.Logo, in.Brand.Backdrop, in.Header.Image, in.Sidebar.Banner} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// oneOf normalises a choice: empty or the default becomes "", anything in
// allowed passes, anything else is refused with the setting's name.
func oneOf(name, v string, allowed []string) (string, error) {
	if v == "" || v == allowed[0] {
		return "", nil
	}
	for _, a := range allowed[1:] {
		if v == a {
			return v, nil
		}
	}
	return "", apperror.NewBadRequest(fmt.Sprintf("invalid %s", name))
}

// calmColour validates an optional #RRGGBB and tones it down the way the
// colour picker does, so a hand-sent request can't store a neon colour the
// page would never offer. deep allows the darker range header colours use.
func calmColour(name, v string, deep bool) (string, error) {
	if v == "" {
		return "", nil
	}
	if !isValidHexColor(v) {
		return "", apperror.NewBadRequest(fmt.Sprintf("invalid %s, expected #RRGGBB", name))
	}
	out, _ := colour.Tame(strings.ToLower(v), deep)
	return out, nil
}

// pictureName accepts a stored media filename, which the media service
// files under its month ("2026/09/<id>.png"), or empty. Ownership is
// checked by the handler; this only refuses anything that could climb out
// of the media directory.
func pictureName(name, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	bad := len(v) > 255 || strings.ContainsRune(v, '\\')
	for _, part := range strings.Split(v, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			bad = true
		}
	}
	if bad {
		return "", apperror.NewBadRequest(fmt.Sprintf("invalid %s picture", name))
	}
	return v, nil
}

// applyAppearance validates in and writes it over s, returning the backdrop
// filename (nil to remove) since the backdrop lives in its own column. It
// is pure so the rules are testable without a database.
func applyAppearance(s *CampaignSettings, in AppearanceInput) (*string, error) {
	var err error
	fail := func(e error) (*string, error) { return nil, e }

	// Brand.
	name := strings.TrimSpace(in.Brand.Name)
	if utf8.RuneCountInString(name) > 40 {
		return fail(apperror.NewBadRequest("brand name must be 40 characters or fewer"))
	}
	if utf8.RuneCountInString(in.Brand.Welcome) > appearanceWelcomeMax {
		return fail(apperror.NewBadRequest("welcome message must be 500 characters or fewer"))
	}
	logo, err := pictureName("logo", in.Brand.Logo)
	if err != nil {
		return fail(err)
	}
	backdrop, err := pictureName("backdrop", in.Brand.Backdrop)
	if err != nil {
		return fail(err)
	}

	// Header background.
	h := in.Header
	style := &TopbarStyle{}
	switch h.Bg {
	case "":
		style = nil
	case "solid":
		style.Mode = "solid"
		if style.Color, err = calmColour("header colour", h.Color, true); err != nil {
			return fail(err)
		}
		if style.Color == "" {
			return fail(apperror.NewBadRequest("a solid header needs a colour"))
		}
	case "gradient", "moving":
		// Moving is the same two colours and direction, drifting; the page
		// decides whether it may move.
		style.Mode = h.Bg
		if style.GradientFrom, err = calmColour("gradient start", h.From, true); err != nil {
			return fail(err)
		}
		if style.GradientTo, err = calmColour("gradient end", h.To, true); err != nil {
			return fail(err)
		}
		if style.GradientFrom == "" || style.GradientTo == "" {
			return fail(apperror.NewBadRequest("a gradient header needs two colours"))
		}
		switch h.Dir {
		case "", "to-r", "to-br", "to-b":
			style.GradientDir = h.Dir
		default:
			return fail(apperror.NewBadRequest("invalid gradient direction"))
		}
	case "sky":
		// The sky draws itself from the campaign's calendar; nothing to set.
		style.Mode = "sky"
	case "image":
		style.Mode = "image"
		if style.ImagePath, err = pictureName("header", h.Image); err != nil {
			return fail(err)
		}
		if style.ImagePath == "" {
			return fail(apperror.NewBadRequest("an image header needs a picture"))
		}
		if style.Scrim, err = oneOf("header shade", h.Scrim, AppearanceScrims); err != nil {
			return fail(err)
		}
	default:
		return fail(apperror.NewBadRequest("invalid header background"))
	}

	// Header widgets, in the owner's order, at most appearanceWidgetMax.
	content := &TopbarContent{Mode: "none", Widgets: []string{}}
	if len(h.Widgets) > appearanceWidgetMax {
		return fail(apperror.NewBadRequest("the header holds at most 4 widgets"))
	}
	seen := map[string]bool{}
	for _, w := range h.Widgets {
		if _, err := oneOf("header widget", w, append([]string{""}, AppearanceWidgets...)); err != nil || w == "" || seen[w] {
			return fail(apperror.NewBadRequest("invalid header widgets"))
		}
		seen[w] = true
		content.Widgets = append(content.Widgets, w)
	}
	if utf8.RuneCountInString(h.Text) > appearanceTextMax {
		return fail(apperror.NewBadRequest("header text must be 200 characters or fewer"))
	}
	if len(h.Links) > 8 {
		return fail(apperror.NewBadRequest("maximum 8 header links"))
	}
	links := make([]TopbarLink, 0, len(h.Links))
	for _, l := range h.Links {
		l.Label = strings.TrimSpace(l.Label)
		l.URL = strings.TrimSpace(l.URL)
		if l.Label == "" && l.URL == "" {
			continue // an empty row the owner never filled in
		}
		if utf8.RuneCountInString(l.Label) > 30 {
			return fail(apperror.NewBadRequest("link label must be 30 characters or fewer"))
		}
		if l.URL == "" {
			return fail(apperror.NewBadRequest("link URL is required"))
		}
		if err := validateNavLinkURL(l.Label, l.URL); err != nil {
			return fail(err)
		}
		links = append(links, l)
	}
	normalizeNavIcons(links, func(l *TopbarLink) *string { return &l.Icon })
	content.Links = links
	content.Quote = strings.TrimSpace(h.Text)
	// Mode keeps older readers working: the first widget, in its old name.
	// Widgets that older readers never knew show as "widgets", which they
	// ignore, so a rollback hides them rather than misreading them.
	if len(content.Widgets) > 0 {
		content.Mode = "widgets"
		if m, ok := map[string]string{"links": "links", "text": "quote"}[content.Widgets[0]]; ok {
			content.Mode = m
		}
	}

	// Colours. Buttons follow the chrome accent and apps follow Surface A,
	// so the older Action and App slots are cleared.
	var accent, s1, s2 string
	if accent, err = calmColour("chrome accent", in.Colours.Accent, false); err != nil {
		return fail(err)
	}
	if s1, err = calmColour("surface A", in.Colours.S1, false); err != nil {
		return fail(err)
	}
	if s2, err = calmColour("surface B", in.Colours.S2, false); err != nil {
		return fail(err)
	}

	var a Appearance
	checks := []struct {
		dst     *string
		name, v string
		allowed []string
	}{
		{&a.Look, "look", in.Look, AppearanceLooks},
		{&a.NavStyle, "menu highlight", in.Nav.Style, AppearanceNavStyles},
		{&a.NavStrength, "highlight strength", in.Nav.Strength, AppearanceNavStrengths},
		{&a.NavPageName, "page name", in.Nav.PageName, AppearanceNavPageNames},
		{&a.PageTone, "page tone", in.Colours.Page, AppearancePageTones},
		{&a.Contrast, "text contrast", in.Colours.Contrast, AppearanceContrasts},
		{&a.BodyFont, "text font", in.Type.Body, AppearanceBodyFonts},
		{&a.HeadingFont, "heading font", in.Type.Heading, AppearanceHeadingFonts},
		{&a.TypeScale, "text size", in.Type.Scale, AppearanceTypeScales},
		{&a.ButtonStyle, "button style", in.Buttons.Style, AppearanceButtonStyles},
		{&a.Elevation, "elevation", in.Motion.Elevation, AppearanceElevations},
		{&a.MotionSpeed, "motion speed", in.Motion.Speed, AppearanceSpeeds},
		{&a.HeaderHeight, "header height", h.Height, AppearanceHeights},
		{&a.SidebarColour, "menu colour", in.Sidebar.Colour, AppearanceSidebars},
		{&a.SidebarCorner, "menu corner", in.Sidebar.Corner, AppearanceCorners},
		{&a.PeekGlow, "peek glow", in.Sidebar.Glow, AppearanceGlows},
		{&a.HoverCard, "hover card look", in.Hover.Look, AppearanceHoverCards},
		{&a.SheetStyle, "character sheet style", in.Sheet.Style, AppearanceSheetStyles},
	}
	for _, c := range checks {
		if *c.dst, err = oneOf(c.name, c.v, c.allowed); err != nil {
			return fail(err)
		}
	}
	a.ReduceMotion = in.Motion.ReduceAll
	if err := applySidebar(&a, in.Sidebar); err != nil {
		return fail(err)
	}

	s.BrandName = name
	s.BrandLogo = logo
	s.WelcomeMessage = in.Brand.Welcome
	s.TopbarStyle = style
	s.TopbarContent = content
	s.AccentColor = accent
	s.AccentSurface1 = s1
	s.AccentSurface2 = s2
	s.AccentAction = ""
	s.AccentApp = ""
	// The text font replaces the older font_family choice.
	s.FontFamily = ""
	if a == (Appearance{}) {
		s.Appearance = nil
	} else {
		s.Appearance = &a
	}

	if backdrop == "" {
		return nil, nil
	}
	return &backdrop, nil
}

// applySidebar validates the choices that go with the menu colour, corner and
// glow already normalised into a. A companion value (own colour, banner,
// subtitle, glow colour) is kept only while the choice that uses it is
// selected, and a choice that needs its companion is refused without one.
func applySidebar(a *Appearance, in AppearanceSidebarInput) error {
	var err error
	if a.SidebarColour == "own" {
		if a.SidebarOwn, err = calmColour("menu colour", in.Own, true); err != nil {
			return err
		}
		if a.SidebarOwn == "" {
			return apperror.NewBadRequest("your own menu colour needs a colour")
		}
	}
	switch a.SidebarCorner {
	case "subtitle":
		sub := strings.TrimSpace(in.Subtitle)
		if utf8.RuneCountInString(sub) > appearanceSubtitleMax {
			return apperror.NewBadRequest("the menu subtitle must be 40 characters or fewer")
		}
		if strings.IndexFunc(sub, unicode.IsControl) >= 0 {
			return apperror.NewBadRequest("the menu subtitle must be one line of text")
		}
		a.SidebarSubtitle = sub
	case "banner":
		if a.SidebarBanner, err = pictureName("menu banner", in.Banner); err != nil {
			return err
		}
		if a.SidebarBanner == "" {
			return apperror.NewBadRequest("a banner corner needs a picture")
		}
	}
	if a.PeekGlow == "own" {
		if a.PeekGlowColour, err = calmColour("glow colour", in.GlowColour, false); err != nil {
			return err
		}
		if a.PeekGlowColour == "" {
			return apperror.NewBadRequest("your own glow needs a colour")
		}
	}
	return nil
}

// SaveAppearance writes one Customize page Save: every setting at once, so
// a failure leaves the campaign exactly as it was.
func (s *campaignService) SaveAppearance(ctx context.Context, campaignID string, in AppearanceInput) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	settings := campaign.ParseSettings()
	backdrop, err := applyAppearance(&settings, in)
	if err != nil {
		return err
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	// The backdrop has its own column. It goes first: a failure there
	// leaves the settings untouched, and the next Save retries both.
	if !samePath(campaign.BackdropPath, backdrop) {
		if err := s.repo.UpdateBackdropPath(ctx, campaignID, backdrop); err != nil {
			return err
		}
	}
	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// samePath compares two optional paths, treating nil and "" alike.
func samePath(a, b *string) bool {
	av, bv := "", ""
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}
