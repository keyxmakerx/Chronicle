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
	// Defense-in-depth egress sanitize — see egress_sanitize.go.
	sanitizeNotesHTMLForEgress(result)
	return c.JSON(http.StatusOK, result)
}

// GetNote returns a single note by ID.
// GET /api/v1/campaigns/:id/notes/:noteID
func (h *NoteAPIHandler) GetNote(c echo.Context) error {
	noteID := c.Param("noteID")
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	note, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}

	// Verify the note belongs to the campaign in the URL AND that the caller may
	// read it. The middleware chain only proves the caller is a member of the
	// campaign with the "read" permission — it knows nothing about note
	// ownership, and the service/repository address the note by bare `WHERE id
	// = ?`. Without this gate any campaign member reads any other member's
	// private journal. Same predicate the web route applies (ADR-013: private
	// notes are owner-only); 404 rather than 403 so the response does not
	// confirm that a note the caller may not see exists.
	campaignID := c.Param("id")
	if !note.CanAccess(key.UserID, campaignID) {
		return apperror.NewNotFound("note not found")
	}

	// Defense-in-depth egress sanitize — see egress_sanitize.go.
	sanitizeNoteHTMLForEgress(note)
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

	// Verify the note belongs to the campaign AND that the caller may edit it.
	// A campaign-wide "write" permission is not authority over another member's
	// note: without this gate any Scribe overwrites any other member's private
	// journal. Mirrors the web route — owner or share recipient may edit.
	existing, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	campaignID := c.Param("id")
	if !existing.CanAccess(key.UserID, campaignID) {
		return apperror.NewNotFound("note not found")
	}

	var req apiUpdateNoteRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Only the owner can change sharing/pinned status — a share recipient may
	// edit the body but must not be able to re-share or un-share someone else's
	// note. Mirrors the web route.
	if !existing.IsOwnedBy(key.UserID, campaignID) {
		req.IsShared = nil
		req.SharedWith = nil
		req.Pinned = nil
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
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	// Verify the note belongs to the campaign AND that the caller owns it.
	// Deletion is owner-only on the web route, and campaign-wide "write" is not
	// authority to destroy another member's journal — not even for a note that
	// was shared with the caller.
	existing, err := h.noteSvc.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	campaignID := c.Param("id")
	if !existing.IsOwnedBy(key.UserID, campaignID) {
		return apperror.NewNotFound("note not found")
	}

	if err := h.noteSvc.Delete(c.Request().Context(), noteID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
