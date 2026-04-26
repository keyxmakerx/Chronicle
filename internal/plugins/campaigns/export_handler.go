// Package campaigns — export_handler.go provides HTTP handlers for campaign
// export and import. Export downloads a JSON file (or a zip with media bytes
// when ?include_media=1 is set). Import accepts either format and creates a
// new campaign from it.
package campaigns

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// maxImportSize is the maximum allowed import file size for JSON-only
// uploads (10 MB).
const maxImportSize = 10 * 1024 * 1024

// maxImportZipSize is the maximum allowed size for zip uploads. Zip
// imports include media bytes, so the cap is larger than the JSON-only
// path. Mirrors the export-side cap to keep round-trips symmetric.
const maxImportZipSize = 500 * 1024 * 1024

// maxExportMediaBytes caps the total uncompressed media size we are
// willing to embed in one export bundle. Refuse before opening any
// file when the campaign exceeds this — the operator should partition
// the campaign or ask the admin for a server-side backup instead.
const maxExportMediaBytes int64 = 500 * 1024 * 1024

// zipMagic is the four-byte signature at the start of every zip archive
// (PK\x03\x04). We sniff this to distinguish zip uploads from raw JSON
// uploads in ImportCampaign without forcing the operator to pick a
// content-type.
var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// ExportHandler handles export/import HTTP requests.
type ExportHandler struct {
	exportSvc *ExportImportService
}

// NewExportHandler creates a new export/import handler.
func NewExportHandler(exportSvc *ExportImportService) *ExportHandler {
	return &ExportHandler{exportSvc: exportSvc}
}

// ExportCampaign exports a campaign as a JSON download (GET /campaigns/:id/export).
// When ?include_media=1 is set, the response is a zip containing the JSON
// envelope as campaign.json plus a media/ directory with each campaign-owned
// media file. Requires campaign owner role.
func (h *ExportHandler) ExportCampaign(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	ctx := c.Request().Context()
	export, err := h.exportSvc.Export(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal export: %w", err))
	}

	includeMedia := c.QueryParam("include_media") == "1"
	bundler := h.exportSvc.MediaBundlerImpl()
	safeName := sanitizeFilename(cc.Campaign.Name)
	dateStamp := time.Now().Format("2006-01-02")

	if !includeMedia || bundler == nil {
		filename := fmt.Sprintf("chronicle-%s-%s.json", safeName, dateStamp)
		c.Response().Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s"`, filename))
		return c.Blob(http.StatusOK, "application/json", jsonData)
	}

	// Media-bundle path. List files first so we can enforce the size
	// cap before doing any IO on the actual file bytes.
	files, err := bundler.BundleMedia(ctx, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("list bundled media: %w", err))
	}
	var totalBytes int64
	for _, f := range files {
		totalBytes += f.SizeBytes
	}
	if totalBytes > maxExportMediaBytes {
		return apperror.NewBadRequest(fmt.Sprintf(
			"campaign media is %d MB which exceeds the %d MB bundle cap; export without ?include_media=1 and back up media separately",
			totalBytes/(1024*1024), maxExportMediaBytes/(1024*1024),
		))
	}

	zipName := fmt.Sprintf("chronicle-%s-%s.zip", safeName, dateStamp)
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, zipName))
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().WriteHeader(http.StatusOK)

	zw := zip.NewWriter(c.Response().Writer)
	defer func() { _ = zw.Close() }()

	// 1. campaign.json at the root.
	jsonEntry, err := zw.Create("campaign.json")
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("create campaign.json entry: %w", err))
	}
	if _, err := jsonEntry.Write(jsonData); err != nil {
		return apperror.NewInternal(fmt.Errorf("write campaign.json: %w", err))
	}

	// 2. media/<filename> for each file. UUID-based filenames mean no
	// collisions and no path-traversal risk. Sanitize defensively
	// anyway: any filename that contains a separator gets dropped with
	// a warning rather than the whole bundle aborting.
	for _, f := range files {
		if strings.ContainsAny(f.Filename, `/\`) || strings.Contains(f.Filename, "..") {
			slog.Warn("export: skipping media file with unsafe basename",
				slog.String("filename", f.Filename),
			)
			continue
		}
		entry, err := zw.Create("media/" + f.Filename)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("create zip entry %q: %w", f.Filename, err))
		}
		src, err := f.Open()
		if err != nil {
			// Don't abort the whole bundle for one missing file —
			// the JSON manifest still records its metadata, and the
			// operator can investigate from logs.
			slog.Warn("export: failed to open media file; skipping",
				slog.String("filename", f.Filename),
				slog.Any("error", err),
			)
			continue
		}
		_, copyErr := io.Copy(entry, src)
		_ = src.Close()
		if copyErr != nil {
			return apperror.NewInternal(fmt.Errorf("copy media bytes for %q: %w", f.Filename, copyErr))
		}
	}
	return nil
}

// ImportCampaignForm renders the import page with a file upload form
// (GET /campaigns/import).
func (h *ExportHandler) ImportCampaignForm(c echo.Context) error {
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, ImportCampaignPage(csrfToken))
}

// ImportCampaign imports a campaign from an uploaded JSON file or zip
// bundle (POST /campaigns/import). Creates a new campaign owned by the
// current user. The zip path accepts files produced by ?include_media=1
// exports; embedded media bytes are NOT yet restored automatically (see
// docs/campaign-import.md), but the structural data still lands.
func (h *ExportHandler) ImportCampaign(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("file upload required")
	}

	// Pick the size cap based on whether this looks like a zip. We
	// can't know for sure until we sniff the bytes, so the cap is the
	// larger zip ceiling — once we know the format, we re-check
	// against the format-specific cap.
	if file.Size > maxImportZipSize {
		return apperror.NewBadRequest(fmt.Sprintf("file too large, maximum %d MB", maxImportZipSize/(1024*1024)))
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("open uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(io.LimitReader(src, maxImportZipSize+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("read uploaded file: %w", err))
	}
	if int64(len(data)) > maxImportZipSize {
		return apperror.NewBadRequest(fmt.Sprintf("file too large, maximum %d MB", maxImportZipSize/(1024*1024)))
	}

	jsonData := data
	mediaCount := 0
	if isZip(data) {
		extracted, count, extractErr := extractCampaignJSONFromZip(data)
		if extractErr != nil {
			return apperror.NewBadRequest(extractErr.Error())
		}
		jsonData = extracted
		mediaCount = count
	} else {
		// JSON-only upload — enforce the tighter cap.
		if int64(len(data)) > maxImportSize {
			return apperror.NewBadRequest(fmt.Sprintf("JSON file too large, maximum %d MB; export with media as a zip if larger", maxImportSize/(1024*1024)))
		}
	}

	export, err := DetectCampaignExport(jsonData)
	if err != nil {
		return err
	}

	if err := h.exportSvc.Validate(export); err != nil {
		return err
	}

	campaign, err := h.exportSvc.Import(c.Request().Context(), userID, export)
	if err != nil {
		return err
	}

	if mediaCount > 0 {
		// Surfaced in logs so the operator knows the limitation kicked
		// in. Per-user UI messaging would require a flash-message
		// surface that isn't built yet; logging is the v1 compromise.
		slog.Info("campaign import: media bytes in zip not yet restored",
			slog.String("campaign", campaign.ID),
			slog.Int("skipped_media_files", mediaCount),
		)
	}

	redirectURL := "/campaigns/" + campaign.ID
	return middleware.HTMXRedirect(c, redirectURL)
}

// isZip checks the four-byte zip magic. False for any input shorter
// than the magic so a stub upload isn't misclassified.
func isZip(data []byte) bool {
	return len(data) >= len(zipMagic) && bytes.Equal(data[:len(zipMagic)], zipMagic)
}

// extractCampaignJSONFromZip pulls campaign.json from a zip blob and
// returns its bytes plus the count of media/* entries we observed but
// did not restore. Refuses zips that don't have campaign.json at the
// root (those aren't ours).
func extractCampaignJSONFromZip(data []byte) ([]byte, int, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid zip: %w", err)
	}
	var jsonBytes []byte
	mediaCount := 0
	for _, entry := range r.File {
		switch {
		case entry.Name == "campaign.json":
			rc, err := entry.Open()
			if err != nil {
				return nil, 0, fmt.Errorf("open campaign.json in zip: %w", err)
			}
			b, err := io.ReadAll(io.LimitReader(rc, maxImportSize+1))
			_ = rc.Close()
			if err != nil {
				return nil, 0, fmt.Errorf("read campaign.json in zip: %w", err)
			}
			if int64(len(b)) > maxImportSize {
				return nil, 0, fmt.Errorf("campaign.json inside zip exceeds %d MB", maxImportSize/(1024*1024))
			}
			jsonBytes = b
		case strings.HasPrefix(entry.Name, "media/"):
			mediaCount++
		}
	}
	if jsonBytes == nil {
		return nil, 0, fmt.Errorf("zip is missing campaign.json at the root; upload a chronicle export")
	}
	return jsonBytes, mediaCount, nil
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
