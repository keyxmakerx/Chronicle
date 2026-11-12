package designs

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxBodyBytes bounds every request body: a design is capped at 16 KB and a
// share code is its base64, so 64 KB leaves room for JSON around either.
const maxBodyBytes = 64 * 1024

// Handler is the JSON surface for the library. It is thin on purpose: it binds,
// calls the service and shapes the answer; every rule lives in the service.
type Handler struct{ svc Service }

// NewHandler creates the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// ctxOf reads the route's campaign and the signed-in user.
func ctxOf(c *echo.Context) (campaignID, userID string, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", "", apperror.NewMissingContext()
	}
	return cc.Campaign.ID, auth.GetUserID(c), nil
}

// bind reads a JSON body, refusing one past the cap or with unknown shape.
func bind(c *echo.Context, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes {
		return apperror.NewBadRequest("the request body is missing or too large")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperror.NewBadRequest("the request body is not valid JSON of the expected shape")
	}
	return nil
}

// List handles GET /campaigns/:id/designs?kind=.
func (h *Handler) List(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	out, err := h.svc.List(c.Request().Context(), cid, uid, c.QueryParam("kind"))
	if err != nil {
		return err
	}
	// The campaign's game system rides along so a preview can say "this
	// campaign uses X" without a second request.
	return c.JSON(http.StatusOK, map[string]any{"designs": out, "systemId": campaigns.GetCampaignContext(c).Campaign.ParseSettings().SystemID})
}

// Get handles GET /campaigns/:id/designs/:did.
func (h *Handler) Get(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	d, err := h.svc.Get(c.Request().Context(), cid, uid, c.Param("did"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, d)
}

// Save handles POST /campaigns/:id/designs.
func (h *Handler) Save(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req struct {
		Kind string          `json:"kind"`
		Name string          `json:"name"`
		Note string          `json:"note"`
		Data json.RawMessage `json:"data"`
	}
	if err := bind(c, &req); err != nil {
		return err
	}
	d, err := h.svc.Save(c.Request().Context(), cid, uid, SaveInput{Kind: req.Kind, Name: req.Name, Note: req.Note, Data: req.Data})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// Update handles PATCH /campaigns/:id/designs/:did. A partial update: only the
// keys sent change.
func (h *Handler) Update(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var in UpdateDesignInput
	if err := bind(c, &in); err != nil {
		return err
	}
	d, err := h.svc.Update(c.Request().Context(), cid, uid, c.Param("did"), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, d)
}

// Delete handles DELETE /campaigns/:id/designs/:did.
func (h *Handler) Delete(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, uid, c.Param("did")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
