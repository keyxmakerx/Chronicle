// customize_pages.go: the pages of Customize. Customize opens on a wall of
// pictures (customize_wall.go); each picture leads to a part (Look, a
// category, a home, an app a plugin contributes), each with its own address,
// drawn inside one frame that carries the one draft and its save bar
// (customize_shell.templ, customize_shell.js).

package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// addonCharacterClaiming is the addon that lets players claim a page; the
// "Players can claim" switch only means something while it is on.
const addonCharacterClaiming = "player-character-claiming"

// CustomizeArea is one part of Customize as Find and the save bar know it.
type CustomizeArea struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
	Href  string `json:"href"`
	// Section is the draft section this area edits, so its tab can mark
	// the areas holding unsaved changes. Empty for areas that save by
	// themselves.
	Section string `json:"section,omitempty"`
	// Parts are what the tab's drawer lists: the Look's rooms, every
	// category with its kinds, each home. Empty for an area with none.
	Parts []CustomizePart `json:"-"`
	// Part names the part open now, shown after the tab's label.
	Part string `json:"-"`
	// Blurb is what the area holds, for its Overview card, and Finds are the
	// named things inside it for Find. Both come from a plugin's tab.
	Blurb string          `json:"-"`
	Finds []CustomizeFind `json:"-"`
}

// CustomizePart is one line in a tab's drawer.
type CustomizePart struct {
	Label string
	Href  string
	// Key names a Look room, so the Look editor can mark the one open.
	Key   string
	Color string // A category's colour; empty for other parts.
	Pages int    // A category's live pages.
	// Kind is a sub-category, drawn under its parent.
	Kind bool
	// Quiet is a line about the area rather than one part of it ("All
	// categories", "New category").
	Quiet   bool
	Current bool
}

// lookRooms are the Look editor's rooms, in its order (customize_look.js
// ROOMS); a room's key in the address opens it.
var lookRooms = []struct{ Key, Label string }{
	{"frame", "Site frame"},
	{"style", "Colours and type"},
	{"pages", "Pages and sheets"},
	{"people", "People"},
}

// CustomizeCrumb is one step in the area header's trail.
type CustomizeCrumb struct {
	Label string
	Href  string // Empty for the current page.
}

// customizeFindEntry is one result the Find palette can jump to.
type customizeFindEntry struct {
	Label string `json:"label"`
	Path  string `json:"path"` // Where it lives, e.g. "Look".
	Href  string `json:"href"`
	Icon  string `json:"icon"`
	Words string `json:"words,omitempty"` // Other words someone might type for it.
}

// customizeShellData is everything the frame draws around an area.
type customizeShellData struct {
	Title      string
	CampaignID string
	CSRF       string
	Area       string
	Areas      []CustomizeArea
	Crumbs     []CustomizeCrumb
	Version    int
	Draft      string // The draft as JSON, never empty ("{}" when there is none).
	Find       string // The Find palette's entries as JSON.
	// Cat is set inside a category: its one bar. Bar is the bar of any other
	// part. Both are nil on the wall, which has no bar.
	Cat *customizeCatBar
	Bar *customizeBar
	// Wall marks the first page; PartKey is the wall picture this page grew
	// from, so the way back lands on it.
	Wall    bool
	PartKey string
}

// customizeBar is the bar inside a part: the way back to the wall, the part's
// name and its own tabs.
type customizeBar struct {
	Base string
	Name string
	// Dot is the area whose unsaved changes mark the way back.
	Dot  string
	Tabs []customizeBarTab
}

// customizeBarTab is one of a part's tabs. Key names a Look room, so the Look
// editor can mark the one open and the ones holding unsaved changes.
type customizeBarTab struct {
	Label, Href, Key string
	Current          bool
}

// customizeCatBar is a category's bar: the way back to Categories, its name
// (which opens the Categories drawer to switch to another), and its tabs.
// customize_categories.js keeps the name and the tabs' marks in step with the
// draft.
type customizeCatBar struct {
	Key  string
	Name string
	Tab  string
	New  bool // made in the draft: only Settings opens until it is saved
	Base string
}

// customizeCatTab is one of a category's tabs in its bar.
type customizeCatTab struct{ ID, Label string }

// catBarTabs lists a category's tabs for its bar.
func catBarTabs() []customizeCatTab {
	out := make([]customizeCatTab, 0, len(categoryTabs))
	for _, t := range categoryTabs {
		out = append(out, customizeCatTab{ID: t.ID, Label: t.Label})
	}
	return out
}

// lookSections are the Look editor's sections, in its own order, so Find
// can open one by name (the editor reads the id from the address's #).
var lookSections = []struct{ ID, Name, Words string }{
	{"brand", "Brand", "name logo welcome backdrop"},
	{"header", "Header", "top bar background widgets links"},
	{"sidebar", "Sidebar", "menu colour corner banner glow"},
	{"nav", "Navigation", "menu highlight current page"},
	{"colours", "Colours", "colour color accent theme contrast menus links buttons headers events tags badges marks"},
	{"type", "Type", "font fonts text headings size"},
	{"buttons", "Buttons", "button shape press"},
	{"icons", "Icons", "icon icons category colour accent ink names search list"},
	{"motion", "Motion and depth", "animation calm shadows elevation speed"},
	{"page", "Pages", "page look style kit material sharp ornate"},
	{"hover", "Hover cards", "peek preview tooltip popup time line"},
	{"sheet", "Character sheets", "sheet style"},
	{"email", "Emails", "email emails mail invite reminder layout note poster ticket chat test"},
	{"people", "Who can change", "access members players co-dm permission pick own look sheet dm screen quick setups locked table open"},
	{"book", "Rulebook", "rules book"},
}

// categoryTabs are a category workspace's tabs, in order. A category made in
// the draft has only its Settings until it is saved.
var categoryTabs = []struct{ ID, Label, Icon string }{
	{"layout", "Page layout", "fa-table-columns"},
	{"home", "Category home", "fa-house"},
	{"fields", "Fields", "fa-list-ul"},
	{"text", "Starting text", "fa-align-left"},
	{"settings", "Settings", "fa-sliders"},
}

func isCategoryTab(tab string) bool {
	for _, t := range categoryTabs {
		if t.ID == tab {
			return true
		}
	}
	return false
}

// newCategoryKey matches the draft key of a category made in the draft;
// the categories section uses the same shape.
var newCategoryKey = regexp.MustCompile(`^new-[a-z0-9-]{1,40}$`)

func (h *Handler) customizeBase(cc *CampaignContext) string {
	return fmt.Sprintf("/campaigns/%s/customize", cc.Campaign.ID)
}

// customizeAreas lists Customize's parts: its own, then any a plugin
// contributes.
func (h *Handler) customizeAreas(ctx context.Context, cc *CampaignContext) []CustomizeArea {
	base := h.customizeBase(cc)
	areas := []CustomizeArea{
		{ID: "look", Label: "Look", Icon: "fa-palette", Href: base + "/look", Section: "look"},
		{ID: "categories", Label: "Categories", Icon: "fa-shapes", Href: base + "/categories", Section: "categories"},
		{ID: "home", Label: "Home pages", Icon: "fa-table-cells-large", Href: base + "/home"},
	}
	for _, t := range h.shownCustomizeTabs(ctx, cc) {
		// A tab's icon may carry its own style class; the rail adds fa-solid.
		icon := strings.TrimSpace(strings.TrimPrefix(t.Icon, "fa-solid "))
		areas = append(areas, CustomizeArea{ID: "x-" + t.ID, Label: t.Label, Icon: icon, Href: base + "/x/" + t.ID, Section: t.Section, Blurb: t.Blurb, Finds: t.Finds})
	}
	return areas
}

// customizeParts fills the Categories drawer, the switcher in a category's
// bar, and names the category open now. The part comes from the trail the
// area passed: its second step is the category.
func (h *Handler) customizeParts(ctx context.Context, cc *CampaignContext, areas []CustomizeArea, area string, crumbs []CustomizeCrumb, cats []CustomizeCategory) {
	base := h.customizeBase(cc)
	open := ""
	if len(crumbs) > 1 {
		open = crumbs[1].Label
	}
	for i := range areas {
		a := &areas[i]
		here := a.ID == area
		switch a.ID {
		case "categories":
			a.Parts = append(a.Parts, CustomizePart{Label: "All categories", Href: base + "/categories", Quiet: true, Current: here && open == ""})
			for _, c := range cats {
				if c.ParentID != nil {
					continue
				}
				a.Parts = append(a.Parts, categoryPart(base, c, here && open == c.NamePlural, false))
				for _, k := range cats {
					if k.ParentID != nil && *k.ParentID == c.ID {
						a.Parts = append(a.Parts, categoryPart(base, k, here && open == k.NamePlural, true))
					}
				}
			}
			a.Parts = append(a.Parts, CustomizePart{Label: "New category", Href: base + "/categories#new", Quiet: true})
			if here {
				a.Part = open
				if a.Part == "" {
					a.Part = "All categories"
				}
			}
		}
	}
}

// categoryPart is a category's line in the Categories drawer.
func categoryPart(base string, c CustomizeCategory, current, kind bool) CustomizePart {
	return CustomizePart{
		Label: c.NamePlural, Href: fmt.Sprintf("%s/categories/%d/layout", base, c.ID),
		Color: c.Color, Pages: c.Pages, Kind: kind, Current: current,
	}
}

// customizeFind lists what the Find palette can open: every area, every
// Look section and every category.
func (h *Handler) customizeFind(cc *CampaignContext, areas []CustomizeArea, cats []CustomizeCategory) string {
	b, err := json.Marshal(h.customizeFindEntries(cc, areas, cats))
	if err != nil {
		return "[]"
	}
	return string(b)
}

// customizeFindEntries is the Customizer's Find list as data, so the Manage
// desk's Find can offer the same settings from the same source.
func (h *Handler) customizeFindEntries(cc *CampaignContext, areas []CustomizeArea, cats []CustomizeCategory) []customizeFindEntry {
	base := h.customizeBase(cc)
	out := make([]customizeFindEntry, 0, len(areas)+len(lookSections)+len(cats)*2+1)
	for _, a := range areas {
		out = append(out, customizeFindEntry{Label: a.Label, Path: "Customize", Href: a.Href, Icon: a.Icon})
		for _, f := range a.Finds {
			out = append(out, customizeFindEntry{Label: f.Label, Path: a.Label, Href: a.Href, Icon: a.Icon, Words: f.Words})
		}
	}
	for _, s := range lookSections {
		out = append(out, customizeFindEntry{Label: s.Name, Path: "Look", Href: base + "/look#" + s.ID, Icon: "fa-palette", Words: s.Words})
	}
	out = append(out, customizeFindEntry{Label: "New category", Path: "Categories", Href: base + "/categories#new", Icon: "fa-plus", Words: "add create kind"})
	for _, c := range cats {
		href := fmt.Sprintf("%s/categories/%d", base, c.ID)
		out = append(out,
			customizeFindEntry{Label: c.NamePlural, Path: "Categories", Href: href + "/layout", Icon: c.Icon, Words: c.Name + " page layout"},
			customizeFindEntry{Label: c.NamePlural + " settings", Path: "Categories › " + c.NamePlural, Href: href + "/settings", Icon: "fa-sliders", Words: "name icon colour color claim menu kinds delete rename"},
		)
	}
	return out
}

// renderCustomize draws an area inside the frame.
func (h *Handler) renderCustomize(c *echo.Context, cc *CampaignContext, area, title string, crumbs []CustomizeCrumb, cats []CustomizeCategory, body templ.Component) error {
	return h.renderCustomizeIn(c, cc, area, title, crumbs, cats, nil, body)
}

// renderCustomizeIn draws an area inside the frame, inside a category when
// cat is set.
func (h *Handler) renderCustomizeIn(c *echo.Context, cc *CampaignContext, area, title string, crumbs []CustomizeCrumb, cats []CustomizeCategory, cat *customizeCatBar, body templ.Component) error {
	ctx := c.Request().Context()
	version, draft := h.CustomizeDraftFor(ctx, cc.Campaign.ID, auth.GetUserID(c))
	areas := h.customizeAreas(ctx, cc)
	if cats == nil {
		cats = h.CustomizeCategories(ctx, cc.Campaign.ID)
	}
	h.customizeParts(ctx, cc, areas, area, crumbs, cats)
	d := customizeShellData{
		Title:      cc.Campaign.Name + " - " + title,
		CampaignID: cc.Campaign.ID,
		CSRF:       middleware.GetCSRFToken(c),
		Area:       area,
		Areas:      areas,
		Crumbs:     append([]CustomizeCrumb{{Label: "Customize", Href: h.customizeBase(cc)}}, crumbs...),
		Version:    version,
		Draft:      string(draft),
		Find:       h.customizeFind(cc, areas, cats),
		Cat:        cat,
		Wall:       area == "overview",
	}
	if !d.Wall && cat == nil {
		d.Bar = h.customizePartBar(ctx, cc, area, crumbs)
	}
	d.PartKey = h.customizePartKey(ctx, area, crumbs, cat)
	if d.Draft == "" {
		d.Draft = "{}"
	}
	return middleware.Render(c, http.StatusOK, CustomizeShell(d, body))
}

// customizePartBar is the bar of a part other than a category: its name and
// its own tabs. Look's rooms are tabs on the same page; the homes are tabs
// that open each home.
func (h *Handler) customizePartBar(ctx context.Context, cc *CampaignContext, area string, crumbs []CustomizeCrumb) *customizeBar {
	base := h.customizeBase(cc)
	b := &customizeBar{Base: base}
	switch area {
	case "look":
		b.Name, b.Dot = "Look", "look"
		for _, r := range lookRooms {
			b.Tabs = append(b.Tabs, customizeBarTab{Label: r.Label, Href: base + "/look#" + r.Key, Key: r.Key})
		}
	case "categories":
		b.Name, b.Dot = "Categories", "categories"
	case "home":
		b.Name = "Home pages"
		open := ""
		if len(crumbs) > 1 {
			open = crumbs[1].Label
		}
		for _, t := range homeTabs {
			name := t.label(ctx)
			b.Tabs = append(b.Tabs, customizeBarTab{Label: name, Href: base + "/home/" + t.Key, Current: name == open})
		}
	default:
		if len(crumbs) > 0 {
			b.Name = crumbs[0].Label
		}
	}
	return b
}

// customizePartKey names the wall picture a part grew from, for the way back
// when the page was opened without pressing one (a reload, a Find result).
func (h *Handler) customizePartKey(ctx context.Context, area string, crumbs []CustomizeCrumb, cat *customizeCatBar) string {
	switch {
	case cat != nil && !cat.New:
		return "cat-" + cat.Key
	case area == "look":
		return "frame"
	case area == "home" && len(crumbs) > 1:
		for _, t := range homeTabs {
			if t.label(ctx) == crumbs[1].Label {
				return "home-" + t.Key
			}
		}
	case strings.HasPrefix(area, "x-"):
		return "app-" + strings.TrimPrefix(area, "x-")
	}
	return ""
}

// Customize is the wall: GET /campaigns/:id/customize.
func (h *Handler) Customize(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	_, draft := h.CustomizeDraftFor(ctx, cc.Campaign.ID, auth.GetUserID(c))
	cats := h.CustomizeCategories(ctx, cc.Campaign.ID)
	groups, skin := h.customizeWall(ctx, cc, cats, draft)
	return h.renderCustomize(c, cc, "overview", "Customize", nil, cats, customizeWall(cc.Campaign.Name, groups, skin))
}

// CustomizeLook is the signed Look editor: GET /campaigns/:id/customize/look.
func (h *Handler) CustomizeLook(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	_, draft := h.CustomizeDraftFor(ctx, cc.Campaign.ID, auth.GetUserID(c))
	state := buildCustomizeState(ctx, cc)
	// Pictures only the draft uses need addresses too; LookDraftPictures
	// leaves out any the campaign does not own.
	for name, url := range h.LookDraftPictures(ctx, cc, draft) {
		state.Pictures[name] = url
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return h.renderCustomize(c, cc, "look", "Look", []CustomizeCrumb{{Label: "Look"}}, nil,
		lookTab(cc, middleware.GetCSRFToken(c), string(stateJSON)))
}

// CustomizeHome arranges one audience's campaign home: GET
// /campaigns/:id/customize/home[/:who]. The retired Overview tab's address
// goes to the Home pages area instead of a dead end.
func (h *Handler) CustomizeHome(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	key := c.Param("who")
	if key == "" {
		key = HomeDefault
	}
	if key == legacyHomeDM {
		return c.Redirect(http.StatusSeeOther, h.customizeBase(cc)+"/home")
	}
	if !validHomeKey(key) {
		return apperror.NewNotFound("that home doesn't exist")
	}
	ctx := c.Request().Context()
	name := homeTabFor(key).label(ctx)
	crumbs := []CustomizeCrumb{{Label: "Home pages", Href: h.customizeBase(cc) + "/home"}, {Label: name}}
	return h.renderCustomize(c, cc, "home", "Home pages - "+name, crumbs, nil,
		customizeHomePages(cc, key, h.homeHint(ctx, cc, key), h.homeEditorState(ctx, cc, key)))
}

// customizeCategoriesData is what the Categories area's script draws from.
type customizeCategoriesData struct {
	Base       string              `json:"base"`
	Categories []CustomizeCategory `json:"categories"`
	InMenu     map[int]bool        `json:"inMenu"`
	Claiming   bool                `json:"claiming"`
	Key        string              `json:"key,omitempty"` // The open category: its id, or a new-… draft key.
	Tab        string              `json:"tab,omitempty"`
}

func (h *Handler) categoriesData(ctx context.Context, cc *CampaignContext, cats []CustomizeCategory, key, tab string) string {
	claiming := false
	if h.extensionEnableChecker != nil {
		claiming, _ = h.extensionEnableChecker.IsEnabledForCampaign(ctx, cc.Campaign.ID, addonCharacterClaiming)
	}
	if cats == nil {
		cats = []CustomizeCategory{}
	}
	b, err := json.Marshal(customizeCategoriesData{
		Base:       h.customizeBase(cc),
		Categories: cats,
		InMenu:     h.CategoryMenuVisible(cc),
		Claiming:   claiming,
		Key:        key,
		Tab:        tab,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// CustomizeCategoriesPage is the grid of categories:
// GET /campaigns/:id/customize/categories.
func (h *Handler) CustomizeCategoriesPage(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	cats := h.CustomizeCategories(ctx, cc.Campaign.ID)
	return h.renderCustomize(c, cc, "categories", "Categories", []CustomizeCrumb{{Label: "Categories"}}, cats,
		customizeCategoriesArea(cc, h.categoriesData(ctx, cc, cats, "", ""), "", ""))
}

// CustomizeCategory is one category's workspace:
// GET /campaigns/:id/customize/categories/:key and .../:key/:tab. The key is
// the category's id, or the draft key of one made in the draft.
func (h *Handler) CustomizeCategory(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	key, tab := c.Param("key"), c.Param("tab")
	base := h.customizeBase(cc)
	if tab == "" {
		return c.Redirect(http.StatusSeeOther, base+"/categories/"+key+"/layout")
	}
	if !isCategoryTab(tab) {
		return apperror.NewNotFound("that part of a category doesn't exist")
	}
	ctx := c.Request().Context()
	cats := h.CustomizeCategories(ctx, cc.Campaign.ID)

	name, etID := "New category", 0
	if newCategoryKey.MatchString(key) {
		// Made in the draft: only its settings exist until it is saved.
		if tab != "settings" {
			return c.Redirect(http.StatusSeeOther, base+"/categories/"+key+"/settings")
		}
	} else {
		id, err := strconv.Atoi(key)
		if err != nil {
			return apperror.NewNotFound("category not found")
		}
		found := false
		for _, cat := range cats {
			if cat.ID == id {
				found, name, etID = true, cat.NamePlural, id
				break
			}
		}
		// Only this campaign's categories open here.
		if !found {
			return apperror.NewNotFound("category not found")
		}
	}

	var label string
	for _, t := range categoryTabs {
		if t.ID == tab {
			label = t.Label
		}
	}
	crumbs := []CustomizeCrumb{
		{Label: "Categories", Href: base + "/categories"},
		{Label: name, Href: base + "/categories/" + key + "/layout"},
		{Label: label},
	}
	cat := &customizeCatBar{Key: key, Name: name, Tab: tab, New: etID == 0, Base: base}
	return h.renderCustomizeIn(c, cc, "categories", name+" - "+label, crumbs, cats, cat,
		customizeCategoryWorkspace(cc, h.categoriesData(ctx, cc, cats, key, tab), key, tab, etID))
}

// CustomizePluginArea is an area a plugin contributes:
// GET /campaigns/:id/customize/x/:tab.
func (h *Handler) CustomizePluginArea(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id := c.Param("tab")
	for _, t := range h.customizeTabs(cc) {
		if t.ID == id {
			return h.renderCustomize(c, cc, "x-"+t.ID, t.Label, []CustomizeCrumb{{Label: t.Label}}, nil, customizePluginArea(t))
		}
	}
	return apperror.NewNotFound("that part of Customize doesn't exist")
}
