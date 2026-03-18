// Package campaigns — export_handler.go provides HTTP handlers for campaign
// export and import. Export downloads a JSON file. Import accepts a JSON file
// upload and creates a new campaign from it.
package campaigns

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// maxImportSize is the maximum allowed import file size (10 MB).
const maxImportSize = 10 * 1024 * 1024

// ExportHandler handles export/import HTTP requests.
type ExportHandler struct {
	exportSvc *ExportImportService
}

// NewExportHandler creates a new export/import handler.
func NewExportHandler(exportSvc *ExportImportService) *ExportHandler {
	return &ExportHandler{exportSvc: exportSvc}
}

// ExportCampaign exports a campaign as a JSON download (GET /campaigns/:id/export).
// Requires campaign owner role.
func (h *ExportHandler) ExportCampaign(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	export, err := h.exportSvc.Export(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal export: %w", err))
	}

	// Build filename from campaign name.
	safeName := sanitizeFilename(cc.Campaign.Name)
	filename := fmt.Sprintf("chronicle-%s-%s.json",
		safeName,
		time.Now().Format("2006-01-02"),
	)

	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "application/json", data)
}

// ImportCampaignForm renders the import page with a file upload form
// (GET /campaigns/import).
func (h *ExportHandler) ImportCampaignForm(c echo.Context) error {
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, ImportCampaignPage(csrfToken))
}

// ImportCampaign imports a campaign from a JSON file upload (POST /campaigns/import).
// Creates a new campaign owned by the current user.
func (h *ExportHandler) ImportCampaign(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	// Read the uploaded file.
	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file upload required")
	}

	if file.Size > maxImportSize {
		return apperror.NewBadRequest("file too large, maximum 10 MB")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("open uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(io.LimitReader(src, maxImportSize+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("read uploaded file: %w", err))
	}
	if int64(len(data)) > maxImportSize {
		return apperror.NewBadRequest("file too large, maximum 10 MB")
	}

	// Parse and validate the export.
	export, err := DetectCampaignExport(data)
	if err != nil {
		return err
	}

	if err := h.exportSvc.Validate(export); err != nil {
		return err
	}

	// Import the campaign.
	campaign, err := h.exportSvc.Import(c.Request().Context(), userID, export)
	if err != nil {
		return err
	}

	// Redirect to the new campaign.
	redirectURL := "/campaigns/" + campaign.ID
	return middleware.HTMXRedirect(c, redirectURL)
}

// sanitizeFilename converts a campaign name to a safe filename component.
func sanitizeFilename(name string) string {
	// Replace spaces and special chars with hyphens.
	replacer := strings.NewReplacer(
		" ", "-", "/", "-", "\\", "-", ":", "-",
		"*", "", "?", "", "\"", "", "<", "", ">", "", "|", "",
	)
	safe := replacer.Replace(strings.ToLower(name))

	// Collapse multiple hyphens.
	for strings.Contains(safe, "--") {
		safe = strings.ReplaceAll(safe, "--", "-")
	}

	// Trim leading/trailing hyphens and limit length.
	safe = strings.Trim(safe, "-")
	if len(safe) > 50 {
		safe = safe[:50]
	}
	if safe == "" {
		safe = "campaign"
	}
	return safe
}
