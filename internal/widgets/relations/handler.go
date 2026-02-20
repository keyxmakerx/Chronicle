package relations

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
		return apperror.NewInternal(nil)
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	relations, err := h.service.ListByEntity(c.Request().Context(), entityID)
	if err != nil {
		return err
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
		return apperror.NewInternal(nil)
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

	rel, err := h.service.Create(
		c.Request().Context(),
		cc.Campaign.ID,
		sourceEntityID,
		req.TargetEntityID,
		req.RelationType,
		req.ReverseRelationType,
		userID,
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
		return apperror.NewInternal(nil)
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

// GetCommonTypes returns the predefined relation type pairs for the frontend
// UI suggestion list (GET /campaigns/:id/relation-types).
func (h *Handler) GetCommonTypes(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.GetCommonTypes())
}
