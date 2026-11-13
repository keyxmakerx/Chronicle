package syncapi

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// LayersReader serves a page's transformation layers to the Foundry module,
// which turns each one into an Active Effect on the hero's actor. It returns
// the same `{layers, canEdit}` value the sheet reads, as an opaque value so
// this plugin need not import the transformations plugin; the reader decides
// who may see the page.
type LayersReader interface {
	PageLayers(ctx context.Context, campaignID, entityID, userID string, role int) (any, error)
}

// SetLayersReader wires the layers. Until it is set the route answers 404,
// which the module reads as "no layers".
func (h *APIHandler) SetLayersReader(r LayersReader) {
	h.layers = r
}

// GetEntityLayers handles GET /api/v1/campaigns/:id/entities/:entityID/layers.
func (h *APIHandler) GetEntityLayers(c *echo.Context) error {
	if h.layers == nil {
		return apperror.NewNotFound("this Chronicle has no transformations")
	}
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("authentication required")
	}
	out, err := h.layers.PageLayers(c.Request().Context(), c.Param("id"), c.Param("entityID"), key.UserID, h.resolveRole(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
