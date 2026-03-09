// campaign_handler.go adds HTTP endpoints for campaign owners to upload,
// view, and remove custom game systems.
package systems

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CampaignSystemHandler handles upload and management of per-campaign
// custom game systems. Only campaign owners can upload/remove.
type CampaignSystemHandler struct {
	mgr *CampaignSystemManager
}

// NewCampaignSystemHandler creates a handler for custom system endpoints.
func NewCampaignSystemHandler(mgr *CampaignSystemManager) *CampaignSystemHandler {
	return &CampaignSystemHandler{mgr: mgr}
}

// UploadSystem handles POST /campaigns/:id/systems/upload.
// Accepts a ZIP file containing manifest.json + data/*.json, validates it,
// and installs it as the campaign's custom game system.
func (h *CampaignSystemHandler) UploadSystem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	// Only campaign owners can upload custom systems.
	if cc.MemberRole != campaigns.RoleOwner {
		return apperror.NewForbidden("only campaign owners can upload game systems")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file is required")
	}

	if file.Size > maxSystemZipSize {
		return apperror.NewBadRequest(fmt.Sprintf("file exceeds maximum size of %d MB", maxSystemZipSize/(1024*1024)))
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("opening uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	// Read into memory for zip.NewReader (needs io.ReaderAt).
	data, err := io.ReadAll(io.LimitReader(src, maxSystemZipSize+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("reading uploaded file: %w", err))
	}

	manifest, err := h.mgr.Install(cc.Campaign.ID, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return apperror.NewBadRequest(err.Error())
	}

	// Return updated section fragment for HTMX swap.
	if middleware.IsHTMX(c) {
		totalItems := 0
		if mod := h.mgr.GetSystem(cc.Campaign.ID); mod != nil {
			dp := mod.DataProvider()
			if dp != nil {
				for _, cat := range manifest.Categories {
					if items, err := dp.List(cat.Slug); err == nil {
						totalItems += len(items)
					}
				}
			}
		}
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, CustomSystemSection(cc.Campaign.ID, manifest, totalItems, csrfToken))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":   "Custom game system installed",
		"system_id": manifest.ID,
		"name":      manifest.Name,
	})
}

// DeleteSystem handles DELETE /campaigns/:id/systems/custom.
// Removes the campaign's custom game system from disk and memory.
func (h *CampaignSystemHandler) DeleteSystem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole != campaigns.RoleOwner {
		return apperror.NewForbidden("only campaign owners can remove game systems")
	}

	if err := h.mgr.Uninstall(cc.Campaign.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("removing custom system: %w", err))
	}

	// Return empty upload form for HTMX swap.
	if middleware.IsHTMX(c) {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, CustomSystemSection(cc.Campaign.ID, nil, 0, csrfToken))
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Custom game system removed",
	})
}

// GetCustomSystem handles GET /campaigns/:id/systems/custom.
// Returns an HTMX fragment showing the custom system status with
// upload or manage controls.
func (h *CampaignSystemHandler) GetCustomSystem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	manifest := h.mgr.GetManifest(cc.Campaign.ID)

	// Count items if a system is loaded.
	totalItems := 0
	if manifest != nil {
		if mod := h.mgr.GetSystem(cc.Campaign.ID); mod != nil {
			dp := mod.DataProvider()
			if dp != nil {
				for _, cat := range manifest.Categories {
					if items, err := dp.List(cat.Slug); err == nil {
						totalItems += len(items)
					}
				}
			}
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CustomSystemSection(cc.Campaign.ID, manifest, totalItems, csrfToken))
}
