package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// SetCheckupService wires the site check-up. runCtx is the server's lifetime
// context: a run started by Check now outlives the request that asked.
func (h *Handler) SetCheckupService(svc *CheckupService, runCtx context.Context) {
	h.checkup = svc
	h.checkupRunCtx = runCtx
}

// CheckupShield handles GET /admin/checkup: the top bar's shield and sheet,
// plus the reminder strip swapped in out of band. It may peek; the answers to
// Check now and Remind me tomorrow never do.
func (h *Handler) CheckupShield(c echo.Context) error {
	if h.checkup == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return h.renderCheckup(c, false)
}

// RunCheckup handles POST /admin/checkup/run (Check now). The run goes on in
// the background; the answer is the shield showing it in progress.
func (h *Handler) RunCheckup(c echo.Context) error {
	if h.checkup == nil {
		return apperror.NewMissingContext()
	}
	ctx := h.checkupRunCtx
	if ctx == nil {
		ctx = context.Background()
	}
	h.checkup.Start(ctx)
	return h.renderCheckup(c, true)
}

// SnoozeCheckup handles POST /admin/checkup/snooze (Remind me tomorrow).
func (h *Handler) SnoozeCheckup(c echo.Context) error {
	if h.checkup == nil {
		return apperror.NewMissingContext()
	}
	if err := h.checkup.Snooze(c.Request().Context(), auth.GetUserID(c)); err != nil {
		return err
	}
	return h.renderCheckup(c, true)
}

func (h *Handler) renderCheckup(c echo.Context, noPeek bool) error {
	var signedInAt time.Time
	if s := auth.GetSession(c); s != nil {
		signedInAt = s.CreatedAt
	}
	v := h.checkup.View(c.Request().Context(), auth.GetUserID(c), signedInAt, noPeek)
	return middleware.Render(c, http.StatusOK, CheckupShield(v))
}
