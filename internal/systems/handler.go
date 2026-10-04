package systems

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	bookEdits       BookEditService
}

// NewSystemHandler creates a new system handler.
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// SetCampaignSystems wires the per-campaign custom system manager.
func (h *SystemHandler) SetCampaignSystems(mgr *CampaignSystemManager) {
	h.campaignSystems = mgr
}

// SetBookEdits wires the service holding a campaign's edits to a system's
// book. Without it the book is served exactly as the package ships it and
// the editor routes answer 404.
func (h *SystemHandler) SetBookEdits(svc BookEditService) {
	h.bookEdits = svc
}

// SetAddonService wires the addon service for checking which system is
// enabled per campaign. Used by widget metadata methods.
func (h *SystemHandler) SetAddonService(svc addonChecker) {
	h.addonSvc = svc
}

// OperatorDiagnosticsAPI is a CATALOG of named, read-only, secret-redacted
// diagnostics the assistant requests one at a time, so the operator only ever
// pastes back the small, targeted result it asked for. Admin-gated.
//
//	GET /admin/diagnostics                      → the catalog (tiny menu, no payload)
//	GET /admin/diagnostics?name=system.versions → run one named diagnostic
//	GET /admin/diagnostics?name=system.files&arg=drawsteel
func (h *SystemHandler) OperatorDiagnosticsAPI(c echo.Context) error {
	cat := diagnosticCatalog()
	name := c.QueryParam("name")
	if name == "" {
		// A browser (clicking the admin card) gets the interactive HTML page;
		// programmatic/AI clients get the markdown catalog.
		if strings.Contains(c.Request().Header.Get("Accept"), "text/html") {
			return c.HTML(http.StatusOK, RenderDiagnosticsHTML(cat))
		}
		return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(renderCatalog(cat)))
	}
	out, ok := RunDiagnostic(cat, name, c.QueryParam("arg"))
	if !ok {
		return apperror.NewNotFound(fmt.Sprintf("unknown diagnostic %q — GET /admin/diagnostics for the catalog", name))
	}
	return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(out))
}

// ExtensionsHealthAPI returns read-only deployment health for every LOADED
// system — the version + on-disk directory the loader actually serves from,
// plus a content fingerprint (size + sha256 + mtime) of each widget/manifest
// file. Admin-gated. Lets an operator tell "registry didn't pick up the
// install" (loaded_version disagrees) apart from "extraction is wrong"
// (version agrees, file hash doesn't). GET /admin/extensions/health
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

	hasBook := HasBook(h.systemDir(c, mod))
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SystemIndexContent(cc, manifest, cats, hasBook))
	}
	return middleware.Render(c, http.StatusOK, SystemIndexPage(cc, manifest, cats, hasBook))
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

// RulesGlossaryAPI serves a system's raw data/rules-glossary.json — the
// authored [{slug,name,description,properties:{category}}] array a system's
// client-side reference-renderer needs to resolve {@category term} tokens.
// Served raw, not via the DataProvider's rendered-HTML category view. Returns
// an empty array when the system ships no glossary so references stay literal
// rather than erroring.
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

// systemDataFilePattern restricts SystemDataAPI to a plain JSON basename in a
// system's data/ dir: no path separators, no leading dot, no traversal. The
// within-dir clamp in the handler is a second backstop.
var systemDataFilePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*\.json$`)

// SystemDataAPI serves a system's raw data/<file>.json verbatim — the generic
// form of RulesGlossaryAPI, for widgets that need the complete authored file
// as JSON rather than the DataProvider's rendered-HTML category view. The
// filename is validated to a safe JSON basename and the resolved path is
// clamped within the system's data dir. Returns 404 for an absent/unknown
// file so callers degrade gracefully instead of erroring.
//
// GET /campaigns/:id/systems/:mod/data/:file
func (h *SystemHandler) SystemDataAPI(c echo.Context) error {
	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	file := c.Param("file")
	if !systemDataFilePattern.MatchString(file) {
		return apperror.NewBadRequest("invalid data file")
	}

	sysDir := h.systemDir(c, mod)
	if sysDir == "" {
		return apperror.NewNotFound("data file not found")
	}

	dataDir := filepath.Clean(filepath.Join(sysDir, "data"))
	p := filepath.Clean(filepath.Join(dataDir, file))
	if !strings.HasPrefix(p, dataDir+string(os.PathSeparator)) {
		return apperror.NewBadRequest("invalid path")
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return apperror.NewNotFound("data file not found")
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.Blob(http.StatusOK, "application/json", data)
}

// systemDir is the on-disk folder of a resolved system: the global
// install, or the campaign's custom upload. "" when it has none.
func (h *SystemHandler) systemDir(c echo.Context, mod System) string {
	sysDir := Dir(mod.Info().ID)
	if sysDir == "" && h.campaignSystems != nil {
		if cc := campaigns.GetCampaignContext(c); cc != nil {
			sysDir = h.campaignSystems.Dir(cc.Campaign.ID)
		}
	}
	return sysDir
}

// BookAPI serves the system's Rulebook book, checked and filtered for the
// viewer: Directors (the owner, or a member with Director visibility) get
// everything, players only the player view (FilterBook). Never cached
// shared, since the body depends on who asks.
//
// GET /campaigns/:id/systems/:mod/book
func (h *SystemHandler) BookAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}
	sysDir := h.systemDir(c, mod)
	if !HasBook(sysDir) {
		return apperror.NewNotFound("this system has no book")
	}
	manifest := mod.Info()
	pkg, err := LoadBookPackage(sysDir, manifest)
	if err != nil {
		slog.Warn("rulebook book failed to load",
			slog.String("system", manifest.ID), slog.Any("error", err))
		// The detail names package files; only Directors need it.
		msg := "The rulebook could not be opened."
		if bookViewerIsDirector(cc) {
			msg += " " + err.Error()
		}
		return apperror.NewBadRequest(msg)
	}
	book := pkg.Book
	if h.bookEdits != nil {
		// The campaign's edits are merged in BEFORE the filter, so a
		// Director-only flag on an edited page or block is honoured.
		if book, err = h.bookEdits.Edition(c.Request().Context(), cc.Campaign.ID, pkg); err != nil {
			return err
		}
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	out := FilterBook(book, bookViewerIsDirector(cc))
	out.CanEdit = h.bookEdits != nil && bookEditorAllowed(cc)
	return c.JSON(http.StatusOK, out)
}

// viewerBook loads the package book filtered for the viewer, for the
// rules-index endpoints. Campaign edits are not merged: generated chapters
// can't be edited, so the package's are the ones readers see.
func (h *SystemHandler) viewerBook(c echo.Context) (*Book, string, *SystemManifest, bool, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, "", nil, false, apperror.NewMissingContext()
	}
	mod := h.resolveSystem(c)
	if mod == nil {
		return nil, "", nil, false, apperror.NewNotFound("system not found")
	}
	sysDir := h.systemDir(c, mod)
	if !HasBook(sysDir) {
		return nil, "", nil, false, apperror.NewNotFound("this system has no book")
	}
	manifest := mod.Info()
	b, err := LoadBook(sysDir, manifest)
	if err != nil {
		return nil, "", nil, false, apperror.NewNotFound("the rulebook could not be opened")
	}
	director := bookViewerIsDirector(cc)
	return FilterBook(b, director), sysDir, manifest, director, nil
}

// BookIndexAPI returns the entries one rules-index page shows. The query must
// name a slice a page of the viewer's own book shows, so a player can only
// list what their book lists (Director-only chapters are already gone).
// GET /campaigns/:id/systems/:mod/book/index/:cat?key=&value=&from=&to=
func (h *SystemHandler) BookIndexAPI(c echo.Context) error {
	b, sysDir, manifest, director, err := h.viewerBook(c)
	if err != nil {
		return err
	}
	cat := c.Param("cat")
	want := bookIndexSlice{Key: c.QueryParam("key"), Value: c.QueryParam("value")}
	want.From, _ = strconv.Atoi(c.QueryParam("from"))
	want.To, _ = strconv.Atoi(c.QueryParam("to"))
	shown := false
	for _, ch := range indexChapters(b) {
		for _, p := range ch.Pages {
			if pc, s, ok := pageSlice(p); ok && pc == cat && s == want {
				shown = true
			}
		}
	}
	if !shown {
		return apperror.NewNotFound("this book has no such index page")
	}
	ie, err := loadIndexEntries(sysDir, manifest, cat)
	if err != nil {
		return apperror.NewNotFound("this index could not be read")
	}
	items := ie.slice(want)
	if len(items) > maxBookIndexItems {
		items = items[:maxBookIndexItems]
	}
	out := make([]BookIndexEntry, 0, len(items))
	for _, it := range items {
		out = append(out, ie.indexEntry(it, director))
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"items": out})
}

// BookFindAPI searches the rules-index chapters the viewer's book shows.
// GET /campaigns/:id/systems/:mod/book/find?q=
func (h *SystemHandler) BookFindAPI(c echo.Context) error {
	b, sysDir, manifest, _, err := h.viewerBook(c)
	if err != nil {
		return err
	}
	q := c.QueryParam("q")
	if len(q) > 100 {
		q = q[:100]
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"results": findInIndex(sysDir, manifest, b, q)})
}

// bookViewerIsDirector: the campaign owner, or a member the owner granted
// Director (dm_only) visibility. The same rule as every other dm_only surface.
func bookViewerIsDirector(cc *campaigns.CampaignContext) bool {
	return cc.VisibilityRole() >= int(campaigns.RoleOwner)
}

// EnabledSystem returns the System enabled for a campaign, or nil. Other
// plugins (the DM Screen) read a system's manifest through it.
func (h *SystemHandler) EnabledSystem(ctx context.Context, campaignID string) System {
	return h.resolveEnabledSystem(ctx, campaignID)
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

	return placeableWidgetMetas(sys.Info())
}

// placeableWidgetMetas lists the manifest widgets a layout may place. A widget
// the host mounts itself, as a page RENDERER (manifest.renderers[].widget) or
// an entity PANEL (manifest.entity_panels[].widget), is excluded: a renderer
// owns a whole page and would mount without its page context, and a panel is
// already mounted by the host with its own context, so a hand-placed copy would
// duplicate it.
func placeableWidgetMetas(manifest *SystemManifest) []entities.BlockMeta {
	if len(manifest.Widgets) == 0 {
		return nil
	}
	hostMounted := make(map[string]struct{}, len(manifest.Renderers)+len(manifest.EntityPanels))
	for _, r := range manifest.Renderers {
		hostMounted[r.Widget] = struct{}{}
	}
	for _, p := range manifest.EntityPanels {
		hostMounted[p.Widget] = struct{}{}
	}

	metas := make([]entities.BlockMeta, 0, len(manifest.Widgets))
	for _, w := range manifest.Widgets {
		if _, skip := hostMounted[w.Slug]; skip {
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
	// Version-stamp the URL so an update changes it and the browser fetches
	// fresh JS instead of serving a stale cached copy (paired with the
	// immutable cache in WidgetScriptAPI).
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
