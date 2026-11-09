package restore

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// confirmationToken is the literal string the operator must type into
// the confirmation field for /admin/restore/run to proceed. Matches the
// shell script's interactive convention so muscle memory transfers.
const confirmationToken = "RESTORE"

// Paths the maintenance gate lets the restoring admin's session reach while
// the site is closed. They are served by OwnerView, not the router.
const (
	pagePath   = "/admin/restore"
	statusPath = "/admin/restore/status"
	leavePath  = "/admin/restore/leave"
)

// Handler renders the restore page and accepts the "run restore" action.
// All routes are mounted under /admin/restore and inherit
// RequireSiteAdmin from the parent group.
type Handler struct {
	svc   Service
	actor func(*echo.Context) string
}

// NewHandler constructs a Handler against the given Service.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// SetActor tells the handler how to read the signed-in admin's user id. The
// auth plugin owns that lookup, so app wiring supplies it. Optional: without
// it the audit trail records no user.
func (h *Handler) SetActor(fn func(*echo.Context) string) { h.actor = fn }

func (h *Handler) actorID(c *echo.Context) string {
	if h.actor == nil {
		return ""
	}
	return h.actor(c)
}

// Page renders the restore dashboard (GET /admin/restore).
func (h *Handler) Page(c *echo.Context) error {
	data := RestorePageData{
		BackupDir:  h.svc.BackupDir(),
		Status:     h.svc.Status(),
		RunningNow: h.svc.IsRunning(),
		CSRFToken:  middleware.GetCSRFToken(c),
	}
	if data.Status.Safety != "" {
		data.HandCommand = h.svc.HandRestoreCommand(data.Status.Safety)
	}
	manifests, err := h.svc.ListManifests()
	if err != nil {
		data.ListError = err.Error()
	} else {
		data.Manifests = manifests
	}
	return middleware.Render(c, http.StatusOK, RestorePage(data))
}

// Run starts a restore (POST /admin/restore/run). Requires:
//   - manifest=<basename>: which backup to restore from.
//   - confirm=RESTORE: literal-string confirmation; the typed-in
//     confirmation surface that distinguishes a deliberate restore
//     from a mis-click. Anything else is rejected before the site closes.
//
// It returns as soon as the site is closed; the restore itself runs in the
// background and the admin's page follows it through the status endpoint.
func (h *Handler) Run(c *echo.Context) error {
	manifest := c.FormValue("manifest")
	if manifest == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "manifest is required")
	}
	confirm := c.FormValue("confirm")
	if confirm != confirmationToken {
		return echo.NewHTTPError(http.StatusBadRequest, "type RESTORE in the confirmation field")
	}
	// The closed site recognises its restoring admin by session; without one
	// nobody could watch the restore or leave maintenance if it failed.
	sessionHash := maintenance.SessionHash(c.Request())
	if sessionHash == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "sign in again before restoring")
	}

	err := h.svc.Start(c.Request().Context(), StartRequest{
		Manifest:    manifest,
		UserID:      h.actorID(c),
		SessionHash: sessionHash,
	})
	switch {
	case err == nil:
	case errors.Is(err, ErrAlreadyRunning):
		return echo.NewHTTPError(http.StatusConflict, "a restore is already running or the site is still in maintenance; open the restore page to see where it stands")
	case errors.Is(err, ErrInvalidManifest):
		return echo.NewHTTPError(http.StatusBadRequest, "that backup could not be found")
	default:
		slog.Error("restore start failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "The restore could not be started. Nothing was changed. Check the server log.")
	}
	return middleware.HTMXRedirect(c, pagePath)
}

// statusBody is the poll's JSON. It carries no script output; that stays on
// the page the owner already holds.
type statusBody struct {
	Active         bool   `json:"active"`
	Phase          string `json:"phase"`
	Step           string `json:"step"`
	Summary        string `json:"summary"`
	Manifest       string `json:"manifest"`
	Safety         string `json:"safety"`
	StartedAt      string `json:"startedAt,omitempty"`
	ElapsedSeconds int    `json:"elapsedSeconds"`
}

func bodyFor(s maintenance.Snapshot, now time.Time) statusBody {
	b := statusBody{
		Active:   s.Active,
		Phase:    string(s.Phase),
		Step:     s.Step,
		Summary:  s.Summary,
		Manifest: s.Manifest,
		Safety:   s.Safety,
	}
	if !s.StartedAt.IsZero() {
		b.StartedAt = s.StartedAt.UTC().Format(time.RFC3339)
		end := now
		if !s.FinishedAt.IsZero() {
			end = s.FinishedAt
		}
		b.ElapsedSeconds = int(end.Sub(s.StartedAt).Seconds())
	}
	return b
}

// Status answers the progress poll (GET /admin/restore/status), site admins
// only. While the site is closed the gate serves the same answer through
// OwnerView, because the router and database are not available then.
func (h *Handler) Status(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, bodyFor(h.svc.Status(), time.Now()))
}

// OwnerView is the maintenance.OwnerHandler: it serves the restoring admin's
// progress page, status poll and "leave maintenance" button with no database
// and no router. The gate only calls it for the session that started the
// restore, so nothing here re-checks who the caller is.
func (h *Handler) OwnerView(c *echo.Context) (bool, error) {
	req := c.Request()
	switch {
	case req.URL.Path == pagePath && req.Method == http.MethodGet:
		return true, h.maintenancePage(c)
	case req.URL.Path == statusPath && req.Method == http.MethodGet:
		return true, h.Status(c)
	case req.URL.Path == leavePath && req.Method == http.MethodPost:
		return true, h.leave(c)
	}
	return false, nil
}

func (h *Handler) maintenancePage(c *echo.Context) error {
	snap := h.svc.Status()
	data := MaintenancePageData{Status: snap}
	if snap.Safety != "" {
		data.HandCommand = h.svc.HandRestoreCommand(snap.Safety)
	}
	// The leave form exists only when leaving is allowed.
	if snap.Phase == maintenance.PhaseFailed || snap.Phase == maintenance.PhaseInterrupted {
		data.LeaveToken = h.svc.LeaveToken()
	}
	hdr := c.Response().Header()
	hdr.Set("Cache-Control", "no-store")
	hdr.Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return MaintenancePage(data).Render(c.Request().Context(), c.Response())
}

// leave opens the site after a failed or interrupted restore. The gate has
// already checked the session, but the global CSRF middleware sits behind the
// gate, so this is checked here: the per-restore token only the owner's page
// carries, and a same-origin check.
func (h *Handler) leave(c *echo.Context) error {
	req := c.Request()
	if !sameOrigin(req) {
		return echo.NewHTTPError(http.StatusForbidden, "request did not come from this site")
	}
	if !h.svc.CheckLeaveToken(c.FormValue("leave_token")) {
		return echo.NewHTTPError(http.StatusForbidden, "this page is out of date; reload it and try again")
	}
	snap := h.svc.Status()
	if err := h.svc.Leave(req.Context(), snap.Actor); err != nil {
		if errors.Is(err, maintenance.ErrStillBusy) {
			return echo.NewHTTPError(http.StatusConflict, "the restore is still running; wait for it to finish or fail")
		}
		if errors.Is(err, maintenance.ErrNotActive) {
			return c.Redirect(http.StatusSeeOther, pagePath)
		}
		slog.Error("leaving maintenance failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not leave maintenance")
	}
	return c.Redirect(http.StatusSeeOther, pagePath)
}

// sameOrigin rejects a cross-site POST using the browser's own headers.
func sameOrigin(r *http.Request) bool {
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" && sfs != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}
