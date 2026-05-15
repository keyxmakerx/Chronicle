package foundry_vtt

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler is the HTTP boundary. Echo handlers stay thin per the
// project conventions — bind, validate, call service, render.
//
// Three responsibilities:
//   - Owner endpoints: pin / rotate / install-url / owner-tab fragment
//   - Public endpoints: per-campaign manifest + download
//   - Error mapping: foundry_vtt.Error → categorized JSON response
type Handler struct {
	svc Service
}

// NewHandler constructs the Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// --- owner: tab fragment ---

// OwnerTabFragmentHandler serves the per-campaign settings tab as
// an HTMX fragment. Called by the campaigns settings.templ's
// VTT Setup Guides → Foundry VTT disclosure section via hx-get.
//
// GET /campaigns/:id/foundry-vtt/settings-tab
func (h *Handler) OwnerTabFragmentHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	data, err := h.svc.OwnerTabData(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return h.respondError(c, err)
	}
	data.CSRFToken = middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, OwnerTabFragment(data))
}

// --- owner: pin / rotate / install-url ---

// SetPinAPI updates the calling campaign's FoundryModulePin.
// PUT /campaigns/:id/foundry-vtt/pin   Body: { "version": "v0.1.5" }
//
// Empty version clears the pin (latest-tracking mode).
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
		return h.respondError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RotateTokenAPI bumps the per-campaign signing version and
// returns the freshly-minted install URL.
// POST /campaigns/:id/foundry-vtt/token/rotate
func (h *Handler) RotateTokenAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	url, err := h.svc.RotateCampaignToken(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// InstallURLAPI returns the campaign's current install URL.
// GET /campaigns/:id/foundry-vtt/install-url
func (h *Handler) InstallURLAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	url, err := h.svc.BuildInstallURL(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// --- public: manifest + download (token-gated, no campaign middleware) ---

// PublicManifestAPI is the endpoint Foundry hits on install + every
// update check. Token-gated; no campaign middleware (the per-
// campaign signed token is the only access control).
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.json?token=...
//
// Error responses are JSON-shaped per the operator's contract so
// Foundry's FM-CSU-DIAG can parse them and surface the message
// inline. See errors.go for the categorized formats.
func (h *Handler) PublicManifestAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return h.respondError(c, ErrInvalidToken(nil))
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return h.respondError(c, err)
	}
	manifest, _, err := h.svc.BuildManifestForCampaign(c.Request().Context(), cid)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSONBlob(http.StatusOK, manifest)
}

// PublicDownloadAPI streams the zip for the campaign's currently-
// resolved version. The per-campaign token is the only access
// control; same shape as manifest endpoint.
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.zip?token=...
//
// NOTE on download semantics: foundry_vtt does NOT bake URLs into
// a per-version cached zip (foundry_modules' RepackFoundryZip
// approach). Instead, the manifest endpoint rewrites URLs at serve-
// time, and the download endpoint streams the version's on-disk
// module dir as a fresh zip. This is the C-FMC-5b architectural
// shift the operator called out: per-campaign URLs require per-
// request URL rewriting; cached repacked zips are incompatible
// with that.
//
// For now this PR streams the packages plugin's installed-version
// dir as-is (re-zipped on the fly). Profiling under load may
// reveal we need a per-version cached zip with manifest stamped
// in — defer that to a future PR.
func (h *Handler) PublicDownloadAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return h.respondError(c, ErrInvalidToken(nil))
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return h.respondError(c, err)
	}
	_, installDir, err := h.svc.BuildManifestForCampaign(c.Request().Context(), cid)
	if err != nil {
		return h.respondError(c, err)
	}
	// Stream the directory as a zip. zipDir handles the directory
	// walk + zip encoding to the response writer.
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="chronicle-foundry-module.zip"`)
	c.Response().WriteHeader(http.StatusOK)
	if err := zipDirToWriter(installDir, c.Response()); err != nil {
		// Headers already sent; can't return a JSON error. Log
		// path via the framework's request logger.
		return err
	}
	return nil
}

// --- error mapping ---

// respondError converts an error to the right HTTP shape. For
// foundry_vtt typed errors, returns the categorized JSON body
// Foundry's FM-CSU-DIAG knows how to parse. For other errors,
// re-returns them so Echo's apperror middleware handles them.
func (h *Handler) respondError(c echo.Context, err error) error {
	fe := AsError(err)
	if fe == nil {
		return err
	}
	body := map[string]any{
		"error":    fe.Code,
		"message":  fe.Message,
		"category": string(fe.Category),
	}
	return c.JSON(fe.HTTPStatus(), body)
}

// --- zip helpers ---

// zipDirToWriter walks installDir and writes a zip stream to w.
// Files are stored at paths relative to installDir so the zip
// extracts to the same structure Foundry expects.
//
// Skips chronicle-package.json — the descriptor is Chronicle-side
// metadata, not part of the module Foundry installs. Including it
// would leak the descriptor contract into the client's filesystem
// and confuse Foundry's manifest reader.
func zipDirToWriter(installDir string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	return filepath.Walk(installDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(installDir, path)
		if err != nil {
			return err
		}
		// Exclude Chronicle-side descriptor — see comment above.
		if rel == descriptorFilename {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		entry, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, f)
		return err
	})
}
