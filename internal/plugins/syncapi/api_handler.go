package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// AddonLister provides campaign addon listing for the discovery endpoint.
// Implemented by the addons plugin's service.
type AddonLister interface {
	ListForCampaign(ctx context.Context, campaignID string) ([]AddonInfo, error)
}

// AddonInfo is the API-safe representation of a campaign addon.
// Defined here to avoid importing the addons package directly.
type AddonInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Category  string `json:"category"`
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"installed"`
}

// APIHandler serves the versioned REST API for external tool integration.
// External clients (Foundry VTT, custom scripts) use these endpoints to
// read and write campaign data programmatically via API key authentication.
type APIHandler struct {
	syncSvc              SyncAPIService
	entitySvc            entities.EntityService
	campaignSvc          campaigns.CampaignService
	relationSvc          relations.RelationService
	addonChecker         AddonChecker
	addonLister          AddonLister
	systemEnabler        SystemEnabler
	campaignSystemLister CampaignSystemLister
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

// SetCampaignSystemLister sets the custom campaign system lister for including
// per-campaign custom systems in API responses.
func (h *APIHandler) SetCampaignSystemLister(csl CampaignSystemLister) {
	h.campaignSystemLister = csl
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

// ListMembers returns all members of the campaign.
// GET /api/v1/campaigns/:id/members
func (h *APIHandler) ListMembers(c echo.Context) error {
	campaignID := c.Param("id")
	members, err := h.campaignSvc.ListMembers(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: list members failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list members"))
	}
	return c.JSON(http.StatusOK, members)
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
	Name              string         `json:"name"`
	TypeLabel         string         `json:"type_label"`
	IsPrivate         bool           `json:"is_private"`
	Entry             string         `json:"entry"`
	PlayerNotes       *string        `json:"player_notes"`
	FieldsData        map[string]any `json:"fields_data"`
	ExpectedUpdatedAt *time.Time     `json:"expected_updated_at"`
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
		Name:              req.Name,
		TypeLabel:         req.TypeLabel,
		IsPrivate:         req.IsPrivate,
		Entry:             req.Entry,
		PlayerNotes:       req.PlayerNotes,
		FieldsData:        req.FieldsData,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
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

// ToggleEntityReveal sets or toggles an entity's is_private flag via the REST API.
// POST /api/v1/campaigns/:id/entities/:entityID/reveal
// Used by Foundry VTT to sync NPC visibility changes bidirectionally.
// Body: {"is_private": true|false} — if omitted, toggles current state.
func (h *APIHandler) ToggleEntityReveal(c echo.Context) error {
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

	var req struct {
		IsPrivate *bool `json:"is_private"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// If explicit value matches current state, no-op.
	if req.IsPrivate != nil && *req.IsPrivate == entity.IsPrivate {
		return c.JSON(http.StatusOK, map[string]any{
			"entity_id":  entityID,
			"is_private": entity.IsPrivate,
		})
	}

	// Toggle (or set to desired value — same effect since we checked above).
	newPrivate, err := h.entitySvc.TogglePrivate(ctx, entityID)
	if err != nil {
		return err
	}

	slog.Info("entity visibility changed via API",
		slog.String("entity_id", entityID),
		slog.Bool("is_private", newPrivate),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"entity_id":  entityID,
		"is_private": newPrivate,
	})
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

// --- Entity Permissions ---

// permissionsAPIResponse is the JSON response for entity permission queries.
type permissionsAPIResponse struct {
	Visibility  entities.VisibilityMode    `json:"visibility"`
	IsPrivate   bool                       `json:"is_private"`
	Permissions []entities.EntityPermission `json:"permissions"`
}

// GetEntityPermissions returns the visibility mode and permission grants for an entity.
// GET /api/v1/campaigns/:id/entities/:entityID/permissions
func (h *APIHandler) GetEntityPermissions(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	grants, err := h.entitySvc.GetEntityPermissions(ctx, entityID)
	if err != nil {
		slog.Error("fetching entity permissions",
			slog.String("entity_id", entityID),
			slog.String("error", err.Error()))
		return apperror.NewInternal(fmt.Errorf("failed to fetch permissions"))
	}

	if grants == nil {
		grants = []entities.EntityPermission{}
	}

	return c.JSON(http.StatusOK, permissionsAPIResponse{
		Visibility:  entity.Visibility,
		IsPrivate:   entity.IsPrivate,
		Permissions: grants,
	})
}

// SetEntityPermissions updates the visibility mode and permission grants for an entity.
// PUT /api/v1/campaigns/:id/entities/:entityID/permissions
func (h *APIHandler) SetEntityPermissions(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	// Only campaign owners can modify permissions.
	if role < int(campaigns.RoleOwner) {
		return apperror.NewForbidden("only campaign owners can modify entity permissions")
	}

	var input entities.SetPermissionsInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.entitySvc.SetEntityPermissions(ctx, entityID, input); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetAddonChecker injects the addon checker for system-aware endpoints.
// Called after construction because the addon service is wired separately.
func (h *APIHandler) SetAddonChecker(ac AddonChecker) {
	h.addonChecker = ac
}

// SetAddonLister injects the addon lister for the discovery endpoint.
func (h *APIHandler) SetAddonLister(al AddonLister) {
	h.addonLister = al
}

// SetSystemEnabler injects the system enabler for API-level self-healing.
// When a campaign has a selected system that isn't enabled as an addon,
// the ListSystems endpoint auto-enables it so the Foundry module sees
// the system with enabled=true.
func (h *APIHandler) SetSystemEnabler(se SystemEnabler) {
	h.systemEnabler = se
}

// --- Systems ---

// systemInfoResponse is the API-safe representation of a game system.
type systemInfoResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	HasCharacterFields bool   `json:"has_character_fields"`
	HasItemFields      bool   `json:"has_item_fields"`
	FoundrySystemID    string `json:"foundry_system_id"`
	Enabled            bool   `json:"enabled"`
}

// SystemEnabler enables a game system addon for a campaign. Used for
// API-level self-healing: if the campaign's selected system isn't enabled
// as an addon (e.g., set before self-healing was deployed), the API
// auto-enables it on read so the Foundry module sees enabled=true.
type SystemEnabler interface {
	EnableSystemForCampaign(ctx context.Context, campaignID, systemSlug, userID string) error
}

// CampaignSystemLister provides access to per-campaign custom systems.
type CampaignSystemLister interface {
	GetManifest(campaignID string) *systems.SystemManifest
}

// ListSystems returns game systems available for the campaign.
// Includes built-in systems from the global registry with an enabled flag
// based on per-campaign addon state. Used by the Foundry module to detect
// whether the current game system matches a Chronicle system.
//
// Self-healing: if the campaign has a selected system (in settings) but
// the addon isn't enabled — e.g., the system was set before self-healing
// was deployed — the endpoint auto-enables the addon on read so the
// Foundry module sees enabled=true without manual intervention.
// GET /api/v1/campaigns/:id/systems
func (h *APIHandler) ListSystems(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	// Resolve the campaign's selected system for self-healing.
	selectedSystemID := h.getSelectedSystemID(ctx, campaignID)

	registry := systems.Registry()
	result := make([]systemInfoResponse, 0, len(registry))

	for _, manifest := range registry {
		enabled := h.checkOrHealSystemAddon(ctx, campaignID, manifest.ID, selectedSystemID)

		result = append(result, systemInfoResponse{
			ID:                 manifest.ID,
			Name:               manifest.Name,
			Status:             string(manifest.Status),
			HasCharacterFields: manifest.CharacterPreset() != nil,
			HasItemFields:      manifest.ItemPreset() != nil,
			FoundrySystemID:    manifest.FoundrySystemID,
			Enabled:            enabled,
		})
	}

	// Include the campaign's custom system if one is uploaded.
	if h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil {
			enabled := h.checkOrHealSystemAddon(ctx, campaignID, custom.ID, selectedSystemID)
			result = append(result, systemInfoResponse{
				ID:                 custom.ID,
				Name:               custom.Name,
				Status:             string(custom.Status),
				HasCharacterFields: custom.CharacterPreset() != nil,
				FoundrySystemID:    custom.FoundrySystemID,
				Enabled:            enabled,
			})
		}
	}

	slog.Debug("ListSystems response",
		slog.String("campaign_id", campaignID),
		slog.Int("count", len(result)),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"data":  result,
		"total": len(result),
	})
}

// getSelectedSystemID returns the campaign's configured system ID from
// settings, or empty string if none is set or the campaign can't be loaded.
func (h *APIHandler) getSelectedSystemID(ctx context.Context, campaignID string) string {
	campaign, err := h.campaignSvc.GetByID(ctx, campaignID)
	if err != nil || campaign == nil {
		return ""
	}
	return campaign.ParseSettings().SystemID
}

// checkOrHealSystemAddon checks if a system addon is enabled for a campaign.
// If the addon is NOT enabled but the system IS the campaign's selected system,
// it auto-enables the addon (self-healing for pre-deployment system selections).
func (h *APIHandler) checkOrHealSystemAddon(ctx context.Context, campaignID, systemID, selectedSystemID string) bool {
	if h.addonChecker == nil {
		return false
	}

	ok, err := h.addonChecker.IsEnabledForCampaign(ctx, campaignID, systemID)
	if err == nil && ok {
		return true
	}

	// Self-heal: system is selected in campaign settings but addon not enabled.
	if h.systemEnabler != nil && selectedSystemID != "" && systemID == selectedSystemID {
		if err := h.systemEnabler.EnableSystemForCampaign(ctx, campaignID, systemID, ""); err == nil {
			slog.Info("auto-healed system addon via API",
				slog.String("campaign_id", campaignID),
				slog.String("system_id", systemID),
			)
			return true
		}
		slog.Warn("API self-heal failed for system addon",
			slog.String("campaign_id", campaignID),
			slog.String("system_id", systemID),
		)
	}

	return false
}

// GetCharacterFields returns the character preset field definitions for a
// specific system, including Foundry path annotations. Used by the Foundry
// module's generic adapter to auto-generate field mappings at runtime.
// GET /api/v1/campaigns/:id/systems/:systemId/character-fields
func (h *APIHandler) GetCharacterFields(c echo.Context) error {
	campaignID := c.Param("id")
	systemID := c.Param("systemId")

	// Look up the system manifest: first in global registry, then custom.
	manifest := systems.Find(systemID)
	if manifest == nil && h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil && custom.ID == systemID {
			manifest = custom
		}
	}

	if manifest == nil {
		return apperror.NewNotFound("system not found: " + systemID)
	}

	resp := manifest.CharacterFieldsForAPI()
	if resp == nil {
		return apperror.NewNotFound("character fields not found for system: " + systemID)
	}

	return c.JSON(http.StatusOK, resp)
}

// GetItemFields returns the item preset field definitions for a specific
// system, including Foundry path annotations. Used by the Foundry module
// for item sync field mappings.
// GET /api/v1/campaigns/:id/systems/:systemId/item-fields
func (h *APIHandler) GetItemFields(c echo.Context) error {
	campaignID := c.Param("id")
	systemID := c.Param("systemId")

	// Look up the system manifest: first in global registry, then custom.
	manifest := systems.Find(systemID)
	if manifest == nil && h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil && custom.ID == systemID {
			manifest = custom
		}
	}

	if manifest == nil {
		return apperror.NewNotFound("system not found: " + systemID)
	}

	resp := manifest.ItemFieldsForAPI()
	if resp == nil {
		// System has no item preset — return empty fields instead of 404.
		// This is expected for systems like Draw Steel that have character
		// and creature presets but no formal item preset.
		return c.JSON(http.StatusOK, map[string]any{
			"system_id": systemID,
			"fields":    []any{},
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// --- Addon Discovery ---

// ListAddons returns all addons for the campaign with their enabled state.
// Used by external clients to discover available features without probing.
// GET /api/v1/campaigns/:id/addons
func (h *APIHandler) ListAddons(c echo.Context) error {
	campaignID := c.Param("id")

	if h.addonLister == nil {
		return c.JSON(http.StatusOK, map[string]any{"data": []any{}, "total": 0})
	}

	addons, err := h.addonLister.ListForCampaign(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: list addons failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list addons"))
	}

	if addons == nil {
		addons = []AddonInfo{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  addons,
		"total": len(addons),
	})
}

// --- Relation Types & CRUD ---

// ListRelationTypes returns the predefined relation type pairs for the frontend.
// GET /api/v1/campaigns/:id/relations/types
func (h *APIHandler) ListRelationTypes(c echo.Context) error {
	types := h.relationSvc.GetCommonTypes()
	return c.JSON(http.StatusOK, map[string]any{
		"data":  types,
		"total": len(types),
	})
}

// apiCreateRelationRequest is the JSON body for creating a relation.
type apiCreateRelationRequest struct {
	TargetEntityID      string          `json:"target_entity_id"`
	RelationType        string          `json:"relation_type"`
	ReverseRelationType string          `json:"reverse_relation_type"`
	Metadata            json.RawMessage `json:"metadata"`
	DmOnly              bool            `json:"dm_only"`
}

// CreateRelation creates a new relation between two entities.
// POST /api/v1/campaigns/:id/entities/:entityID/relations
func (h *APIHandler) CreateRelation(c echo.Context) error {
	campaignID := c.Param("id")
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify source entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}

	var req apiCreateRelationRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if req.TargetEntityID == "" {
		return apperror.NewBadRequest("target_entity_id is required")
	}
	if req.RelationType == "" {
		return apperror.NewBadRequest("relation_type is required")
	}

	userID := h.resolveUserID(c)
	rel, err := h.relationSvc.Create(ctx, campaignID, entityID, req.TargetEntityID,
		req.RelationType, req.ReverseRelationType, userID, req.Metadata, req.DmOnly)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, rel)
}

// apiUpdateRelationRequest is the JSON body for updating a relation's metadata.
type apiUpdateRelationRequest struct {
	Metadata json.RawMessage `json:"metadata"`
}

// UpdateRelation updates a relation's metadata.
// PUT /api/v1/campaigns/:id/relations/:relationId
func (h *APIHandler) UpdateRelation(c echo.Context) error {
	relationID, err := strconv.Atoi(c.Param("relationId"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	var req apiUpdateRelationRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.relationSvc.UpdateMetadata(c.Request().Context(), relationID, req.Metadata); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteRelation removes a relation and its reverse.
// DELETE /api/v1/campaigns/:id/relations/:relationId
func (h *APIHandler) DeleteRelation(c echo.Context) error {
	relationID, err := strconv.Atoi(c.Param("relationId"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	if err := h.relationSvc.Delete(c.Request().Context(), relationID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Entity Type CRUD ---

// apiCreateEntityTypeRequest is the JSON body for creating an entity type.
type apiCreateEntityTypeRequest struct {
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

// CreateEntityType creates a new entity type for the campaign.
// POST /api/v1/campaigns/:id/entity-types
func (h *APIHandler) CreateEntityType(c echo.Context) error {
	campaignID := c.Param("id")

	var req apiCreateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	et, err := h.entitySvc.CreateEntityType(c.Request().Context(), campaignID, entities.CreateEntityTypeInput{
		Name:       req.Name,
		NamePlural: req.NamePlural,
		Icon:       req.Icon,
		Color:      req.Color,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, et)
}

// apiUpdateEntityTypeRequest is the JSON body for updating an entity type.
type apiUpdateEntityTypeRequest struct {
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

// UpdateEntityType updates an existing entity type.
// PUT /api/v1/campaigns/:id/entity-types/:typeID
func (h *APIHandler) UpdateEntityType(c echo.Context) error {
	campaignID := c.Param("id")
	typeID, err := strconv.Atoi(c.Param("typeID"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	// Verify entity type belongs to this campaign.
	existing, err := h.entitySvc.GetEntityTypeByID(c.Request().Context(), typeID)
	if err != nil {
		return apperror.NewNotFound("entity type not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("entity type not found")
	}

	var req apiUpdateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	et, err := h.entitySvc.UpdateEntityType(c.Request().Context(), typeID, entities.UpdateEntityTypeInput{
		Name:       req.Name,
		NamePlural: req.NamePlural,
		Icon:       req.Icon,
		Color:      req.Color,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, et)
}

// --- Bulk Operations ---

// apiBulkUpdateEntityTypeRequest is the JSON body for bulk entity type reassignment.
type apiBulkUpdateEntityTypeRequest struct {
	EntityIDs    []string `json:"entity_ids"`
	EntityTypeID int      `json:"entity_type_id"`
}

// BulkUpdateEntityType changes the entity type for multiple entities at once.
// POST /api/v1/campaigns/:id/entities/bulk-update
func (h *APIHandler) BulkUpdateEntityType(c echo.Context) error {
	campaignID := c.Param("id")

	var req apiBulkUpdateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	const maxBulkEntities = 200
	if len(req.EntityIDs) == 0 {
		return apperror.NewBadRequest("entity_ids is required")
	}
	if len(req.EntityIDs) > maxBulkEntities {
		return apperror.NewBadRequest(fmt.Sprintf("too many entities; maximum is %d per request", maxBulkEntities))
	}
	if req.EntityTypeID == 0 {
		return apperror.NewBadRequest("entity_type_id is required")
	}

	updated, err := h.entitySvc.BulkUpdateType(c.Request().Context(), campaignID, req.EntityIDs, req.EntityTypeID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"updated": updated,
	})
}
