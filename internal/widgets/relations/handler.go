package relations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	service    RelationService
	typeLister EntityTypeListerForGraph
	entityGate EntityGate
}

// NewHandler creates a new relation handler backed by the given service.
func NewHandler(service RelationService) *Handler {
	return &Handler{service: service}
}

// SetEntityTypeLister injects the entity type lister for the graph page.
func (h *Handler) SetEntityTypeLister(lister EntityTypeListerForGraph) {
	h.typeLister = lister
}

// SetEntityGate injects the entity-visibility gate. Called during app wiring.
// When set, the public relations list enforces entity privacy + campaign
// binding on the source entity and filters private relation targets.
func (h *Handler) SetEntityGate(gate EntityGate) {
	h.entityGate = gate
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

	// Source-entity gate (mirrors the entity Show page): resolve the entity,
	// require it to belong to the URL campaign (kills the cross-campaign IDOR),
	// and require the caller's view access (anon = RoleNone). Without this an
	// anonymous visitor could read the relation list — including private target
	// names/slugs — of a private entity, or of any entity in another campaign.
	if h.entityGate == nil {
		// Fail closed: a missing gate must never serve ungated relations.
		return apperror.NewInternal(errors.New("relations: entity gate not configured"))
	}
	userID := auth.GetUserID(c)
	campaignID, canView, err := h.entityGate.ResolveViewableEntity(
		c.Request().Context(), entityID, int(cc.MemberRole), userID)
	if err != nil {
		return err // NotFound for a missing entity; propagates real errors.
	}
	if campaignID != cc.Campaign.ID || !canView {
		return apperror.NewNotFound("entity not found")
	}

	relations, err := h.service.ListByEntity(c.Request().Context(), cc.Campaign.ID, entityID)
	if err != nil {
		return err
	}

	// Owner / site-admin / DM-granted viewers keep the full picture (dm_only
	// relations + private targets). Everyone else is filtered.
	privileged := cc.MemberRole == campaigns.RoleOwner || cc.IsSiteAdmin || cc.IsDmGranted
	if !privileged {
		// Drop dm_only relations.
		filtered := make([]Relation, 0, len(relations))
		for _, r := range relations {
			if !r.DmOnly {
				filtered = append(filtered, r)
			}
		}
		relations = filtered

		// Drop relations whose TARGET entity the viewer cannot see — the target
		// name/slug/type are the leak. Batched visibility check (no N+1).
		relations, err = h.filterByTargetVisibility(c.Request().Context(), cc.Campaign.ID, relations, int(cc.MemberRole), userID)
		if err != nil {
			return err
		}
	}

	// Return empty array instead of null when no relations exist.
	if relations == nil {
		relations = []Relation{}
	}

	return c.JSON(http.StatusOK, relations)
}

// filterByTargetVisibility drops relations whose target entity the viewer cannot
// see, using one batched visibility query for all distinct targets (no per-row
// N+1). Callers must have already confirmed h.entityGate is non-nil.
func (h *Handler) filterByTargetVisibility(ctx context.Context, campaignID string, rels []Relation, role int, userID string) ([]Relation, error) {
	if len(rels) == 0 {
		return rels, nil
	}

	// Collect distinct target IDs.
	ids := make([]string, 0, len(rels))
	seen := make(map[string]bool, len(rels))
	for _, r := range rels {
		if !seen[r.TargetEntityID] {
			seen[r.TargetEntityID] = true
			ids = append(ids, r.TargetEntityID)
		}
	}

	viewable, err := h.entityGate.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
	if err != nil {
		return nil, err
	}

	out := make([]Relation, 0, len(rels))
	for _, r := range rels {
		if viewable[r.TargetEntityID] {
			out = append(out, r)
		}
	}
	return out, nil
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
//
// Query parameters:
//   - types:            comma-separated entity type slugs to filter by
//   - search:           filter nodes whose name matches (case-insensitive)
//   - focus:            entity ID for local/ego graph (BFS from this node)
//   - hops:             number of hops from focus entity (default 2)
//   - include_mentions: include @mention edges (default "true")
//   - include_orphans:  include entities with no connections (default "false")
//
// GET /campaigns/:id/relations-graph
func (h *Handler) GraphAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	includeDmOnly := cc.MemberRole == campaigns.RoleOwner || cc.IsSiteAdmin || cc.IsDmGranted

	// Parse filter parameters.
	filter := GraphFilter{
		Search:          c.QueryParam("search"),
		FocusEntityID:   c.QueryParam("focus"),
		IncludeMentions: c.QueryParam("include_mentions") != "false", // default true
		IncludeOrphans:  c.QueryParam("include_orphans") == "true",
	}

	if typesParam := c.QueryParam("types"); typesParam != "" {
		for _, t := range strings.Split(typesParam, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				filter.Types = append(filter.Types, t)
			}
		}
	}

	if hopsParam := c.QueryParam("hops"); hopsParam != "" {
		if n, err := strconv.Atoi(hopsParam); err == nil && n > 0 && n <= 10 {
			filter.Hops = n
		}
	}
	if filter.Hops == 0 {
		filter.Hops = 2
	}

	// Determine user ID + role for visibility filtering. viewerRole drives the
	// per-viewer entity-visibility filter that hides private-entity nodes from
	// non-privileged viewers (includeDmOnly viewers keep the full picture).
	userID := auth.GetUserID(c)

	data, err := h.service.GetFilteredGraphData(c.Request().Context(), cc.Campaign.ID, filter, includeDmOnly, int(cc.MemberRole), userID)
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

	// Fetch entity types for the filter dropdown.
	var entityTypes []EntityTypeSummary
	if h.typeLister != nil {
		var err error
		entityTypes, err = h.typeLister.ListEntityTypesForGraph(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			return apperror.NewInternal(err)
		}
	}

	return middleware.Render(c, http.StatusOK, GraphPage(cc, entityTypes))
}
