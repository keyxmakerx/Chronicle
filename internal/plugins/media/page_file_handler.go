package media

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageFileHandler serves the Files section of a page and its downloads. It is
// thin: the access rule and the storage live in PageFileService.
type PageFileHandler struct {
	svc PageFileService
	// media only resolves where a stored file lives on disk.
	media          MediaService
	securityLogger SecurityEventLogger
}

// NewPageFileHandler creates the page file handler.
func NewPageFileHandler(svc PageFileService, media MediaService) *PageFileHandler {
	return &PageFileHandler{svc: svc, media: media}
}

// SetSecurityLogger wires the audit log for attaches and removals.
func (h *PageFileHandler) SetSecurityLogger(l SecurityEventLogger) {
	h.securityLogger = l
}

func (h *PageFileHandler) logEvent(c echo.Context, eventType string, details map[string]any) {
	if h.securityLogger != nil {
		_ = h.securityLogger.LogEvent(c.Request().Context(), eventType, auth.GetUserID(c), "", c.RealIP(), c.Request().UserAgent(), details)
	}
}

// viewer builds who is asking from the request: the promoted role, so a co-DM
// counts as the owner, the same as every other visibility check.
func pageFileViewer(c echo.Context, cc *campaigns.CampaignContext) PageFileViewer {
	return PageFileViewer{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
}

// Section renders the Files section (GET /campaigns/:id/entities/:eid/files).
//
// It is loaded into an already-shown page by HTMX, where a server error would
// retarget the whole page body, so no failure here is ever sent as an error:
// the section simply does not appear, and the cause is logged.
func (h *PageFileHandler) Section(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	entityID := c.Param("eid")
	listing, err := h.svc.List(c.Request().Context(), cc.Campaign.ID, entityID, pageFileViewer(c, cc))
	if err != nil {
		if apperror.SafeCode(err) >= http.StatusInternalServerError {
			slog.Error("page files: listing failed", slog.String("entity_id", entityID), slog.Any("error", err))
		}
		return c.NoContent(http.StatusNoContent)
	}
	// Someone who can only read, on a page with no files, has nothing to see.
	if len(listing.Files) == 0 && !listing.CanAttach {
		return c.NoContent(http.StatusNoContent)
	}
	return middleware.Render(c, http.StatusOK,
		PageFilesSection(cc.Campaign.ID, entityID, listing, c.QueryParam("first") == "1"))
}

// Upload attaches a file (POST /campaigns/:id/entities/:eid/files).
func (h *PageFileHandler) Upload(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}
	src, err := fh.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(src)
	if err != nil {
		if err.Error() == "http: request body too large" {
			return apperror.NewBadRequest("file too large")
		}
		return apperror.NewInternal(err)
	}

	gmOnly, ok := parseGMOnlyForm(c.FormValue("gmOnly"))
	// The other spelling is refused rather than ignored, so a client that
	// sends it is told instead of publishing a file it meant to hide.
	if !ok || c.FormValue("gm_only") != "" {
		return apperror.NewBadRequest("gmOnly must be true or false")
	}
	f, err := h.svc.Attach(c.Request().Context(), PageFileUpload{
		CampaignID: cc.Campaign.ID,
		EntityID:   c.Param("eid"),
		Viewer:     pageFileViewer(c, cc),
		Name:       fh.Filename,
		Bytes:      data,
		GMOnly:     gmOnly,
	})
	if err != nil {
		return err
	}
	h.logEvent(c, "media.uploaded", map[string]any{
		"file_id": f.ID, "campaign_id": f.CampaignID, "entity_id": f.EntityID,
		"mime_type": f.MimeType, "size": f.Size, "usage_type": UsagePageFile,
	})
	return c.JSON(http.StatusCreated, f)
}

// Download sends a file as an attachment
// (GET /campaigns/:id/entities/:eid/files/:fid/download).
//
// Nothing a person uploaded is ever shown by the browser itself: the type is
// one of the stored allow-list (never HTML or SVG), the response is an
// attachment, and sniffing is switched off, so a file that lied about being a
// document still cannot run as a page.
func (h *PageFileHandler) Download(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	f, stored, err := h.svc.Open(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), c.Param("fid"), pageFileViewer(c, cc))
	if err != nil {
		return err
	}
	setDownloadHeaders(c, f)
	return c.File(h.media.FilePath(stored))
}

// setDownloadHeaders is the one place a page file's response headers are set.
func setDownloadHeaders(c echo.Context, f *PageFile) {
	hdr := c.Response().Header()
	ctype := f.MimeType
	if !PageFileMimeTypes[ctype] {
		ctype = "application/octet-stream"
	}
	hdr.Set("Content-Type", ctype)
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName(f)}))
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hdr.Set("X-Frame-Options", "DENY")
	hdr.Set("Referrer-Policy", "no-referrer")
	hdr.Set("Cross-Origin-Resource-Policy", "same-origin")
	// Who may fetch it changes with the page's sharing, so no cache keeps it.
	hdr.Set("Cache-Control", "private, no-store, max-age=0")
}

// downloadName is the name offered to the browser: the original, with anything
// that could confuse a header or a file system turned into an underscore.
func downloadName(f *PageFile) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '"', '\\', '/', ':', '*', '?', '<', '>', '|':
			return '_'
		}
		return r
	}, f.Name)
	if strings.TrimSpace(name) == "" {
		return "file"
	}
	// Pictures are re-encoded on upload (it strips location and camera data),
	// so a .webp may now be a PNG. The offered name must match what is sent, or
	// the file will not open.
	if strings.HasPrefix(f.MimeType, "image/") {
		ext := strings.ToLower(filepath.Ext(name))
		if pageFileExtensions[ext] != f.MimeType {
			if want := MimeToExtension[f.MimeType]; want != "" {
				name = strings.TrimSuffix(name, filepath.Ext(name)) + want
			}
		}
	}
	return name
}

// Remove deletes a file (DELETE /campaigns/:id/entities/:eid/files/:fid).
func (h *PageFileHandler) Remove(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	f, err := h.svc.Remove(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), c.Param("fid"), pageFileViewer(c, cc))
	if err != nil {
		return err
	}
	h.logEvent(c, "media.deleted", map[string]any{
		"file_id": f.ID, "campaign_id": f.CampaignID, "entity_id": f.EntityID, "usage_type": UsagePageFile,
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// pageFileVisibilityBody is the one field the visibility route takes.
type pageFileVisibilityBody struct {
	// A pointer so an absent field is told apart from false: a body that
	// misspells the key must not quietly publish a GM-only file.
	GMOnly *bool `json:"gmOnly"`
}

// parseGMOnlyForm reads the attach form's checkbox. "on" is what a browser
// sends for a ticked box; anything else that is not plainly yes or no is
// refused instead of being read as "not GM only".
func parseGMOnlyForm(v string) (gmOnly, ok bool) {
	switch v {
	case "":
		return false, true
	case "1", "true", "on":
		return true, true
	case "0", "false":
		return false, true
	}
	return false, false
}

// SetVisibility marks a file GM-only or lets the page's readers see it again
// (PUT /campaigns/:id/entities/:eid/files/:fid/visibility).
func (h *PageFileHandler) SetVisibility(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var body pageFileVisibilityBody
	if err := c.Bind(&body); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if body.GMOnly == nil {
		return apperror.NewBadRequest("gmOnly is required")
	}
	f, err := h.svc.SetGMOnly(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), c.Param("fid"), pageFileViewer(c, cc), *body.GMOnly)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"id": f.ID, "gmOnly": f.GMOnly})
}
