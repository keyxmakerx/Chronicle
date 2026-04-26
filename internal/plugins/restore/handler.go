package restore

import (
	"errors"
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
	svc Service
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
	_, err := h.svc.RunRestore(ctx, manifest)
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			return echo.NewHTTPError(http.StatusConflict, "a restore is already running; wait for it to finish")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return middleware.HTMXRedirect(c, "/admin/restore")
}
