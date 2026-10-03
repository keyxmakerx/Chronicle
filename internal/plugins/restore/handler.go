package restore

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// confirmationToken is the literal string the operator must type into
// the confirmation field for /admin/restore/run to proceed. Matches the
// shell script's interactive convention so muscle memory transfers.
const confirmationToken = "RESTORE"

// Handler renders the restore page and accepts the "run restore" action.
// All routes are mounted under /admin/restore and inherit
// RequireSiteAdmin from the parent group.
type Handler struct {
	activity ActivityRecorder
	svc      Service
}

// NewHandler constructs a Handler against the given Service.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// Page renders the restore dashboard (GET /admin/restore).
func (h *Handler) Page(c echo.Context) error {
	manifests, err := h.svc.ListManifests()
	if err != nil {
		return middleware.Render(c, http.StatusOK, RestorePage(RestorePageData{
			BackupDir:  h.svc.BackupDir(),
			Manifests:  nil,
			ListError:  err.Error(),
			LastRun:    h.svc.LastRun(),
			RunningNow: h.svc.IsRunning(),
			CSRFToken:  middleware.GetCSRFToken(c),
		}))
	}
	return middleware.Render(c, http.StatusOK, RestorePage(RestorePageData{
		BackupDir:  h.svc.BackupDir(),
		Manifests:  manifests,
		LastRun:    h.svc.LastRun(),
		RunningNow: h.svc.IsRunning(),
		CSRFToken:  middleware.GetCSRFToken(c),
	}))
}

// Run triggers a restore (POST /admin/restore/run). Requires:
//   - manifest=<basename>: which backup to restore from.
//   - confirm=RESTORE: literal-string confirmation; the typed-in
//     confirmation surface that distinguishes a deliberate restore
//     from a mis-click. Anything else is rejected before the shell-out.
//
// The 30-minute shell-out is synchronous on the request: the admin's
// browser hangs for the duration. That's the right shape because the
// site is unavailable during restore anyway — there's no "background
// completion" the operator could navigate to.
func (h *Handler) Run(c echo.Context) error {
	manifest := c.FormValue("manifest")
	if manifest == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "manifest is required")
	}
	confirm := c.FormValue("confirm")
	if confirm != confirmationToken {
		return echo.NewHTTPError(http.StatusBadRequest, "type RESTORE in the confirmation field")
	}

	ctx := c.Request().Context()

	// A restore replaces the database, and the change log lives in it, so a
	// row written only afterwards survives while one written only before is
	// overwritten. Record both: "started" is the only trace if the restore
	// fails or the swap is partial, "run" lands in the restored database.
	// The IsRunning check keeps a refused concurrent attempt from logging a
	// restore that never began.
	if !h.svc.IsRunning() {
		h.recordActivity(c, "restore.started", "backup", "", manifest)
	}
	_, err := h.svc.RunRestore(ctx, manifest)
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			return echo.NewHTTPError(http.StatusConflict, "a restore is already running; wait for it to finish")
		}
		slog.Error("restore run failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "The restore could not be completed. Open the technical details on this page, or check the server log.")
	}
	h.recordActivity(c, "restore.run", "backup", "", manifest)
	return middleware.HTMXRedirect(c, "/admin/restore")
}
