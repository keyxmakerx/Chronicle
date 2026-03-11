package syncapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// APIHandler serves the versioned REST API for external tool integration.
// External clients (Foundry VTT, custom scripts) use these endpoints to
// read and write campaign data programmatically via API key authentication.
type APIHandler struct {
	syncSvc     SyncAPIService
	entitySvc   entities.EntityService
	campaignSvc campaigns.CampaignService
	relationSvc relations.RelationService
}

// NewAPIHandler creates a new API handler with the required service dependencies.
func NewAPIHandler(syncSvc SyncAPIService, entitySvc entities.EntityService, campaignSvc campaigns.CampaignService, relationSvc relations.RelationService) *APIHandler {
	return &APIHandler{
		syncSvc:     syncSvc,
		entitySvc:   entitySvc,
		campaignSvc: campaignSvc,
		relationSvc: relationSvc,
	}
}

// resolveRole returns the API key owner's role in the campaign for privacy filtering.
// Falls back to RoleNone if the key owner is no longer a campaign member.
func (h *APIHandler) resolveRole(c echo.Context) int {
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

// resolveUserID returns the API key owner's user ID for permission checks.
func (h *APIHandler) resolveUserID(c echo.Context) string {
	key := GetAPIKey(c)
	if key == nil {
		return ""
	}
	return key.UserID
}

// --- Campaign Info ---

// apiCampaignResponse is the API-safe representation of a campaign.
// Omits internal fields like Settings and SidebarConfig.
type apiCampaignResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetCampaign returns campaign details for the API key's campaign.
// GET /api/v1/campaigns/:id
func (h *APIHandler) GetCampaign(c echo.Context) error {
	campaignID := c.Param("id")
	campaign, err := h.campaignSvc.GetByID(c.Request().Context(), campaignID)
	if err != nil {
		return apperror.NewNotFound("campaign not found")
	}
	return c.JSON(http.StatusOK, apiCampaignResponse{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Slug:        campaign.Slug,
		Description: campaign.Description,
		IsPublic:    campaign.IsPublic,
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
	})
}

// --- Entity Types ---

// ListEntityTypes returns all entity types for the campaign.
// GET /api/v1/campaigns/:id/entity-types
func (h *APIHandler) ListEntityTypes(c echo.Context) error {
	campaignID := c.Param("id")
	types, err := h.entitySvc.GetEntityTypes(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: failed to list entity types", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list entity types"))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data":  types,
		"total": len(types),
	})
}

// GetEntityType returns a single entity type by ID.
// GET /api/v1/campaigns/:id/entity-types/:typeID
func (h *APIHandler) GetEntityType(c echo.Context) error {
	typeID, err := strconv.Atoi(c.Param("typeID"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.entitySvc.GetEntityTypeByID(c.Request().Context(), typeID)
	if err != nil {
		return apperror.NewNotFound("entity type not found")
	}

	// Verify it belongs to the API key's campaign.
	if et.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity type not found")
	}

	return c.JSON(http.StatusOK, et)
}

// --- Entity Read ---

// ListEntities returns entities with pagination and optional filters.
// GET /api/v1/campaigns/:id/entities?type_id=N&page=1&per_page=20&q=search
func (h *APIHandler) ListEntities(c echo.Context) error {
	campaignID := c.Param("id")
	role := h.resolveRole(c)

	typeID, _ := strconv.Atoi(c.QueryParam("type_id"))
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	query := c.QueryParam("q")

	opts := entities.ListOptions{Page: page, PerPage: perPage}
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 20
	}

	var (
		items []entities.Entity
		total int
		err   error
	)

	userID := h.resolveUserID(c)
	if query != "" {
		items, total, err = h.entitySvc.Search(c.Request().Context(), campaignID, query, typeID, role, userID, opts)
	} else {
		items, total, err = h.entitySvc.List(c.Request().Context(), campaignID, typeID, role, userID, opts)
	}
	if err != nil {
		slog.Error("api: failed to list entities", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list entities"))
	}

	if items == nil {
		items = []entities.Entity{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":     items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// GetEntity returns a single entity by ID.
// GET /api/v1/campaigns/:id/entities/:entityID
func (h *APIHandler) GetEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	role := h.resolveRole(c)
	ctx := c.Request().Context()

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify the entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	// Enforce visibility: check both legacy is_private and custom permissions.
	userID := h.resolveUserID(c)
	access, accessErr := h.entitySvc.CheckEntityAccess(ctx, entity.ID, role, userID)
	if accessErr != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}

	return c.JSON(http.StatusOK, entity)
}

// --- Entity Write ---

// apiCreateEntityRequest is the JSON body for creating an entity via the API.
type apiCreateEntityRequest struct {
	Name         string         `json:"name"`
	EntityTypeID int            `json:"entity_type_id"`
	TypeLabel    string         `json:"type_label"`
	IsPrivate    bool           `json:"is_private"`
	FieldsData   map[string]any `json:"fields_data"`
}

// CreateEntity creates a new entity in the campaign.
// POST /api/v1/campaigns/:id/entities
func (h *APIHandler) CreateEntity(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// If no entity type specified, use the first available type for the campaign.
	if req.EntityTypeID == 0 {
		types, err := h.entitySvc.GetEntityTypes(c.Request().Context(), c.Param("id"))
		if err != nil || len(types) == 0 {
			return apperror.NewBadRequest("no entity types available")
		}
		req.EntityTypeID = types[0].ID
	}

	entity, err := h.entitySvc.Create(c.Request().Context(), c.Param("id"), key.UserID, entities.CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: req.EntityTypeID,
		TypeLabel:    req.TypeLabel,
		IsPrivate:    req.IsPrivate,
		FieldsData:   req.FieldsData,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, entity)
}

// apiUpdateEntityRequest is the JSON body for updating an entity via the API.
type apiUpdateEntityRequest struct {
	Name       string         `json:"name"`
	TypeLabel  string         `json:"type_label"`
	IsPrivate  bool           `json:"is_private"`
	Entry      string         `json:"entry"`
	FieldsData map[string]any `json:"fields_data"`
}

// UpdateEntity updates an existing entity.
// PUT /api/v1/campaigns/:id/entities/:entityID
func (h *APIHandler) UpdateEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	var req apiUpdateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	updated, err := h.entitySvc.Update(ctx, entityID, entities.UpdateEntityInput{
		Name:       req.Name,
		TypeLabel:  req.TypeLabel,
		IsPrivate:  req.IsPrivate,
		Entry:      req.Entry,
		FieldsData: req.FieldsData,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, updated)
}

// apiUpdateFieldsRequest is the JSON body for updating entity custom fields.
type apiUpdateFieldsRequest struct {
	FieldsData map[string]any `json:"fields_data"`
}

// UpdateEntityFields updates only the custom fields for an entity.
// PUT /api/v1/campaigns/:id/entities/:entityID/fields
func (h *APIHandler) UpdateEntityFields(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	var req apiUpdateFieldsRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.entitySvc.UpdateFields(ctx, entityID, req.FieldsData); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEntity deletes an entity from the campaign.
// DELETE /api/v1/campaigns/:id/entities/:entityID
func (h *APIHandler) DeleteEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	if err := h.entitySvc.Delete(ctx, entityID); err != nil {
		slog.Error("api: failed to delete entity", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to delete entity"))
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Sync Endpoint ---

// syncMaxPullPages caps the number of internal pages fetched during sync pull
// to prevent unbounded queries on large campaigns.
const syncMaxPullPages = 10

// syncPageSize is the per-page size used for internal pagination during sync.
const syncPageSize = 100

// syncRequest is the JSON body for the bulk sync endpoint.
type syncRequest struct {
	Since   *time.Time   `json:"since"`   // Pull entities modified after this time.
	Changes []syncChange `json:"changes"` // Batch of create/update/delete operations.
}

// syncChange describes a single mutation in a sync batch.
type syncChange struct {
	Action       string         `json:"action"`         // "create", "update", "delete".
	EntityID     string         `json:"entity_id"`      // Required for update/delete.
	EntityTypeID int            `json:"entity_type_id"` // Required for create.
	Name         string         `json:"name"`
	TypeLabel    string         `json:"type_label"`
	IsPrivate    bool           `json:"is_private"`
	Entry        string         `json:"entry"`
	FieldsData   map[string]any `json:"fields_data"`
}

// syncResult describes the outcome of a single sync operation.
type syncResult struct {
	Action   string `json:"action"`
	EntityID string `json:"entity_id"`
	Status   string `json:"status"` // "ok" or "error".
	Error    string `json:"error,omitempty"`
}

// syncResponse is the full response from the sync endpoint.
type syncResponse struct {
	ServerTime time.Time         `json:"server_time"`
	Entities   []entities.Entity `json:"entities"`
	HasMore    bool              `json:"has_more"`
	Results    []syncResult      `json:"results"`
}

// Sync performs a bidirectional sync operation.
// POST /api/v1/campaigns/:id/sync
//
// Pull: if "since" is provided, returns entities modified after that timestamp.
// Push: if "changes" is provided, applies the batch of create/update/delete operations.
// Returns server_time for the client to use as the next "since" parameter.
func (h *APIHandler) Sync(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	campaignID := c.Param("id")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	var req syncRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Reject oversized sync batches to prevent memory/CPU exhaustion.
	const maxSyncChanges = 2000
	if len(req.Changes) > maxSyncChanges {
		return apperror.NewBadRequest(
			fmt.Sprintf("too many changes; maximum is %d per request", maxSyncChanges))
	}

	serverTime := time.Now().UTC()

	// Pull: get entities modified since the given timestamp.
	var pulledEntities []entities.Entity
	hasMore := false

	if req.Since != nil {
		since := *req.Since
		syncUserID := h.resolveUserID(c)
		for page := 1; page <= syncMaxPullPages; page++ {
			items, total, err := h.entitySvc.List(ctx, campaignID, 0, role, syncUserID, entities.ListOptions{
				Page:    page,
				PerPage: syncPageSize,
			})
			if err != nil {
				slog.Error("api: sync pull failed", slog.Any("error", err))
				return apperror.NewInternal(fmt.Errorf("failed to pull entities"))
			}

			for _, e := range items {
				if e.UpdatedAt.After(since) || e.CreatedAt.After(since) {
					pulledEntities = append(pulledEntities, e)
				}
			}

			// Check if there are more pages beyond what we've fetched.
			if page*syncPageSize >= total {
				break
			}
			if page == syncMaxPullPages && page*syncPageSize < total {
				hasMore = true
			}
		}
	}

	if pulledEntities == nil {
		pulledEntities = []entities.Entity{}
	}

	// Push: apply batch changes.
	var results []syncResult
	for _, change := range req.Changes {
		result := syncResult{Action: change.Action, EntityID: change.EntityID}

		switch change.Action {
		case "create":
			entity, err := h.entitySvc.Create(ctx, campaignID, key.UserID, entities.CreateEntityInput{
				Name:         change.Name,
				EntityTypeID: change.EntityTypeID,
				TypeLabel:    change.TypeLabel,
				IsPrivate:    change.IsPrivate,
				FieldsData:   change.FieldsData,
			})
			if err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
			} else {
				result.Status = "ok"
				result.EntityID = entity.ID
			}

		case "update":
			// Verify entity belongs to this campaign before updating.
			existing, err := h.entitySvc.GetByID(ctx, change.EntityID)
			if err != nil || existing.CampaignID != campaignID {
				result.Status = "error"
				result.Error = "entity not found"
			} else {
				_, err := h.entitySvc.Update(ctx, change.EntityID, entities.UpdateEntityInput{
					Name:       change.Name,
					TypeLabel:  change.TypeLabel,
					IsPrivate:  change.IsPrivate,
					Entry:      change.Entry,
					FieldsData: change.FieldsData,
				})
				if err != nil {
					result.Status = "error"
					result.Error = apperror.SafeMessage(err)
				} else {
					result.Status = "ok"
				}
			}

		case "delete":
			// Verify entity belongs to this campaign before deleting.
			existing, err := h.entitySvc.GetByID(ctx, change.EntityID)
			if err != nil || existing.CampaignID != campaignID {
				result.Status = "error"
				result.Error = "entity not found"
			} else {
				if err := h.entitySvc.Delete(ctx, change.EntityID); err != nil {
					result.Status = "error"
					result.Error = apperror.SafeMessage(err)
				} else {
					result.Status = "ok"
				}
			}

		default:
			result.Status = "error"
			result.Error = "unknown action; expected create, update, or delete"
		}

		results = append(results, result)
	}

	if results == nil {
		results = []syncResult{}
	}

	return c.JSON(http.StatusOK, syncResponse{
		ServerTime: serverTime,
		Entities:   pulledEntities,
		HasMore:    hasMore,
		Results:    results,
	})
}

// --- Entity Relations ---

// ListEntityRelations returns all relations for an entity, enriched with target
// entity display data and metadata (price, quantity for shop inventory).
// GET /api/v1/campaigns/:id/entities/:entityID/relations
func (h *APIHandler) ListEntityRelations(c echo.Context) error {
	entityID := c.Param("entityID")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID required")
	}

	rels, err := h.relationSvc.ListByEntity(c.Request().Context(), entityID)
	if err != nil {
		slog.Error("listing entity relations", slog.String("entity_id", entityID), slog.String("error", err.Error()))
		return apperror.NewInternal(fmt.Errorf("failed to list relations"))
	}

	if rels == nil {
		rels = []relations.Relation{}
	}

	return c.JSON(http.StatusOK, rels)
}
