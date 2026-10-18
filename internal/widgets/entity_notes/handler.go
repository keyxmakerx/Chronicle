package entity_notes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityGate is the narrow cross-plugin seam used to honor entity visibility
// and campaign binding without importing the entities repo (plugin
// isolation). Implemented by an adapter over the entity service, wired in
// app/routes.go.
type EntityGate interface {
	// ResolveViewableEntity returns the entity's owning campaign ID and whether
	// the viewer (role, userID) may view it. A missing entity returns a
	// not-found error.
	ResolveViewableEntity(ctx context.Context, entityID string, role int, userID string) (campaignID string, canView bool, err error)
}

// Handler is the HTTP boundary for entity_notes. Handlers stay thin:
// build a ViewerContext from CampaignContext + auth, decode the
// request body, delegate to the service, return JSON.
type Handler struct {
	svc        Service
	entityGate EntityGate
}

// NewHandler constructs an HTTP handler against a service.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// SetEntityGate injects the entity-visibility gate. Called during app wiring.
func (h *Handler) SetEntityGate(gate EntityGate) { h.entityGate = gate }

// requireViewableEntity rejects an entity that is not in the URL campaign or
// that the caller may not view. Both cases are the same 404 so a foreign or
// hidden entity ID is indistinguishable from a missing one.
func (h *Handler) requireViewableEntity(c echo.Context, cc *campaigns.CampaignContext, entityID string) error {
	if h.entityGate == nil {
		// Fail closed: a missing gate must never serve ungated notes.
		return apperror.NewInternal(errors.New("entity_notes: entity gate not configured"))
	}
	// VisibilityRole, not MemberRole (ADR-057): a Co-DM sees dm_only entities.
	campaignID, canView, err := h.entityGate.ResolveViewableEntity(
		c.Request().Context(), entityID, int(cc.VisibilityRole()), auth.GetUserID(c))
	if err != nil {
		return err
	}
	if campaignID != cc.Campaign.ID || !canView {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// viewerFrom collapses the framework-supplied CampaignContext into the
// audience-relevant facts the service needs. The IsScribe assertion
// excludes Owner because Owner > Scribe in the role enum and the ACL
// helpers explicitly check both flags; conflating them would make the
// dm_scribe vs dm_only asymmetry harder to read.
func viewerFrom(cc *campaigns.CampaignContext, userID string) ViewerContext {
	return ViewerContext{
		UserID:      userID,
		CampaignID:  cc.Campaign.ID,
		IsOwner:     cc.MemberRole == campaigns.RoleOwner,
		IsScribe:    cc.MemberRole == campaigns.RoleScribe,
		IsDMGranted: cc.IsDmGranted,
	}
}

// List returns notes attached to an entity that the viewer can see.
// GET /campaigns/:id/entities/:eid/notes
func (h *Handler) List(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}
	if err := h.requireViewableEntity(c, cc, entityID); err != nil {
		return err
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	notes, err := h.svc.List(c.Request().Context(), entityID, viewer)
	if err != nil {
		return err
	}
	if notes == nil {
		notes = []Note{}
	}
	return c.JSON(http.StatusOK, notes)
}

// Create authors a new note. The author is taken from auth context;
// any author_user_id in the body is ignored.
// POST /campaigns/:id/entities/:eid/notes
func (h *Handler) Create(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}
	if err := h.requireViewableEntity(c, cc, entityID); err != nil {
		return err
	}
	var req CreateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Create(c.Request().Context(), entityID, viewer, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, note)
}

// Get returns one note if the viewer can read it.
// GET /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	noteID := c.Param("nid")
	if entityID == "" || noteID == "" {
		return apperror.NewBadRequest("entity and note ID are required")
	}
	if err := h.requireViewableEntity(c, cc, entityID); err != nil {
		return err
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Get(c.Request().Context(), noteID, viewer)
	if err != nil {
		return err
	}
	// The note must hang off the gated entity, not just share the campaign.
	if note.CampaignID != cc.Campaign.ID || note.EntityID != entityID {
		return apperror.NewNotFound("note not found")
	}
	return c.JSON(http.StatusOK, note)
}

// Update mutates a note. Author-only enforced inside the service.
// PUT /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Update(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")
	if noteID == "" {
		return apperror.NewBadRequest("note ID is required")
	}
	var req UpdateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Update(c.Request().Context(), noteID, viewer, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// Delete removes a note. Author-only enforced inside the service.
// DELETE /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Delete(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")
	if noteID == "" {
		return apperror.NewBadRequest("note ID is required")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	if err := h.svc.Delete(c.Request().Context(), noteID, viewer); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
