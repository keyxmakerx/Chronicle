// Package syncapi — note_api_handler.go provides REST API v1 endpoints for
// note CRUD. External clients (Foundry VTT) use these endpoints to synchronize
// campaign notes via API key auth.
package syncapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// NoteAPIHandler serves note-related REST API endpoints for external tools.
type NoteAPIHandler struct {
	syncSvc SyncAPIService
	noteSvc notes.NoteService
}

// NewNoteAPIHandler creates a new note API handler.
func NewNoteAPIHandler(syncSvc SyncAPIService, noteSvc notes.NoteService) *NoteAPIHandler {
	return &NoteAPIHandler{
		syncSvc: syncSvc,
		noteSvc: noteSvc,
	}
}

// --- Note CRUD ---

// ListNotes returns all notes visible to the API key owner for a campaign.
// GET /api/v1/campaigns/:id/notes
func (h *NoteAPIHandler) ListNotes(c echo.Context) error {
	campaignID := c.Param("id")
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	result, err := h.noteSvc.ListByUserAndCampaign(c.Request().Context(), key.UserID, campaignID)
	if err != nil {
		slog.Error("api: list notes failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list notes"))
	}
	return c.JSON(http.StatusOK, result)
}

// GetNote returns a single note by ID.
// GET /api/v1/campaigns/:id/notes/:noteID
func (h *NoteAPIHandler) GetNote(c echo.Context) error {
	noteID := c.Param("noteID")

	note, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}

	// Verify the note belongs to the campaign in the URL.
	campaignID := c.Param("id")
	if note.CampaignID != campaignID {
		return apperror.NewNotFound("note not found")
	}

	return c.JSON(http.StatusOK, note)
}

// apiCreateNoteRequest is the JSON body for creating a note via the API.
type apiCreateNoteRequest struct {
	EntityID   *string       `json:"entity_id"`
	ParentID   *string       `json:"parent_id"`
	IsFolder   bool          `json:"is_folder"`
	Title      string        `json:"title"`
	Content    []notes.Block `json:"content"`
	Entry      *string       `json:"entry"`
	EntryHTML  *string       `json:"entry_html"`
	Color      string        `json:"color"`
	IsShared   bool          `json:"is_shared"`
	SharedWith []string      `json:"shared_with"`
}

// CreateNote creates a new note in a campaign.
// POST /api/v1/campaigns/:id/notes
func (h *NoteAPIHandler) CreateNote(c echo.Context) error {
	campaignID := c.Param("id")
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateNoteRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	note, err := h.noteSvc.Create(c.Request().Context(), campaignID, key.UserID, notes.CreateNoteRequest{
		EntityID:   req.EntityID,
		ParentID:   req.ParentID,
		IsFolder:   req.IsFolder,
		Title:      req.Title,
		Content:    req.Content,
		Color:      req.Color,
		IsShared:   req.IsShared,
		SharedWith: req.SharedWith,
	})
	if err != nil {
		return err
	}

	// If entry/entryHTML were provided, apply them via update (Create doesn't
	// accept ProseMirror content directly).
	if req.Entry != nil || req.EntryHTML != nil {
		note, err = h.noteSvc.Update(c.Request().Context(), note.ID, key.UserID, notes.UpdateNoteRequest{
			Entry:     req.Entry,
			EntryHTML: req.EntryHTML,
		})
		if err != nil {
			return err
		}
	}

	return c.JSON(http.StatusCreated, note)
}

// apiUpdateNoteRequest is the JSON body for updating a note.
type apiUpdateNoteRequest struct {
	Title      *string       `json:"title"`
	Content    *[]notes.Block `json:"content"`
	Entry      *string       `json:"entry"`
	EntryHTML  *string       `json:"entry_html"`
	Color      *string       `json:"color"`
	Pinned     *bool         `json:"pinned"`
	IsShared   *bool         `json:"is_shared"`
	SharedWith []string      `json:"shared_with"`
	ParentID   *string       `json:"parent_id"`
}

// UpdateNote updates an existing note.
// PUT /api/v1/campaigns/:id/notes/:noteID
func (h *NoteAPIHandler) UpdateNote(c echo.Context) error {
	noteID := c.Param("noteID")
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	// Verify the note belongs to the campaign.
	existing, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	campaignID := c.Param("id")
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("note not found")
	}

	var req apiUpdateNoteRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	note, err := h.noteSvc.Update(c.Request().Context(), noteID, key.UserID, notes.UpdateNoteRequest{
		Title:      req.Title,
		Content:    req.Content,
		Entry:      req.Entry,
		EntryHTML:  req.EntryHTML,
		Color:      req.Color,
		Pinned:     req.Pinned,
		IsShared:   req.IsShared,
		SharedWith: req.SharedWith,
		ParentID:   req.ParentID,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// DeleteNote removes a note.
// DELETE /api/v1/campaigns/:id/notes/:noteID
func (h *NoteAPIHandler) DeleteNote(c echo.Context) error {
	noteID := c.Param("noteID")

	// Verify the note belongs to the campaign.
	existing, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	campaignID := c.Param("id")
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("note not found")
	}

	if err := h.noteSvc.Delete(c.Request().Context(), noteID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
