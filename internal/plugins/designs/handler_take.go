package designs

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

type codeRequest struct {
	Code string `json:"code"`
}

// PreviewCode handles POST /campaigns/:id/designs/code/preview.
func (h *Handler) PreviewCode(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req codeRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	p, err := h.svc.PreviewCode(c.Request().Context(), cid, uid, req.Code)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, p)
}

// TakeCode handles POST /campaigns/:id/designs/code/take.
func (h *Handler) TakeCode(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req codeRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	d, err := h.svc.TakeCode(c.Request().Context(), cid, uid, req.Code)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// SharedWithMe handles GET /campaigns/:id/designs/shared-with-me?kind=.
func (h *Handler) SharedWithMe(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	out, err := h.svc.SharedWithMe(c.Request().Context(), cid, uid, c.QueryParam("kind"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"designs": out})
}

// PreviewShared handles GET /campaigns/:id/designs/shared/:did/preview.
func (h *Handler) PreviewShared(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	p, err := h.svc.PreviewShared(c.Request().Context(), cid, uid, c.Param("did"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, p)
}

// TakeShared handles POST /campaigns/:id/designs/shared/:did/take.
func (h *Handler) TakeShared(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	d, err := h.svc.TakeShared(c.Request().Context(), cid, uid, c.Param("did"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// Link handles GET /designs/link/:token. A browser gets the page, which loads
// the preview from this same route as JSON; the page itself reveals nothing.
func (h *Handler) Link(c *echo.Context) error {
	if wantsPage(c) {
		return middleware.Render(c, http.StatusOK, LinkPage(c.Param("token")))
	}
	p, err := h.svc.PreviewLink(c.Request().Context(), auth.GetUserID(c), c.Param("token"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, p)
}

// TakeLink handles POST /designs/link/:token/take with {"campaignId": "..."}.
func (h *Handler) TakeLink(c *echo.Context) error {
	var req struct {
		CampaignID string `json:"campaignId"`
	}
	if err := bind(c, &req); err != nil {
		return err
	}
	d, err := h.svc.TakeLink(c.Request().Context(), auth.GetUserID(c), c.Param("token"), req.CampaignID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// wantsPage is true for a browser navigation, which asks for HTML first and
// does not ask for JSON.
func wantsPage(c *echo.Context) bool {
	a := c.Request().Header.Get("Accept")
	return strings.Contains(a, "text/html") && !strings.Contains(a, "application/json")
}
