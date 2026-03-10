package relations

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles HTTP requests for entity relation operations. Handlers are
// thin: bind request, call service, render response. No business logic lives
// here.
type Handler struct {
	service RelationService
}

// NewHandler creates a new relation handler backed by the given service.
func NewHandler(service RelationService) *Handler {
	return &Handler{service: service}
}

// ListRelations returns all relations for an entity as JSON
// (GET /campaigns/:id/entities/:eid/relations).
func (h *Handler) ListRelations(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	relations, err := h.service.ListByEntity(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// Filter dm_only relations for non-DM users.
	if cc.MemberRole != campaigns.RoleOwner && !cc.IsSiteAdmin && !cc.IsDmGranted {
		filtered := make([]Relation, 0, len(relations))
		for _, r := range relations {
			if !r.DmOnly {
				filtered = append(filtered, r)
			}
		}
		relations = filtered
	}

	// Return empty array instead of null when no relations exist.
	if relations == nil {
		relations = []Relation{}
	}

	return c.JSON(http.StatusOK, relations)
}

// CreateRelation creates a new bi-directional relation between two entities
// (POST /campaigns/:id/entities/:eid/relations).
func (h *Handler) CreateRelation(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	sourceEntityID := c.Param("eid")
	if sourceEntityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	var req CreateRelationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if req.TargetEntityID == "" {
		return apperror.NewBadRequest("target entity ID is required")
	}

	userID := auth.GetUserID(c)

	// Only DMs (owners) and site admins can create DM-only relations.
	dmOnly := req.DmOnly
	if dmOnly && cc.MemberRole != campaigns.RoleOwner && !cc.IsSiteAdmin {
		dmOnly = false
	}

	rel, err := h.service.Create(
		c.Request().Context(),
		cc.Campaign.ID,
		sourceEntityID,
		req.TargetEntityID,
		req.RelationType,
		req.ReverseRelationType,
		userID,
		req.Metadata,
		dmOnly,
	)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, rel)
}

// DeleteRelation removes a relation and its reverse direction
// (DELETE /campaigns/:id/entities/:eid/relations/:rid).
func (h *Handler) DeleteRelation(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	relationID, err := strconv.Atoi(c.Param("rid"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	// Verify the relation belongs to this campaign before deleting.
	existing, err := h.service.GetByID(c.Request().Context(), relationID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("relation not found")
	}

	if err := h.service.Delete(c.Request().Context(), relationID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateRelationMetadata updates the metadata JSON for a single relation
// (PUT /campaigns/:id/entities/:eid/relations/:rid/metadata).
func (h *Handler) UpdateRelationMetadata(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	relationID, err := strconv.Atoi(c.Param("rid"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	existing, err := h.service.GetByID(c.Request().Context(), relationID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("relation not found")
	}

	var req UpdateRelationMetadataRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateMetadata(c.Request().Context(), relationID, req.Metadata); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetCommonTypes returns the predefined relation type pairs for the frontend
// UI suggestion list (GET /campaigns/:id/relation-types).
func (h *Handler) GetCommonTypes(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.GetCommonTypes())
}

// GraphAPI returns the relations graph data (nodes + edges) for a campaign
// as JSON. Used by the D3 force-directed graph widget.
// GET /campaigns/:id/relations-graph
func (h *Handler) GraphAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	includeDmOnly := cc.MemberRole == campaigns.RoleOwner || cc.IsSiteAdmin || cc.IsDmGranted
	data, err := h.service.GetGraphData(c.Request().Context(), cc.Campaign.ID, includeDmOnly)
	if err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, data)
}

// GraphPage renders the standalone relations graph visualization page.
// GET /campaigns/:id/relations-graph/page
func (h *Handler) GraphPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	return middleware.Render(c, http.StatusOK, GraphPage(cc))
}
