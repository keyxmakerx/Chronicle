package syncapi

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DMScreenProvider serves the DM Screen to the Foundry module. It returns
// the screen as an opaque JSON-ready value so this plugin need not import
// the dmscreen plugin; the adapter in internal/app passes the same View the
// site panel draws. The provider enforces who may see and change what.
type DMScreenProvider interface {
	Screen(ctx context.Context, campaignID, userID string, role int) (any, error)
	Reveal(ctx context.Context, entityID, campaignID, userID string, role int) (string, error)
	SetDowntime(ctx context.Context, campaignID, userID string, role int, open bool) (any, error)
}

// SetDMScreen wires the DM Screen. Until it is set the routes answer 404,
// which the module reads as "this Chronicle has no DM Screen".
func (h *APIHandler) SetDMScreen(p DMScreenProvider) {
	h.dmScreen = p
}

func (h *APIHandler) dmScreenCaller(c echo.Context) (userID string, role int, err error) {
	if h.dmScreen == nil {
		return "", 0, apperror.NewNotFound("this Chronicle has no DM Screen")
	}
	key := GetAPIKey(c)
	if key == nil {
		return "", 0, apperror.NewUnauthorized("authentication required")
	}
	return key.UserID, h.resolveRole(c), nil
}

// GetDMScreen handles GET /api/v1/campaigns/:id/dm-screen.
func (h *APIHandler) GetDMScreen(c echo.Context) error {
	userID, role, err := h.dmScreenCaller(c)
	if err != nil {
		return err
	}
	view, err := h.dmScreen.Screen(c.Request().Context(), c.Param("id"), userID, role)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, view)
}

// RevealDMScreenCharacter handles POST
// /api/v1/campaigns/:id/dm-screen/reveal/:entityID: it makes one hidden
// character from the screen's list visible to players.
func (h *APIHandler) RevealDMScreenCharacter(c echo.Context) error {
	userID, role, err := h.dmScreenCaller(c)
	if err != nil {
		return err
	}
	entityID := c.Param("entityID")
	name, err := h.dmScreen.Reveal(c.Request().Context(), entityID, c.Param("id"), userID, role)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"id": entityID, "name": name, "revealed": true})
}

type dmScreenDowntimeRequest struct {
	Open *bool `json:"open"`
}

// SetDMScreenDowntime handles POST /api/v1/campaigns/:id/dm-screen/downtime.
// The body must say {"open": true|false}; a missing field is refused rather
// than read as "close".
func (h *APIHandler) SetDMScreenDowntime(c echo.Context) error {
	userID, role, err := h.dmScreenCaller(c)
	if err != nil {
		return err
	}
	var req dmScreenDowntimeRequest
	if err := c.Bind(&req); err != nil || req.Open == nil {
		return apperror.NewBadRequest(`body must be {"open": true} or {"open": false}`)
	}
	res, err := h.dmScreen.SetDowntime(c.Request().Context(), c.Param("id"), userID, role, *req.Open)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}
