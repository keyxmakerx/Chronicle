package crafting

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Character handles GET /campaigns/:id/crafting/characters/:eid: the
// Crafting block on a character's page.
func (h *Handler) Character(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Character(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// StartProject handles POST /campaigns/:id/crafting/characters/:eid/projects.
func (h *Handler) StartProject(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in StartInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.StartProject(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// RollProject handles POST /campaigns/:id/crafting/projects/:pid/roll.
func (h *Handler) RollProject(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RollInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.RollProject(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Experiment handles POST /campaigns/:id/crafting/characters/:eid/experiment.
func (h *Handler) Experiment(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in TryInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Experiment(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// BreakDown handles POST /campaigns/:id/crafting/characters/:eid/break-down.
func (h *Handler) BreakDown(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in BreakInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.BreakDown(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Help handles POST /campaigns/:id/crafting/projects/:pid/help.
func (h *Handler) Help(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in HelpInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Help(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// CancelProject handles DELETE /campaigns/:id/crafting/projects/:pid.
func (h *Handler) CancelProject(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.CancelProject(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Projects handles GET /campaigns/:id/crafting/projects: the Director's tab.
func (h *Handler) Projects(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Projects(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// LogRespite handles POST /campaigns/:id/crafting/respites.
func (h *Handler) LogRespite(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.LogRespite(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// GiveRoll handles POST /campaigns/:id/crafting/projects/:pid/rolls.
func (h *Handler) GiveRoll(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GiveRoll(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetPoints handles PUT /campaigns/:id/crafting/projects/:pid/points.
func (h *Handler) SetPoints(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in SetPointsInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SetPoints(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
