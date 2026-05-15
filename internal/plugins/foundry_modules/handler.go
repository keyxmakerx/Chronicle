package foundry_modules

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves HTTP for the foundry_modules plugin. Echo handlers
// stay thin per the project conventions — bind, validate, call the
// service, render JSON.
type Handler struct {
	svc Service
}

// NewHandler constructs the Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// --- admin HTML fragment ---

// AdminVersionsSectionHandler serves the catalog as an HTMX fragment
// embedded in the admin Packages page. Returns the raw section
// content (no @layouts.App wrapper); the surrounding chrome — sidebar,
// breadcrumbs, page title — comes from packages.templ.
//
// Queries the catalog + per-version usage in N+1 calls (one list,
// one usage call per version). Admin catalogs are small (typically
// < 20 rows) so the N+1 is acceptable; refactor to a joined query
// if a deployment shows > 100 versions.
//
// GET /admin/modules/foundry/section
func (h *Handler) AdminVersionsSectionHandler(c echo.Context) error {
	ctx := c.Request().Context()
	versions, err := h.svc.ListVersions(ctx, true)
	if err != nil {
		return err
	}
	usageByVersion := make(map[string][]CampaignUsage, len(versions))
	for _, v := range versions {
		u, err := h.svc.CampaignsUsingVersion(ctx, v.Version)
		if err != nil {
			return err
		}
		usageByVersion[v.Version] = u
	}
	return middleware.Render(c, http.StatusOK,
		AdminVersionsSection(versions, usageByVersion, middleware.GetCSRFToken(c)))
}

// --- admin handlers ---

// ListVersionsAPI returns every cataloged version (including withdrawn)
// for the admin UI. GET /admin/modules/foundry/versions
func (h *Handler) ListVersionsAPI(c echo.Context) error {
	versions, err := h.svc.ListVersions(c.Request().Context(), true)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, versions)
}

// UploadAPI accepts a multipart upload of a Foundry module zip and
// returns the new catalog row. POST /admin/modules/foundry/upload
//
// Field name: "file". Optional form field: "release_notes" (admin
// override of the auto-derived description).
func (h *Handler) UploadAPI(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file field is required")
	}
	f, err := fh.Open()
	if err != nil {
		return apperror.NewBadRequest("could not open uploaded file")
	}
	defer func() { _ = f.Close() }()

	v, err := h.svc.UploadVersion(c.Request().Context(), UploadVersionInput{
		File:         f,
		FileSize:     fh.Size,
		UploaderID:   session.UserID,
		ReleaseNotes: c.FormValue("release_notes"),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, v)
}

// SetStatusAPI updates a version's status (deprecated / withdrawn /
// back to available). PUT /admin/modules/foundry/:version/status
func (h *Handler) SetStatusAPI(c echo.Context) error {
	version := c.Param("version")
	var req struct {
		Status ModuleStatus `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.svc.SetVersionStatus(c.Request().Context(), version, req.Status); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// UsageAPI returns the list of campaigns currently pinned to a
// version. GET /admin/modules/foundry/:version/usage
func (h *Handler) UsageAPI(c echo.Context) error {
	version := c.Param("version")
	usage, err := h.svc.CampaignsUsingVersion(c.Request().Context(), version)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, usage)
}

// NotifyAPI fires the "tell older-version campaigns this version is
// out" fan-out. POST /admin/modules/foundry/:version/notify
func (h *Handler) NotifyAPI(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	notified, err := h.svc.NotifyOlderCampaigns(
		c.Request().Context(), version,
		session.UserID, c.RealIP(), c.Request().UserAgent(),
	)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"notified": notified})
}

// ForcePinAPI directly pins one or more campaigns to a version.
// POST /admin/modules/foundry/:version/force-pin
// Body: { "campaign_ids": ["..."] }
// Requires admin password re-auth (applied at the route level).
func (h *Handler) ForcePinAPI(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	var req struct {
		CampaignIDs []string `json:"campaign_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if len(req.CampaignIDs) == 0 {
		return apperror.NewBadRequest("campaign_ids is required and must be non-empty")
	}
	for _, cid := range req.CampaignIDs {
		if err := h.svc.ForcePinCampaign(c.Request().Context(),
			cid, version, session.UserID, c.RealIP(), c.Request().UserAgent()); err != nil {
			return err
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// --- owner HTML fragment ---

// OwnerTabFragmentHandler serves the Foundry Module settings tab as an
// HTMX fragment. Called by the empty <div hx-get=...> placeholder in
// campaigns/settings.templ. Separated from a full page because the
// surrounding chrome (settings page + tab nav) lives in the campaigns
// plugin, which can't import this plugin without a cycle.
//
// GET /campaigns/:id/foundry/settings-tab
func (h *Handler) OwnerTabFragmentHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	// List of pinnable versions (excludes withdrawn).
	versions, err := h.svc.ListVersions(ctx, false)
	if err != nil {
		return err
	}
	current, err := h.svc.GetVersionForCampaign(ctx, cc.Campaign.ID)
	if err != nil {
		// 404 from the resolver means the catalog is empty OR pin is
		// withdrawn. Either way, render the tab without a "current"
		// section rather than failing the whole fragment.
		current = nil
	}
	installURL, err := h.svc.BuildInstallURL(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	// Current pin string (may be empty for latest-tracking campaigns).
	// The selector preselects this so the dropdown defaults match
	// what's actually saved.
	pin, _ := h.svc.GetCampaignPin(ctx, cc.Campaign.ID)
	return middleware.Render(c, http.StatusOK,
		OwnerTabFragment(OwnerTabData{
			CampaignID:     cc.Campaign.ID,
			CurrentPin:     pin,
			CurrentVersion: current,
			Versions:       versions,
			InstallURL:     installURL,
			CSRFToken:      middleware.GetCSRFToken(c),
		}))
}

// --- owner handlers ---

// SetPinAPI updates the calling campaign's FoundryModulePin.
// PUT /campaigns/:id/foundry/pin   Body: { "version": "0.1.5" }
//
// Owner-only. Empty version clears the pin (latest-tracking mode).
func (h *Handler) SetPinAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.svc.SetPinnedVersion(c.Request().Context(), cc.Campaign.ID, req.Version); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// RotateTokenAPI bumps the campaign's signing version and returns the
// new install URL. POST /campaigns/:id/foundry/token/rotate
func (h *Handler) RotateTokenAPI(c echo.Context) error {
	session := auth.GetSession(c)
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actorID := ""
	if session != nil {
		actorID = session.UserID
	}
	url, err := h.svc.RotateCampaignToken(c.Request().Context(),
		cc.Campaign.ID, actorID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// InstallURLAPI returns the campaign's current install URL.
// GET /campaigns/:id/foundry/install-url
func (h *Handler) InstallURLAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	url, err := h.svc.BuildInstallURL(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// --- public handlers (no campaign middleware; token-gated) ---

// PublicManifestAPI returns the per-campaign manifest JSON that
// Foundry hits at install + on every update check.
// GET /api/v1/campaigns/:cid/foundry/module.json?token=...
//
// 404 on token failure (don't leak whether the campaign exists).
func (h *Handler) PublicManifestAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return apperror.NewNotFound("invalid token")
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return err
	}
	manifest, _, err := h.svc.BuildManifestForCampaign(c.Request().Context(), cid)
	if err != nil {
		return err
	}
	return c.JSONBlob(http.StatusOK, manifest)
}

// PublicDownloadAPI serves the zip for the campaign's currently-
// resolved version. GET /api/v1/campaigns/:cid/foundry/module.zip?token=...
//
// Streams the file directly. The zip is server-global (not campaign-
// scoped) so there's no signed-media indirection — the per-campaign
// token already gates access.
func (h *Handler) PublicDownloadAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return apperror.NewNotFound("invalid token")
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return err
	}
	_, downloadPath, err := h.svc.BuildManifestForCampaign(c.Request().Context(), cid)
	if err != nil {
		return err
	}
	f, err := os.Open(downloadPath)
	if err != nil {
		// Inconsistent state (row says file is here, FS says no).
		// 500 rather than 404 — this is a server bug, not a client error.
		return apperror.NewInternal(err)
	}
	defer f.Close()
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="chronicle-foundry-module.zip"`)
	return c.Stream(http.StatusOK, "application/zip", f)
}
