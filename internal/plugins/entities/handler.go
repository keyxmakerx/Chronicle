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
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
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

// SystemSearcher provides game system search results for the quick
// search popup. Implemented by systems.SystemSearchAdapter.
type SystemSearcher interface {
	SearchSystemContent(ctx context.Context, campaignID, query string) ([]map[string]string, error)
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
	systemSearcher     SystemSearcher
	memberLister       MemberLister
	groupLister        GroupLister
	widgetBlockLister  WidgetBlockLister
	contentTemplateSvc ContentTemplateService
	sidebarNodeRepo    SidebarNodeRepository
	favoriteRepo       FavoriteRepository
	savedFilterRepo    SavedFilterRepository
	blockRegistry      *BlockRegistry
	cache              *redis.Client
}

// NewHandler creates a new entity handler.
func NewHandler(service EntityService) *Handler {
	return &Handler{service: service}
}

// SetContentTemplateService sets the content template service for applying
// templates during entity creation.
func (h *Handler) SetContentTemplateService(svc ContentTemplateService) {
	h.contentTemplateSvc = svc
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

// isAddonEnabled checks whether a specific addon is enabled for the campaign.
// Fails open (returns true) if the addon service is not wired or on DB errors,
// matching the fail-open convention used by RequireAddon middleware.
func (h *Handler) isAddonEnabled(ctx context.Context, campaignID, slug string) bool {
	if h.addonSvc == nil {
		return true
	}
	enabled, err := h.addonSvc.IsEnabledForCampaign(ctx, campaignID, slug)
	return err != nil || enabled
}

// toAnyMaps converts []map[string]string to []map[string]any for JSON
// serialization alongside entity results that include non-string fields.
func toAnyMaps(src []map[string]string) []map[string]any {
	out := make([]map[string]any, len(src))
	for i, m := range src {
		a := make(map[string]any, len(m))
		for k, v := range m {
			a[k] = v
		}
		out[i] = a
	}
	return out
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

// SetSystemSearcher sets the system searcher for quick search results.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetSystemSearcher(ms SystemSearcher) {
	h.systemSearcher = ms
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

// SetSidebarNodeRepo sets the sidebar node repository for folder operations.
func (h *Handler) SetSidebarNodeRepo(repo SidebarNodeRepository) {
	h.sidebarNodeRepo = repo
}

// SetFavoriteRepo sets the favorite repository for bookmark operations.
func (h *Handler) SetFavoriteRepo(repo FavoriteRepository) {
	h.favoriteRepo = repo
}

// SetSavedFilterRepo sets the saved filter repository for tag filter presets.
func (h *Handler) SetSavedFilterRepo(repo SavedFilterRepository) {
	h.savedFilterRepo = repo
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

	role := cc.VisibilityRole()
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
		includeDmOnly := cc != nil && (cc.MemberRole >= campaigns.RoleScribe || cc.IsDmGranted)
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

	// Apply campaign default visibility if the user didn't explicitly set private.
	isPrivate := req.IsPrivate
	if !isPrivate {
		defaultVis := cc.Campaign.ParseSettings().DefaultVisibility
		if defaultVis == "dm_only" || defaultVis == "private" {
			isPrivate = true
		}
	}

	input := CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: req.EntityTypeID,
		TypeLabel:    req.TypeLabel,
		ParentID:     req.ParentID,
		IsPrivate:    isPrivate,
		FieldsData:   fieldsData,
	}

	entity, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to create entity")
		return middleware.Render(c, http.StatusOK, EntityNewPage(cc, entityTypes, req.EntityTypeID, nil, csrfToken, errMsg))
	}

	// Apply content template if one was selected.
	if req.TemplateID > 0 && h.contentTemplateSvc != nil {
		tmpl, tmplErr := h.contentTemplateSvc.GetByID(c.Request().Context(), req.TemplateID)
		if tmplErr == nil && tmpl.ContentJSON != "" {
			// IDOR check: template must belong to this campaign or be global.
			if tmpl.CampaignID != nil && *tmpl.CampaignID != cc.Campaign.ID {
				slog.Warn("content template IDOR blocked",
					slog.Int("template_id", req.TemplateID),
					slog.String("campaign_id", cc.Campaign.ID),
				)
			} else if err := h.service.UpdateEntry(c.Request().Context(), entity.ID, tmpl.ContentJSON, tmpl.ContentHTML); err != nil {
				slog.Warn("failed to apply content template",
					slog.Int("template_id", req.TemplateID),
					slog.String("entity_id", entity.ID),
					slog.Any("error", err),
				)
			}
		}
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, entity.ID, entity.Name)

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/entities/"+entity.ID)
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

	// Fetch ancestor chain for breadcrumbs and children for sub-page listing.
	// Backlinks load asynchronously via HTMX to keep page load fast.
	ancestors, _ := h.service.GetAncestors(c.Request().Context(), entity.ID)
	children, _ := h.service.GetChildren(c.Request().Context(), entity.ID, int(cc.MemberRole), userID)

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

	// Override the active path to the entity's category URL so the sidebar
	// stays drilled into the correct category (e.g., /campaigns/{id}/characters)
	// instead of collapsing because /entities/{eid} doesn't match any category.
	categoryPath := fmt.Sprintf("/campaigns/%s/%s", cc.Campaign.ID, entityType.Slug)
	ctx := layouts.SetActivePath(c.Request().Context(), categoryPath)
	c.SetRequest(c.Request().WithContext(ctx))

	return middleware.Render(c, http.StatusOK, EntityShowPage(cc, entity, entityType, ancestors, children, showAttributes, showCalendar, csrfToken))
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
	return middleware.HTMXRedirect(c, fmt.Sprintf("/campaigns/%s/entities/%s/edit", cc.Campaign.ID, clone.ID))
}

// EditForm redirects to the entity show page. The separate edit form has been
// replaced by inline editing on the show page (metadata panel + rich text editor).
// Kept as a redirect for backwards compatibility with bookmarks and old links.
func (h *Handler) EditForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/entities/%s", cc.Campaign.ID, entityID))
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
		Name:              req.Name,
		TypeLabel:         req.TypeLabel,
		ParentID:          req.ParentID,
		IsPrivate:         req.IsPrivate,
		Entry:             req.Entry,
		FieldsData:        fieldsData,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}

	_, err = h.service.Update(c.Request().Context(), entityID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		entityType, _ := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to update entity")
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

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/entities/"+entityID)
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

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/entities")
}

// SearchPageHandler renders the dedicated search page with real-time filtering.
// GET /campaigns/:id/search
func (h *Handler) SearchPageHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityTypes, err := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		entityTypes = nil
	}

	return middleware.Render(c, http.StatusOK, SearchPage(cc, entityTypes))
}

// SearchAPI handles entity search requests (GET /campaigns/:id/entities/search).
// Returns HTML fragments for HTMX callers and JSON for API callers (e.g., the
// @mention widget) based on the Accept header.
func (h *Handler) SearchAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)
	query := c.QueryParam("q")
	typeID, _ := strconv.Atoi(c.QueryParam("type"))

	opts := DefaultListOptions()
	opts.PerPage = 20

	// Sidebar drill panel uses chunked loading: first 50, then more on demand.
	isSidebar := c.QueryParam("sidebar") == "1"
	if isSidebar {
		opts.PerPage = 50
	}

	// Tag filtering: comma-separated tag slugs (AND logic).
	if tags := c.QueryParam("tags"); tags != "" {
		opts.TagSlugs = strings.Split(tags, ",")
	}

	// Pagination: allow callers to request a specific page.
	if p, _ := strconv.Atoi(c.QueryParam("page")); p > 1 {
		opts.Page = p
	}

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
		// Batch-fetch tags for all entities in the result set.
		var tagMap map[string][]EntityTagInfo
		if h.tagFetcher != nil && len(results) > 0 {
			ids := make([]string, len(results))
			for i, e := range results {
				ids[i] = e.ID
			}
			includeDM := role >= int(campaigns.RoleScribe)
			tagMap, _ = h.tagFetcher.GetEntityTagsBatch(c.Request().Context(), ids, includeDM)
		}

		items := make([]map[string]any, len(results))
		for i, e := range results {
			tags := tagMap[e.ID]
			if tags == nil {
				tags = []EntityTagInfo{}
			}
			item := map[string]any{
				"id":         e.ID,
				"name":       e.Name,
				"type_name":  e.TypeName,
				"type_icon":  e.TypeIcon,
				"type_color": e.TypeColor,
				"url":        fmt.Sprintf("/campaigns/%s/entities/%s", cc.Campaign.ID, e.ID),
				"tags":       tags,
				"sort_order": e.SortOrder,
			}
			if e.ParentID != nil {
				item["parent_id"] = *e.ParentID
			}
			items[i] = item
		}
		// Append cross-plugin search results from registered searchers.
		// Each searcher is gated by its addon being enabled for the campaign,
		// so disabled features don't leak results into search.
		ctx := c.Request().Context()
		if h.timelineSearcher != nil && query != "" && h.isAddonEnabled(ctx, cc.Campaign.ID, "timeline") {
			if tlResults, err := h.timelineSearcher.SearchTimelines(
				ctx, cc.Campaign.ID, query, role,
			); err == nil {
				items = append(items, toAnyMaps(tlResults)...)
				total += len(tlResults)
			}
		}
		if h.mapSearcher != nil && query != "" && h.isAddonEnabled(ctx, cc.Campaign.ID, "maps") {
			if mapResults, err := h.mapSearcher.SearchMaps(
				ctx, cc.Campaign.ID, query,
			); err == nil {
				items = append(items, toAnyMaps(mapResults)...)
				total += len(mapResults)
			}
		}
		if h.calendarSearcher != nil && query != "" && h.isAddonEnabled(ctx, cc.Campaign.ID, "calendar") {
			if calResults, err := h.calendarSearcher.SearchCalendarEvents(
				ctx, cc.Campaign.ID, query, role,
			); err == nil {
				items = append(items, toAnyMaps(calResults)...)
				total += len(calResults)
			}
		}
		if h.sessionSearcher != nil && query != "" && h.isAddonEnabled(ctx, cc.Campaign.ID, "calendar") {
			if sessResults, err := h.sessionSearcher.SearchSessions(
				ctx, cc.Campaign.ID, query,
			); err == nil {
				items = append(items, toAnyMaps(sessResults)...)
				total += len(sessResults)
			}
		}
		if h.systemSearcher != nil && query != "" {
			if modResults, err := h.systemSearcher.SearchSystemContent(
				c.Request().Context(), cc.Campaign.ID, query,
			); err == nil {
				items = append(items, toAnyMaps(modResults)...)
				total += len(modResults)
			}
		}
		return c.JSON(http.StatusOK, map[string]any{"results": items, "total": total})
	}

	// Sidebar mode returns a compact list for the sidebar drill panel.
	// Hidden entities are filtered out for players but shown dimmed for owners.
	if c.QueryParam("sidebar") == "1" {
		sidebarCfg := cc.Campaign.ParseSidebarConfig()
		hiddenIDs := make(map[string]bool, len(sidebarCfg.HiddenEntityIDs))
		for _, id := range sidebarCfg.HiddenEntityIDs {
			hiddenIDs[id] = true
		}

		// Players don't see individually hidden entities at all.
		if cc.MemberRole < campaigns.RoleScribe && len(hiddenIDs) > 0 {
			filtered := results[:0]
			for _, e := range results {
				if !hiddenIDs[e.ID] {
					filtered = append(filtered, e)
				}
			}
			results = filtered
			total = len(results)
		}

		// Fetch sidebar folder nodes for this category.
		var nodes []SidebarNode
		if h.sidebarNodeRepo != nil && typeID > 0 {
			nodes, _ = h.sidebarNodeRepo.ListByType(c.Request().Context(), cc.Campaign.ID, typeID)
		}

		return middleware.Render(c, http.StatusOK, SidebarEntityList(results, nodes, total, cc, hiddenIDs))
	}

	return middleware.Render(c, http.StatusOK, SearchResultsFragment(results, total, cc))
}

// --- Reorder API (sidebar tree drag-and-drop) ---

// ReorderAPI updates an entity's parent and sort order for sidebar tree reordering.
// PUT /campaigns/:id/entities/:eid/reorder
func (h *Handler) ReorderAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewForbidden("no campaign context")
	}
	entityID := c.Param("eid")

	var input struct {
		ParentID     *string `json:"parent_id"`
		ParentNodeID *string `json:"parent_node_id"`
		SortOrder    *int    `json:"sort_order"`
	}
	if err := c.Bind(&input); err != nil {
		return apperror.NewValidation("invalid request body")
	}

	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	if err := h.service.ReorderEntity(c.Request().Context(), cc.Campaign.ID, entityID, input.ParentID, input.ParentNodeID, sortOrder); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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

// --- Bulk Operations API (multi-select in reorg mode) ---

// BulkMoveAPI reparents multiple entities under a target parent (entity or folder node).
// POST /campaigns/:id/entities/bulk-move
func (h *Handler) BulkMoveAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req struct {
		EntityIDs    []string `json:"entity_ids"`
		ParentID     *string  `json:"parent_id"`
		ParentNodeID *string  `json:"parent_node_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if len(req.EntityIDs) == 0 {
		return apperror.NewBadRequest("no entities selected")
	}

	ctx := c.Request().Context()
	for i, eid := range req.EntityIDs {
		if err := h.service.ReorderEntity(ctx, cc.Campaign.ID, eid, req.ParentID, req.ParentNodeID, i); err != nil {
			slog.Warn("bulk move: entity failed",
				slog.String("entity_id", eid),
				slog.Any("error", err),
			)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"moved":  len(req.EntityIDs),
	})
}

// --- Favorites API ---

// ToggleFavoriteAPI adds or removes an entity from the user's favorites.
// POST /campaigns/:id/entities/:eid/favorite
func (h *Handler) ToggleFavoriteAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	userID := auth.GetUserID(c)

	favorited, err := h.favoriteRepo.Toggle(c.Request().Context(), userID, entityID, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("toggling favorite: %w", err))
	}

	return c.JSON(http.StatusOK, map[string]bool{"favorited": favorited})
}

// ListFavoritesAPI returns the user's favorites for a campaign as JSON.
// GET /campaigns/:id/favorites
func (h *Handler) ListFavoritesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	items, err := h.favoriteRepo.List(c.Request().Context(), userID, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing favorites: %w", err))
	}
	if items == nil {
		items = []FavoriteItem{}
	}

	return c.JSON(http.StatusOK, items)
}

// FavoriteIDsAPI returns a set of entity IDs favorited by the user.
// Used by the sidebar to mark starred entities.
// GET /campaigns/:id/favorite-ids
func (h *Handler) FavoriteIDsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	ids, err := h.favoriteRepo.ListIDs(c.Request().Context(), userID, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing favorite IDs: %w", err))
	}

	// Convert to array for JSON.
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}

	return c.JSON(http.StatusOK, result)
}

// --- Saved Filter Presets API ---

// ListSavedFiltersAPI returns the user's saved tag filter presets.
// GET /campaigns/:id/saved-filters
func (h *Handler) ListSavedFiltersAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	filters, err := h.savedFilterRepo.List(c.Request().Context(), userID, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing saved filters: %w", err))
	}
	if filters == nil {
		filters = []SavedFilter{}
	}
	return c.JSON(http.StatusOK, filters)
}

// CreateSavedFilterAPI creates a new saved tag filter preset.
// POST /campaigns/:id/saved-filters
func (h *Handler) CreateSavedFilterAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req struct {
		Name         string   `json:"name"`
		TagSlugs     []string `json:"tag_slugs"`
		EntityTypeID *int     `json:"entity_type_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if len(req.TagSlugs) == 0 {
		return apperror.NewBadRequest("at least one tag is required")
	}

	userID := auth.GetUserID(c)
	filter := &SavedFilter{
		ID:           generateFilterID(),
		UserID:       userID,
		CampaignID:   cc.Campaign.ID,
		EntityTypeID: req.EntityTypeID,
		Name:         strings.TrimSpace(req.Name),
		TagSlugs:     req.TagSlugs,
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.savedFilterRepo.Create(c.Request().Context(), filter); err != nil {
		return apperror.NewInternal(fmt.Errorf("creating saved filter: %w", err))
	}

	return c.JSON(http.StatusCreated, filter)
}

// DeleteSavedFilterAPI deletes a saved filter preset.
// DELETE /campaigns/:id/saved-filters/:fid
func (h *Handler) DeleteSavedFilterAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	filterID := c.Param("fid")

	if err := h.savedFilterRepo.Delete(c.Request().Context(), filterID, userID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Sidebar Node API (pure folders) ---

// CreateSidebarNodeAPI creates a new sidebar folder node.
// POST /campaigns/:id/sidebar-nodes
func (h *Handler) CreateSidebarNodeAPI(c echo.Context) error {
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

	node := &SidebarNode{
		ID:           generateNodeID(),
		CampaignID:   cc.Campaign.ID,
		EntityTypeID: req.EntityTypeID,
		Name:         strings.TrimSpace(req.Name),
		SortOrder:    0,
		NodeType:     "folder",
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.sidebarNodeRepo.Create(c.Request().Context(), node); err != nil {
		return apperror.NewInternal(fmt.Errorf("creating sidebar node: %w", err))
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"id":   node.ID,
		"name": node.Name,
	})
}

// DeleteSidebarNodeAPI deletes a sidebar folder node. Children are
// reparented to root.
// DELETE /campaigns/:id/sidebar-nodes/:nid
func (h *Handler) DeleteSidebarNodeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	nodeID := c.Param("nid")
	node, err := h.sidebarNodeRepo.FindByID(c.Request().Context(), nodeID)
	if err != nil {
		return err
	}
	if node.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("sidebar node not found")
	}

	if err := h.sidebarNodeRepo.Delete(c.Request().Context(), nodeID); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting sidebar node: %w", err))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// RenameSidebarNodeAPI renames a sidebar folder node.
// PUT /campaigns/:id/sidebar-nodes/:nid
func (h *Handler) RenameSidebarNodeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	nodeID := c.Param("nid")
	node, err := h.sidebarNodeRepo.FindByID(c.Request().Context(), nodeID)
	if err != nil {
		return err
	}
	if node.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("sidebar node not found")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}

	node.Name = strings.TrimSpace(req.Name)
	if err := h.sidebarNodeRepo.Update(c.Request().Context(), node); err != nil {
		return apperror.NewInternal(fmt.Errorf("renaming sidebar node: %w", err))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ReorderSidebarNodeAPI updates a sidebar node's parent and sort order.
// PUT /campaigns/:id/sidebar-nodes/:nid/reorder
func (h *Handler) ReorderSidebarNodeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	nodeID := c.Param("nid")
	node, err := h.sidebarNodeRepo.FindByID(c.Request().Context(), nodeID)
	if err != nil {
		return err
	}
	if node.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("sidebar node not found")
	}

	var req struct {
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.sidebarNodeRepo.UpdateParent(c.Request().Context(), nodeID, cc.Campaign.ID, req.ParentID); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating node parent: %w", err))
	}
	if req.SortOrder != nil {
		if err := h.sidebarNodeRepo.UpdateSortOrder(c.Request().Context(), nodeID, cc.Campaign.ID, *req.SortOrder); err != nil {
			return apperror.NewInternal(fmt.Errorf("updating node sort order: %w", err))
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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
			"id":          t.ID,
			"name":        t.Name,
			"name_plural": t.NamePlural,
			"slug":        t.Slug,
			"icon":        t.Icon,
			"color":       t.Color,
			"sort_order":  t.SortOrder,
			"enabled":     t.Enabled,
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

// --- Player Notes API ---

// GetPlayerNotes returns the entity's player-facing notes as JSON.
// GET /campaigns/:id/entities/:eid/player-notes
func (h *Handler) GetPlayerNotes(c echo.Context) error {
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

	return c.JSON(http.StatusOK, map[string]any{
		"player_notes":      entity.PlayerNotes,
		"player_notes_html": entity.PlayerNotesHTML,
	})
}

// UpdatePlayerNotesAPI updates only the entity's player-facing notes.
// PUT /campaigns/:id/entities/:eid/player-notes
func (h *Handler) UpdatePlayerNotesAPI(c echo.Context) error {
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
		PlayerNotes     string `json:"player_notes"`
		PlayerNotesHTML string `json:"player_notes_html"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdatePlayerNotes(c.Request().Context(), entityID, body.PlayerNotes, body.PlayerNotesHTML); err != nil {
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

// UpdateCoverImageAPI updates the entity's cover/banner image path.
// PUT /campaigns/:id/entities/:eid/cover-image
func (h *Handler) UpdateCoverImageAPI(c echo.Context) error {
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

	if err := h.service.UpdateCoverImage(c.Request().Context(), entityID, body.ImagePath); err != nil {
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

// UpdateMetadataAPI saves entity metadata (name, descriptor, parent, privacy)
// without touching entry content or field values. Used by the inline metadata
// panel on the entity show page.
// PUT /campaigns/:id/entities/:eid/metadata
func (h *Handler) UpdateMetadataAPI(c echo.Context) error {
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

	var req struct {
		Name      string `json:"name"`
		TypeLabel string `json:"type_label"`
		ParentID  string `json:"parent_id"`
		IsPrivate bool   `json:"is_private"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	// Use the existing Update service with empty entry/fields so they stay unchanged.
	input := UpdateEntityInput{
		Name:      req.Name,
		TypeLabel: req.TypeLabel,
		ParentID:  req.ParentID,
		IsPrivate: req.IsPrivate,
	}

	updated, err := h.service.Update(c.Request().Context(), entityID, input)
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityUpdated, entityID, updated.Name)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"name":   updated.Name,
		"slug":   updated.Slug,
	})
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
	role := cc.VisibilityRole()
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
	role := cc.VisibilityRole()
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

	input := CreateEntityTypeInput{
		Name:         req.Name,
		NamePlural:   req.NamePlural,
		Icon:         req.Icon,
		Color:        req.Color,
		ParentTypeID: req.ParentTypeID,
	}

	et, err := h.service.CreateEntityType(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		role := cc.VisibilityRole()
		counts, _ := h.service.CountByType(c.Request().Context(), cc.Campaign.ID, role, auth.GetUserID(c))
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to create entity type")
		// Return partial for HTMX requests so the swap target (#entity-type-list) gets correct content.
		// Use HX-Trigger to show a toast notification with the error message.
		if middleware.IsHTMX(c) {
			c.Response().Header().Set("HX-Retarget", "#entity-type-list")
			c.Response().Header().Set("HX-Reswap", "innerHTML")
			triggerData := map[string]any{
				"chronicle:notify": map[string]string{"message": errMsg, "type": "error"},
			}
			if triggerJSON, err := json.Marshal(triggerData); err == nil {
				c.Response().Header().Set("HX-Trigger", string(triggerJSON))
			}
			return middleware.Render(c, http.StatusOK,
				EntityTypeListContent(cc, entityTypes, counts, csrfToken))
		}
		return middleware.Render(c, http.StatusOK,
			EntityTypesManagePage(cc, entityTypes, counts, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityTypeCreated, strconv.Itoa(et.ID), et.Name)

	// JSON response for API callers (Layout Studio, sidebar quick-create).
	if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
		return c.JSON(http.StatusCreated, map[string]any{
			"id":             et.ID,
			"name":           et.Name,
			"name_plural":    et.NamePlural,
			"slug":           et.Slug,
			"icon":           et.Icon,
			"color":          et.Color,
			"parent_type_id": et.ParentTypeID,
		})
	}

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/entity-types")
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

// BlockTypesAPI returns the available block types for the layout editor,
// filtered by which addons are enabled for the current campaign and
// optionally by editor context (?context=dashboard|template).
// GET /campaigns/:id/entity-types/block-types
func (h *Handler) BlockTypesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if h.blockRegistry == nil {
		return c.JSON(http.StatusOK, []BlockMeta{})
	}

	editorCtx := c.QueryParam("context") // "dashboard", "template", or "" (all)
	types := h.blockRegistry.TypesForCampaignAndContext(c.Request().Context(), cc.Campaign.ID, h.addonSvc, editorCtx)

	// Append extension widget blocks from enabled extensions (template context only).
	if h.widgetBlockLister != nil && (editorCtx == "" || editorCtx == "template") {
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

// --- Entity Aliases API ---

// GetAliasesAPI returns all aliases for an entity.
// GET /campaigns/:id/entities/:eid/aliases
func (h *Handler) GetAliasesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	aliases, err := h.service.GetAliases(c.Request().Context(), entityID)
	if err != nil {
		return err
	}
	if aliases == nil {
		aliases = []EntityAlias{}
	}
	return c.JSON(http.StatusOK, map[string]any{"aliases": aliases})
}

// SetAliasesAPI replaces all aliases for an entity.
// PUT /campaigns/:id/entities/:eid/aliases
func (h *Handler) SetAliasesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	var input SetAliasesInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.SetAliases(c.Request().Context(), entityID, input.Aliases); err != nil {
		return err
	}

	// Invalidate entity names cache for the campaign so auto-linker picks up changes.
	if h.cache != nil {
		pattern := fmt.Sprintf("entity-names:%s:*", cc.Campaign.ID)
		h.invalidateCachePattern(c.Request().Context(), pattern)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Backlinks API ---

// backlinksCacheTTL is how long backlink results are cached in Redis.
const backlinksCacheTTL = 5 * time.Minute

// BacklinksFragment returns the "Referenced by" section as an HTMX fragment
// or JSON. Results are cached in Redis for 5 minutes.
// GET /campaigns/:id/entities/:eid/backlinks
func (h *Handler) BacklinksFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	ctx := c.Request().Context()
	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)

	// Try Redis cache for JSON response.
	cacheKey := fmt.Sprintf("backlinks:%s:%d:%s", entityID, role, userID)
	var entries []BacklinkEntry

	if h.cache != nil {
		cached, err := h.cache.Get(ctx, cacheKey).Result()
		if err == nil {
			if err := json.Unmarshal([]byte(cached), &entries); err == nil {
				if isHTMX(c) {
					return middleware.Render(c, http.StatusOK, blockBacklinks(cc, entries))
				}
				return c.JSON(http.StatusOK, map[string]any{"backlinks": entries})
			}
		}
	}

	entries, err := h.service.GetBacklinksWithSnippets(ctx, entityID, role, userID)
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []BacklinkEntry{}
	}

	// Cache in Redis.
	if h.cache != nil {
		if data, err := json.Marshal(entries); err == nil {
			if err := h.cache.Set(ctx, cacheKey, string(data), backlinksCacheTTL).Err(); err != nil {
				slog.Error("failed to cache backlinks", slog.Any("error", err))
			}
		}
	}

	if isHTMX(c) {
		return middleware.Render(c, http.StatusOK, blockBacklinks(cc, entries))
	}
	return c.JSON(http.StatusOK, map[string]any{"backlinks": entries})
}

// isHTMX returns true if the request was sent by HTMX.
func isHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") != ""
}

// invalidateCachePattern deletes all Redis keys matching a glob pattern.
// Used for cache invalidation when entity data changes.
func (h *Handler) invalidateCachePattern(ctx context.Context, pattern string) {
	iter := h.cache.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		h.cache.Del(ctx, iter.Val())
	}
	if err := iter.Err(); err != nil {
		slog.Error("failed to invalidate cache", slog.String("pattern", pattern), slog.Any("error", err))
	}
}
