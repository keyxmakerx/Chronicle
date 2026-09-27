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
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// NoteAPIHandler serves note-related REST API endpoints for external tools.
type NoteAPIHandler struct {
	syncSvc     SyncAPIService
	noteSvc     notes.NoteService
	campaignSvc campaigns.CampaignService
}

// SetCampaignService supplies the live member lookup that decides whether a
// caller counts as a GM for notes shared with the GM. Without it every caller
// is read as a plain member, which only ever sees less.
func (h *NoteAPIHandler) SetCampaignService(svc campaigns.CampaignService) {
	h.campaignSvc = svc
}

// viewer is the note viewer for the API caller: the key's (or session's)
// user at their live campaign role, a co-DM grant counting as GM, like the
// web routes' CampaignContext.VisibilityRole. A failed lookup falls back to a
// plain member.
func (h *NoteAPIHandler) viewer(c echo.Context, key *APIKey) permissions.Viewer {
	role := permissions.RolePlayer
	if h.campaignSvc != nil {
		ctx := c.Request().Context()
		campaignID := c.Param("id")
		if m, err := h.campaignSvc.GetMember(ctx, campaignID, key.UserID); err == nil && m != nil {
			role = int(m.Role)
		}
		if role < permissions.RoleOwner {
			if granted, err := h.campaignSvc.IsUserDmGranted(ctx, campaignID, key.UserID); err == nil && granted {
				role = permissions.RoleOwner
			}
		}
	}
	return permissions.RequestViewer(role, key.UserID)
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

	result, err := h.noteSvc.ListVisible(c.Request().Context(), campaignID, h.viewer(c, key), notes.ListScope{})
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

	// Visibility gate: the middleware only proves campaign membership, and the
	// repository addresses the note by bare `WHERE id = ?` with no ownership
	// filter, so this check is what stops any member reading another
	// member's private journal (ADR-013: private notes are owner-only). 404
	// rather than 403 so the response doesn't confirm the note exists.
	campaignID := c.Param("id")
	if !note.CanView(h.viewer(c, key), campaignID) {
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

	caller := h.viewer(c, key)
	note, err := h.noteSvc.Create(c.Request().Context(), campaignID, caller, notes.CreateNoteRequest{
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
		note, err = h.noteSvc.Update(c.Request().Context(), note.ID, caller, notes.UpdateNoteRequest{
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
	caller := h.viewer(c, key)
	if !existing.CanView(caller, campaignID) {
		return apperror.NewNotFound("note not found")
	}

	var req apiUpdateNoteRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	update := notes.UpdateNoteRequest{
		Title:      req.Title,
		Content:    req.Content,
		Entry:      req.Entry,
		EntryHTML:  req.EntryHTML,
		Color:      req.Color,
		Pinned:     req.Pinned,
		IsShared:   req.IsShared,
		SharedWith: req.SharedWith,
		ParentID:   req.ParentID,
	}
	// Only the owner can change sharing, pinning or the folder — a person the
	// note is shared with may edit the body but must not be able to re-share,
	// un-share or refile someone else's note. Mirrors the web route.
	if !existing.IsOwnedBy(key.UserID, campaignID) {
		update.StripOwnerOnly()
	}

	note, err := h.noteSvc.Update(c.Request().Context(), noteID, caller, update)
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
