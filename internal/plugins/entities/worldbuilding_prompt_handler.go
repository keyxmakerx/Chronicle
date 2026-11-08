package entities

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// WorldbuildingPromptHandler handles HTTP requests for worldbuilding prompts.
type WorldbuildingPromptHandler struct {
	service WorldbuildingPromptService
}

// NewWorldbuildingPromptHandler creates a new worldbuilding prompt handler.
func NewWorldbuildingPromptHandler(service WorldbuildingPromptService) *WorldbuildingPromptHandler {
	return &WorldbuildingPromptHandler{service: service}
}

// ListAPI returns worldbuilding prompts for a campaign as JSON.
// GET /campaigns/:id/worldbuilding-prompts
// Optional query: ?entity_type_id=N to filter by entity type.
func (h *WorldbuildingPromptHandler) ListAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var prompts []WorldbuildingPrompt
	var err error

	if etID := c.QueryParam("entity_type_id"); etID != "" {
		entityTypeID, parseErr := strconv.Atoi(etID)
		if parseErr != nil {
			return apperror.NewBadRequest("invalid entity_type_id")
		}
		prompts, err = h.service.ListForType(c.Request().Context(), cc.Campaign.ID, entityTypeID)
	} else {
		prompts, err = h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	}
	if err != nil {
		return err
	}

	if prompts == nil {
		prompts = []WorldbuildingPrompt{}
	}

	// HTMX request: return HTML fragment for the prompts panel.
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, WorldbuildingPromptsFragment(prompts))
	}
	return c.JSON(http.StatusOK, prompts)
}

// CreateAPI creates a new worldbuilding prompt.
// POST /campaigns/:id/worldbuilding-prompts
func (h *WorldbuildingPromptHandler) CreateAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var input CreatePromptInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	p, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, p)
}

// UpdateAPI updates an existing worldbuilding prompt.
// PUT /campaigns/:id/worldbuilding-prompts/:pid
func (h *WorldbuildingPromptHandler) UpdateAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		return apperror.NewBadRequest("invalid prompt ID")
	}

	// IDOR: verify prompt belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), pid)
	if err != nil {
		return err
	}
	if existing.CampaignID == nil || *existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("worldbuilding prompt not found")
	}
	if existing.IsGlobal {
		return apperror.NewForbidden("cannot edit global prompts")
	}

	var input UpdatePromptInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.Update(c.Request().Context(), pid, input); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteAPI deletes a worldbuilding prompt.
// DELETE /campaigns/:id/worldbuilding-prompts/:pid
func (h *WorldbuildingPromptHandler) DeleteAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		return apperror.NewBadRequest("invalid prompt ID")
	}

	// IDOR: verify prompt belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), pid)
	if err != nil {
		return err
	}
	if existing.CampaignID == nil || *existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("worldbuilding prompt not found")
	}
	if existing.IsGlobal {
		return apperror.NewForbidden("cannot delete global prompts")
	}

	if err := h.service.Delete(c.Request().Context(), pid); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
