package modules

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ModuleHandler serves reference pages and JSON API endpoints for any
// module. It is stateless — module data is looked up from the global
// registry on each request.
type ModuleHandler struct{}

// NewModuleHandler creates a new module handler.
func NewModuleHandler() *ModuleHandler {
	return &ModuleHandler{}
}

// resolveModule extracts the :mod param and looks up the live module.
// Returns the module or writes a 404 error response.
func resolveModule(c echo.Context) Module {
	modID := c.Param("mod")
	mod := FindModule(modID)
	return mod
}

// Index lists all categories for a module.
// GET /campaigns/:id/modules/:mod
func (h *ModuleHandler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := resolveModule(c)
	if mod == nil {
		return apperror.NewNotFound("module not found")
	}

	manifest := mod.Info()

	// Build category counts from the data provider.
	var cats []categoryInfo
	dp := mod.DataProvider()
	for _, cat := range manifest.Categories {
		count := 0
		if dp != nil {
			if items, err := dp.List(cat.Slug); err == nil {
				count = len(items)
			}
		}
		cats = append(cats, categoryInfo{
			Slug:  cat.Slug,
			Name:  cat.Name,
			Icon:  cat.Icon,
			Count: count,
		})
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ModuleIndexContent(cc, manifest, cats))
	}
	return middleware.Render(c, http.StatusOK, ModuleIndexPage(cc, manifest, cats))
}

// CategoryList lists items in a module category.
// GET /campaigns/:id/modules/:mod/:cat
func (h *ModuleHandler) CategoryList(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := resolveModule(c)
	if mod == nil {
		return apperror.NewNotFound("module not found")
	}

	catSlug := c.Param("cat")
	dp := mod.DataProvider()
	if dp == nil {
		return apperror.NewNotFound("module has no data")
	}

	items, err := dp.List(catSlug)
	if err != nil {
		return err
	}

	// Find the category definition for display info.
	manifest := mod.Info()
	var catDef *CategoryDef
	for i := range manifest.Categories {
		if manifest.Categories[i].Slug == catSlug {
			catDef = &manifest.Categories[i]
			break
		}
	}
	if catDef == nil {
		return apperror.NewNotFound("category not found")
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CategoryListContent(cc, manifest, catDef, items))
	}
	return middleware.Render(c, http.StatusOK, CategoryListPage(cc, manifest, catDef, items))
}

// ItemDetail shows a single reference item.
// GET /campaigns/:id/modules/:mod/:cat/:item
func (h *ModuleHandler) ItemDetail(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := resolveModule(c)
	if mod == nil {
		return apperror.NewNotFound("module not found")
	}

	catSlug := c.Param("cat")
	itemID := c.Param("item")
	dp := mod.DataProvider()
	if dp == nil {
		return apperror.NewNotFound("module has no data")
	}

	item, err := dp.Get(catSlug, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return apperror.NewNotFound("item not found")
	}

	// Find category definition for field schema.
	manifest := mod.Info()
	var catDef *CategoryDef
	for i := range manifest.Categories {
		if manifest.Categories[i].Slug == catSlug {
			catDef = &manifest.Categories[i]
			break
		}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ItemDetailContent(cc, manifest, catDef, item))
	}
	return middleware.Render(c, http.StatusOK, ItemDetailPage(cc, manifest, catDef, item))
}

// SearchAPI returns JSON search results across all module categories.
// GET /campaigns/:id/modules/:mod/search?q=...
func (h *ModuleHandler) SearchAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := resolveModule(c)
	if mod == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "module not found"})
	}

	query := strings.TrimSpace(c.QueryParam("q"))
	dp := mod.DataProvider()
	if dp == nil {
		return c.JSON(http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	}

	results, err := dp.Search(query)
	if err != nil {
		return err
	}

	items := make([]map[string]string, len(results))
	manifest := mod.Info()
	for i, r := range results {
		items[i] = map[string]string{
			"id":        r.ID,
			"name":      r.Name,
			"category":  r.Category,
			"summary":   r.Summary,
			"module_id": manifest.ID,
			"url":       "/campaigns/" + cc.Campaign.ID + "/modules/" + manifest.ID + "/" + r.Category + "/" + r.ID,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"results": items,
		"total":   len(items),
	})
}

// TooltipAPI returns a JSON tooltip payload for a specific item.
// GET /campaigns/:id/modules/:mod/:cat/:item/tooltip
func (h *ModuleHandler) TooltipAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := resolveModule(c)
	if mod == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "module not found"})
	}

	catSlug := c.Param("cat")
	itemID := c.Param("item")
	dp := mod.DataProvider()
	if dp == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no data"})
	}

	item, err := dp.Get(catSlug, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "item not found"})
	}

	// Try the module's tooltip renderer for rich HTML.
	var tooltipHTML string
	if tr := mod.TooltipRenderer(); tr != nil {
		if html, err := tr.RenderTooltip(item); err == nil {
			tooltipHTML = html
		}
	}

	// Short cache — module data is static.
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")

	return c.JSON(http.StatusOK, map[string]any{
		"name":         item.Name,
		"category":     item.Category,
		"summary":      item.Summary,
		"properties":   item.Properties,
		"tags":         item.Tags,
		"source":       item.Source,
		"tooltip_html": tooltipHTML,
	})
}
