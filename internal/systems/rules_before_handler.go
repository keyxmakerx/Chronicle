package systems

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// BeforeYouPlay is "Before you play in <campaign>": what the campaign wrote,
// rewrote and left out, as the reader may see it.
// GET /campaigns/:id/systems/:mod/before-you-play
func (h *SystemHandler) BeforeYouPlay(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	v, err := h.rulesPages().Before(c.Request().Context(), mod, rulesReader(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, BeforePageContent(v))
	}
	return middleware.Render(c, http.StatusOK, BeforePage(cc, v))
}
