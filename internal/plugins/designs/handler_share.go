package designs

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// ShareState handles GET /campaigns/:id/designs/:did/share.
func (h *Handler) ShareState(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	st, err := h.svc.ShareState(c.Request().Context(), cid, uid, c.Param("did"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, st)
}

// SetLink handles PUT /campaigns/:id/designs/:did/share/link with {"on": bool}.
func (h *Handler) SetLink(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req struct {
		On bool `json:"on"`
	}
	if err := bind(c, &req); err != nil {
		return err
	}
	st, err := h.svc.SetLink(c.Request().Context(), cid, uid, c.Param("did"), req.On)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, st)
}

// SearchOwners handles GET /campaigns/:id/designs/owners?q=.
func (h *Handler) SearchOwners(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	out, err := h.svc.SearchOwners(c.Request().Context(), cid, uid, c.QueryParam("q"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"owners": out})
}

// AddOwner handles POST /campaigns/:id/designs/:did/share/owners.
func (h *Handler) AddOwner(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req struct {
		UserID string `json:"userId"`
	}
	if err := bind(c, &req); err != nil {
		return err
	}
	st, err := h.svc.AddOwner(c.Request().Context(), cid, uid, c.Param("did"), req.UserID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, st)
}

// RemoveOwner handles DELETE /campaigns/:id/designs/:did/share/owners/:uid.
func (h *Handler) RemoveOwner(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	st, err := h.svc.RemoveOwner(c.Request().Context(), cid, uid, c.Param("did"), c.Param("uid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, st)
}

// StopSharing handles DELETE /campaigns/:id/designs/:did/share.
func (h *Handler) StopSharing(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	st, err := h.svc.StopSharing(c.Request().Context(), cid, uid, c.Param("did"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, st)
}
