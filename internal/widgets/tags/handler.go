package tags

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityGate is the narrow cross-plugin seam the tags widget uses to honor entity
// visibility + campaign binding on the public per-entity tag read, without
// importing the entities repo (plugin-isolation). Implemented by an adapter over
// the entity service (wired in app/routes.go). Introduced by
// cordinator/dispatches/chronicle/C-PUBLIC-VIEW-FIX-R2.md (extra hole found in
// Step-0: GetEntityTags leaked private-entity tag names + was cross-campaign).
type EntityGate interface {
	// ResolveViewableEntity returns the entity's owning campaign ID and whether
	// the viewer (role, userID) may view it. Missing entity → not-found error.
	ResolveViewableEntity(ctx context.Context, entityID string, role int, userID string) (campaignID string, canView bool, err error)
}

// Handler handles HTTP requests for tag operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service    TagService
	grantSvc   TagGrantService
	auditSvc   audit.AuditService
	entityGate EntityGate
}

// NewHandler creates a new tag handler backed by the given service.
func NewHandler(service TagService) *Handler {
	return &Handler{service: service}
}

// SetEntityGate injects the entity-visibility gate. Called during app wiring.
// When set, the public per-entity tag read enforces entity privacy + campaign
// binding.
func (h *Handler) SetEntityGate(gate EntityGate) {
	h.entityGate = gate
}

// SetAuditService sets the audit service for recording tag mutations.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetAuditService(svc audit.AuditService) {
	h.auditSvc = svc
}

// SetGrantService sets the tag visibility-grant service. Called after all
// plugins are wired (it depends on the campaign + group services).
func (h *Handler) SetGrantService(svc TagGrantService) {
	h.grantSvc = svc
}

// logAudit fires a fire-and-forget audit entry. Errors are logged but
// never block the primary operation.
func (h *Handler) logAudit(c echo.Context, campaignID, action, tagName string) {
	if h.auditSvc == nil {
		return
	}
	userID := auth.GetUserID(c)
	if err := h.auditSvc.Log(c.Request().Context(), &audit.AuditEntry{
		CampaignID: campaignID,
		UserID:     userID,
		Action:     action,
		Details:    map[string]any{"tag_name": tagName},
	}); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("error", err))
	}
}

// canSeeDmOnly returns true if the current user's role permits viewing dm_only
// tags. Owners, site admins, and DM-granted users can see dm_only tags.
func canSeeDmOnly(cc *campaigns.CampaignContext) bool {
	return cc.MemberRole >= campaigns.RoleOwner || cc.IsSiteAdmin || cc.IsDmGranted
}

// ListTags returns all tags for a campaign as JSON (GET /campaigns/:id/tags).
// Players and Scribes see only public tags; Owners see all tags including dm_only.
func (h *Handler) ListTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tags, err := h.service.ListByCampaign(c.Request().Context(), cc.Campaign.ID, canSeeDmOnly(cc))
	if err != nil {
		return err
	}

	// Return empty array instead of null when no tags exist.
	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}

// CreateTag creates a new tag in the campaign (POST /campaigns/:id/tags).
func (h *Handler) CreateTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req CreateTagRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	// Only Owners and site admins can create dm_only tags.
	dmOnly := req.DmOnly
	if dmOnly && cc.MemberRole < campaigns.RoleOwner && !cc.IsSiteAdmin {
		dmOnly = false
	}

	tag, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, req.Name, req.Color, dmOnly)
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionTagCreated, tag.Name)

	return c.JSON(http.StatusCreated, tag)
}

// UpdateTag updates an existing tag (PUT /campaigns/:id/tags/:tagId).
func (h *Handler) UpdateTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify the tag belongs to this campaign before updating.
	existing, err := h.service.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("tag not found")
	}

	var req UpdateTagRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	// Only Owners and site admins can set the dm_only flag on tags.
	dmOnly := req.DmOnly
	if dmOnly && cc.MemberRole < campaigns.RoleOwner && !cc.IsSiteAdmin {
		dmOnly = false
	}

	tag, err := h.service.Update(c.Request().Context(), tagID, req.Name, req.Color, dmOnly)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, tag)
}

// DeleteTag removes a tag from the campaign (DELETE /campaigns/:id/tags/:tagId).
func (h *Handler) DeleteTag(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	// Verify the tag belongs to this campaign before deleting.
	existing, err := h.service.GetByID(c.Request().Context(), tagID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("tag not found")
	}

	if err := h.service.Delete(c.Request().Context(), tagID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, audit.ActionTagDeleted, existing.Name)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetEntityTags replaces all tags on an entity with the provided set of tag
// IDs (PUT /campaigns/:id/entities/:eid/tags). Accepts a JSON body with a
// "tagIds" array. This is an idempotent replacement operation: the entity
// will have exactly the tags specified, with old ones removed and new ones added.
func (h *Handler) SetEntityTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	var req SetEntityTagsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.SetEntityTags(c.Request().Context(), entityID, cc.Campaign.ID, req.TagIDs); err != nil {
		return err
	}

	// Return the updated set of tags for the entity (Scribes see all tags).
	tags, err := h.service.GetEntityTags(c.Request().Context(), entityID, canSeeDmOnly(cc))
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}

// GetEntityTags returns all tags for an entity as JSON
// (GET /campaigns/:id/entities/:eid/tags).
// Players see only public tags; Scribes and Owners see all tags including dm_only.
func (h *Handler) GetEntityTags(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	// Entity-privacy gate (mirrors the entity Show page): resolve the entity,
	// require it to belong to the URL campaign (kills the cross-campaign IDOR),
	// and require the caller's view access. Without this an anonymous visitor
	// could read the (often spoilery) tag names of a private entity, or of any
	// entity in another campaign given its ID.
	if h.entityGate == nil {
		// Fail closed: a missing gate must never serve ungated entity tags.
		return apperror.NewInternal(errors.New("tags: entity gate not configured"))
	}
	campaignID, canView, err := h.entityGate.ResolveViewableEntity(
		c.Request().Context(), entityID, int(cc.MemberRole), auth.GetUserID(c))
	if err != nil {
		return err // NotFound for a missing entity; propagates real errors.
	}
	if campaignID != cc.Campaign.ID || !canView {
		return apperror.NewNotFound("entity not found")
	}

	tags, err := h.service.GetEntityTags(c.Request().Context(), entityID, canSeeDmOnly(cc))
	if err != nil {
		return err
	}

	if tags == nil {
		tags = []Tag{}
	}

	return c.JSON(http.StatusOK, tags)
}
