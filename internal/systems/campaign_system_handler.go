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

// EntityCountProvider returns entity counts grouped by type for a campaign.
// Decouples the system handler from the entities plugin.
type EntityCountProvider func(ctx context.Context, campaignID string) (map[int]int, error)

// EntityTypeProvider returns entity types for a campaign.
// Decouples the system handler from the entities plugin.
type EntityTypeProvider func(ctx context.Context, campaignID string) ([]DiagEntityType, error)

// DiagEntityType holds entity type info needed by the diagnostics page.
type DiagEntityType struct {
	ID             int
	Name           string
	Slug           string
	PresetCategory string // Empty if not from a system preset.
	Count          int    // Populated separately from EntityCountProvider.
}

// CampaignSystemHandler handles upload and management of per-campaign
// custom game systems. Only campaign owners can upload/remove.
type CampaignSystemHandler struct {
	mgr              *CampaignSystemManager
	uploadPolicy     UploadPolicyProvider // Returns "auto_approve", "require_approval", or "disabled".
	entityCounter    EntityCountProvider
	entityTypeLister EntityTypeProvider
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

// SetEntityDeps wires entity data providers for the diagnostics page.
func (h *CampaignSystemHandler) SetEntityDeps(counter EntityCountProvider, typeLister EntityTypeProvider) {
	h.entityCounter = counter
	h.entityTypeLister = typeLister
}

// SystemStatus handles GET /campaigns/:id/systems/status.
// Shows a diagnostics page for the currently enabled game system: manifest
// details, category item counts, entity presets, Foundry compatibility, and
// validation warnings.
func (h *CampaignSystemHandler) SystemStatus(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	diag := h.buildDiagnostics(c.Request().Context(), cc.Campaign.ID)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SystemStatusContent(cc, diag))
	}
	return middleware.Render(c, http.StatusOK, SystemStatusPage(cc, diag))
}

// SystemDiagnostics holds all data for the diagnostics page.
type SystemDiagnostics struct {
	// SystemFound is true if any game system is enabled or custom-uploaded.
	SystemFound bool
	Manifest    *SystemManifest

	// Source is "builtin", "custom", or "package".
	Source string

	// Category item counts.
	Categories []DiagCategory

	// Entity types derived from this system's presets.
	EntityTypes []DiagEntityType

	// Total reference data items.
	TotalItems int

	// Foundry compatibility.
	FoundrySystemID string
	MappedFields    int
	TotalFields     int

	// Validation warnings.
	Warnings []string
}

// DiagCategory holds a category with its item count.
type DiagCategory struct {
	Slug  string
	Name  string
	Icon  string
	Count int
}

// buildDiagnostics gathers all diagnostic data for a campaign's system.
func (h *CampaignSystemHandler) buildDiagnostics(ctx context.Context, campaignID string) *SystemDiagnostics {
	diag := &SystemDiagnostics{}

	// Try custom uploaded system first, then built-in.
	manifest := h.mgr.GetManifest(campaignID)
	var sys System
	source := "custom"

	if manifest == nil {
		// No custom system — find enabled built-in system via registry scan.
		for _, m := range Registry() {
			manifest = m
			sys = FindSystem(m.ID)
			source = "builtin"
			// We don't know which is enabled — check all. For now, show the first
			// that has a live system instance. The handler should ideally know
			// the enabled addon, but we keep it simple.
			if sys != nil {
				break
			}
		}
		// If no live system, still nil — diagnostics shows "no system".
		if sys == nil {
			manifest = nil
		}
	} else {
		sys = h.mgr.GetSystem(campaignID)
	}

	if manifest == nil {
		return diag
	}

	diag.SystemFound = true
	diag.Manifest = manifest
	diag.Source = source
	diag.FoundrySystemID = manifest.FoundrySystemID

	// Count reference items per category.
	if sys != nil {
		dp := sys.DataProvider()
		if dp != nil {
			for _, cat := range manifest.Categories {
				count := 0
				if items, err := dp.List(cat.Slug); err == nil {
					count = len(items)
				}
				diag.Categories = append(diag.Categories, DiagCategory{
					Slug:  cat.Slug,
					Name:  cat.Name,
					Icon:  cat.Icon,
					Count: count,
				})
				diag.TotalItems += count
			}
		}
	} else {
		for _, cat := range manifest.Categories {
			diag.Categories = append(diag.Categories, DiagCategory{
				Slug: cat.Slug,
				Name: cat.Name,
				Icon: cat.Icon,
			})
		}
	}

	// Count mapped Foundry fields across all presets and categories.
	for _, preset := range manifest.EntityPresets {
		for _, f := range preset.Fields {
			diag.TotalFields++
			if f.FoundryPath != "" {
				diag.MappedFields++
			}
		}
	}
	for _, cat := range manifest.Categories {
		for _, f := range cat.Fields {
			diag.TotalFields++
			if f.FoundryPath != "" {
				diag.MappedFields++
			}
		}
	}

	// Get entity types for this campaign.
	if h.entityTypeLister != nil {
		if types, err := h.entityTypeLister(ctx, campaignID); err == nil {
			diag.EntityTypes = types
		}
	}

	// Merge entity counts.
	if h.entityCounter != nil {
		if counts, err := h.entityCounter(ctx, campaignID); err == nil {
			for i := range diag.EntityTypes {
				diag.EntityTypes[i].Count = counts[diag.EntityTypes[i].ID]
			}
		}
	}

	// Validation warnings.
	report := manifest.BuildValidationReport()
	diag.Warnings = report.Warnings

	return diag
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
