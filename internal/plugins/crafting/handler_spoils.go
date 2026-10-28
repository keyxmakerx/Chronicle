package crafting

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// lineID reads a spoil line's id from the path; anything else is a line
// that isn't there.
func lineID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("lid"), 10, 64)
	if err != nil || id < 1 {
		return 0, apperror.NewNotFound("those spoils are gone")
	}
	return id, nil
}

// Spoils handles GET /campaigns/:id/crafting/spoils: the Director's tab.
func (h *Handler) Spoils(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Spoils(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SaveDrops handles PUT /campaigns/:id/crafting/drops/:eid.
func (h *Handler) SaveDrops(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in DropInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveDrops(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteDrops handles DELETE /campaigns/:id/crafting/drops/:eid.
func (h *Handler) DeleteDrops(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeleteDrops(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// PutSpoils handles POST /campaigns/:id/crafting/spoils.
func (h *Handler) PutSpoils(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in PutInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.PutSpoils(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// ClearTray handles DELETE /campaigns/:id/crafting/spoils.
func (h *Handler) ClearTray(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.ClearTray(c.Request().Context(), cc.Campaign.ID, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearSpoil handles DELETE /campaigns/:id/crafting/spoils/:sid.
func (h *Handler) ClearSpoil(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.ClearSpoil(c.Request().Context(), cc.Campaign.ID, c.Param("sid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearSpoilLine handles DELETE /campaigns/:id/crafting/spoil-lines/:lid.
func (h *Handler) ClearSpoilLine(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	id, err := lineID(c)
	if err != nil {
		return err
	}
	if err := h.svc.ClearSpoilLine(c.Request().Context(), cc.Campaign.ID, id, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// TakeSpoil handles POST /campaigns/:id/crafting/spoil-lines/:lid/take:
// taking or harvesting one line for a hero.
func (h *Handler) TakeSpoil(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	id, err := lineID(c)
	if err != nil {
		return err
	}
	var in TakeInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.TakeSpoil(c.Request().Context(), cc.Campaign.ID, id, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
