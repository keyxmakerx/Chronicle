package syncapi

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CutsceneAPIService is the cut scenes feature as the sync API needs it,
// implemented in internal/app so this plugin never imports the cutscenes
// plugin. Items are opaque here and serialised as the service returns them.
type CutsceneAPIService interface {
	// MemberShowings lists one member's showings still inside their rewatch
	// window, newest first, each with the scene as it was shown.
	MemberShowings(ctx context.Context, campaignID, userID string) (items any, total int, err error)
}

// CutsceneAPIHandler serves the cut scene endpoint the Foundry module's GM
// client calls when a player in Foundry asks to watch a scene again. Only the
// campaign owner or a co-DM may call it: it names whose showings to list, so
// a player's key could otherwise read someone else's.
type CutsceneAPIHandler struct {
	svc         CutsceneAPIService
	campaignSvc campaigns.CampaignService
	addonSlug   string
}

// NewCutsceneAPIHandler creates the handler; addonSlug is the Cut scenes
// add-on, which gates the route.
func NewCutsceneAPIHandler(svc CutsceneAPIService, campaignSvc campaigns.CampaignService, addonSlug string) *CutsceneAPIHandler {
	return &CutsceneAPIHandler{svc: svc, campaignSvc: campaignSvc, addonSlug: addonSlug}
}

// WithCutscenes mounts the cut scene endpoint.
func WithCutscenes(h *CutsceneAPIHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.cutscenes = h }
}

// dmUser returns the caller when they are the campaign owner or a co-DM, and
// fails closed on any lookup error.
func (h *CutsceneAPIHandler) dmUser(c *echo.Context) (string, error) {
	key := GetAPIKey(c)
	if key == nil {
		return "", apperror.NewUnauthorized("api key required")
	}
	ctx := c.Request().Context()
	if member, err := h.campaignSvc.GetMember(ctx, key.CampaignID, key.UserID); err == nil && member.Role >= campaigns.RoleOwner {
		return key.UserID, nil
	}
	if granted, err := h.campaignSvc.IsUserDmGranted(ctx, key.CampaignID, key.UserID); err == nil && granted {
		return key.UserID, nil
	}
	return "", apperror.NewForbidden("cut scenes over the API need owner or co-DM access")
}

// Showings lists a member's showings still inside their window.
// GET /api/v1/campaigns/:id/cutscenes/showings?user=<chronicleUserId>
// Response: {"data": [{showingId, name, shownAt, rewatchUntil, scene}], "total": N}
func (h *CutsceneAPIHandler) Showings(c *echo.Context) error {
	if _, err := h.dmUser(c); err != nil {
		return err
	}
	items, total, err := h.svc.MemberShowings(c.Request().Context(), c.Param("id"), c.QueryParam("user"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": items, "total": total})
}
