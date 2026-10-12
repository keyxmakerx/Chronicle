package syncapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EncounterAPIService is the encounters feature as the sync API needs it,
// implemented in internal/app so this plugin never imports the encounters
// plugin. Payloads are opaque here and serialised as the service returns
// them.
type EncounterAPIService interface {
	// Search lists encounters, ready ones first, optionally matching q.
	Search(ctx context.Context, campaignID, userID, q string, limit int) (items any, total int, err error)
	// Get returns one encounter with its creature rows.
	Get(ctx context.Context, campaignID, userID, id string) (any, error)
}

// EncounterAPIHandler serves the encounter endpoints the Foundry module
// loads fights from. Every route is for the campaign owner or a co-DM:
// encounters are the DM's plans, and players have no key of their own.
type EncounterAPIHandler struct {
	svc         EncounterAPIService
	campaignSvc campaigns.CampaignService
}

// NewEncounterAPIHandler creates the encounter API handler.
func NewEncounterAPIHandler(svc EncounterAPIService, campaignSvc campaigns.CampaignService) *EncounterAPIHandler {
	return &EncounterAPIHandler{svc: svc, campaignSvc: campaignSvc}
}

// WithEncounters mounts the encounter endpoints.
func WithEncounters(h *EncounterAPIHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.encounters = h }
}

// dmUser returns the caller when they are the campaign owner or a co-DM, and
// fails closed on any lookup error.
func (h *EncounterAPIHandler) dmUser(c echo.Context) (string, error) {
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
	return "", apperror.NewForbidden("encounters over the API need owner or co-DM access")
}

// List returns the campaign's encounters, ready ones first.
// GET /api/v1/campaigns/:id/encounters[?q=&limit=]
// Response: {"data": [...], "total": N}
func (h *EncounterAPIHandler) List(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	items, total, err := h.svc.Search(c.Request().Context(), c.Param("id"), uid, c.QueryParam("q"), limit)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": items, "total": total})
}

// Get returns one encounter.
// GET /api/v1/campaigns/:id/encounters/:encounterID
func (h *EncounterAPIHandler) Get(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), c.Param("id"), uid, c.Param("encounterID"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
