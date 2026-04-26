package backup

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// Handler renders the admin backup page and accepts the "run backup" and
// "download artifact" actions. All routes are mounted under /admin/backup
// and inherit RequireSiteAdmin from the parent group.
type Handler struct {
	svc Service
}

// NewHandler constructs a Handler against the given Service.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// Page renders the backup dashboard (GET /admin/backup).
func (h *Handler) Page(c echo.Context) error {
	artifacts, err := h.svc.ListBackups()
	if err != nil {
		// Listing failure is not fatal — we still render the page so the
		// operator can at least try to start a backup. Surface the error
		// inline so they know why the table is empty.
		return middleware.Render(c, http.StatusOK, BackupPage(BackupPageData{
			BackupDir:    h.svc.BackupDir(),
			Artifacts:    nil,
			ListError:    err.Error(),
			LastRun:      h.svc.LastRun(),
			RunningNow:   h.svc.IsRunning(),
			CSRFToken:    middleware.GetCSRFToken(c),
		}))
	}
	return middleware.Render(c, http.StatusOK, BackupPage(BackupPageData{
		BackupDir:  h.svc.BackupDir(),
		Artifacts:  artifacts,
		LastRun:    h.svc.LastRun(),
		RunningNow: h.svc.IsRunning(),
		CSRFToken:  middleware.GetCSRFToken(c),
	}))
}

// Run triggers a backup (POST /admin/backup/run). The actual shell-out
// happens synchronously here — backups are infrequent and a 20-minute
// browser hang on the admin who clicked the button is acceptable. The
// rate limiter on the route prevents click-flooding.
//
// If a backup is already in flight (e.g. the admin double-clicked, or
// another admin is running one), we return 409 rather than coalescing
// — operators benefit from knowing their click didn't start a fresh run.
func (h *Handler) Run(c echo.Context) error {
	ctx := c.Request().Context()
	_, err := h.svc.RunBackup(ctx)
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			return echo.NewHTTPError(http.StatusConflict, "a backup is already running; wait for it to finish")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return middleware.HTMXRedirect(c, "/admin/backup")
}

// Download serves a single artifact file (GET /admin/backup/files/:name).
// The :name parameter is validated against BACKUP_DIR; any attempt at
// path traversal is rejected with 400.
//
// Uses echo.Context.Attachment which RFC 5987-encodes the filename in
// the Content-Disposition header. ResolveArtifactPath rejects path
// separators and ".." but does not reject quotes or newlines in
// basenames — Linux filesystems allow both. Without proper encoding,
// an admin who renamed a backup artifact on disk to include `"` or
// `\n` could inject HTTP headers via this response.
func (h *Handler) Download(c echo.Context) error {
	name := c.Param("name")
	full, err := ResolveArtifactPath(h.svc.BackupDir(), name)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.Attachment(full, name)
}
