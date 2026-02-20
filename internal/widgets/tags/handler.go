package tags

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles HTTP requests for tag operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service TagService
}

// NewHandler creates a new tag handler backed by the given service.
func NewHandler(service TagService) *Handler {
	return &Handler{service: service}
}

// ListTags returns all tags for a campaign as JSON (GET /campaigns/:id/tags).
func (h *Handler) ListTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	tags, err := h.service.ListByCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Return empty array instead of null when no tags exist.
	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}

// CreateTag creates a new tag in the campaign (POST /campaigns/:id/tags).
func (h *Handler) CreateTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	var req CreateTagRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	tag, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, req.Name, req.Color)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, tag)
}

// UpdateTag updates an existing tag (PUT /campaigns/:id/tags/:tagId).
func (h *Handler) UpdateTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify the tag belongs to this campaign before updating.
	existing, err := h.service.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("tag not found")
	}

	var req UpdateTagRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	tag, err := h.service.Update(c.Request().Context(), tagID, req.Name, req.Color)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, tag)
}

// DeleteTag removes a tag from the campaign (DELETE /campaigns/:id/tags/:tagId).
func (h *Handler) DeleteTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify the tag belongs to this campaign before deleting.
	existing, err := h.service.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("tag not found")
	}

	if err := h.service.Delete(c.Request().Context(), tagID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetEntityTags replaces all tags on an entity with the provided set of tag
// IDs (PUT /campaigns/:id/entities/:eid/tags). Accepts a JSON body with a
// "tagIds" array. This is an idempotent replacement operation: the entity
// will have exactly the tags specified, with old ones removed and new ones added.
func (h *Handler) SetEntityTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	var req SetEntityTagsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.SetEntityTags(c.Request().Context(), entityID, cc.Campaign.ID, req.TagIDs); err != nil {
		return err
	}

	// Return the updated set of tags for the entity.
	tags, err := h.service.GetEntityTags(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}

// GetEntityTags returns all tags for an entity as JSON
// (GET /campaigns/:id/entities/:eid/tags).
func (h *Handler) GetEntityTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	tags, err := h.service.GetEntityTags(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}
