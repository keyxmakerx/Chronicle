// customize_summary.go words what is chosen now in each of Customize's own
// areas, the way a phone's settings show the network's name on the Wi-Fi
// row: the look by its starter's name, the colour by its name, the page
// style by its kit. The Customize block of Campaign settings reads these;
// the names are the ones the Look editor shows (customize_look.js), never a
// hex code or an internal id.

package campaigns

import (
	"context"
	"strings"
)

// lookColourNames are the names the Look editor gives the colours it offers,
// in the order its colourName() looks them up (PRESETS, ACCENT_PRESETS,
// HDR_COLOURS, then LOOK_COLOUR_NAMES); the first name for a hex wins.
// TestLookColourNamesMatchEditor keeps the two tables equal.
var lookColourNames = [][2]string{
	// PRESETS
	{"#6366f1", "Indigo"}, {"#3b82f6", "Blue"}, {"#06b6d4", "Cyan"}, {"#10b981", "Emerald"},
	{"#f59e0b", "Amber"}, {"#f43f5e", "Rose"}, {"#a855f7", "Purple"}, {"#f97316", "Orange"},
	// ACCENT_PRESETS
	{"#6366f1", "Indigo"}, {"#3b82f6", "Blue"}, {"#0ea5e9", "Sky"}, {"#06b6d4", "Cyan"},
	{"#0d9488", "Teal"}, {"#10b981", "Emerald"}, {"#a855f7", "Purple"}, {"#ec4899", "Pink"},
	// HDR_COLOURS
	{"#0f172a", "Night"}, {"#1e2a5a", "Deep blue"}, {"#1f2937", "Slate"}, {"#3b2a1c", "Umber"},
	{"#14352a", "Pine"}, {"#2f4a2c", "Moss"}, {"#4a1512", "Oxblood"}, {"#3b1d5e", "Plum"},
	// LOOK_COLOUR_NAMES
	{"#9a4a26", "Sienna"}, {"#8f2d2d", "Wax red"}, {"#8a6a1f", "Ochre"}, {"#5b6be0", "Moonlit blue"},
	{"#2563eb", "Blue"}, {"#64748b", "Slate"}, {"#2f7d4f", "Moss green"}, {"#a16207", "Lantern"},
	{"#4d7c0f", "Fern"}, {"#c2410c", "Ember"}, {"#b91c1c", "Garnet"}, {"#b45309", "Brass"},
	{"#0e7490", "Glacier"}, {"#7c3aed", "Violet"}, {"#a21caf", "Orchid"}, {"#94651b", "Candle gold"},
	{"#0f766e", "Console teal"}, {"#0369a1", "Signal blue"},
}

// lookColourName is a colour's name in the Look editor, or "" for a colour
// the owner mixed themselves, which has none.
func lookColourName(hex string) string {
	h := strings.ToLower(hex)
	for _, c := range lookColourNames {
		if c[0] == h {
			return c[1]
		}
	}
	return ""
}

// lookStarterNames are the starter looks' names, in AppearanceLooks order.
var lookStarterNames = map[string]string{
	"classic": "Classic", "parchment": "Parchment", "midnight": "Midnight", "forest": "Forest",
	"ember": "Ember", "frost": "Frost", "arcane": "Arcane", "starship": "Starship",
}

// matchesLookPreset reports whether the saved style is exactly a starter
// look's, the test the Look editor's matchesLook makes. A header's unused
// colours are not compared: a solid header stores no gradient, so its
// gradient colours are only the editor's starting values.
func matchesLookPreset(d customizeDraft, p lookPreset) bool {
	eq := strings.EqualFold
	opt := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	if d.Header.Bg != p.HeaderBg {
		return false
	}
	if p.HeaderBg == "solid" {
		if !eq(d.Header.Solid, p.HeaderSolid) {
			return false
		}
	} else if !eq(d.Header.From, p.HeaderFrom) || !eq(d.Header.To, p.HeaderTo) || d.Header.Dir != p.HeaderDir {
		return false
	}
	return d.Nav.Style == p.NavStyle && d.Nav.Strength == p.NavStrength &&
		eq(d.Colours.Accent, p.Accent) && eq(opt(d.Colours.S1), p.S1) && eq(opt(d.Colours.S2), p.S2) &&
		d.Colours.Sidebar == p.Sidebar && d.Colours.Page == p.Page && d.Colours.Contrast == p.Contrast &&
		d.Type.Body == p.Body && d.Type.Heading == p.Heading && d.Type.Scale == p.Scale &&
		d.Buttons.Style == p.Button && d.Motion.Elevation == p.Elevation && d.Motion.Speed == p.Speed
}

// lookStarterName is the starter look the saved style matches, the one it
// was started from first; "" when it matches none (the owner changed it).
func lookStarterName(d customizeDraft) string {
	if p, ok := findLookPreset(d.Look); ok && matchesLookPreset(d, p) {
		return lookStarterNames[p.ID]
	}
	for _, p := range lookPresets {
		if matchesLookPreset(d, p) {
			return lookStarterNames[p.ID]
		}
	}
	return ""
}

// lookPagesWords names how pages are drawn: the style kit under the Material
// page look ("Sketchbook pages"), or "Standard pages".
func lookPagesWords(d customizeDraft) string {
	if d.Page.Look != "annals" {
		return "Standard pages"
	}
	kit := d.Page.Kit
	if kit == "" {
		kit = "material"
	}
	return LookChoiceLabel(kit) + " pages"
}

// LookSummary is the look in a DM's words: the starter look's name or
// "Custom", the colour's name ("" for a colour of the owner's own, which the
// row shows as its swatch alone) and the page style.
type LookSummary struct {
	Look, Colour, Pages string
}

// lookSummaryOf reads the saved look of a campaign into words.
func lookSummaryOf(ctx context.Context, c *Campaign) LookSummary {
	st := buildCustomizeState(ctx, &CampaignContext{Campaign: c})
	d := st.Draft
	s := LookSummary{Look: lookStarterName(d), Colour: lookColourName(d.Colours.Accent), Pages: lookPagesWords(d)}
	if s.Look == "" {
		s.Look = "Custom"
	}
	return s
}

// categoriesSummaryOf counts the campaign's categories as the sidebar does
// (kinds sit under their category and are not counted again) and how many
// of those only the DM team sees.
func categoriesSummaryOf(cats []CustomizeCategory) (count, hidden int) {
	for _, c := range cats {
		if c.ParentID != nil {
			continue
		}
		count++
		if c.DMOnly {
			hidden++
		}
	}
	return count, hidden
}

// homesSummaryOf says who sees which home in a few words: the Everyone home,
// then the audiences that have a home of their own, or that players have
// none yet.
func homesSummaryOf(c *Campaign, gm string) (text, sub string) {
	var own []string
	for _, t := range homeTabs {
		if t.Key == HomeDefault || !c.HasSavedHome(t.Key) {
			continue
		}
		own = append(own, strings.ReplaceAll(t.Label, "{gm}", gm))
	}
	if len(own) == 0 {
		return "Everyone", "no player home yet"
	}
	return "Everyone", "own home for " + strings.Join(own, ", ")
}
