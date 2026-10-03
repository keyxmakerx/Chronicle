package campaigns

// lookPreset is one Customize look's values, the Go twin of an entry in the
// LOOKS table in static/js/widgets/customize_look.js. A new campaign is seeded
// from it so it renders in the site look at once and Customize opens on it.
// test/js/fixtures/look_pins.json pins both tables to the same values.
type lookPreset struct {
	ID string

	// Header: HeaderBg is "solid" or "gradient". A solid header whose
	// colour is "page" is the default header, which stores nothing.
	HeaderBg, HeaderSolid, HeaderFrom, HeaderTo, HeaderDir string

	NavStyle, NavStrength string

	// Accent, S1 and S2 are the chrome accent and the two surface colours;
	// an empty S1 or S2 follows the accent.
	Accent, S1, S2           string
	Sidebar, Page, Contrast  string
	Body, Heading, Scale     string
	Button, Elevation, Speed string
}

// lookPresets are the eight looks, in AppearanceLooks order.
var lookPresets = []lookPreset{
	{ID: "classic", HeaderBg: "solid", HeaderSolid: "page", HeaderFrom: "#0f172a", HeaderTo: "#1e2a5a", HeaderDir: "r",
		NavStyle: "ring", NavStrength: "calm", Accent: "#6366f1", Sidebar: "charcoal", Page: "cool", Contrast: "standard",
		Body: "inter", Heading: "same", Scale: "standard", Button: "lift", Elevation: "standard", Speed: "standard"},
	{ID: "parchment", HeaderBg: "solid", HeaderSolid: "#3b2a1c", HeaderFrom: "#3b2a1c", HeaderTo: "#4a1512", HeaderDir: "r",
		NavStyle: "tab", NavStrength: "calm", Accent: "#9a4a26", S1: "#8f2d2d", S2: "#8a6a1f", Sidebar: "tinted", Page: "paper", Contrast: "standard",
		Body: "literata", Heading: "imfell", Scale: "roomy", Button: "press", Elevation: "flat", Speed: "leisurely"},
	{ID: "midnight", HeaderBg: "gradient", HeaderSolid: "#0f172a", HeaderFrom: "#0f172a", HeaderTo: "#1e2a5a", HeaderDir: "r",
		NavStyle: "ring", NavStrength: "calm", Accent: "#5b6be0", S1: "#2563eb", S2: "#64748b", Sidebar: "ink", Page: "cool", Contrast: "standard",
		Body: "inter", Heading: "cormorant", Scale: "standard", Button: "glow", Elevation: "dramatic", Speed: "standard"},
	{ID: "forest", HeaderBg: "gradient", HeaderSolid: "#14352a", HeaderFrom: "#14352a", HeaderTo: "#2f4a2c", HeaderDir: "r",
		NavStyle: "breathe", NavStrength: "calm", Accent: "#2f7d4f", S1: "#a16207", S2: "#4d7c0f", Sidebar: "tinted", Page: "warm", Contrast: "standard",
		Body: "sourceserif", Heading: "marcellus", Scale: "standard", Button: "lift", Elevation: "standard", Speed: "leisurely"},
	{ID: "ember", HeaderBg: "gradient", HeaderSolid: "#1f2937", HeaderFrom: "#1f2937", HeaderTo: "#4a1512", HeaderDir: "br",
		NavStyle: "comet", NavStrength: "calm", Accent: "#c2410c", S1: "#b91c1c", S2: "#b45309", Sidebar: "charcoal", Page: "warm", Contrast: "standard",
		Body: "merriweather", Heading: "cinzel", Scale: "standard", Button: "press", Elevation: "dramatic", Speed: "snappy"},
	{ID: "frost", HeaderBg: "solid", HeaderSolid: "page", HeaderFrom: "#0f172a", HeaderTo: "#1e2a5a", HeaderDir: "r",
		NavStyle: "edge", NavStrength: "calm", Accent: "#0e7490", S1: "#2563eb", S2: "#64748b", Sidebar: "ink", Page: "cool", Contrast: "high",
		Body: "sourcesans", Heading: "josefin", Scale: "standard", Button: "ink", Elevation: "flat", Speed: "snappy"},
	{ID: "arcane", HeaderBg: "gradient", HeaderSolid: "#3b1d5e", HeaderFrom: "#0f172a", HeaderTo: "#3b1d5e", HeaderDir: "r",
		NavStyle: "tide", NavStrength: "calm", Accent: "#7c3aed", S1: "#a21caf", S2: "#94651b", Sidebar: "tinted", Page: "cool", Contrast: "standard",
		Body: "lora", Heading: "fraunces", Scale: "standard", Button: "glow", Elevation: "dramatic", Speed: "leisurely"},
	{ID: "starship", HeaderBg: "solid", HeaderSolid: "#0f172a", HeaderFrom: "#0f172a", HeaderTo: "#1f2937", HeaderDir: "r",
		NavStyle: "rail", NavStrength: "lively", Accent: "#0f766e", S1: "#b45309", S2: "#0369a1", Sidebar: "ink", Page: "cool", Contrast: "standard",
		Body: "atkinson", Heading: "chakra", Scale: "compact", Button: "ink", Elevation: "standard", Speed: "snappy"},
}

// lookDirs maps the editor's gradient direction codes to the stored ones.
var lookDirs = map[string]string{"r": "to-r", "br": "to-br", "b": "to-b"}

// findLookPreset returns the preset for a look id.
func findLookPreset(id string) (lookPreset, bool) {
	for _, p := range lookPresets {
		if p.ID == id {
			return p, true
		}
	}
	return lookPreset{}, false
}

// input is the Customize Save a person would send after picking this look,
// so seeding goes through applyAppearance and its validation like any save.
func (p lookPreset) input() AppearanceInput {
	var in AppearanceInput
	in.Look = p.ID
	switch {
	case p.HeaderBg == "solid" && p.HeaderSolid == "page":
		in.Header.Bg = "" // The default header matches the page.
	case p.HeaderBg == "solid":
		in.Header.Bg, in.Header.Color = "solid", p.HeaderSolid
	default:
		in.Header.Bg, in.Header.From, in.Header.To, in.Header.Dir = p.HeaderBg, p.HeaderFrom, p.HeaderTo, lookDirs[p.HeaderDir]
	}
	in.Nav.Style, in.Nav.Strength = p.NavStyle, p.NavStrength
	in.Colours = AppearanceColourInput{Accent: p.Accent, S1: p.S1, S2: p.S2, Page: p.Page, Contrast: p.Contrast}
	in.Sidebar.Colour = p.Sidebar
	in.Type.Body, in.Type.Heading, in.Type.Scale = p.Body, p.Heading, p.Scale
	in.Buttons.Style = p.Button
	in.Motion.Elevation, in.Motion.Speed = p.Elevation, p.Speed
	return in
}

// seedLook writes the look's full style over s, as Customize would on Save.
func seedLook(s *CampaignSettings, id string) bool {
	p, ok := findLookPreset(id)
	if !ok {
		return false
	}
	_, err := applyAppearance(s, p.input())
	return err == nil
}
