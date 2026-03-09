package systems

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SystemHandler serves reference pages and JSON API endpoints for any
// system. It checks both global built-in systems and per-campaign custom
// systems uploaded by campaign owners.
type SystemHandler struct {
	campaignSystems *CampaignSystemManager
}

// NewSystemHandler creates a new system handler.
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// SetCampaignSystems wires the per-campaign custom system manager.
func (h *SystemHandler) SetCampaignSystems(mgr *CampaignSystemManager) {
	h.campaignSystems = mgr
}

// resolveSystem extracts the :mod param and looks up the live system.
// Checks global registry first, then campaign-specific custom systems.
func (h *SystemHandler) resolveSystem(c echo.Context) System { 
	sysID := c.Param("mod")

	// Check global built-in systems first.
	if mod := FindSystem(sysID); mod != nil {
		return mod
	}

	// Check campaign-specific custom systems.
	if h.campaignSystems != nil {
		cc := campaigns.GetCampaignContext(c)
		if cc != nil {
			if mod := h.campaignSystems.GetSystem(cc.Campaign.ID); mod != nil {
				if mod.Info().ID == sysID {
					return mod
				}
			}
		}
	}

	return nil
}

// Index lists all categories for a system.
// GET /campaigns/:id/systems/:mod
func (h *SystemHandler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
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
		return middleware.Render(c, http.StatusOK, SystemIndexContent(cc, manifest, cats))
	}
	return middleware.Render(c, http.StatusOK, SystemIndexPage(cc, manifest, cats))
}

// CategoryList lists items in a system category.
// GET /campaigns/:id/systems/:mod/:cat
func (h *SystemHandler) CategoryList(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	catSlug := c.Param("cat")
	dp := mod.DataProvider()
	if dp == nil {
		return apperror.NewNotFound("system has no data")
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
// GET /campaigns/:id/systems/:mod/:cat/:item
func (h *SystemHandler) ItemDetail(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	catSlug := c.Param("cat")
	itemID := c.Param("item")
	dp := mod.DataProvider()
	if dp == nil {
		return apperror.NewNotFound("system has no data")
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

// SearchAPI returns JSON search results across all system categories.
// GET /campaigns/:id/systems/:mod/search?q=...
func (h *SystemHandler) SearchAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "system not found"})
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
			"system_id": manifest.ID,
			"url":       "/campaigns/" + cc.Campaign.ID + "/systems/" + manifest.ID + "/" + r.Category + "/" + r.ID,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"results": items,
		"total":   len(items),
	})
}

// TooltipAPI returns a JSON tooltip payload for a specific item.
// GET /campaigns/:id/systems/:mod/:cat/:item/tooltip
func (h *SystemHandler) TooltipAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "system not found"})
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

	// Try the system's tooltip renderer for rich HTML.
	var tooltipHTML string
	if tr := mod.TooltipRenderer(); tr != nil {
		if html, err := tr.RenderTooltip(item); err == nil {
			tooltipHTML = html
		}
	}

	// Short cache — system data is static.
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
