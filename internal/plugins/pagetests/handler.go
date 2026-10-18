// The browser HTTP surface for tests. Handlers bind, call the service and
// shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package pagetests

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the test routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// ViewerOf is who is asking, from the campaign context. VisibilityRole is
// used as on the page itself, so a granted co-DM is the DM here.
func ViewerOf(cc *campaigns.CampaignContext, userID string) Viewer {
	return Viewer{UserID: userID, VisibilityRole: cc.VisibilityRole()}
}

func viewerFrom(c echo.Context) (campaignID string, v Viewer, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, ViewerOf(cc, auth.GetUserID(c)), nil
}

func bind(c echo.Context, into any) error {
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// List handles GET /campaigns/:id/pages/:eid/tests.
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

// Create handles POST /campaigns/:id/pages/:eid/tests.
func (h *Handler) Create(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in SaveInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), cid, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Update handles PUT /campaigns/:id/tests/:tid.
func (h *Handler) Update(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in SaveInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), cid, c.Param("tid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Tally handles POST /campaigns/:id/tests/:tid/tally.
func (h *Handler) Tally(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in TallyInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Tally(c.Request().Context(), cid, c.Param("tid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Record handles POST /campaigns/:id/tests/:tid/record.
func (h *Handler) Record(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RecordInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Record(c.Request().Context(), cid, c.Param("tid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Delete handles DELETE /campaigns/:id/tests/:tid.
func (h *Handler) Delete(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, c.Param("tid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Choices handles GET /campaigns/:id/tests: every test of the campaign, for
// the encounter builder's version picks.
func (h *Handler) Choices(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Choices(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
