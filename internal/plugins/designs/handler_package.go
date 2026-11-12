package designs

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Package handles GET /campaigns/:id/designs/package?kind=: the read-only
// group the campaign's game system package ships.
func (h *Handler) Package(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	g, err := h.svc.Package(c.Request().Context(), cid, uid, c.QueryParam("kind"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, g)
}

// CopyPackage handles POST /campaigns/:id/designs/package/:pid/copy
// ("Save as my design").
func (h *Handler) CopyPackage(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	d, err := h.svc.CopyPackage(c.Request().Context(), cid, uid, c.Param("pid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// Suggest handles POST /campaigns/:id/designs/:did/suggestion. The body is
// {note, credit}; the answer carries the file's name and content for the page
// to download. POST keeps the maintainer note out of URLs and access logs.
func (h *Handler) Suggest(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var in SuggestInput
	if err := bind(c, &in); err != nil {
		return err
	}
	s, err := h.svc.Suggest(c.Request().Context(), cid, uid, c.Param("did"), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"filename": s.Filename, "suggestion": s})
}
