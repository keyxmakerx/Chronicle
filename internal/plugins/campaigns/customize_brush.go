// customize_brush.go serves the header's Customize brush: the one row of its
// menu that depends on where the owner is ("How Character pages are laid
// out" on a character's page), and the person's own choice to hide the brush.
//
// The header sits outside #main-content, so a boosted page change never
// redraws it; the brush asks for its row by the page's address instead, and
// the server decides from that address what the place has of its own. No
// plugin has to know the brush exists: a plugin's Customize tab names its
// own pages through the tab registration (CustomizeTab.Paths).

package campaigns

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// BrushPrefs reads one person's choice to hide the header's brush in one
// campaign. It is an
// interface so the campaigns plugin reads the person's display choices
// through the auth service, never its tables.
type BrushPrefs interface {
	BrushHidden(ctx context.Context, userID, campaignID string) (bool, error)
}

// PageCategoryLookup says which category a page of this campaign is in, so
// the brush can offer that category's page layout. False when the page is
// not this campaign's or does not exist.
type PageCategoryLookup interface {
	PageCategory(ctx context.Context, campaignID, entityID string) (typeID int, ok bool, err error)
}

// SetBrushPrefs wires the person's brush choice.
func (h *Handler) SetBrushPrefs(p BrushPrefs) { h.brushPrefs = p }

// SetPageCategoryLookup wires the page-to-category lookup the brush uses.
func (h *Handler) SetPageCategoryLookup(l PageCategoryLookup) { h.pageCategory = l }

// brushHidden reports the person's choice in one campaign; one that cannot be read leaves
// the brush shown, its default.
func (h *Handler) brushHidden(ctx context.Context, userID, campaignID string) bool {
	if h.brushPrefs == nil || userID == "" {
		return false
	}
	hidden, err := h.brushPrefs.BrushHidden(ctx, userID, campaignID)
	if err != nil {
		slog.Warn("brush choice not read", slog.String("user_id", userID), slog.Any("error", err))
		return false
	}
	return hidden
}

// brushRow is the brush menu's first row: words, the quiet path under them,
// and where it leads.
type brushRow struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Href  string `json:"href"`
}

// brushPlace is what the server knows about the page the owner is on.
type brushPlace struct {
	Base string // the campaign's Customize address
	Cats []CustomizeCategory
	Tabs []CustomizeTab
	// pageCategory finds a page's category id; nil finds none.
	pageCategory func(entityID string) (int, bool)
}

// entityRouteWords are the words after /entities/ that name a list or a tool,
// not a page.
var entityRouteWords = map[string]bool{"new": true, "types": true, "members": true, "search": true, "viewable": true}

// brushRowFor decides the first row from the page's address relative to the
// campaign ("/entities/<id>", "/character", "/maps/<id>", ""). It returns
// nil when the place has nothing of its own in Customize.
func brushRowFor(rel string, p brushPlace) *brushRow {
	rel = "/" + strings.Trim(rel, "/")
	if rel == "/" {
		return &brushRow{Label: "How the campaign home is laid out", Path: "Customize › Home pages", Href: p.Base + "/home"}
	}
	segs := strings.Split(strings.TrimPrefix(rel, "/"), "/")
	if segs[0] == "entities" && len(segs) == 2 && !entityRouteWords[segs[1]] && p.pageCategory != nil {
		id, ok := p.pageCategory(segs[1])
		if !ok {
			return nil
		}
		for _, c := range p.Cats {
			if c.ID == id {
				return &brushRow{
					Label: "How " + c.Name + " pages are laid out",
					Path:  "Customize › Categories › " + c.NamePlural,
					Href:  fmt.Sprintf("%s/categories/%d/layout", p.Base, c.ID),
				}
			}
		}
		return nil
	}
	if len(segs) == 1 {
		for _, c := range p.Cats {
			if c.Slug != "" && c.Slug == segs[0] {
				return &brushRow{
					Label: "How the " + c.NamePlural + " home is laid out",
					Path:  "Customize › Categories › " + c.NamePlural,
					Href:  fmt.Sprintf("%s/categories/%d/home", p.Base, c.ID),
				}
			}
		}
	}
	for _, t := range p.Tabs {
		if t.owns(rel) {
			label := t.Here
			if label == "" {
				label = t.Label
			}
			return &brushRow{Label: label, Path: "Customize › " + t.Label, Href: p.Base + "/x/" + t.ID}
		}
	}
	return nil
}

// CustomizeHere answers the brush's first row for the page at ?path=, as
// {"row": {label, path, href}} or {"row": null}: GET
// /campaigns/:id/customize/here. Owner-only, like Customize itself. A path
// outside this campaign has no row.
func (h *Handler) CustomizeHere(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	c.Response().Header().Set("Cache-Control", "private, no-store")
	prefix := "/campaigns/" + cc.Campaign.ID
	path := c.QueryParam("path")
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		return c.JSON(http.StatusOK, map[string]any{"row": nil})
	}
	var addons map[string]bool
	if h.addonLister != nil {
		addons = map[string]bool{}
		if list, err := h.addonLister.ListForPluginHub(ctx, cc.Campaign.ID); err == nil {
			for _, a := range list {
				addons[a.Slug] = a.Enabled
			}
		}
	}
	place := brushPlace{
		Base: h.customizeBase(cc),
		Cats: h.CustomizeCategories(ctx, cc.Campaign.ID),
		Tabs: h.customizeTabsOn(cc, addons),
	}
	if h.pageCategory != nil {
		place.pageCategory = func(eid string) (int, bool) {
			id, ok, err := h.pageCategory.PageCategory(ctx, cc.Campaign.ID, eid)
			if err != nil {
				slog.Warn("brush: page category not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
				return 0, false
			}
			return id, ok
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"row": brushRowFor(strings.TrimPrefix(path, prefix), place)})
}
