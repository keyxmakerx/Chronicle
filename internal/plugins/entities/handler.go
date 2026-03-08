package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// EntityTagFetcher retrieves tags for entities in batch. Defined here to avoid
// importing the tags widget package, keeping plugins loosely coupled via interfaces.
// includeDmOnly controls whether dm_only tags are returned (true for Scribes+).
type EntityTagFetcher interface {
	GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]EntityTagInfo, error)
}

// AddonChecker is a narrow interface for checking whether an addon is enabled
// for a campaign. Satisfied by addons.AddonService.
type AddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// TimelineSearcher provides timeline search results for the @mention popup.
// Implemented by the timeline plugin and injected via SetTimelineSearcher.
type TimelineSearcher interface {
	SearchTimelines(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)
}

// MapSearcher provides map search results for the quick search popup.
// Implemented by the maps plugin and injected via SetMapSearcher.
type MapSearcher interface {
	SearchMaps(ctx context.Context, campaignID, query string) ([]map[string]string, error)
}

// CalendarSearcher provides calendar event search results for the quick search popup.
// Implemented by the calendar plugin and injected via SetCalendarSearcher.
type CalendarSearcher interface {
	SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)
}

// SessionSearcher provides session search results for the quick search popup.
// Implemented by the sessions plugin and injected via SetSessionSearcher.
type SessionSearcher interface {
	SearchSessions(ctx context.Context, campaignID, query string) ([]map[string]string, error)
}

// MemberLister retrieves campaign members for the permissions UI.
// Satisfied by campaigns.CampaignService.
type MemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// GroupLister retrieves campaign groups for the permissions UI.
// Satisfied by campaigns.GroupService.
type GroupLister interface {
	ListGroups(ctx context.Context, campaignID string) ([]campaigns.CampaignGroup, error)
}

// WidgetBlockLister returns extension widget block metadata for the template
// editor palette. Implemented by the extensions handler.
type WidgetBlockLister interface {
	GetWidgetBlockMetas(ctx context.Context, campaignID string) []BlockMeta
}

// Handler handles HTTP requests for entity operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service            EntityService
	auditSvc           audit.AuditService
	tagFetcher         EntityTagFetcher
	addonSvc           AddonChecker
	timelineSearcher   TimelineSearcher
	mapSearcher        MapSearcher
	calendarSearcher   CalendarSearcher
	sessionSearcher    SessionSearcher
	memberLister       MemberLister
	groupLister        GroupLister
	widgetBlockLister  WidgetBlockLister
	blockRegistry      *BlockRegistry
	cache              *redis.Client
}

// NewHandler creates a new entity handler.
func NewHandler(service EntityService) *Handler {
	return &Handler{service: service}
}

// SetAuditService sets the audit service for recording entity mutations.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetAuditService(svc audit.AuditService) {
	h.auditSvc = svc
}

// SetAddonChecker sets the addon checker for conditional feature rendering.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetAddonChecker(svc AddonChecker) {
	h.addonSvc = svc
}

// SetBlockRegistry sets the block registry for the block-types API endpoint.
// Called after all plugins have registered their block types.
func (h *Handler) SetBlockRegistry(reg *BlockRegistry) {
	h.blockRegistry = reg
}

// SetTagFetcher sets the tag fetcher for populating entity tags in list views.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetTagFetcher(f EntityTagFetcher) {
	h.tagFetcher = f
}

// SetTimelineSearcher sets the timeline searcher for @mention results.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetTimelineSearcher(ts TimelineSearcher) {
	h.timelineSearcher = ts
}

// SetMapSearcher sets the map searcher for quick search results.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetMapSearcher(ms MapSearcher) {
	h.mapSearcher = ms
}

// SetCalendarSearcher sets the calendar event searcher for quick search results.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetCalendarSearcher(cs CalendarSearcher) {
	h.calendarSearcher = cs
}

// SetSessionSearcher sets the session searcher for quick search results.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetSessionSearcher(ss SessionSearcher) {
	h.sessionSearcher = ss
}

// SetMemberLister sets the member lister for the permissions UI.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetMemberLister(ml MemberLister) {
	h.memberLister = ml
}

// SetGroupLister sets the group lister for the permissions UI.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetGroupLister(gl GroupLister) {
	h.groupLister = gl
}

// SetWidgetBlockLister sets the extension widget block lister for the template
// editor palette. Extension widgets appear as additional block types.
func (h *Handler) SetWidgetBlockLister(wbl WidgetBlockLister) {
	h.widgetBlockLister = wbl
}

// SetCache sets the Redis client for API response caching (e.g., entity names).
func (h *Handler) SetCache(rdb *redis.Client) {
	h.cache = rdb
}

// logAudit fires a fire-and-forget audit entry. Errors are logged but
// never block the primary operation.
func (h *Handler) logAudit(c echo.Context, campaignID, action, entityID, entityName string) {
	if h.auditSvc == nil {
		return
	}
	userID := auth.GetUserID(c)
	if err := h.auditSvc.Log(c.Request().Context(), &audit.AuditEntry{
		CampaignID: campaignID,
		UserID:     userID,
		Action:     action,
		EntityType: "entity",
		EntityID:   entityID,
		EntityName: entityName,
	}); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("error", err))
	}
}

// --- Entity CRUD ---

// Index renders the entity list page (GET /campaigns/:id/entities).
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	role := int(cc.MemberRole)
	campaignID := cc.Campaign.ID
	userID := auth.GetUserID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	opts := DefaultListOptions()
	if page > 0 {
		opts.Page = page
	}
	if sort := c.QueryParam("sort"); sort == "updated" || sort == "created" || sort == "name" {
		opts.Sort = sort
	}

	// Resolve entity type filter from shortcut route or query param.
	var typeID int
	var activeTypeSlug string
	var activeEntityType *EntityType
	if slug, ok := c.Get("entity_type_slug").(string); ok && slug != "" {
		activeTypeSlug = slug
		et, err := h.service.GetEntityTypeBySlug(c.Request().Context(), campaignID, slug)
		if err == nil {
			typeID = et.ID
			activeEntityType = et
		}
	} else if tid, _ := strconv.Atoi(c.QueryParam("type")); tid > 0 {
		typeID = tid
		et, err := h.service.GetEntityTypeByID(c.Request().Context(), tid)
		if err == nil {
			activeEntityType = et
		}
	}

	// Fetch entity types for sidebar filter and counts. Non-fatal: degrade
	// gracefully if these fail (page still renders, just without filters).
	entityTypes, err := h.service.GetEntityTypes(c.Request().Context(), campaignID)
	if err != nil {
		slog.Warn("failed to load entity types for list page", slog.Any("error", err))
	}
	counts, err := h.service.CountByType(c.Request().Context(), campaignID, role, userID)
	if err != nil {
		slog.Warn("failed to load entity counts for list page", slog.Any("error", err))
	}

	entities, total, err := h.service.List(c.Request().Context(), campaignID, typeID, role, userID, opts)
	if err != nil {
		return err
	}

	// Batch-fetch tags for all entities in the list to show chips on cards.
	if h.tagFetcher != nil && len(entities) > 0 {
		entityIDs := make([]string, len(entities))
		for i := range entities {
			entityIDs[i] = entities[i].ID
		}
		// Scribes+ see all tags including dm_only; Players see only public tags.
		cc := campaigns.GetCampaignContext(c)
		includeDmOnly := cc != nil && cc.MemberRole >= campaigns.RoleScribe
		if tagsMap, err := h.tagFetcher.GetEntityTagsBatch(c.Request().Context(), entityIDs, includeDmOnly); err == nil {
			for i := range entities {
				if t, ok := tagsMap[entities[i].ID]; ok {
					entities[i].Tags = t
				}
			}
		} else {
			slog.Warn("failed to batch-fetch entity tags for list", slog.Any("error", err))
		}
	}

	csrfToken := middleware.GetCSRFToken(c)

	// When viewing a specific category (type), render the category dashboard.
	if activeEntityType != nil {
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK,
				CategoryDashboardContent(cc, activeEntityType, entities, counts, total, opts, csrfToken))
		}
		return middleware.Render(c, http.StatusOK,
			CategoryDashboardPage(cc, activeEntityType, entities, counts, total, opts, csrfToken))
	}

	// Otherwise render the "All Pages" grid.
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK,
			EntityListContent(cc, entities, entityTypes, counts, total, opts, typeID, activeTypeSlug, csrfToken))
	}
	return middleware.Render(c, http.StatusOK,
		EntityIndexPage(cc, entities, entityTypes, counts, total, opts, typeID, activeTypeSlug, csrfToken))
}

// NewForm renders the entity creation form (GET /campaigns/:id/entities/new).
func (h *Handler) NewForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)
	preselect, _ := strconv.Atoi(c.QueryParam("type"))

	// Pre-fill parent if ?parent_id= is set (from "Create sub-page" button).
	var parentEntity *Entity
	if parentID := c.QueryParam("parent_id"); parentID != "" {
		parent, err := h.service.GetByID(c.Request().Context(), parentID)
		if err == nil && parent.CampaignID == cc.Campaign.ID {
			parentEntity = parent
		}
	}

	return middleware.Render(c, http.StatusOK, EntityNewPage(cc, entityTypes, preselect, parentEntity, csrfToken, ""))
}

// Create processes the entity creation form (POST /campaigns/:id/entities).
func (h *Handler) Create(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req CreateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Validate field lengths before processing.
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", req.Name, apperror.MaxNameLength); err != nil {
		return err
	}

	fieldsData := h.parseFieldsFromForm(c, cc.Campaign.ID, req.EntityTypeID)

	userID := auth.GetUserID(c)
	input := CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: req.EntityTypeID,
		TypeLabel:    req.TypeLabel,
		ParentID:     req.ParentID,
		IsPrivate:    req.IsPrivate,
		FieldsData:   fieldsData,
	}

	entity, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to create entity"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK, EntityNewPage(cc, entityTypes, req.EntityTypeID, nil, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, entity.ID, entity.Name)

	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entities/" + entity.ID
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectURL)
}

// Show renders the entity profile page (GET /campaigns/:id/entities/:eid).
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(fmt.Errorf("campaign context is nil for entity show"))
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// IDOR protection: verify entity belongs to the campaign in the URL.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	// Visibility check: verify the user can view this entity.
	userID := auth.GetUserID(c)
	access, err := h.service.CheckEntityAccess(c.Request().Context(), entity.ID, int(cc.MemberRole), userID)
	if err != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}

	entityType, err := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("get entity type %d: %w", entity.EntityTypeID, err))
	}

	// Fetch ancestor chain for breadcrumbs, children for sub-page listing,
	// and backlinks for "Referenced by" section.
	ancestors, _ := h.service.GetAncestors(c.Request().Context(), entity.ID)
	children, _ := h.service.GetChildren(c.Request().Context(), entity.ID, int(cc.MemberRole), userID)
	backlinks, _ := h.service.GetBacklinks(c.Request().Context(), entity.ID, int(cc.MemberRole), userID)

	// Check if the "attributes" addon is enabled for this campaign.
	// Defaults to true (show attributes) if addon checker is not wired or
	// the addon hasn't been explicitly disabled.
	showAttributes := true
	if h.addonSvc != nil {
		enabled, err := h.addonSvc.IsEnabledForCampaign(c.Request().Context(), cc.Campaign.ID, "attributes")
		if err == nil {
			showAttributes = enabled
		}
	}

	// Check if the "calendar" addon is enabled — gates the lazy-loaded
	// entity-events fragment. Without this gate, the HTMX request to the
	// calendar endpoint would fire unconditionally, and any error (auth
	// mismatch, missing calendar, etc.) would trigger HX-Retarget:body
	// in the error handler, replacing the entire entity page.
	showCalendar := false
	if h.addonSvc != nil {
		enabled, err := h.addonSvc.IsEnabledForCampaign(c.Request().Context(), cc.Campaign.ID, "calendar")
		if err == nil {
			showCalendar = enabled
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, EntityShowPage(cc, entity, entityType, ancestors, children, backlinks, showAttributes, showCalendar, csrfToken))
}

// Clone creates a copy of an entity (POST /campaigns/:id/entities/:eid/clone).
// Copies name (with " (Copy)" suffix), entry, fields, image, parent, privacy,
// field overrides, popup config, and tags. Does NOT copy relations.
func (h *Handler) Clone(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(fmt.Errorf("campaign context is nil for entity clone"))
	}

	userID := auth.GetUserID(c)
	entityID := c.Param("eid")
	clone, err := h.service.Clone(c.Request().Context(), cc.Campaign.ID, userID, entityID)
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "entity.clone", clone.ID, clone.Name)

	// Redirect to the edit page of the new clone so user can review/rename.
	editURL := fmt.Sprintf("/campaigns/%s/entities/%s/edit", cc.Campaign.ID, clone.ID)
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", editURL)
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, editURL)
}

// EditForm renders the entity edit form (GET /campaigns/:id/entities/:eid/edit).
func (h *Handler) EditForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// IDOR protection.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	entityType, _ := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	csrfToken := middleware.GetCSRFToken(c)

	// Fetch parent entity for pre-fill in the parent selector.
	var parentEntity *Entity
	if entity.ParentID != nil {
		parent, err := h.service.GetByID(c.Request().Context(), *entity.ParentID)
		if err == nil {
			parentEntity = parent
		}
	}

	return middleware.Render(c, http.StatusOK, EntityEditPage(cc, entity, entityType, entityTypes, parentEntity, csrfToken, ""))
}

// Update processes the entity edit form (PUT /campaigns/:id/entities/:eid).
func (h *Handler) Update(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// IDOR protection.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var req UpdateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	fieldsData := h.parseFieldsFromForm(c, cc.Campaign.ID, entity.EntityTypeID)

	input := UpdateEntityInput{
		Name:       req.Name,
		TypeLabel:  req.TypeLabel,
		ParentID:   req.ParentID,
		IsPrivate:  req.IsPrivate,
		Entry:      req.Entry,
		FieldsData: fieldsData,
	}

	_, err = h.service.Update(c.Request().Context(), entityID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		entityType, _ := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to update entity"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		// Fetch parent for re-rendering form.
		var parentEntity *Entity
		if entity.ParentID != nil {
			parent, pErr := h.service.GetByID(c.Request().Context(), *entity.ParentID)
			if pErr == nil {
				parentEntity = parent
			}
		}
		return middleware.Render(c, http.StatusOK, EntityEditPage(cc, entity, entityType, entityTypes, parentEntity, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, entity.Name)

	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entities/" + entityID
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectURL)
}

// Delete removes an entity (DELETE /campaigns/:id/entities/:eid).
func (h *Handler) Delete(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	// IDOR protection: verify entity belongs to the campaign.
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	if err := h.service.Delete(c.Request().Context(), entityID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityDeleted, entityID, entity.Name)

	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entities"
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectURL)
}

// SearchAPI handles entity search requests (GET /campaigns/:id/entities/search).
// Returns HTML fragments for HTMX callers and JSON for API callers (e.g., the
// @mention widget) based on the Accept header.
func (h *Handler) SearchAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	role := int(cc.MemberRole)
	userID := auth.GetUserID(c)
	query := c.QueryParam("q")
	typeID, _ := strconv.Atoi(c.QueryParam("type"))

	opts := DefaultListOptions()
	opts.PerPage = 20

	// Check if the caller wants JSON (used by the editor @mention widget).
	wantsJSON := strings.Contains(c.Request().Header.Get("Accept"), "application/json")

	// Use List (no search filter) when query is empty, Search when the user
	// has typed a query. This allows the sidebar drill panel to auto-load
	// all pages of a category without requiring a search term.
	var results []Entity
	var total int
	var err error

	if q := strings.TrimSpace(query); len(q) >= 2 {
		results, total, err = h.service.Search(c.Request().Context(), cc.Campaign.ID, query, typeID, role, userID, opts)
	} else if q == "" {
		results, total, err = h.service.List(c.Request().Context(), cc.Campaign.ID, typeID, role, userID, opts)
	} else {
		// 1 character — not enough for search, return empty results.
		results, total = nil, 0
	}
	if err != nil {
		if _, ok := err.(*apperror.AppError); ok {
			if wantsJSON {
				return c.JSON(http.StatusOK, map[string]any{"results": []any{}, "total": 0})
			}
			return middleware.Render(c, http.StatusOK, SearchResultsFragment(nil, 0, cc))
		}
		return err
	}

	if wantsJSON {
		items := make([]map[string]string, len(results))
		for i, e := range results {
			items[i] = map[string]string{
				"id":         e.ID,
				"name":       e.Name,
				"type_name":  e.TypeName,
				"type_icon":  e.TypeIcon,
				"type_color": e.TypeColor,
				"url":        fmt.Sprintf("/campaigns/%s/entities/%s", cc.Campaign.ID, e.ID),
			}
		}
		// Append cross-plugin search results from registered searchers.
		if h.timelineSearcher != nil && query != "" {
			if tlResults, err := h.timelineSearcher.SearchTimelines(
				c.Request().Context(), cc.Campaign.ID, query, role,
			); err == nil {
				items = append(items, tlResults...)
				total += len(tlResults)
			}
		}
		if h.mapSearcher != nil && query != "" {
			if mapResults, err := h.mapSearcher.SearchMaps(
				c.Request().Context(), cc.Campaign.ID, query,
			); err == nil {
				items = append(items, mapResults...)
				total += len(mapResults)
			}
		}
		if h.calendarSearcher != nil && query != "" {
			if calResults, err := h.calendarSearcher.SearchCalendarEvents(
				c.Request().Context(), cc.Campaign.ID, query, role,
			); err == nil {
				items = append(items, calResults...)
				total += len(calResults)
			}
		}
		if h.sessionSearcher != nil && query != "" {
			if sessResults, err := h.sessionSearcher.SearchSessions(
				c.Request().Context(), cc.Campaign.ID, query,
			); err == nil {
				items = append(items, sessResults...)
				total += len(sessResults)
			}
		}
		return c.JSON(http.StatusOK, map[string]any{"results": items, "total": total})
	}

	// Sidebar mode returns a compact list for the sidebar drill panel.
	if c.QueryParam("sidebar") == "1" {
		return middleware.Render(c, http.StatusOK, SidebarEntityList(results, total, cc))
	}

	return middleware.Render(c, http.StatusOK, SearchResultsFragment(results, total, cc))
}

// --- Quick Create API (JSON endpoint for shop widget) ---

// QuickCreateAPI creates a new entity from a JSON request and returns its data.
// POST /campaigns/:id/entities/quick-create
// Used by the shop inventory widget to create items inline.
func (h *Handler) QuickCreateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req struct {
		Name         string `json:"name"`
		EntityTypeID int    `json:"entity_type_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", req.Name, apperror.MaxNameLength); err != nil {
		return err
	}

	// If no entity type specified, use the first available type.
	if req.EntityTypeID == 0 {
		types, err := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		if err != nil || len(types) == 0 {
			return apperror.NewBadRequest("no entity types available")
		}
		req.EntityTypeID = types[0].ID
	}

	userID := auth.GetUserID(c)
	input := CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: req.EntityTypeID,
	}

	entity, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, entity.ID, entity.Name)

	return c.JSON(http.StatusCreated, map[string]string{
		"id":         entity.ID,
		"name":       entity.Name,
		"type_name":  entity.TypeName,
		"type_icon":  entity.TypeIcon,
		"type_color": entity.TypeColor,
	})
}

// EntityTypesAPI returns entity types as JSON for widget dropdowns.
// GET /campaigns/:id/entities/types
func (h *Handler) EntityTypesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	types, err := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	items := make([]map[string]any, len(types))
	for i, t := range types {
		items[i] = map[string]any{
			"id":   t.ID,
			"name": t.Name,
			"icon": t.Icon,
		}
	}
	return c.JSON(http.StatusOK, items)
}

// --- Entry API (JSON endpoints for editor widget) ---

// GetEntry returns the entity's entry content as JSON.
// GET /campaigns/:id/entities/:eid/entry
func (h *Handler) GetEntry(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// IDOR protection.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	// Privacy check.
	if entity.IsPrivate && cc.MemberRole < campaigns.RoleScribe {
		return apperror.NewNotFound("entity not found")
	}

	// Strip inline secrets for non-scribe users so players never receive
	// GM-only text. Owners and scribes see secrets with a visual indicator.
	entry := entity.Entry
	entryHTML := entity.EntryHTML
	if cc.MemberRole < campaigns.RoleScribe {
		if entry != nil {
			stripped := sanitize.StripSecretsJSON(*entry)
			entry = &stripped
		}
		if entryHTML != nil {
			stripped := sanitize.StripSecretsHTML(*entryHTML)
			entryHTML = &stripped
		}
	}

	response := map[string]any{
		"entry":      entry,
		"entry_html": entryHTML,
	}
	return c.JSON(http.StatusOK, response)
}

// UpdateEntryAPI saves the entity's entry content from the editor widget.
// PUT /campaigns/:id/entities/:eid/entry
func (h *Handler) UpdateEntryAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	// IDOR protection.
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var body struct {
		Entry     string `json:"entry"`
		EntryHTML string `json:"entry_html"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateEntry(c.Request().Context(), entityID, body.Entry, body.EntryHTML); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, entity.Name)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Fields API ---

// GetFieldsAPI returns the entity's custom field values and type definitions.
// GET /campaigns/:id/entities/:eid/fields
func (h *Handler) GetFieldsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}
	if entity.IsPrivate && cc.MemberRole < campaigns.RoleScribe {
		return apperror.NewNotFound("entity not found")
	}

	// Look up the entity type to get field definitions.
	et, err := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	if err != nil {
		return err
	}

	// Merge type-level fields with per-entity overrides for effective field list.
	effectiveFields := MergeFields(et.Fields, entity.FieldOverrides)

	// Default to empty overrides so the frontend always gets a valid object.
	overrides := entity.FieldOverrides
	if overrides == nil {
		overrides = &FieldOverrides{}
	}

	response := map[string]any{
		"fields":          effectiveFields,
		"fields_data":     entity.FieldsData,
		"field_overrides": overrides,
		"type_fields":     et.Fields,
	}
	return c.JSON(http.StatusOK, response)
}

// UpdateFieldsAPI saves the entity's custom field values from the attributes widget.
// PUT /campaigns/:id/entities/:eid/fields
func (h *Handler) UpdateFieldsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var body struct {
		FieldsData map[string]any `json:"fields_data"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateFields(c.Request().Context(), entityID, body.FieldsData); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, entity.Name)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateFieldOverridesAPI saves per-entity field customizations (added, hidden,
// modified fields) from the attributes widget's gear menu.
// PUT /campaigns/:id/entities/:eid/field-overrides
func (h *Handler) UpdateFieldOverridesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var body FieldOverrides
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateFieldOverrides(c.Request().Context(), entityID, &body); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, entity.Name)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ResetFieldOverridesAPI clears all per-entity field customizations, restoring
// the entity to its category's default field template.
// DELETE /campaigns/:id/entities/:eid/field-overrides
func (h *Handler) ResetFieldOverridesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	if err := h.service.UpdateFieldOverrides(c.Request().Context(), entityID, nil); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, entity.Name)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Image API ---

// UpdateImageAPI updates the entity's header image path.
// PUT /campaigns/:id/entities/:eid/image
func (h *Handler) UpdateImageAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	// IDOR protection.
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var body struct {
		ImagePath string `json:"image_path"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateImage(c.Request().Context(), entityID, body.ImagePath); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Preview API ---

// htmlTagPattern matches HTML tags for stripping in entry excerpts.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// PreviewAPI returns entity data for tooltip/popover display, respecting the
// entity's popup_config to control which sections are included.
// GET /campaigns/:id/entities/:eid/preview
func (h *Handler) PreviewAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	// IDOR protection: verify entity belongs to the campaign in the URL.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	// Privacy check: private entities return 404 for Players.
	if entity.IsPrivate && cc.MemberRole < campaigns.RoleScribe {
		return apperror.NewNotFound("entity not found")
	}

	// Look up the entity type for icon, color, name, and field definitions.
	entityType, err := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	if err != nil {
		return apperror.NewMissingContext()
	}

	cfg := entity.EffectivePopupConfig()

	// Build an excerpt from entry_html: strip HTML tags, truncate to ~150 chars.
	var entryExcerpt string
	if cfg.ShowEntry && entity.EntryHTML != nil && *entity.EntryHTML != "" {
		plain := htmlTagPattern.ReplaceAllString(*entity.EntryHTML, "")
		plain = strings.Join(strings.Fields(plain), " ") // Normalize whitespace.
		if len(plain) > 150 {
			// Truncate at word boundary.
			truncated := plain[:150]
			if idx := strings.LastIndex(truncated, " "); idx > 100 {
				truncated = truncated[:idx]
			}
			entryExcerpt = truncated + "..."
		} else {
			entryExcerpt = plain
		}
	}

	// Resolve image path when popup config allows it.
	var imagePath string
	if cfg.ShowImage && entity.ImagePath != nil && *entity.ImagePath != "" {
		imagePath = fmt.Sprintf("/media/%s", *entity.ImagePath)
	}

	// Build attributes list: field label + value pairs for the first few fields.
	// Initialize to empty slice so JSON serializes as [] instead of null.
	attributes := make([]map[string]string, 0)
	if cfg.ShowAttributes && entityType != nil {
		effectiveFields := MergeFields(entityType.Fields, entity.FieldOverrides)
		for _, fd := range effectiveFields {
			val, ok := entity.FieldsData[fd.Key]
			if !ok || val == nil || fmt.Sprintf("%v", val) == "" {
				continue
			}
			attributes = append(attributes, map[string]string{
				"label": fd.Label,
				"value": fmt.Sprintf("%v", val),
			})
			if len(attributes) >= 5 {
				break // Limit to 5 attributes in tooltip.
			}
		}
	}

	// Resolve type label.
	var typeLabel string
	if entity.TypeLabel != nil {
		typeLabel = *entity.TypeLabel
	}

	// Set cache headers: short-lived cache for fast repeated hovers.
	c.Response().Header().Set("Cache-Control", "private, max-age=60")

	return c.JSON(http.StatusOK, map[string]any{
		"name":          entity.Name,
		"type_name":     entityType.Name,
		"type_icon":     entityType.Icon,
		"type_color":    entityType.Color,
		"image_path":    imagePath,
		"type_label":    typeLabel,
		"is_private":    entity.IsPrivate,
		"entry_excerpt": entryExcerpt,
		"attributes":    attributes,
	})
}

// UpdatePopupConfigAPI saves the entity's hover preview tooltip configuration.
// PUT /campaigns/:id/entities/:eid/popup-config
func (h *Handler) UpdatePopupConfigAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")

	entity, err := h.service.GetByID(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var body PopupConfig
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdatePopupConfig(c.Request().Context(), entityID, &body); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Per-Entity Permissions API ---

// permissionsResponse is the JSON response for the GET permissions endpoint.
type permissionsResponse struct {
	Visibility  VisibilityMode       `json:"visibility"`
	IsPrivate   bool                 `json:"is_private"`
	Members     []permissionsMember  `json:"members"`
	Groups      []permissionsGroup   `json:"groups"`
	Permissions []EntityPermission   `json:"permissions"`
}

// permissionsGroup is a campaign group summary for the permissions UI.
type permissionsGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// permissionsMember is a campaign member summary for the permissions UI.
type permissionsMember struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        int    `json:"role"`
	AvatarPath  string `json:"avatar_path,omitempty"`
}

// setPermissionsRequest is the JSON body for setting entity permissions.
type setPermissionsRequest struct {
	Visibility  VisibilityMode    `json:"visibility"`
	IsPrivate   bool              `json:"is_private"`
	Permissions []PermissionGrant `json:"permissions"`
}

// GetPermissionsAPI returns the entity's current visibility mode, campaign
// members, and permission grants. Owner only.
// GET /campaigns/:id/entities/:eid/permissions
func (h *Handler) GetPermissionsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	ctx := c.Request().Context()

	entity, err := h.service.GetByID(ctx, entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	// Fetch current permission grants.
	grants, err := h.service.GetEntityPermissions(ctx, entityID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("get entity permissions: %w", err))
	}
	if grants == nil {
		grants = []EntityPermission{}
	}

	// Fetch campaign members for the picker UI.
	var members []permissionsMember
	if h.memberLister != nil {
		campaignMembers, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID)
		if err != nil {
			slog.Error("failed to list campaign members for permissions", slog.Any("error", err))
		} else {
			for _, m := range campaignMembers {
				pm := permissionsMember{
					UserID:      m.UserID,
					DisplayName: m.DisplayName,
					Email:       m.Email,
					Role:        int(m.Role),
				}
				if m.AvatarPath != nil {
					pm.AvatarPath = *m.AvatarPath
				}
				members = append(members, pm)
			}
		}
	}
	if members == nil {
		members = []permissionsMember{}
	}

	// Fetch campaign groups for the group grants selector.
	var groups []permissionsGroup
	if h.groupLister != nil {
		campaignGroups, err := h.groupLister.ListGroups(ctx, cc.Campaign.ID)
		if err != nil {
			slog.Error("failed to list campaign groups for permissions", slog.Any("error", err))
		} else {
			for _, g := range campaignGroups {
				groups = append(groups, permissionsGroup{
					ID:   g.ID,
					Name: g.Name,
				})
			}
		}
	}
	if groups == nil {
		groups = []permissionsGroup{}
	}

	return c.JSON(http.StatusOK, permissionsResponse{
		Visibility:  entity.Visibility,
		IsPrivate:   entity.IsPrivate,
		Members:     members,
		Groups:      groups,
		Permissions: grants,
	})
}

// SetPermissionsAPI updates an entity's visibility mode and permission grants.
// Owner only.
// PUT /campaigns/:id/entities/:eid/permissions
func (h *Handler) SetPermissionsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	ctx := c.Request().Context()

	entity, err := h.service.GetByID(ctx, entityID)
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	var req setPermissionsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.SetEntityPermissions(ctx, entityID, SetPermissionsInput(req)); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetMembersAPI returns campaign members as JSON for the permission user picker.
// Owner only.
// GET /campaigns/:id/entities/members
func (h *Handler) GetMembersAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if h.memberLister == nil {
		return c.JSON(http.StatusOK, []any{})
	}

	members, err := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading members: %w", err))
	}

	type memberJSON struct {
		UserID      string `json:"user_id"`
		DisplayName string `json:"display_name"`
		Role        int    `json:"role"`
	}

	result := make([]memberJSON, 0, len(members))
	for _, m := range members {
		result = append(result, memberJSON{
			UserID:      m.UserID,
			DisplayName: m.DisplayName,
			Role:        int(m.Role),
		})
	}

	return c.JSON(http.StatusOK, result)
}

// --- Entity Type CRUD ---

// --- Auto-Linking API ---

// entityNamesCacheTTL is how long entity names are cached in Redis.
const entityNamesCacheTTL = 5 * time.Minute

// EntityNamesAPI returns a lightweight list of all visible entity names for
// auto-linking in the editor. Results are cached in Redis for 5 minutes.
// GET /campaigns/:id/entity-names
func (h *Handler) EntityNamesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	ctx := c.Request().Context()
	role := int(cc.MemberRole)
	userID := auth.GetUserID(c)
	cacheKey := fmt.Sprintf("entity-names:%s:%d:%s", cc.Campaign.ID, role, userID)

	// Try Redis cache first.
	if h.cache != nil {
		cached, err := h.cache.Get(ctx, cacheKey).Result()
		if err == nil {
			c.Response().Header().Set("Content-Type", "application/json")
			c.Response().Header().Set("X-Cache", "HIT")
			return c.String(http.StatusOK, cached)
		}
	}

	names, err := h.service.ListEntityNames(ctx, cc.Campaign.ID, role, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("list entity names: %w", err))
	}
	if names == nil {
		names = []EntityNameEntry{}
	}

	result, err := json.Marshal(map[string]any{"names": names})
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal entity names: %w", err))
	}

	// Cache in Redis.
	if h.cache != nil {
		if err := h.cache.Set(ctx, cacheKey, string(result), entityNamesCacheTTL).Err(); err != nil {
			slog.Error("failed to cache entity names", slog.Any("error", err))
		}
	}

	c.Response().Header().Set("X-Cache", "MISS")
	return c.JSONBlob(http.StatusOK, result)
}

// EntityTypesPage renders the entity type management page.
// GET /campaigns/:id/entity-types
func (h *Handler) EntityTypesPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityTypes, err := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Get entity counts per type so we can show usage and protect used types.
	role := int(cc.MemberRole)
	counts, _ := h.service.CountByType(c.Request().Context(), cc.Campaign.ID, role, auth.GetUserID(c))

	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK,
			EntityTypeListContent(cc, entityTypes, counts, csrfToken))
	}
	return middleware.Render(c, http.StatusOK,
		EntityTypesManagePage(cc, entityTypes, counts, csrfToken, ""))
}

// CreateEntityType processes the entity type creation form.
// POST /campaigns/:id/entity-types
func (h *Handler) CreateEntityType(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req CreateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := CreateEntityTypeInput(req)

	et, err := h.service.CreateEntityType(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		counts, _ := h.service.CountByType(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), auth.GetUserID(c))
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to create entity type"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK,
			EntityTypesManagePage(cc, entityTypes, counts, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityTypeCreated, strconv.Itoa(et.ID), et.Name)

	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entity-types"
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectURL)
}

// UpdateEntityTypeAPI updates an entity type.
// PUT /campaigns/:id/entity-types/:etid
func (h *Handler) UpdateEntityTypeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	// IDOR protection: verify entity type belongs to this campaign.
	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body UpdateEntityTypeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	input := UpdateEntityTypeInput(body)

	updated, err := h.service.UpdateEntityType(c.Request().Context(), etID, input)
	if err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityTypeUpdated, strconv.Itoa(etID), updated.Name)

	return c.JSON(http.StatusOK, updated)
}

// DeleteEntityType removes an entity type.
// DELETE /campaigns/:id/entity-types/:etid
func (h *Handler) DeleteEntityType(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	// IDOR protection: verify entity type belongs to this campaign.
	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	if err := h.service.DeleteEntityType(c.Request().Context(), etID); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityTypeDeleted, strconv.Itoa(etID), et.Name)

	// HTMX: redirect to entity types page after deletion.
	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entity-types"
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Template Editor ---

// BlockTypesAPI returns the available block types for the template editor,
// filtered by which addons are enabled for the current campaign.
// Also includes extension widget blocks from enabled extensions.
// GET /campaigns/:id/entity-types/block-types
func (h *Handler) BlockTypesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if h.blockRegistry == nil {
		return c.JSON(http.StatusOK, []BlockMeta{})
	}

	types := h.blockRegistry.TypesForCampaign(c.Request().Context(), cc.Campaign.ID, h.addonSvc)

	// Append extension widget blocks from enabled extensions.
	if h.widgetBlockLister != nil {
		extWidgets := h.widgetBlockLister.GetWidgetBlockMetas(c.Request().Context(), cc.Campaign.ID)
		types = append(types, extWidgets...)
	}

	return c.JSON(http.StatusOK, types)
}

// TemplateEditor renders the visual template editor for an entity type.
// GET /campaigns/:id/entity-types/:etid/template
// Kept for backward compatibility; redirects are not needed since the
// config page embeds the same template-editor widget.
func (h *Handler) TemplateEditor(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, TemplateEditorPage(cc, et, csrfToken))
}

// EntityTypeConfig renders the unified entity type configuration page.
// Tabs: Layout, Attributes, Dashboard, Nav Panel.
// GET /campaigns/:id/entity-types/:etid/config
func (h *Handler) EntityTypeConfig(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, EntityTypeConfigPage(cc, et, csrfToken))
}

// EntityTypeCustomizeFragment returns an HTMX fragment for the Customization
// Hub's Categories tab. Contains identity settings, attribute field editor,
// and category dashboard editor for a single entity type.
// GET /campaigns/:id/entity-types/:etid/customize
func (h *Handler) EntityTypeCustomizeFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, EntityTypeCustomizeFragmentTmpl(cc, et, csrfToken))
}

// EntityTypeAttributesFragment returns an HTMX fragment for the Customization
// Hub's Extensions tab. Contains just the attribute field editor for a single
// entity type.
// GET /campaigns/:id/entity-types/:etid/attributes-fragment
func (h *Handler) EntityTypeAttributesFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, EntityTypeAttributesFragmentTmpl(cc, et, csrfToken))
}

// --- Layout API ---

// GetEntityTypeLayout returns the entity type's layout as JSON.
// GET /campaigns/:id/entity-types/:etid/layout
func (h *Handler) GetEntityTypeLayout(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: ensure entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"layout": et.Layout,
		"fields": et.Fields,
	})
}

// UpdateEntityTypeLayout saves the entity type's profile layout.
// PUT /campaigns/:id/entity-types/:etid/layout
func (h *Handler) UpdateEntityTypeLayout(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: ensure entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body struct {
		Layout EntityTypeLayout `json:"layout"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateEntityTypeLayout(c.Request().Context(), etID, body.Layout); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEntityTypeColor saves the entity type's display color.
// PUT /campaigns/:id/entity-types/:etid/color
func (h *Handler) UpdateEntityTypeColor(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: ensure entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body struct {
		Color string `json:"color"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateEntityTypeColor(c.Request().Context(), etID, body.Color); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEntityTypeDashboard updates the category dashboard description and pinned pages
// (PUT /campaigns/:id/entity-types/:etid/dashboard).
func (h *Handler) UpdateEntityTypeDashboard(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: ensure entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body struct {
		Description     *string  `json:"description"`
		PinnedEntityIDs []string `json:"pinned_entity_ids"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateEntityTypeDashboard(c.Request().Context(), etID, body.Description, body.PinnedEntityIDs); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetCategoryDashboardLayout returns the dashboard layout JSON for an entity type
// (GET /campaigns/:id/entity-types/:etid/dashboard-layout).
func (h *Handler) GetCategoryDashboardLayout(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: ensure entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	layoutJSON, err := h.service.GetCategoryDashboardLayout(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// Return null JSON when no custom layout is saved.
	if layoutJSON == nil {
		return c.JSONBlob(http.StatusOK, []byte("null"))
	}
	return c.JSONBlob(http.StatusOK, []byte(*layoutJSON))
}

// UpdateCategoryDashboardLayout saves a custom dashboard layout for an entity type
// (PUT /campaigns/:id/entity-types/:etid/dashboard-layout).
func (h *Handler) UpdateCategoryDashboardLayout(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	// Read raw JSON body.
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20)) // 1 MB max
	if err != nil {
		return apperror.NewBadRequest("failed to read request body")
	}

	layoutJSON := string(body)
	if err := h.service.UpdateCategoryDashboardLayout(c.Request().Context(), etID, layoutJSON); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ResetCategoryDashboardLayout removes the custom dashboard layout for an entity type,
// reverting to the hardcoded default (DELETE /campaigns/:id/entity-types/:etid/dashboard-layout).
func (h *Handler) ResetCategoryDashboardLayout(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	if err := h.service.ResetCategoryDashboardLayout(c.Request().Context(), etID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Helpers ---

// parseFieldsFromForm collects field_<key> form parameters and builds a
// map of field values.
func (h *Handler) parseFieldsFromForm(c echo.Context, campaignID string, entityTypeID int) map[string]any {
	fieldsData := make(map[string]any)

	et, err := h.service.GetEntityTypeByID(c.Request().Context(), entityTypeID)
	if err != nil {
		return fieldsData
	}

	for _, fd := range et.Fields {
		key := "field_" + fd.Key
		value := c.FormValue(key)
		if value != "" {
			fieldsData[fd.Key] = value
		}
	}

	return fieldsData
}
