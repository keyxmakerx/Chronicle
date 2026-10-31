package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// SetReleaseNoticeService wires the new-version notice. Left unset, Home
// shows no notice and the Security page no switch.
func (h *Handler) SetReleaseNoticeService(svc *ReleaseNoticeService) {
	h.releaseNotice = svc
}

// ReleaseNoticeLater handles POST /admin/release-notice/later: hide this
// version from the caller until a newer one is out. The empty answer swaps
// the notice away.
func (h *Handler) ReleaseNoticeLater(c echo.Context) error {
	if h.releaseNotice == nil {
		return apperror.NewMissingContext()
	}
	if err := h.releaseNotice.Later(c.Request().Context(), auth.GetUserID(c), c.FormValue("version")); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// SaveUpdateCheck handles POST /admin/security/update-check: turn the
// calls to GitHub about Chronicle's own version on or off. Reauth-gated like
// the rest of the Security page, since turning it off silences news of
// security fixes.
func (h *Handler) SaveUpdateCheck(c echo.Context) error {
	if h.releaseNotice == nil {
		return apperror.NewMissingContext()
	}
	on := c.FormValue("update_check") != ""
	if err := h.releaseNotice.SetCheckOn(c.Request().Context(), on); err != nil {
		return err
	}
	label := "off"
	if on {
		label = "on"
	}
	h.record(c, "updates.check_changed", "setting", keyUpdateCheck, label)
	slog.Info("new-version check changed", slog.Bool("on", on))
	c.Response().Header().Set("HX-Redirect", securityTabHref(SecurityTabOverview))
	return c.NoContent(http.StatusOK)
}

// releaseLaterVals is the Later button's form value as hx-vals JSON.
func releaseLaterVals(version string) string {
	b, _ := json.Marshal(map[string]string{"version": version})
	return string(b)
}
