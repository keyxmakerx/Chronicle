package notes

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles HTTP requests for note operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service NoteService
}

// NewHandler creates a new note handler backed by the given service.
func NewHandler(service NoteService) *Handler {
	return &Handler{service: service}
}

// List returns notes for the current user in the campaign (GET /campaigns/:id/notes).
// Supports ?scope=all (default), ?scope=campaign (campaign-wide only),
// and ?scope=entity&entity_id=<eid> (entity-scoped).
func (h *Handler) List(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}
	userID := auth.GetUserID(c)

	scope := c.QueryParam("scope")
	entityID := c.QueryParam("entity_id")

	var notes []Note
	var err error

	switch scope {
	case "entity":
		if entityID == "" {
			return apperror.NewBadRequest("entity_id is required for entity scope")
		}
		notes, err = h.service.ListByEntity(c.Request().Context(), userID, cc.Campaign.ID, entityID)
	case "campaign":
		notes, err = h.service.ListCampaignWide(c.Request().Context(), userID, cc.Campaign.ID)
	default:
		notes, err = h.service.ListByUserAndCampaign(c.Request().Context(), userID, cc.Campaign.ID)
	}

	if err != nil {
		return err
	}
	if notes == nil {
		notes = []Note{}
	}
	return c.JSON(http.StatusOK, notes)
}

// Create adds a new note (POST /campaigns/:id/notes).
func (h *Handler) Create(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}
	userID := auth.GetUserID(c)

	var req CreateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	note, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, userID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, note)
}

// Update modifies an existing note (PUT /campaigns/:id/notes/:noteId).
func (h *Handler) Update(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	// Verify ownership before updating.
	existing, err := h.service.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	if existing.UserID != userID || existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("note not found")
	}

	var req UpdateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	note, err := h.service.Update(c.Request().Context(), noteID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, note)
}

// Delete removes a note (DELETE /campaigns/:id/notes/:noteId).
func (h *Handler) Delete(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	// Verify ownership before deleting.
	existing, err := h.service.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	if existing.UserID != userID || existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("note not found")
	}

	if err := h.service.Delete(c.Request().Context(), noteID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ToggleCheck toggles a checklist item (POST /campaigns/:id/notes/:noteId/toggle).
func (h *Handler) ToggleCheck(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	// Verify ownership.
	existing, err := h.service.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	if existing.UserID != userID || existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("note not found")
	}

	var req ToggleCheckRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	note, err := h.service.ToggleCheck(c.Request().Context(), noteID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, note)
}
