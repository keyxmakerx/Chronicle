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

// ContentTemplateHandler handles HTTP requests for content templates.
type ContentTemplateHandler struct {
	service ContentTemplateService
}

// NewContentTemplateHandler creates a new content template handler.
func NewContentTemplateHandler(service ContentTemplateService) *ContentTemplateHandler {
	return &ContentTemplateHandler{service: service}
}

// ListAPI returns content templates for a campaign as JSON.
// GET /campaigns/:id/content-templates
// Optional query: ?entity_type_id=N to filter by entity type.
func (h *ContentTemplateHandler) ListAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var templates []ContentTemplate
	var err error

	if etID := c.QueryParam("entity_type_id"); etID != "" {
		entityTypeID, parseErr := strconv.Atoi(etID)
		if parseErr != nil {
			return apperror.NewBadRequest("invalid entity_type_id")
		}
		templates, err = h.service.ListForCampaignAndType(c.Request().Context(), cc.Campaign.ID, entityTypeID)
	} else {
		templates, err = h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	}
	if err != nil {
		return err
	}

	if templates == nil {
		templates = []ContentTemplate{}
	}
	return c.JSON(http.StatusOK, templates)
}

// GetAPI returns a single content template by ID.
// GET /campaigns/:id/content-templates/:tid
func (h *ContentTemplateHandler) GetAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tid, err := strconv.Atoi(c.Param("tid"))
	if err != nil {
		return apperror.NewBadRequest("invalid template ID")
	}

	t, err := h.service.GetByID(c.Request().Context(), tid)
	if err != nil {
		return err
	}

	// IDOR check: template must belong to this campaign or be global.
	if t.CampaignID != nil && *t.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("content template not found")
	}

	return c.JSON(http.StatusOK, t)
}

// CreateAPI creates a new content template.
// POST /campaigns/:id/content-templates
func (h *ContentTemplateHandler) CreateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var body struct {
		EntityTypeID int    `json:"entity_type_id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		ContentJSON  string `json:"content_json"`
		ContentHTML  string `json:"content_html"`
		Icon         string `json:"icon"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	input := CreateContentTemplateInput{
		CampaignID:   cc.Campaign.ID,
		EntityTypeID: body.EntityTypeID,
		Name:         body.Name,
		Description:  body.Description,
		ContentJSON:  body.ContentJSON,
		ContentHTML:  body.ContentHTML,
		Icon:         body.Icon,
	}

	t, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, t)
}

// UpdateAPI updates an existing content template.
// PUT /campaigns/:id/content-templates/:tid
func (h *ContentTemplateHandler) UpdateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tid, err := strconv.Atoi(c.Param("tid"))
	if err != nil {
		return apperror.NewBadRequest("invalid template ID")
	}

	// IDOR: verify template belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), tid)
	if err != nil {
		return err
	}
	if existing.CampaignID == nil || *existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("content template not found")
	}
	if existing.IsGlobal {
		return apperror.NewForbidden("cannot edit global templates")
	}

	var input UpdateContentTemplateInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	t, err := h.service.Update(c.Request().Context(), tid, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, t)
}

// DeleteAPI deletes a content template.
// DELETE /campaigns/:id/content-templates/:tid
func (h *ContentTemplateHandler) DeleteAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tid, err := strconv.Atoi(c.Param("tid"))
	if err != nil {
		return apperror.NewBadRequest("invalid template ID")
	}

	// IDOR: verify template belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), tid)
	if err != nil {
		return err
	}
	if existing.CampaignID == nil || *existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("content template not found")
	}
	if existing.IsGlobal {
		return apperror.NewForbidden("cannot delete global templates")
	}

	if err := h.service.Delete(c.Request().Context(), tid); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		return c.NoContent(http.StatusOK)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
