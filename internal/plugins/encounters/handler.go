// The browser HTTP surface for encounters. Handlers bind, call the service
// and shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package encounters

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the encounter routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c echo.Context) (campaignID string, v Viewer, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{UserID: auth.GetUserID(c), VisibilityRole: cc.VisibilityRole()}, nil
}

func bindSave(c echo.Context) (SaveInput, error) {
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return SaveInput{}, apperror.NewBadRequest("request body is missing or too large")
	}
	var in SaveInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return SaveInput{}, apperror.NewBadRequest("request body is not valid JSON")
	}
	return in, nil
}

// List handles GET /campaigns/:id/pages/:eid/encounters.
func (h *Handler) List(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ListForPage(c.Request().Context(), cid, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Create handles POST /campaigns/:id/pages/:eid/encounters.
func (h *Handler) Create(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	in, err := bindSave(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), cid, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Get handles GET /campaigns/:id/encounters/:encid.
func (h *Handler) Get(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), cid, c.Param("encid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Update handles PUT /campaigns/:id/encounters/:encid.
func (h *Handler) Update(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	in, err := bindSave(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), cid, c.Param("encid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Delete handles DELETE /campaigns/:id/encounters/:encid.
func (h *Handler) Delete(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, c.Param("encid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
