package entities

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

// Handler handles HTTP requests for entity operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service EntityService
}

// NewHandler creates a new entity handler.
func NewHandler(service EntityService) *Handler {
	return &Handler{service: service}
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
	if slug, ok := c.Get("entity_type_slug").(string); ok && slug != "" {
		activeTypeSlug = slug
		et, err := h.service.GetEntityTypeBySlug(c.Request().Context(), campaignID, slug)
		if err == nil {
			typeID = et.ID
		}
	} else if tid, _ := strconv.Atoi(c.QueryParam("type")); tid > 0 {
		typeID = tid
	}

	// Fetch entity types for sidebar filter and counts.
	entityTypes, _ := h.service.GetEntityTypes(c.Request().Context(), campaignID)
	counts, _ := h.service.CountByType(c.Request().Context(), campaignID, role)

	entities, total, err := h.service.List(c.Request().Context(), campaignID, typeID, role, opts)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)

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

	return middleware.Render(c, http.StatusOK, EntityNewPage(cc, entityTypes, preselect, csrfToken, ""))
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
		return middleware.Render(c, http.StatusOK, EntityNewPage(cc, entityTypes, req.EntityTypeID, csrfToken, errMsg))
	}

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

	entityType, err := h.service.GetEntityTypeByID(c.Request().Context(), entity.EntityTypeID)
	if err != nil {
		return apperror.NewInternal(nil)
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, EntityShowPage(cc, entity, entityType, csrfToken))
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

	return middleware.Render(c, http.StatusOK, EntityEditPage(cc, entity, entityType, entityTypes, csrfToken, ""))
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
		return middleware.Render(c, http.StatusOK, EntityEditPage(cc, entity, entityType, entityTypes, csrfToken, errMsg))
	}

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

	redirectURL := "/campaigns/" + cc.Campaign.ID + "/entities"
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectURL)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectURL)
}

// SearchAPI handles entity search requests (GET /campaigns/:id/entities/search).
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

	results, total, err := h.service.Search(c.Request().Context(), cc.Campaign.ID, query, typeID, role, opts)
	if err != nil {
		if _, ok := err.(*apperror.AppError); ok {
			return middleware.Render(c, http.StatusOK, SearchResultsFragment(nil, 0, cc))
		}
		return err
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

	response := map[string]any{
		"entry":      entity.Entry,
		"entry_html": entity.EntryHTML,
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
