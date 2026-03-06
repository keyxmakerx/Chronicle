package extensions

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves HTTP endpoints for extension management.
type Handler struct {
	svc    ExtensionService
	extDir string // Root dir for serving extension assets.
}

// NewHandler creates a new extension handler.
func NewHandler(svc ExtensionService, extDir string) *Handler {
	return &Handler{svc: svc, extDir: extDir}
}

// --- Admin Endpoints ---

// ListExtensions returns all installed extensions.
// GET /admin/extensions
func (h *Handler) ListExtensions(c echo.Context) error {
	exts, err := h.svc.List(c.Request().Context())
	if err != nil {
		return err
	}
	if exts == nil {
		exts = []Extension{}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, adminExtensionListFragment(exts))
	}
	return middleware.Render(c, http.StatusOK, adminExtensionListPage(exts))
}

// GetExtension returns details for a single extension.
// GET /admin/extensions/:extID
func (h *Handler) GetExtension(c echo.Context) error {
	extID := c.Param("extID")

	ext, err := h.svc.GetByExtID(c.Request().Context(), extID)
	if err != nil {
		return err
	}

	manifest := parseManifestFromExtension(ext)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, adminExtensionDetailFragment(ext, manifest))
	}
	return middleware.Render(c, http.StatusOK, adminExtensionDetailPage(ext, manifest))
}

// InstallExtension handles zip upload for installing a new extension.
// POST /admin/extensions/install
func (h *Handler) InstallExtension(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("authentication required")
	}

	// Read uploaded file.
	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file upload required")
	}

	if file.Size > int64(DefaultMaxZipSize) {
		return apperror.NewBadRequest("file too large (max 50 MB)")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()

	// Write to temp file.
	tmpFile, err := os.CreateTemp("", "chronicle-ext-upload-*.zip")
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := io.Copy(tmpFile, src); err != nil {
		tmpFile.Close()
		return apperror.NewInternal(err)
	}
	tmpFile.Close()

	ext, err := h.svc.Install(c.Request().Context(), tmpFile.Name(), session.UserID)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		// Re-render the extension list.
		exts, _ := h.svc.List(c.Request().Context())
		if exts == nil {
			exts = []Extension{}
		}
		return middleware.Render(c, http.StatusOK, adminExtensionListFragment(exts))
	}

	return c.JSON(http.StatusCreated, ext)
}

// UpdateExtension handles zip upload for updating an existing extension.
// PUT /admin/extensions/:extID
func (h *Handler) UpdateExtension(c echo.Context) error {
	extID := c.Param("extID")

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file upload required")
	}

	if file.Size > int64(DefaultMaxZipSize) {
		return apperror.NewBadRequest("file too large (max 50 MB)")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()

	tmpFile, err := os.CreateTemp("", "chronicle-ext-update-*.zip")
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := io.Copy(tmpFile, src); err != nil {
		tmpFile.Close()
		return apperror.NewInternal(err)
	}
	tmpFile.Close()

	ext, err := h.svc.Update(c.Request().Context(), tmpFile.Name(), extID)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		manifest := parseManifestFromExtension(ext)
		return middleware.Render(c, http.StatusOK, adminExtensionDetailFragment(ext, manifest))
	}

	return c.JSON(http.StatusOK, ext)
}

// UninstallExtension removes an extension site-wide.
// DELETE /admin/extensions/:extID
func (h *Handler) UninstallExtension(c echo.Context) error {
	extID := c.Param("extID")

	if err := h.svc.Uninstall(c.Request().Context(), extID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		exts, _ := h.svc.List(c.Request().Context())
		if exts == nil {
			exts = []Extension{}
		}
		return middleware.Render(c, http.StatusOK, adminExtensionListFragment(exts))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "uninstalled"})
}

// RescanExtensions discovers extensions from the filesystem.
// POST /admin/extensions/rescan
func (h *Handler) RescanExtensions(c echo.Context) error {
	discovered, err := h.svc.Rescan(c.Request().Context())
	if err != nil {
		return err
	}

	if discovered > 0 {
		slog.Info("rescan discovered new extensions",
			slog.Int("count", discovered),
		)
	}

	if middleware.IsHTMX(c) {
		exts, _ := h.svc.List(c.Request().Context())
		if exts == nil {
			exts = []Extension{}
		}
		return middleware.Render(c, http.StatusOK, adminExtensionListFragment(exts))
	}

	total, _ := h.svc.List(c.Request().Context())
	return c.JSON(http.StatusOK, map[string]int{
		"discovered": discovered,
		"total":      len(total),
	})
}

// --- Campaign Endpoints ---

// ListCampaignExtensions returns extensions available for a campaign.
// GET /campaigns/:id/extensions
func (h *Handler) ListCampaignExtensions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	exts, err := h.svc.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	if exts == nil {
		exts = []CampaignExtension{}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, campaignExtensionListFragment(cc, exts))
	}
	return c.JSON(http.StatusOK, map[string]any{"extensions": exts})
}

// PreviewExtension returns what an extension will contribute when enabled.
// GET /campaigns/:id/extensions/:extID/preview
func (h *Handler) PreviewExtension(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	extID := c.Param("extID")
	contributes, err := h.svc.PreviewContributes(c.Request().Context(), extID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, contributes)
}

// EnableExtension enables an extension for a campaign.
// POST /campaigns/:id/extensions/:extID/enable
func (h *Handler) EnableExtension(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("authentication required")
	}

	extID := c.Param("extID")
	if err := h.svc.EnableForCampaign(c.Request().Context(), cc.Campaign.ID, extID, session.UserID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		exts, _ := h.svc.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
		if exts == nil {
			exts = []CampaignExtension{}
		}
		return middleware.Render(c, http.StatusOK, campaignExtensionListFragment(cc, exts))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "enabled"})
}

// DisableExtension disables an extension for a campaign.
// POST /campaigns/:id/extensions/:extID/disable
func (h *Handler) DisableExtension(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	extID := c.Param("extID")
	if err := h.svc.DisableForCampaign(c.Request().Context(), cc.Campaign.ID, extID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		exts, _ := h.svc.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
		if exts == nil {
			exts = []CampaignExtension{}
		}
		return middleware.Render(c, http.StatusOK, campaignExtensionListFragment(cc, exts))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "disabled"})
}

// ServeAsset serves static assets from an extension's directory.
// GET /extensions/:extID/assets/*filepath
func (h *Handler) ServeAsset(c echo.Context) error {
	extID := c.Param("extID")
	assetPath := c.Param("*")

	// Validate the extension ID format.
	if !extIDPattern.MatchString(extID) {
		return apperror.NewBadRequest("invalid extension ID")
	}

	// Prevent path traversal.
	if strings.Contains(assetPath, "..") {
		return apperror.NewBadRequest("invalid asset path")
	}

	// Only serve allowed file types.
	ext := strings.ToLower(filepath.Ext(assetPath))
	allowedAssets := map[string]bool{
		".svg": true, ".png": true, ".webp": true,
		".jpg": true, ".jpeg": true, ".css": true,
	}
	if !allowedAssets[ext] {
		return apperror.NewBadRequest("file type not allowed")
	}

	fullPath := filepath.Join(h.extDir, extID, "assets", filepath.Clean(assetPath))

	// Ensure the resolved path is within the extension directory.
	absExtDir, _ := filepath.Abs(filepath.Join(h.extDir, extID))
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absExtDir) {
		return apperror.NewBadRequest("invalid asset path")
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return apperror.NewNotFound("asset not found")
	}

	// Set long cache for static extension assets.
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	slog.Debug("serving extension asset",
		slog.String("ext_id", extID),
		slog.String("path", assetPath),
	)

	return c.File(fullPath)
}
