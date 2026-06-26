package systems

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// addonChecker tests whether an addon slug is enabled for a campaign.
type addonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// SystemHandler serves reference pages and JSON API endpoints for any
// system. It checks both global built-in systems and per-campaign custom
// systems uploaded by campaign owners.
type SystemHandler struct {
	campaignSystems *CampaignSystemManager
	addonSvc        addonChecker
}

// NewSystemHandler creates a new system handler.
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// SetCampaignSystems wires the per-campaign custom system manager.
func (h *SystemHandler) SetCampaignSystems(mgr *CampaignSystemManager) {
	h.campaignSystems = mgr
}

// SetAddonService wires the addon service for checking which system is
// enabled per campaign. Used by widget metadata methods.
func (h *SystemHandler) SetAddonService(svc addonChecker) {
	h.addonSvc = svc
}

// OperatorDiagnosticsAPI serves the operator diagnostics report as plain-text
// markdown: the served-reality systems table (from LoadedHealth) plus the
// run-and-paste-back probe library. The admin opens it, selects-all, and pastes
// the whole thing to the AI assistant — the operator-facing analogue of the
// campaign AI-Export. Read-only and secret-free by construction. Admin-gated.
// GET /admin/diagnostics
func (h *SystemHandler) OperatorDiagnosticsAPI(c echo.Context) error {
	report := BuildOperatorReport(LoadedHealth(), defaultProbes())
	return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(report))
}

// ExtensionsHealthAPI returns read-only deployment health for every LOADED
// system — the version + on-disk directory the loader actually serves from, plus
// a content fingerprint (size + sha256 + mtime) of each widget/manifest file.
// Admin-gated (registered on the admin route group). It exists to diagnose the
// "Admin▸Packages says 0.13.0 but the old file renders" class of bug from the UI:
// if loaded_version disagrees with the installed version, the in-memory registry
// never picked up the install; if it agrees but a file hash is the old content,
// the extraction is wrong. GET /admin/extensions/health
func (h *SystemHandler) ExtensionsHealthAPI(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"systems": LoadedHealth(),
	})
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
		return apperror.NewNotFound("system not found")
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
		return apperror.NewNotFound("system not found")
	}

	catSlug := c.Param("cat")
	itemID := c.Param("item")
	dp := mod.DataProvider()
	if dp == nil {
		return apperror.NewNotFound("this system has no reference data")
	}

	item, err := dp.Get(catSlug, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return apperror.NewNotFound("item not found")
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

// --- System Widget Support ---

// WidgetScriptAPI serves a system widget's JS file from the system directory.
// GET /campaigns/:id/systems/:mod/widgets/:slug
func (h *SystemHandler) WidgetScriptAPI(c echo.Context) error {
	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	slug := c.Param("slug")
	// Strip .js extension if present in the route param.
	slug = strings.TrimSuffix(slug, ".js")

	if !slugPattern.MatchString(slug) {
		return apperror.NewBadRequest("invalid widget slug")
	}

	manifest := mod.Info()

	// Look up the script file path — check widgets first, then text renderers.
	var scriptFile string
	for i := range manifest.Widgets {
		if manifest.Widgets[i].Slug == slug {
			scriptFile = manifest.Widgets[i].ScriptFile
			break
		}
	}
	if scriptFile == "" {
		for i := range manifest.TextRenderers {
			if manifest.TextRenderers[i].Slug == slug {
				scriptFile = manifest.TextRenderers[i].File
				break
			}
		}
	}
	if scriptFile == "" {
		return apperror.NewNotFound("widget not found")
	}

	// Resolve the system's directory on disk.
	sysDir := Dir(manifest.ID)
	if sysDir == "" {
		// Check campaign custom systems.
		if h.campaignSystems != nil {
			cc := campaigns.GetCampaignContext(c)
			if cc != nil {
				sysDir = h.campaignSystems.Dir(cc.Campaign.ID)
			}
		}
	}
	if sysDir == "" {
		return apperror.NewNotFound("system directory not found")
	}

	// Resolve and validate the script file path.
	scriptPath := filepath.Join(sysDir, scriptFile)
	scriptPath = filepath.Clean(scriptPath)
	// Ensure resolved path stays within system directory.
	if !strings.HasPrefix(scriptPath, filepath.Clean(sysDir)+string(os.PathSeparator)) {
		return apperror.NewBadRequest("invalid script path")
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return apperror.NewNotFound("widget script not found")
	}

	c.Response().Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// A version-stamped request (?v=<pkg version>, emitted by
	// GetSystemWidgetScriptURLs) is safe to cache hard + immutable: the URL itself
	// changes whenever the package version changes, so a stale copy can never
	// outlive an update. Unversioned/direct requests keep the conservative TTL.
	if c.QueryParam("v") != "" {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	}
	return c.Blob(http.StatusOK, "application/javascript", data)
}

// RulesGlossaryAPI serves a system's raw data/rules-glossary.json — the authored
// [{slug,name,description,properties:{category}}] array a system's client-side
// reference-renderer needs to resolve {@category term} tokens. It is served RAW
// (not via the DataProvider, which normalizes into ReferenceItem and drops the
// authored `slug`). Returns an empty array when the system ships no glossary so
// the client degrades gracefully (references stay literal) rather than erroring.
//
// GET /campaigns/:id/systems/:mod/rules-glossary
func (h *SystemHandler) RulesGlossaryAPI(c echo.Context) error {
	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	sysDir := Dir(mod.Info().ID)
	if sysDir == "" && h.campaignSystems != nil {
		if cc := campaigns.GetCampaignContext(c); cc != nil {
			sysDir = h.campaignSystems.Dir(cc.Campaign.ID)
		}
	}
	if sysDir == "" {
		return c.JSON(http.StatusOK, []any{})
	}

	// Fixed filename (no user input), but keep the within-dir guard for parity
	// with WidgetScriptAPI.
	p := filepath.Clean(filepath.Join(sysDir, "data", "rules-glossary.json"))
	if !strings.HasPrefix(p, filepath.Clean(sysDir)+string(os.PathSeparator)) {
		return apperror.NewBadRequest("invalid path")
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return c.JSON(http.StatusOK, []any{})
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.Blob(http.StatusOK, "application/json", data)
}

// resolveEnabledSystem returns the System enabled for the given campaign,
// checking both built-in addon systems and campaign custom systems.
func (h *SystemHandler) resolveEnabledSystem(ctx context.Context, campaignID string) System {
	if h.addonSvc == nil {
		slog.Debug("resolveEnabledSystem: addonSvc is nil", slog.String("campaign_id", campaignID))
		return nil
	}

	// Check all live built-in systems.
	allSys := AllSystems()
	slog.Debug("resolveEnabledSystem: checking systems",
		slog.String("campaign_id", campaignID),
		slog.Int("system_count", len(allSys)),
	)
	for _, sys := range allSys {
		enabled, err := h.addonSvc.IsEnabledForCampaign(ctx, campaignID, sys.Info().ID)
		if err == nil && enabled {
			return sys
		}
	}

	// Check campaign custom system.
	if h.campaignSystems != nil {
		if sys := h.campaignSystems.GetSystem(campaignID); sys != nil {
			return sys
		}
	}

	slog.Debug("resolveEnabledSystem: no enabled system found", slog.String("campaign_id", campaignID))
	return nil
}

// GetSystemWidgetBlockMetas returns BlockMeta entries for widgets provided
// by the campaign's enabled game system. Used by the template editor palette.
func (h *SystemHandler) GetSystemWidgetBlockMetas(ctx context.Context, campaignID string) []entities.BlockMeta {
	sys := h.resolveEnabledSystem(ctx, campaignID)
	if sys == nil {
		return nil
	}

	manifest := sys.Info()
	if len(manifest.Widgets) == 0 {
		return nil
	}

	// A widget the system registers as a page RENDERER (manifest.renderers[].widget)
	// owns a whole entity page — it is NOT a placeable layout block. Exclude those
	// from the palette so they can't be dropped into a layout, where they'd mount
	// without their page context and render empty (the "bare name" trap). Generic
	// across systems — keyed on the manifest, not any system name.
	rendererWidgets := make(map[string]struct{}, len(manifest.Renderers))
	for _, r := range manifest.Renderers {
		rendererWidgets[r.Widget] = struct{}{}
	}

	metas := make([]entities.BlockMeta, 0, len(manifest.Widgets))
	for _, w := range manifest.Widgets {
		if _, isRenderer := rendererWidgets[w.Slug]; isRenderer {
			continue
		}
		icon := w.Icon
		if icon == "" {
			icon = "fa-puzzle-piece"
		}
		metas = append(metas, entities.BlockMeta{
			Type:        "ext_widget",
			Label:       w.Name,
			Icon:        icon,
			Description: w.Description,
			WidgetSlug:  w.Slug,
		})
	}
	return metas
}

// GetSystemWidgetScriptURLs returns the URLs to load system widget JS files
// for the campaign's enabled game system. Injected into pages via base.templ.
// Text renderer scripts are included first so they define globals that
// widget scripts can depend on (e.g., DrawSteelRefRenderer).
func (h *SystemHandler) GetSystemWidgetScriptURLs(ctx context.Context, campaignID string) []string {
	sys := h.resolveEnabledSystem(ctx, campaignID)
	if sys == nil {
		return nil
	}

	manifest := sys.Info()
	total := len(manifest.TextRenderers) + len(manifest.Widgets)
	if total == 0 {
		return nil
	}

	urls := make([]string, 0, total)
	// Version-stamp every asset URL with the loaded package version so a package
	// update changes the URL and the browser fetches fresh JS automatically — no
	// hard-refresh, no incognito. Paired with an immutable cache on the serve side
	// (WidgetScriptAPI). manifest.Version is set at install time (rewritten to the
	// release tag, defaulting to "0.0.0"). Without this, the version-less URL +
	// long cache served stale widget code after every update.
	ver := url.QueryEscape(manifest.Version)
	// Text renderers first — they define globals that widgets depend on.
	for _, tr := range manifest.TextRenderers {
		urls = append(urls, fmt.Sprintf("/campaigns/%s/systems/%s/widgets/%s.js?v=%s", campaignID, manifest.ID, tr.Slug, ver))
	}
	for _, w := range manifest.Widgets {
		urls = append(urls, fmt.Sprintf("/campaigns/%s/systems/%s/widgets/%s.js?v=%s", campaignID, manifest.ID, w.Slug, ver))
	}
	return urls
}
