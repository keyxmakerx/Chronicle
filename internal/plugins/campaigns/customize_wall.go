// customize_wall.go: Customize's first page. It is a wall of small pictures,
// one for everything the owner can change, each drawn in the campaign's own
// look. The pictures are grouped by what they change: the parts every page
// shares, each kind of page, each home, and each app. Pressing a picture opens
// that part's workshop (customize_shell.js grows it; the miniatures are drawn
// by customize_wall.templ).

package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// wallTile is one picture on the wall.
type wallTile struct {
	// Kind picks the miniature; Key is stable for the grow (the tile that
	// grew is the one the way back returns to).
	Kind, Key string
	Name      string
	// Line is what is set now, in a few words.
	Line string
	Href string
	// Share names the shared part this picture stands for ("frame" or
	// "colours"): hovering it lights that part in every other picture.
	Share string
	// Dot is the area whose unsaved changes mark the picture.
	Dot string
	// Page is a real page's name in the category (empty when it has none),
	// Cat the category's own name, Fields the count the picture draws, Colour
	// the category's colour and Icon the tab's icon.
	Page, Cat, Colour, Icon string
	Fields                  int
	// Home is the audience whose home the picture draws.
	Home string
}

// wallGroup is one heading and its pictures.
type wallGroup struct {
	ID, Title, Note string
	// MoreLabel and MoreHref are a quiet word beside the heading ("All
	// categories").
	MoreLabel, MoreHref string
	Tiles               []wallTile
}

// wallApp is a plugin's tab as the wall needs it.
type wallApp struct {
	ID, Label, Icon, Line string
}

// wallHome is one home Customize has.
type wallHome struct {
	Key, Label string
	Own        bool
}

// wallInput is what the groups are made from.
type wallInput struct {
	Base  string
	Cats  []CustomizeCategory
	Homes []wallHome
	Apps  []wallApp
	Look  customizeDraft
}

// buildWallGroups lays the wall out. Everything it names comes from the
// campaign: its categories, its homes and the tabs plugins registered, so a new
// plugin tab adds a picture with no change here.
func buildWallGroups(in wallInput) []wallGroup {
	d := in.Look
	look := in.Base + "/look"
	shared := wallGroup{ID: "shared", Title: "Shared by every page", Note: "Hover one to see where it shows", Tiles: []wallTile{
		{Kind: "frame", Key: "frame", Name: "Site frame", Line: frameWords(d), Href: look + "#frame", Share: "frame", Dot: "look"},
		{Kind: "colours", Key: "colours", Name: "Colours and type", Line: coloursWords(d), Href: look + "#style", Share: "colours", Dot: "look"},
		{Kind: "page", Key: "page", Name: "Page style", Line: lookPagesWords(d), Href: look + "#page", Dot: "look"},
		{Kind: "hover", Key: "hover", Name: "Hover cards", Line: choiceWords(d.Hover.Look, "Paper"), Href: look + "#hover", Dot: "look"},
		{Kind: "sheet", Key: "sheet", Name: "Character sheets", Line: choiceWords(d.Sheet.Style, "Modern"), Href: look + "#sheet", Dot: "look"},
		{Kind: "email", Key: "email", Name: "Emails", Line: choiceWords(d.Email.Layout, "Note"), Href: look + "#email", Dot: "look"},
	}}

	kinds := wallGroup{ID: "kinds", Title: "Each kind of page", MoreLabel: "All categories", MoreHref: in.Base + "/categories"}
	for _, c := range in.Cats {
		layouts := 1
		if c.HomeLayout {
			layouts = 2
		}
		kinds.Tiles = append(kinds.Tiles, wallTile{
			Kind: "category", Key: fmt.Sprintf("cat-%d", c.ID), Name: c.NamePlural,
			Line: fmt.Sprintf("%s · %s", wallCount(layouts, "layout"), wallCount(c.Fields, "field")),
			Href: fmt.Sprintf("%s/categories/%d/layout", in.Base, c.ID), Dot: "categories",
			Page: c.Sample, Cat: c.Name, Colour: partSwatch(c.Color), Icon: c.Icon, Fields: c.Fields,
		})
	}

	homes := wallGroup{ID: "homes", Title: "Home pages"}
	for _, h := range in.Homes {
		line := "Same as the Everyone home"
		switch {
		case h.Key == HomeDefault:
			line = "The campaign home"
		case h.Own:
			line = "Its own home"
		}
		homes.Tiles = append(homes.Tiles, wallTile{
			Kind: "home", Key: "home-" + h.Key, Name: h.Label, Line: line,
			Href: in.Base + "/home/" + h.Key, Home: h.Key,
		})
	}

	apps := wallGroup{ID: "apps", Title: "Apps"}
	for _, a := range in.Apps {
		apps.Tiles = append(apps.Tiles, wallTile{
			Kind: "app", Key: "app-" + a.ID, Name: a.Label, Line: a.Line,
			Href: in.Base + "/x/" + a.ID, Cat: a.ID, Icon: a.Icon,
		})
	}

	out := []wallGroup{shared}
	for _, g := range []wallGroup{kinds, homes, apps} {
		if len(g.Tiles) > 0 {
			out = append(out, g)
		}
	}
	return out
}

func wallCount(n int, word string) string { return plural(n, word, word+"s") }

// choiceWords names a saved choice the way the Look editor does; unset reads
// as the default's name.
func choiceWords(id, dflt string) string {
	if id == "" {
		id = strings.ToLower(dflt)
	}
	l := LookChoiceLabel(id)
	if l == id && l != "" {
		l = strings.ToUpper(l[:1]) + l[1:]
	}
	return l
}

var wallHeaderWords = map[string]string{
	"solid": "Solid", "gradient": "Gradient", "moving": "Moving colour", "image": "Picture", "sky": "Sky",
}

var wallMenuWords = map[string]string{
	"charcoal": "Charcoal", "ink": "Ink", "tinted": "Tinted", "own": "Own colour",
}

// frameWords says the header and the menu in a few words.
func frameWords(d customizeDraft) string {
	h := wallHeaderWords[d.Header.Bg]
	if h == "" {
		h = "Solid"
	}
	if d.Header.Bg == "solid" && d.Header.Solid == "page" {
		h = "Page-coloured"
	}
	m := wallMenuWords[d.Colours.Sidebar]
	if m == "" {
		m = "Charcoal"
	}
	return h + " header · " + m + " menu"
}

// wallBodyFontNames are the text faces' names, in AppearanceBodyFonts order.
var wallBodyFontNames = map[string]string{
	"inter": "Inter", "sourcesans": "Source Sans 3", "atkinson": "Atkinson Hyperlegible", "literata": "Literata",
	"sourceserif": "Source Serif 4", "lora": "Lora", "alegreya": "Alegreya", "merriweather": "Merriweather",
}

// wallHeadingFontNames are the heading faces' names; "same" has none.
var wallHeadingFontNames = map[string]string{
	"cinzel": "Cinzel", "marcellus": "Marcellus", "imfell": "IM Fell English", "cormorant": "Cormorant Garamond",
	"fraunces": "Fraunces", "playfair": "Playfair Display", "josefin": "Josefin Sans", "chakra": "Chakra Petch",
}

// coloursWords is the look's name, its colour and its type, the words the
// Look editor shows.
func coloursWords(d customizeDraft) string {
	parts := []string{}
	if n := lookStarterName(d); n != "" {
		parts = append(parts, n)
	} else {
		parts = append(parts, "Custom")
	}
	if c := lookColourName(d.Colours.Accent); c != "" {
		parts = append(parts, c)
	}
	face := wallHeadingFontNames[d.Type.Heading]
	if face == "" {
		face = wallBodyFontNames[d.Type.Body]
	}
	if face != "" {
		parts = append(parts, face)
	}
	return strings.Join(parts, " · ")
}

// overlayLookDraft lays the owner's unsaved look over the saved one, so the
// pictures show what they will get. A draft that does not read leaves the
// saved look as it is.
func overlayLookDraft(d *customizeDraft, draft []byte) {
	sections, err := parseCustomizeDraft(draft)
	if err != nil {
		return
	}
	raw, ok := sections[customizeSectionLook]
	if !ok {
		return
	}
	var held struct {
		Draft json.RawMessage `json:"draft"`
	}
	if json.Unmarshal(raw, &held) != nil || len(held.Draft) == 0 {
		return
	}
	next := *d
	if json.Unmarshal(held.Draft, &next) == nil {
		*d = next
	}
}

// customizeWall gathers the wall's groups for one campaign.
func (h *Handler) customizeWall(ctx context.Context, cc *CampaignContext, cats []CustomizeCategory, draft []byte) ([]wallGroup, wallSkin) {
	state := buildCustomizeState(ctx, cc)
	overlayLookDraft(&state.Draft, draft)
	in := wallInput{Base: h.customizeBase(cc), Cats: cats, Look: state.Draft}
	for _, t := range homeTabs {
		in.Homes = append(in.Homes, wallHome{Key: t.Key, Label: t.label(ctx), Own: cc.Campaign.HasSavedHome(t.Key)})
	}
	for _, t := range h.shownCustomizeTabs(ctx, cc) {
		line := t.Blurb
		if s := t.summary(ctx); s.Text != "" {
			line = joinParts(s.Text, s.Sub)
		}
		in.Apps = append(in.Apps, wallApp{ID: t.ID, Label: t.Label, Icon: strings.TrimSpace(strings.TrimPrefix(t.Icon, "fa-solid ")), Line: line})
	}
	return buildWallGroups(in), skinOf(state.Draft, cc.Campaign.Name, cats)
}
