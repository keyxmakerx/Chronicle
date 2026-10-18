// The browser HTTP surface for the Homebrew Workshop. Handlers bind, call the
// service and shape the response; the service owns every rule.
package homebrew

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the Workshop routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c echo.Context) (string, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{UserID: auth.GetUserID(c), Director: cc.CanAuthorDmOnly()}, nil
}

// bind reads a JSON body of at most MaxBodyBytes into dst.
func bind(c echo.Context, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// Catalog handles GET /campaigns/:id/homebrew/catalog.
func (h *Handler) Catalog(c echo.Context) error {
	_, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Catalog(v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// List handles GET /campaigns/:id/homebrew/recipes.
func (h *Handler) List(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.List(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": out, "total": len(out)})
}

// Get handles GET /campaigns/:id/homebrew/recipes/:rid.
func (h *Handler) Get(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), cid, c.Param("rid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Create handles POST /campaigns/:id/homebrew/recipes.
func (h *Handler) Create(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RecipeInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Update handles PUT /campaigns/:id/homebrew/recipes/:rid.
func (h *Handler) Update(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RecipeInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), cid, c.Param("rid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Delete handles DELETE /campaigns/:id/homebrew/recipes/:rid.
func (h *Handler) Delete(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, c.Param("rid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
