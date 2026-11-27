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

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
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
	addonLister     campaigns.AddonLister
	bookEdits       BookEditService
	rules           *RulesPages
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

// SetRulesPages wires what a reader sees of a rules category: the
// per-reader catalog, ties, left-out entries and the campaign's characters.
// Without it every system reads as its package.
func (h *SystemHandler) SetRulesPages(p *RulesPages) {
	h.rules = p
}

func (h *SystemHandler) rulesPages() *RulesPages {
	if h.rules == nil {
		return NewRulesPages(nil, nil, nil)
	}
	return h.rules
}

// SetAddonService wires the addon service for checking which system is
// enabled per campaign. Used by widget metadata methods.
func (h *SystemHandler) SetAddonService(svc addonChecker) {
	h.addonSvc = svc
}

// SetAddonLister wires the addons listing the campaign's game system is read
// from when none was saved (see campaigns.CampaignSystemID).
func (h *SystemHandler) SetAddonLister(l campaigns.AddonLister) {
	h.addonLister = l
}

// OperatorDiagnosticsAPI is a CATALOG of named, read-only, secret-redacted
// diagnostics the assistant requests one at a time, so the operator only ever
// pastes back the small, targeted result it asked for. Admin-gated.
//
//	GET /admin/diagnostics                      → the catalog (tiny menu, no payload)
//	GET /admin/diagnostics?name=system.versions → run one named diagnostic
//	GET /admin/diagnostics?name=system.files&arg=drawsteel
func (h *SystemHandler) OperatorDiagnosticsAPI(c *echo.Context) error {
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
func (h *SystemHandler) ExtensionsHealthAPI(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"systems": LoadedHealth(),
	})
}

// resolveSystem extracts the :mod param and looks up the live system.
// Checks global registry first, then campaign-specific custom systems.
func (h *SystemHandler) resolveSystem(c *echo.Context) System {
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
func (h *SystemHandler) Index(c *echo.Context) error {
	cc, mod, cats, err := h.indexData(c)
	if err != nil {
		return err
	}
	hasBook := HasBook(h.systemDir(c, mod))
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SystemIndexContent(cc, mod.Info(), cats, hasBook))
	}
	return middleware.Render(c, http.StatusOK, SystemIndexPage(cc, mod.Info(), cats, hasBook))
}

// Browse is the category browser at an address of its own. Index shows the
// book instead of the browser when a system has one, so without this the
// categories could not be reached from a reference page's breadcrumb.
// GET /campaigns/:id/systems/:mod/browse
func (h *SystemHandler) Browse(c *echo.Context) error {
	cc, mod, cats, err := h.indexData(c)
	if err != nil {
		return err
	}
	hasBook := HasBook(h.systemDir(c, mod))
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SystemBrowseContent(cc, mod.Info(), cats, hasBook))
	}
	return middleware.Render(c, http.StatusOK, SystemBrowsePage(cc, mod.Info(), cats, hasBook))
}

// indexData resolves the system and the categories this viewer may see, with
// their counts. Index and Browse share it so they apply the same hiding rules.
func (h *SystemHandler) indexData(c *echo.Context) (*campaigns.CampaignContext, System, []categoryInfo, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, nil, nil, apperror.NewMissingContext()
	}

	mod := h.resolveSystem(c)
	if mod == nil {
		return nil, nil, nil, apperror.NewNotFound("system not found")
	}

	manifest := mod.Info()

	// Build category counts from the data provider.
	var cats []categoryInfo
	dp := mod.DataProvider()
	for _, cat := range manifest.Categories {
		if hiddenFromViewer(cc, manifest, cat.Slug) {
			continue
		}
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
	return cc, mod, cats, nil
}

// rulesReader is who is reading a rules page, in the terms RulesPages takes.
func rulesReader(c *echo.Context, cc *campaigns.CampaignContext) RulesReader {
	return RulesReader{
		CampaignID: cc.Campaign.ID, CampaignName: cc.Campaign.Name,
		Actor: EntryActor{UserID: auth.GetUserID(c), IsDirector: bookViewerIsDirector(cc)},
		Role:  cc.VisibilityRole(),
	}
}

// rulesTarget resolves the campaign and the system a rules route reads.
func (h *SystemHandler) rulesTarget(c *echo.Context) (*campaigns.CampaignContext, System, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, nil, apperror.NewMissingContext()
	}
	mod := h.resolveSystem(c)
	if mod == nil {
		return nil, nil, apperror.NewNotFound("system not found")
	}
	return cc, mod, nil
}

// CategoryList is a category's page, as the reader's list has it.
// GET /campaigns/:id/systems/:mod/:cat?q=
func (h *SystemHandler) CategoryList(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	q := c.QueryParam("q")
	if len(q) > 100 {
		q = q[:100]
	}
	v, err := h.rulesPages().List(c.Request().Context(), mod, c.Param("cat"), q, rulesReader(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CategoryListContent(v))
	}
	return middleware.Render(c, http.StatusOK, CategoryListPage(cc, v))
}

// ItemDetail is an entry's page. A rewritten entry's standard address and
// an old name redirect to the entry they lead to; a left-out entry answers
// with its one line.
// GET /campaigns/:id/systems/:mod/:cat/:item?printed=1
func (h *SystemHandler) ItemDetail(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	v, err := h.rulesPages().Page(c.Request().Context(), mod, c.Param("cat"), c.Param("item"),
		c.QueryParam("printed") == "1", rulesReader(c, cc))
	if err != nil {
		return err
	}
	if v.Redirect != "" {
		if middleware.IsHTMX(c) {
			c.Response().Header().Set("HX-Redirect", v.Redirect)
			return c.NoContent(http.StatusOK)
		}
		return c.Redirect(http.StatusSeeOther, v.Redirect)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, EntryPageContent(v))
	}
	return middleware.Render(c, http.StatusOK, EntryPage(cc, v))
}

// EntryPeek is an entry as a fragment for the page peek; ?hero= names a
// character whose traits it marks, when the reader may see that character.
// GET /campaigns/:id/systems/:mod/:cat/:item/peek?hero=&printed=1
func (h *SystemHandler) EntryPeek(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	v, err := h.rulesPages().Peek(c.Request().Context(), mod, c.Param("cat"), c.Param("item"),
		c.QueryParam("printed") == "1", c.QueryParam("hero"), rulesReader(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return middleware.Render(c, http.StatusOK, EntryPeekPanel(v))
}

// SearchAPI returns JSON search results across all system categories, from
// the reader's own list.
// GET /campaigns/:id/systems/:mod/search?q=...
func (h *SystemHandler) SearchAPI(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	hits, err := h.rulesPages().Search(c.Request().Context(), mod, c.QueryParam("q"), rulesReader(c, cc))
	if err != nil {
		return err
	}
	items := make([]map[string]string, 0, len(hits))
	for _, r := range hits {
		item := map[string]string{
			"id":        r.Slug,
			"name":      r.Name,
			"category":  r.Category,
			"summary":   r.Summary,
			"system_id": mod.Info().ID,
			"url":       r.URL,
		}
		// Optional fields: what kind of hit it is, the entry a trait
		// belongs to, and "Rewritten" on a rewritten entry.
		if r.Kind != "entry" {
			item["kind"] = r.Kind
		}
		if r.EntryName != "" {
			item["entry"] = r.EntryName
		}
		if r.Whose != "" {
			item["whose"] = r.Whose
		}
		items = append(items, item)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"results": items,
		"total":   len(items),
	})
}

// TooltipAPI returns a JSON tooltip payload for one address, resolved for
// the reader: a rewritten or old name shows the entry it leads to, a
// left-out name its one line.
// GET /campaigns/:id/systems/:mod/:cat/:item/tooltip
func (h *SystemHandler) TooltipAPI(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	tip, err := h.rulesPages().Tooltip(c.Request().Context(), mod, c.Param("cat"), c.Param("item"),
		c.QueryParam("printed") == "1", rulesReader(c, cc))
	if err != nil {
		return err
	}
	e := tip.Entry
	item := &ReferenceItem{ID: e.Slug, Category: e.Category, Name: e.Name, Summary: e.Summary,
		Description: e.Description, Properties: e.Properties, Tags: e.Tags}
	var tooltipHTML string
	if tip.LeftOut {
		item.Summary, item.Properties, item.Tags = tip.Line, nil, nil
	} else if tr := mod.TooltipRenderer(); tr != nil {
		if html, err := tr.RenderTooltip(item); err == nil {
			tooltipHTML = html
		}
	}
	source := ""
	if e.Item != nil {
		source = e.Item.Source
	}
	// Private: what a viewer may read depends on who they are.
	c.Response().Header().Set("Cache-Control", "private, max-age=60")
	out := map[string]any{
		"name":         item.Name,
		"category":     item.Category,
		"summary":      item.Summary,
		"properties":   item.Properties,
		"tags":         item.Tags,
		"source":       source,
		"tooltip_html": tooltipHTML,
		"url":          tip.URL,
		"origin":       e.Origin,
	}
	if tip.Whose != "" {
		out["whose"] = tip.Whose
	}
	if tip.LeftOut {
		out["left_out"] = true
	}
	return c.JSON(http.StatusOK, out)
}

// --- System Widget Support ---

// WidgetScriptAPI serves a system widget's JS file from the system directory.
// GET /campaigns/:id/systems/:mod/widgets/:slug
func (h *SystemHandler) WidgetScriptAPI(c *echo.Context) error {
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

	// Only installed packages serve scripts. A campaign's own custom system
	// is data-only (errCustomSystemScripts), including one uploaded before
	// that rule, whose scripts may still sit on disk.
	sysDir := Dir(manifest.ID)
	if sysDir == "" {
		return apperror.NewNotFound("widget not found")
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
func (h *SystemHandler) RulesGlossaryAPI(c *echo.Context) error {
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

	// Private: only the campaign's readers may have it, so no shared cache may keep it.
	c.Response().Header().Set("Cache-Control", "private, max-age=3600")
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
func (h *SystemHandler) SystemDataAPI(c *echo.Context) error {
	mod := h.resolveSystem(c)
	if mod == nil {
		return apperror.NewNotFound("system not found")
	}

	file := c.Param("file")
	if !systemDataFilePattern.MatchString(file) {
		return apperror.NewBadRequest("invalid data file")
	}
	if hiddenFromViewer(campaigns.GetCampaignContext(c), mod.Info(), strings.TrimSuffix(file, ".json")) {
		return apperror.NewNotFound("data file not found")
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
	// A kind's file is the reader's own (rules_data.go), so it is never kept.
	cache := "private, max-age=3600"
	if cc := campaigns.GetCampaignContext(c); cc != nil {
		out, handled, err := h.rulesPages().KindData(c.Request().Context(), mod, strings.TrimSuffix(file, ".json"), data, rulesReader(c, cc))
		if err != nil {
			return err
		}
		if handled {
			data, cache = out, "private, no-store"
		}
	}

	// Private: the answer depends on the viewer's role and membership.
	c.Response().Header().Set("Cache-Control", cache)
	return c.Blob(http.StatusOK, "application/json", data)
}

// systemDir is the on-disk folder of a resolved system: the global
// install, or the campaign's custom upload. "" when it has none.
func (h *SystemHandler) systemDir(c *echo.Context, mod System) string {
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
// everything, players only the player view (FilterBook), and a Director who
// passes ?view=player gets what a player gets. Never cached shared, since the
// body depends on who asks.
//
// GET /campaigns/:id/systems/:mod/book
func (h *SystemHandler) BookAPI(c *echo.Context) error {
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
	// ?view=player asks a Director for the player's book (the campaign home's
	// leaves when the Director is viewing as a player); it can only narrow.
	director := bookViewerIsDirector(cc) && c.QueryParam("view") != "player"
	// A chapter listing a kind lists what this reader has (book_reading.go).
	filtered, err := h.forReader(c, FilterBook(book, director), manifest, director)
	if err != nil {
		return err
	}
	out := filtered.WithAssetBase("/campaigns/" + cc.Campaign.ID + "/systems/" + manifest.ID + "/book/files")
	out.CanEdit = h.bookEdits != nil && bookEditorAllowed(cc)
	return c.JSON(http.StatusOK, out)
}

// BookFileAPI serves one cover or font file the book declares, behind the
// same access checks as the book itself. The SVG cover is drawn by an <img>,
// where scripts never run; the sandbox policy keeps it inert if someone opens
// the address directly, and nosniff stops a font being read as anything else.
//
// GET /campaigns/:id/systems/:mod/book/files/:name
func (h *SystemHandler) BookFileAPI(c *echo.Context) error {
	sysDir, _, _, err := h.viewerBookTarget(c)
	if err != nil {
		return err
	}
	data, contentType, err := BookAsset(sysDir, c.Param("name"))
	if err != nil {
		return apperror.NewNotFound("this book has no such file")
	}
	hdr := c.Response().Header()
	hdr.Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "private, max-age=3600")
	return c.Blob(http.StatusOK, contentType, data)
}

// viewerBook loads the package book filtered for the viewer (or for a player,
// when a Director previews the player view), for the
// rules-index endpoints. Campaign edits are not merged: generated chapters
// can't be edited, so the package's are the ones readers see.
func (h *SystemHandler) viewerBook(c *echo.Context) (*Book, string, *SystemManifest, bool, error) {
	sysDir, manifest, director, err := h.viewerBookTarget(c)
	if err != nil {
		return nil, "", nil, false, err
	}
	b, err := LoadBook(sysDir, manifest)
	if err != nil {
		return nil, "", nil, false, apperror.NewNotFound("the rulebook could not be opened")
	}
	rb, err := h.forReader(c, FilterBook(b, director), manifest, director)
	if err != nil {
		return nil, "", nil, false, err
	}
	return rb, sysDir, manifest, director, nil
}

// viewerBookTarget resolves which book a request reads and as whom, without
// loading it, so the search can answer from its cached index.
func (h *SystemHandler) viewerBookTarget(c *echo.Context) (string, *SystemManifest, bool, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", nil, false, apperror.NewMissingContext()
	}
	mod := h.resolveSystem(c)
	if mod == nil {
		return "", nil, false, apperror.NewNotFound("system not found")
	}
	sysDir := h.systemDir(c, mod)
	if !HasBook(sysDir) {
		return "", nil, false, apperror.NewNotFound("this system has no book")
	}
	// A Director previewing the player view asks for exactly what players get.
	director := bookViewerIsDirector(cc) && c.QueryParam("view") != "player"
	return sysDir, mod.Info(), director, nil
}

// BookIndexAPI returns the entries one rules-index page shows. The query must
// name a slice a page of the viewer's own book shows, so a player can only
// list what their book lists (Director-only chapters are already gone).
// GET /campaigns/:id/systems/:mod/book/index/:cat?key=&value=&from=&to=
func (h *SystemHandler) BookIndexAPI(c *echo.Context) error {
	b, sysDir, manifest, director, err := h.viewerBook(c)
	if err != nil {
		return err
	}
	cat := c.Param("cat")
	want := bookIndexSlice{Key: c.QueryParam("key"), Value: c.QueryParam("value")}
	want.From, _ = strconv.Atoi(c.QueryParam("from"))
	want.To, _ = strconv.Atoi(c.QueryParam("to"))
	list, shown := sliceList(b, cat, want)
	if !shown {
		return apperror.NewNotFound("this book has no such index page")
	}
	ie, err := h.readerIndex(c, sysDir, manifest, cat, director)
	if err != nil {
		return apperror.NewNotFound("this index could not be read")
	}
	// Chapters are built so no page lists more than maxBookIndexItems.
	items := ie.slice(want)
	if sliceIsLast(b, cat, want) {
		items = append(items, ie.closing...)
	}
	out := make([]BookIndexEntry, 0, len(items))
	for _, it := range items {
		e := ie.indexEntry(it, director, want.Key)
		e.Facets = ie.facets(it, list, director)
		out = append(out, e)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"items": out})
}

// BookEntryAPI returns one rules-index entry, for a name in the book's text
// that opens its entry in place, with the chapter and page that list it and the page's title. Only
// an entry a page of the viewer's own book lists is served, as BookIndexAPI.
// GET /campaigns/:id/systems/:mod/book/entry/:cat/:entry
func (h *SystemHandler) BookEntryAPI(c *echo.Context) error {
	b, sysDir, manifest, director, err := h.viewerBook(c)
	if err != nil {
		return err
	}
	cat, id := c.Param("cat"), c.Param("entry")
	ie, hit, err := findBookEntry(b, cat, id, func() (*indexEntries, error) {
		return h.readerIndex(c, sysDir, manifest, cat, director)
	})
	if err != nil {
		return apperror.NewNotFound("this index could not be read")
	}
	if hit == nil {
		return apperror.NewNotFound("this book lists no such entry")
	}
	e := ie.indexEntry(hit.Item, director, hit.Slice.Key)
	e.Facets = ie.facets(hit.Item, hit.List, director)
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"entry": e, "chapter": hit.Chapter, "page": hit.Page, "pageTitle": hit.PageTitle, "list": hit.List,
	})
}

// BookFindAPI searches the rules-index chapters the viewer's book shows.
// GET /campaigns/:id/systems/:mod/book/find?q=
func (h *SystemHandler) BookFindAPI(c *echo.Context) error {
	sysDir, manifest, director, err := h.viewerBookTarget(c)
	if err != nil {
		return err
	}
	idx, err := cachedBookFindIndex(sysDir, manifest, director)
	if err == nil {
		idx, err = h.readerFind(c, sysDir, manifest, director, idx)
	}
	if err != nil {
		return apperror.NewNotFound("the rulebook could not be opened")
	}
	q := c.QueryParam("q")
	if len(q) > 100 {
		q = q[:100]
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"results": idx.search(q)})
}

// bookViewerIsDirector: the campaign owner, or a member the owner granted
// Director (dm_only) visibility. The same rule as every other dm_only surface.
func bookViewerIsDirector(cc *campaigns.CampaignContext) bool {
	return cc.VisibilityRole() >= int(campaigns.RoleOwner)
}

// hiddenFromViewer reports whether a category is Director-only and the viewer
// is not a Director. With no campaign context nobody is a Director.
func hiddenFromViewer(cc *campaigns.CampaignContext, manifest *SystemManifest, slug string) bool {
	if !manifest.DirectorOnly(slug) {
		return false
	}
	return cc == nil || !bookViewerIsDirector(cc)
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

	return placeableWidgetMetas(sys.Info(), bookWidgetSlugs(Dir(sys.Info().ID)))
}

// placeableWidgetMetas lists the manifest widgets a layout may place. A widget
// the host mounts itself, as a page RENDERER (manifest.renderers[].widget) or
// an entity PANEL (manifest.entity_panels[].widget), is excluded: a renderer
// owns a whole page and would mount without its page context, and a panel is
// already mounted by the host with its own context, so a hand-placed copy would
// duplicate it.
//
// A widget the package book places (bookWidgets) is a book pop-up, and one
// marked host_mounted is mounted by Chronicle elsewhere, so each is listed
// as Retired: a page that already holds one still draws it under its own
// name, but the Customizer no longer offers it.
func placeableWidgetMetas(manifest *SystemManifest, bookWidgets map[string]bool) []entities.BlockMeta {
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
			TileSizes:   w.TileSizes,
			Retired:     bookWidgets[w.Slug] || w.HostMounted,
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
	// A campaign's custom system serves no scripts (see WidgetScriptAPI).
	if total == 0 || Dir(manifest.ID) == "" {
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
