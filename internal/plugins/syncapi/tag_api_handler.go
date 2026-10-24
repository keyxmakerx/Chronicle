// Package syncapi — tag_api_handler.go provides REST API v1 endpoints for
// tag CRUD and entity tag assignment. External clients (Foundry VTT) use
// these endpoints to manage campaign tags via API key auth.
package syncapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// TagAPIHandler serves tag-related REST API endpoints for external tools.
type TagAPIHandler struct {
	syncSvc     SyncAPIService
	tagSvc      tags.TagService
	entitySvc   entities.EntityService
	campaignSvc campaigns.CampaignService
}

// NewTagAPIHandler creates a new tag API handler with required dependencies.
func NewTagAPIHandler(syncSvc SyncAPIService, tagSvc tags.TagService, entitySvc entities.EntityService, campaignSvc campaigns.CampaignService) *TagAPIHandler {
	return &TagAPIHandler{
		syncSvc:     syncSvc,
		tagSvc:      tagSvc,
		entitySvc:   entitySvc,
		campaignSvc: campaignSvc,
	}
}

// resolveRole returns the API key owner's role in the campaign for visibility filtering.
func (h *TagAPIHandler) resolveRole(c echo.Context) int {
	key := GetAPIKey(c)
	if key == nil {
		return 0
	}
	member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
	if err != nil {
		return 0
	}
	return int(member.Role)
}

// seesDmOnly reports whether the caller may read and keep dm_only tags: the
// owner and Co-DMs, as on the web. A Scribe sees what players see.
func (h *TagAPIHandler) seesDmOnly(c echo.Context, campaignID string) bool {
	if h.resolveRole(c) >= int(campaigns.RoleOwner) {
		return true
	}
	key := GetAPIKey(c)
	if key == nil {
		return false
	}
	granted, err := h.campaignSvc.IsUserDmGranted(c.Request().Context(), campaignID, key.UserID)
	return err == nil && granted
}

// --- Tag CRUD ---

// ListTags returns all tags for a campaign with DM-only filtering based on role.
// GET /api/v1/campaigns/:id/tags
func (h *TagAPIHandler) ListTags(c echo.Context) error {
	campaignID := c.Param("id")

	// The owner and Co-DMs can see DM-only tags; Scribes and Players cannot.
	includeDmOnly := h.seesDmOnly(c, campaignID)

	tagList, err := h.tagSvc.ListByCampaign(c.Request().Context(), campaignID, includeDmOnly)
	if err != nil {
		slog.Error("api: list tags failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list tags"))
	}

	if tagList == nil {
		tagList = []tags.Tag{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  tagList,
		"total": len(tagList),
	})
}

// apiCreateTagRequest is the JSON body for creating a tag via the API.
type apiCreateTagRequest struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	DmOnly bool   `json:"dm_only"`
}

// CreateTag creates a new tag in the campaign.
// POST /api/v1/campaigns/:id/tags
func (h *TagAPIHandler) CreateTag(c echo.Context) error {
	campaignID := c.Param("id")

	var req apiCreateTagRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	tag, err := h.tagSvc.Create(c.Request().Context(), campaignID, req.Name, req.Color, req.DmOnly)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, tag)
}

// apiUpdateTagRequest is the JSON body for updating a tag via the API.
// PARTIAL update: absent preserves, a present value replaces (ADR-056) —
// see tags.UpdateTagInput. Name, Color and DmOnly must stay pointers: a value
// type would let a {name}-only rename push turn every DM-only tag public, and
// would make a {color}-only push fail the service's non-empty-name check.
type apiUpdateTagRequest struct {
	Name   *string `json:"name"`
	Color  *string `json:"color"`
	DmOnly *bool   `json:"dm_only"`
}

// UpdateTag updates an existing tag.
// PUT /api/v1/campaigns/:id/tags/:tagId
func (h *TagAPIHandler) UpdateTag(c echo.Context) error {
	campaignID := c.Param("id")
	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify tag belongs to this campaign.
	existing, err := h.tagSvc.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return apperror.NewNotFound("tag not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("tag not found")
	}

	var req apiUpdateTagRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// The service requires a name, so an absent one is filled from the stored
	// tag; a present-but-blank one still reaches it and is rejected.
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}

	tag, err := h.tagSvc.Update(c.Request().Context(), tagID, tags.UpdateTagInput{
		Name:   name,
		Color:  req.Color,
		DmOnly: req.DmOnly,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, tag)
}

// DeleteTag removes a tag from the campaign.
// DELETE /api/v1/campaigns/:id/tags/:tagId
func (h *TagAPIHandler) DeleteTag(c echo.Context) error {
	campaignID := c.Param("id")
	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify tag belongs to this campaign.
	existing, err := h.tagSvc.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return apperror.NewNotFound("tag not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("tag not found")
	}

	if err := h.tagSvc.Delete(c.Request().Context(), tagID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Entity Tag Assignment ---

// apiSetEntityTagsRequest is the JSON body for setting entity tags.
type apiSetEntityTagsRequest struct {
	TagIDs []int `json:"tag_ids"`
}

// SetEntityTags replaces all tags on an entity with the given set.
// PUT /api/v1/campaigns/:id/entities/:entityID/tags
func (h *TagAPIHandler) SetEntityTags(c echo.Context) error {
	campaignID := c.Param("id")
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}

	var req apiSetEntityTagsRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	includeDmOnly := h.seesDmOnly(c, campaignID)
	if err := h.tagSvc.SetEntityTagsVisible(ctx, entityID, campaignID, req.TagIDs, includeDmOnly); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Bulk Tag Operations ---

// apiBulkTagRequest is the JSON body for bulk tag assignment.
type apiBulkTagRequest struct {
	EntityIDs []string `json:"entity_ids"`
	TagIDs    []int    `json:"tag_ids"`
	Action    string   `json:"action"` // "add", "remove", or "set"
}

// bulkTagResult describes the outcome of a bulk tag operation.
type bulkTagResult struct {
	EntityID string `json:"entity_id"`
	Status   string `json:"status"` // "ok" or "error"
	Error    string `json:"error,omitempty"`
}

// BulkAssignTags applies tag changes to multiple entities at once.
// POST /api/v1/campaigns/:id/entities/bulk-tags
func (h *TagAPIHandler) BulkAssignTags(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	var req apiBulkTagRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Validate request.
	const maxBulkEntities = 200
	if len(req.EntityIDs) == 0 {
		return apperror.NewBadRequest("entity_ids is required")
	}
	if len(req.EntityIDs) > maxBulkEntities {
		return apperror.NewBadRequest(fmt.Sprintf("too many entities; maximum is %d per request", maxBulkEntities))
	}
	if req.Action != "add" && req.Action != "remove" && req.Action != "set" {
		return apperror.NewBadRequest("action must be 'add', 'remove', or 'set'")
	}

	includeDmOnly := h.seesDmOnly(c, campaignID)

	var results []bulkTagResult
	processed := 0

	for _, entityID := range req.EntityIDs {
		result := bulkTagResult{EntityID: entityID}

		// Verify entity belongs to this campaign.
		entity, err := h.entitySvc.GetByID(ctx, entityID)
		if err != nil || entity.CampaignID != campaignID {
			result.Status = "error"
			result.Error = "entity not found"
			results = append(results, result)
			continue
		}

		switch req.Action {
		case "set":
			// Replace all tags with the given set.
			if err := h.tagSvc.SetEntityTagsVisible(ctx, entityID, campaignID, req.TagIDs, includeDmOnly); err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
			} else {
				result.Status = "ok"
				processed++
			}

		case "add":
			// Get current tags and merge.
			current, err := h.tagSvc.GetEntityTags(ctx, entityID, includeDmOnly)
			if err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
				results = append(results, result)
				continue
			}
			merged := mergeTagIDs(current, req.TagIDs)
			if err := h.tagSvc.SetEntityTagsVisible(ctx, entityID, campaignID, merged, includeDmOnly); err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
			} else {
				result.Status = "ok"
				processed++
			}

		case "remove":
			// Get current tags and subtract.
			current, err := h.tagSvc.GetEntityTags(ctx, entityID, includeDmOnly)
			if err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
				results = append(results, result)
				continue
			}
			filtered := subtractTagIDs(current, req.TagIDs)
			if err := h.tagSvc.SetEntityTagsVisible(ctx, entityID, campaignID, filtered, includeDmOnly); err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
			} else {
				result.Status = "ok"
				processed++
			}
		}

		results = append(results, result)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":    "ok",
		"processed": processed,
		"results":   results,
	})
}

// mergeTagIDs returns current tag IDs plus new ones (no duplicates).
func mergeTagIDs(current []tags.Tag, add []int) []int {
	seen := make(map[int]bool, len(current)+len(add))
	var result []int
	for _, t := range current {
		seen[t.ID] = true
		result = append(result, t.ID)
	}
	for _, id := range add {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// subtractTagIDs returns current tag IDs with the given ones removed.
func subtractTagIDs(current []tags.Tag, remove []int) []int {
	removeSet := make(map[int]bool, len(remove))
	for _, id := range remove {
		removeSet[id] = true
	}
	var result []int
	for _, t := range current {
		if !removeSet[t.ID] {
			result = append(result, t.ID)
		}
	}
	return result
}
