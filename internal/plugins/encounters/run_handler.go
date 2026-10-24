package encounters

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RunHandler serves the Fight tab's routes.
type RunHandler struct{ svc RunService }

// NewRunHandler builds the run handler.
func NewRunHandler(svc RunService) *RunHandler { return &RunHandler{svc: svc} }

func bindJSON(c echo.Context, limit int64, into any) error {
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, limit+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > limit {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// Current handles GET /campaigns/:id/encounter-run.
func (h *RunHandler) Current(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Current(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Start handles POST /campaigns/:id/encounter-run.
func (h *RunHandler) Start(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RunStart
	if err := bindJSON(c, 1<<10, &in); err != nil {
		return err
	}
	out, err := h.svc.Start(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Save handles PUT /campaigns/:id/encounter-run.
func (h *RunHandler) Save(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RunSave
	if err := bindJSON(c, MaxRunBodyBytes, &in); err != nil {
		return err
	}
	out, err := h.svc.Save(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// End handles DELETE /campaigns/:id/encounter-run.
func (h *RunHandler) End(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.End(c.Request().Context(), cid, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
