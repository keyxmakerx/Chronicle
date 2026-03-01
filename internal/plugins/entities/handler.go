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

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// EntityTagFetcher retrieves tags for entities in batch. Defined here to avoid
// importing the tags widget package, keeping plugins loosely coupled via interfaces.
type EntityTagFetcher interface {
	GetEntityTagsBatch(ctx context.Context, entityIDs []string) (map[string][]EntityTagInfo, error)
}

// AddonChecker is a narrow interface for checking whether an addon is enabled
// for a campaign. Satisfied by addons.AddonService.
type AddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// Handler handles HTTP requests for entity operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service    EntityService
	auditSvc   audit.AuditService
	tagFetcher EntityTagFetcher
	addonSvc   AddonChecker
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

// SetTagFetcher sets the tag fetcher for populating entity tags in list views.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetTagFetcher(f EntityTagFetcher) {
	h.tagFetcher = f
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
		return apperror.NewInternal(nil)
	}

	role := int(cc.MemberRole)
	campaignID := cc.Campaign.ID

	page, _ := strconv.Atoi(c.QueryParam("page"))
	opts := DefaultListOptions()
	if page > 0 {
		opts.Page = page
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

	// Fetch entity types for sidebar filter and counts.
	entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), campaignID)
	counts, _ := h.service.CountByType(c.Request().Context(), campaignID, role)

	entities, total, err := h.service.List(c.Request().Context(), campaignID, typeID, role, opts)
	if err != nil {
		return err
	}

	// Batch-fetch tags for all entities in the list to show chips on cards.
	if h.tagFetcher != nil && len(entities) > 0 {
		entityIDs := make([]string, len(entities))
		for i := range entities {
			entityIDs[i] = entities[i].ID
		}
		if tagsMap, err := h.tagFetcher.GetEntityTagsBatch(c.Request().Context(), entityIDs); err == nil {
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
	}

	var req CreateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
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

	// Privacy check: private entities return 404 for Players.
	if entity.IsPrivate && cc.MemberRole < campaigns.RoleScribe {
		return apperror.NewNotFound("entity not found")
	}

	entityType, err := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("get entity type %d: %w", entity.EntityTypeID, err))
	}

	// Fetch ancestor chain for breadcrumbs, children for sub-page listing,
	// and backlinks for "Referenced by" section.
	ancestors, _ := h.service.GetAncestors(c.Request().Context(), entity.ID)
	children, _ := h.service.GetChildren(c.Request().Context(), entity.ID, int(cc.MemberRole))
	backlinks, _ := h.service.GetBacklinks(c.Request().Context(), entity.ID, int(cc.MemberRole))

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

// EditForm renders the entity edit form (GET /campaigns/:id/entities/:eid/edit).
func (h *Handler) EditForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
	}

	role := int(cc.MemberRole)
	query := c.QueryParam("q")
	typeID, _ := strconv.Atoi(c.QueryParam("type"))

	opts := DefaultListOptions()
	opts.PerPage = 20

	// Check if the caller wants JSON (used by the editor @mention widget).
	wantsJSON := strings.Contains(c.Request().Header.Get("Accept"), "application/json")

	results, total, err := h.service.Search(c.Request().Context(), cc.Campaign.ID, query, typeID, role, opts)
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
		return c.JSON(http.StatusOK, map[string]any{"results": items, "total": total})
	}

	return middleware.Render(c, http.StatusOK, SearchResultsFragment(results, total, cc))
}

// --- Entry API (JSON endpoints for editor widget) ---

// GetEntry returns the entity's entry content as JSON.
// GET /campaigns/:id/entities/:eid/entry
func (h *Handler) GetEntry(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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

// --- Image API ---

// UpdateImageAPI updates the entity's header image path.
// PUT /campaigns/:id/entities/:eid/image
func (h *Handler) UpdateImageAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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

// --- Entity Type CRUD ---

// EntityTypesPage renders the entity type management page.
// GET /campaigns/:id/entity-types
func (h *Handler) EntityTypesPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	entityTypes, err := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Get entity counts per type so we can show usage and protect used types.
	role := int(cc.MemberRole)
	counts, _ := h.service.CountByType(c.Request().Context(), cc.Campaign.ID, role)

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
		return apperror.NewInternal(nil)
	}

	var req CreateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := CreateEntityTypeInput{
		Name:       req.Name,
		NamePlural: req.NamePlural,
		Icon:       req.Icon,
		Color:      req.Color,
	}

	et, err := h.service.CreateEntityType(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), cc.Campaign.ID)
		counts, _ := h.service.CountByType(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole))
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
		return apperror.NewInternal(nil)
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

	input := UpdateEntityTypeInput{
		Name:       body.Name,
		NamePlural: body.NamePlural,
		Icon:       body.Icon,
		Color:      body.Color,
		Fields:     body.Fields,
	}

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
		return apperror.NewInternal(nil)
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

// TemplateEditor renders the visual template editor for an entity type.
// GET /campaigns/:id/entity-types/:etid/template
// Kept for backward compatibility; redirects are not needed since the
// config page embeds the same template-editor widget.
func (h *Handler) TemplateEditor(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
