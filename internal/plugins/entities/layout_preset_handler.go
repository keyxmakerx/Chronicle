package entities

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// LayoutPresetHandler handles HTTP requests for layout presets.
type LayoutPresetHandler struct {
	service LayoutPresetService
}

// NewLayoutPresetHandler creates a new layout preset handler.
func NewLayoutPresetHandler(service LayoutPresetService) *LayoutPresetHandler {
	return &LayoutPresetHandler{service: service}
}

// ListAPI returns layout presets for a campaign as JSON.
// GET /campaigns/:id/layout-presets
func (h *LayoutPresetHandler) ListAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	presets, err := h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	if presets == nil {
		presets = []LayoutPreset{}
	}
	return c.JSON(http.StatusOK, presets)
}

// GetAPI returns a single layout preset by ID.
// GET /campaigns/:id/layout-presets/:pid
func (h *LayoutPresetHandler) GetAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		return apperror.NewBadRequest("invalid preset ID")
	}

	p, err := h.service.GetByID(c.Request().Context(), pid)
	if err != nil {
		return err
	}

	// IDOR check: preset must belong to this campaign.
	if p.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("layout preset not found")
	}

	return c.JSON(http.StatusOK, p)
}

// CreateAPI creates a new layout preset.
// POST /campaigns/:id/layout-presets
func (h *LayoutPresetHandler) CreateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		LayoutJSON  string `json:"layout_json"`
		Icon        string `json:"icon"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	input := CreateLayoutPresetInput{
		CampaignID:  cc.Campaign.ID,
		Name:        body.Name,
		Description: body.Description,
		LayoutJSON:  body.LayoutJSON,
		Icon:        body.Icon,
	}

	p, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, p)
}

// UpdateAPI updates an existing layout preset.
// PUT /campaigns/:id/layout-presets/:pid
func (h *LayoutPresetHandler) UpdateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		return apperror.NewBadRequest("invalid preset ID")
	}

	// IDOR: verify preset belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), pid)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("layout preset not found")
	}
	if existing.IsBuiltin {
		return apperror.NewForbidden("cannot edit built-in presets")
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		LayoutJSON  string `json:"layout_json"`
		Icon        string `json:"icon"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	input := UpdateLayoutPresetInput{
		Name:        body.Name,
		Description: body.Description,
		LayoutJSON:  body.LayoutJSON,
		Icon:        body.Icon,
	}

	p, err := h.service.Update(c.Request().Context(), pid, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, p)
}

// DeleteAPI deletes a layout preset.
// DELETE /campaigns/:id/layout-presets/:pid
func (h *LayoutPresetHandler) DeleteAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		return apperror.NewBadRequest("invalid preset ID")
	}

	// IDOR: verify preset belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), pid)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("layout preset not found")
	}
	if existing.IsBuiltin {
		return apperror.NewForbidden("cannot delete built-in presets")
	}

	if err := h.service.Delete(c.Request().Context(), pid); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		return c.NoContent(http.StatusOK)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
