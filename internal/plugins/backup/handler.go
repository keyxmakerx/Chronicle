package backup

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// Handler renders the admin backup page and accepts the "run backup" and
// "download artifact" actions. All routes are mounted under /admin/backup
// and inherit RequireSiteAdmin from the parent group.
type Handler struct {
	activity ActivityRecorder
	svc      Service
	signer   *downloadSigner
	userID   func(echo.Context) string
	settings SettingsStore
}

// SetScheduleStore wires the site settings that hold the daily backup
// schedule. Without it the page leaves the schedule card out.
func (h *Handler) SetScheduleStore(st SettingsStore) { h.settings = st }

// NewHandler constructs a Handler against the given Service. Downloads fail
// closed until SetDownloadAuth supplies the identity lookup.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc, signer: newRandomDownloadSigner(), userID: func(echo.Context) string { return "" }}
}

// SetDownloadAuth wires the signing secret and the current-user lookup used
// to bind download tokens to the admin who re-confirmed. The lookup is
// injected so this plugin does not import the auth plugin. An empty secret
// keeps the per-process random key.
func (h *Handler) SetDownloadAuth(secret string, userID func(echo.Context) string) {
	if secret != "" {
		h.signer.key = []byte(secret)
	}
	if userID != nil {
		h.userID = userID
	}
}

// Page renders the backup dashboard (GET /admin/backup).
func (h *Handler) Page(c echo.Context) error {
	d := BackupPageData{
		BackupDir:  h.svc.BackupDir(),
		LastRun:    h.svc.LastRun(),
		RunningNow: h.svc.IsRunning(),
		CSRFToken:  middleware.GetCSRFToken(c),
		ServerZone: time.Now().Format("MST"),
	}
	artifacts, err := h.svc.ListBackups()
	if err != nil {
		// Listing failure is not fatal — we still render the page so the
		// operator can at least try to start a backup. Surface the error
		// inline so they know why the table is empty.
		d.ListError = err.Error()
	} else {
		d.Artifacts = artifacts
	}
	if h.settings != nil {
		ctx := c.Request().Context()
		sched, sErr := LoadSchedule(ctx, h.settings)
		last, lErr := LoadLastScheduledRun(ctx, h.settings)
		if sErr != nil || lErr != nil {
			slog.Warn("backup page: could not read schedule", slog.Any("error", errors.Join(sErr, lErr)))
		} else {
			d.Schedule = &sched
			d.LastScheduled = last
		}
	}
	return middleware.Render(c, http.StatusOK, BackupPage(d))
}

// SaveSchedule stores the daily backup choice (POST /admin/backup/schedule).
func (h *Handler) SaveSchedule(c echo.Context) error {
	if h.settings == nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	hour, hErr := strconv.Atoi(c.FormValue("hour"))
	keep, kErr := strconv.Atoi(c.FormValue("keep_days"))
	if hErr != nil || kErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Pick an hour and how many days to keep.")
	}
	sched := Schedule{Enabled: c.FormValue("enabled") == "on", Hour: hour, KeepDays: keep}
	if err := SaveSchedule(c.Request().Context(), h.settings, sched); err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusUnprocessableEntity {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, ae.Message)
		}
		slog.Error("backup schedule save failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "The schedule could not be saved. Try again.")
	}
	label := "off"
	if sched.Enabled {
		label = fmt.Sprintf("daily at %02d:00, keep %d days", sched.Hour, sched.KeepDays)
	}
	h.recordActivity(c, "backup.schedule", "backup", "", label)
	return middleware.HTMXRedirect(c, "/admin/backup")
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
		// The script output is on the page's "technical details" block and in
		// the log; the error text can carry paths and command output.
		slog.Error("backup run failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "The backup could not be completed. Open the technical details on this page, or check the server log.")
	}
	h.recordActivity(c, "backup.run", "backup", "", "")
	return middleware.HTMXRedirect(c, "/admin/backup")
}

// DownloadLink mints a short-lived download URL (POST
// /admin/backup/files/:name/link). The route sits behind the reauth
// middleware, so a token only exists after a recent password confirmation.
// The name is validated exactly as Download does.
func (h *Handler) DownloadLink(c echo.Context) error {
	name := c.Param("name")
	if _, err := ResolveArtifactPath(h.svc.BackupDir(), name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	uid := h.userID(c)
	if uid == "" {
		return echo.NewHTTPError(http.StatusForbidden, "Sign in again to download this file.")
	}
	target := "/admin/backup/files/" + url.PathEscape(name) + "?t=" + url.QueryEscape(h.signer.Issue(uid, name))
	return middleware.HTMXRedirect(c, target)
}

// Download serves a single artifact file (GET /admin/backup/files/:name).
// It requires a valid token from DownloadLink for this user and file name,
// so the file cannot be fetched without a recent re-confirmation. The :name
// parameter is validated against BACKUP_DIR; any attempt at
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
	if !h.signer.Verify(c.QueryParam("t"), h.userID(c), name) {
		return echo.NewHTTPError(http.StatusForbidden, "This download link is missing or has expired. Use the Download button on the Backups page.")
	}
	return c.Attachment(full, name)
}
