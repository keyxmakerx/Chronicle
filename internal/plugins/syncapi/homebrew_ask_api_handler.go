package syncapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// HomebrewAskAPIService is the Homebrew Workshop's "Ask the Director" step as
// the sync API needs it, implemented in internal/app so this plugin never
// imports the homebrew plugin. Items are opaque here and serialised as the
// service returns them.
type HomebrewAskAPIService interface {
	// Waiting lists the asks waiting for a Director, oldest first.
	Waiting(ctx context.Context, campaignID, userID string) (items any, total int, err error)
	// Answer answers one; body is the answer as the drawer sends it.
	Answer(ctx context.Context, campaignID, userID, runID string, body json.RawMessage) (any, error)
}

// HomebrewAskAPIHandler serves the asks the Foundry module's GM client shows
// as a GM card. Only the campaign owner or a co-DM may call it: answering an
// ask is the Director's act, and the waiting list names who asked for what.
type HomebrewAskAPIHandler struct {
	svc         HomebrewAskAPIService
	campaignSvc campaigns.CampaignService
}

// NewHomebrewAskAPIHandler creates the handler.
func NewHomebrewAskAPIHandler(svc HomebrewAskAPIService, campaignSvc campaigns.CampaignService) *HomebrewAskAPIHandler {
	return &HomebrewAskAPIHandler{svc: svc, campaignSvc: campaignSvc}
}

// WithHomebrewAsks mounts the ask endpoints.
func WithHomebrewAsks(h *HomebrewAskAPIHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.homebrewAsks = h }
}

// maxAnswerBytes bounds an answer body: a few changed steps' args.
const maxAnswerBytes = 64 << 10

// dmUser returns the caller when they are the campaign owner or a co-DM, and
// fails closed on any lookup error.
func (h *HomebrewAskAPIHandler) dmUser(c *echo.Context) (string, error) {
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
	return "", apperror.NewForbidden("asks over the API need owner or co-DM access")
}

// Waiting lists the asks waiting for a Director.
// GET /api/v1/campaigns/:id/homebrew/asks
// Response: {"data": [run views with "ask"], "total": N}
func (h *HomebrewAskAPIHandler) Waiting(c *echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	items, total, err := h.svc.Waiting(c.Request().Context(), c.Param("id"), uid)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": items, "total": total})
}

// Answer answers one ask.
// POST /api/v1/campaigns/:id/homebrew/asks/:runID/answer
// Body: {"answer": "yes"|"no"|"other", "reply"?: "...", "steps"?: [{"step", "part"?, "args"?}]}
// Response: the run as it now stands; 409 when it was already answered.
func (h *HomebrewAskAPIHandler) Answer(c *echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxAnswerBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxAnswerBytes || !json.Valid(raw) {
		return apperror.NewBadRequest("send the answer as JSON")
	}
	out, err := h.svc.Answer(c.Request().Context(), c.Param("id"), uid, c.Param("runID"), raw)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
