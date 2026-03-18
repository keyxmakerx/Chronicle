// campaign_handler.go adds HTTP endpoints for campaign owners to upload,
// view, and remove custom game systems.
package systems

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// UploadPolicyProvider returns the current owner upload policy setting.
// Decouples the campaign handler from the packages plugin.
type UploadPolicyProvider func(ctx context.Context) string

// CampaignSystemHandler handles upload and management of per-campaign
// custom game systems. Only campaign owners can upload/remove.
type CampaignSystemHandler struct {
	mgr          *CampaignSystemManager
	uploadPolicy UploadPolicyProvider // Returns "auto_approve", "require_approval", or "disabled".
}

// NewCampaignSystemHandler creates a handler for custom system endpoints.
func NewCampaignSystemHandler(mgr *CampaignSystemManager) *CampaignSystemHandler {
	return &CampaignSystemHandler{mgr: mgr}
}

// SetUploadPolicy wires the upload policy provider so the handler can
// check whether campaign owners are allowed to upload custom systems.
func (h *CampaignSystemHandler) SetUploadPolicy(provider UploadPolicyProvider) {
	h.uploadPolicy = provider
}

// PreviewSystem handles POST /campaigns/:id/systems/preview.
// Accepts a ZIP file, validates it without writing to disk, and returns
// a full preview of what the system contains (impact tree, categories,
// entity presets, warnings). The owner can then confirm installation.
func (h *CampaignSystemHandler) PreviewSystem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole != campaigns.RoleOwner {
		return apperror.NewForbidden("only campaign owners can upload game systems")
	}

	// Check upload policy.
	if h.uploadPolicy != nil {
		policy := h.uploadPolicy(c.Request().Context())
		if policy == "disabled" {
			return apperror.NewForbidden("custom system uploads are disabled by the site administrator")
		}
	}

	data, err := h.readUploadedZIP(c)
	if err != nil {
		return err
	}

	preview, err := PreviewFromZIP(data)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("previewing system: %w", err))
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, SystemPreviewPage(preview, cc.Campaign.ID, csrfToken))
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

	// Check upload policy.
	if h.uploadPolicy != nil {
		policy := h.uploadPolicy(c.Request().Context())
		if policy == "disabled" {
			return apperror.NewForbidden("custom system uploads are disabled by the site administrator")
		}
	}

	data, err := h.readUploadedZIP(c)
	if err != nil {
		return err
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

// readUploadedZIP reads and validates the uploaded ZIP file from the request.
func (h *CampaignSystemHandler) readUploadedZIP(c echo.Context) ([]byte, error) {
	file, err := c.FormFile("file")
	if err != nil {
		return nil, apperror.NewBadRequest("file is required")
	}

	if file.Size > maxSystemZipSize {
		return nil, apperror.NewBadRequest(fmt.Sprintf("file exceeds maximum size of %d MB", maxSystemZipSize/(1024*1024)))
	}

	src, err := file.Open()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("opening uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(io.LimitReader(src, maxSystemZipSize+1))
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading uploaded file: %w", err))
	}

	return data, nil
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
