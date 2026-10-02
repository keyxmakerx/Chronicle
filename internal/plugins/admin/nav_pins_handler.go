package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// SetNavPinService wires the admin's own sidebar pins.
func (h *Handler) SetNavPinService(svc AdminNavPinService) {
	h.navPins = svc
}

// adminNavPinsRequest is the body of PUT /admin/nav/pins. It carries the whole
// ordered list on purpose, as a campaign member's pins do; it is not a partial
// update of a stored record.
type adminNavPinsRequest struct {
	Pins []string `json:"pins"`
}

// UpdateNavPins replaces the signed-in admin's pinned pages with the full
// ordered list in the body ({"pins": ["/admin/users"]}) and answers with the
// re-rendered admin nav block, so the page can swap it in. The client reads
// the current list from the page before it sends.
func (h *Handler) UpdateNavPins(c echo.Context) error {
	if h.navPins == nil {
		return apperror.NewInternal(fmt.Errorf("admin nav pins are not wired"))
	}
	var req adminNavPinsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	if _, err := h.navPins.UpdatePins(c.Request().Context(), auth.GetUserID(c), req.Pins); err != nil {
		return err
	}

	// The fragment is drawn for the page the admin is on (the browser sends
	// it as the referrer), so that page's row stays marked current.
	ctx := c.Request().Context()
	if ref, err := url.Parse(c.Request().Referer()); err == nil && strings.HasPrefix(ref.Path, "/admin") {
		ctx = layouts.SetActivePath(ctx, ref.Path)
	}
	c.SetRequest(c.Request().WithContext(ctx))
	return middleware.Render(c, http.StatusOK, layouts.AdminSidebarNav())
}
