package notes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MemberLister is satisfied by CampaignService for fetching campaign members.
type MemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// memberRef is a compact member representation for the sharing UI.
type memberRef struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// MediaUploader is an interface for uploading files to the media system.
type MediaUploader interface {
	UploadRaw(ctx context.Context, campaignID, userID string, fileBytes []byte, originalName, mimeType string) (filePath string, err error)
}

// Handler handles HTTP requests for note operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service       NoteService
	attService    AttachmentService
	mediaUploader MediaUploader
	memberLister  MemberLister
}

// NewHandler creates a new note handler backed by the given service.
func NewHandler(service NoteService) *Handler {
	return &Handler{service: service}
}

// SetAttachmentService sets the attachment service for audio upload support.
func (h *Handler) SetAttachmentService(as AttachmentService) {
	h.attService = as
}

// SetMediaUploader sets the media uploader for attachment file storage.
func (h *Handler) SetMediaUploader(mu MediaUploader) {
	h.mediaUploader = mu
}

// SetMemberLister sets the member lister for the share-with-players picker.
func (h *Handler) SetMemberLister(ml MemberLister) {
	h.memberLister = ml
}

// List returns notes for the current user in the campaign (GET /campaigns/:id/notes).
// Returns own notes + shared notes from other users.
// Supports ?scope=all (default), ?scope=campaign (campaign-wide only),
// and ?scope=entity&entity_id=<eid> (entity-scoped).
func (h *Handler) List(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var scope ListScope
	switch c.QueryParam("scope") {
	case "entity":
		scope = ListScope{Kind: "entity", EntityID: c.QueryParam("entity_id")}
		if scope.EntityID == "" {
			return apperror.NewBadRequest("entity_id is required for entity scope")
		}
	case "campaign":
		scope = ListScope{Kind: "campaign"}
	case "jots":
		scope = ListScope{Kind: "jots"}
	}

	notes, err := h.service.ListVisible(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc), scope)
	if err != nil {
		return err
	}
	if notes == nil {
		notes = []Note{}
	}
	return c.JSON(http.StatusOK, notes)
}

// Get returns one note (GET /campaigns/:id/notes/:noteId).
func (h *Handler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	note, err := h.viewableNote(c, cc, c.Param("noteId"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// Index returns the viewer's Journal list (GET /campaigns/:id/notes/index).
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	idx, err := h.service.JournalIndex(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, idx)
}

// Search finds the viewer's notes by title or text
// (GET /campaigns/:id/notes/search?q=…[&jots=1]).
func (h *Handler) Search(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	hits, err := h.service.Search(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc),
		c.QueryParam("q"), c.QueryParam("jots") == "1")
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, hits)
}

// Backlinks lists the viewer's notes that link to a note
// (GET /campaigns/:id/notes/:noteId/backlinks).
func (h *Handler) Backlinks(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("noteId")
	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}
	refs, err := h.service.Backlinks(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc), noteID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, refs)
}

// PageRefs lists the viewer's Journal notes that link to a page
// (GET /campaigns/:id/notes/page-refs?entity_id=…).
func (h *Handler) PageRefs(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	refs, err := h.service.PageRefs(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc), c.QueryParam("entity_id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, refs)
}

// Labels resolves [[note]] link labels for the viewer
// (GET /campaigns/:id/notes/labels?ids=a,b,…). A note the viewer cannot see
// is absent from the result, exactly like one that does not exist.
func (h *Handler) Labels(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	labels, err := h.service.Labels(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc),
		strings.Split(c.QueryParam("ids"), ","))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, labels)
}

// Bulk applies one action to several of the viewer's own notes
// (POST /campaigns/:id/notes/bulk).
func (h *Handler) Bulk(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req BulkRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	res, err := h.service.Bulk(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc), req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

// Create adds a new note (POST /campaigns/:id/notes).
func (h *Handler) Create(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req CreateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	note, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, viewerFor(c, cc), req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, note)
}

// Update modifies an existing note (PUT /campaigns/:id/notes/:noteId).
// Access: anyone who can view the note may edit its title and body; only its
// owner may change sharing, pinning, archiving or its folder.
func (h *Handler) Update(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	viewer := viewerFor(c, cc)
	noteID := c.Param("noteId")

	existing, err := h.viewableNote(c, cc, noteID)
	if err != nil {
		return err
	}

	var req UpdateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	if !existing.IsOwnedBy(viewer.UserID(), cc.Campaign.ID) {
		req.StripOwnerOnly()
	}

	note, err := h.service.Update(c.Request().Context(), noteID, viewer, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, note)
}

// Delete removes a note (DELETE /campaigns/:id/notes/:noteId).
// Only the note owner can delete.
func (h *Handler) Delete(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

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
		return apperror.NewMissingContext()
	}
	noteID := c.Param("noteId")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
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

// Lock acquires the edit lock on a shared note (POST /campaigns/:id/notes/:noteId/lock).
func (h *Handler) Lock(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	note, err := h.service.AcquireLock(c.Request().Context(), noteID, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// Unlock releases the edit lock (POST /campaigns/:id/notes/:noteId/unlock).
func (h *Handler) Unlock(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	if err := h.service.ReleaseLock(c.Request().Context(), noteID, userID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Heartbeat keeps the edit lock alive (POST /campaigns/:id/notes/:noteId/heartbeat).
func (h *Handler) Heartbeat(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	if err := h.service.Heartbeat(c.Request().Context(), noteID, userID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ForceUnlock releases any user's lock (POST /campaigns/:id/notes/:noteId/force-unlock).
// Requires campaign owner role.
func (h *Handler) ForceUnlock(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("noteId")

	if cc.MemberRole < campaigns.RoleOwner {
		return apperror.NewForbidden("only campaign owners can force-unlock notes")
	}
	// The note must be one this campaign's owner can see: an owner of one
	// campaign may not reach into another campaign's notes by ID.
	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	if err := h.service.ForceReleaseLock(c.Request().Context(), noteID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ListVersions returns version history (GET /campaigns/:id/notes/:noteId/versions).
func (h *Handler) ListVersions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("noteId")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	versions, err := h.service.ListVersions(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	if versions == nil {
		versions = []NoteVersion{}
	}
	return c.JSON(http.StatusOK, versions)
}

// GetVersion returns a specific version (GET /campaigns/:id/notes/:noteId/versions/:vid).
func (h *Handler) GetVersion(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("noteId")
	versionID := c.Param("vid")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	version, err := h.service.GetVersion(c.Request().Context(), versionID)
	if err != nil {
		return err
	}
	// Access to the note in the URL is not access to a version of some
	// other note: the version must belong to it.
	if version.NoteID != noteID {
		return apperror.NewNotFound("note version not found")
	}
	return c.JSON(http.StatusOK, version)
}

// RestoreVersion reverts a note to a previous version
// (POST /campaigns/:id/notes/:noteId/versions/:vid/restore).
func (h *Handler) RestoreVersion(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("noteId")
	versionID := c.Param("vid")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	note, err := h.service.RestoreVersion(c.Request().Context(), noteID, versionID, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// ShowJournal renders the full-page journal view (GET /campaigns/:id/journal).
// Returns full page or HTMX fragment based on request headers.
func (h *Handler) ShowJournal(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if middleware.IsHTMX(c) {
		return JournalFragment(cc).Render(c.Request().Context(), c.Response())
	}
	return JournalPage(cc).Render(c.Request().Context(), c.Response())
}

// MembersAPI returns campaign members as JSON for the share-with-players picker.
// GET /campaigns/:id/notes/members
func (h *Handler) MembersAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.memberLister == nil {
		return c.JSON(http.StatusOK, []memberRef{})
	}

	ms, err := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	refs := make([]memberRef, 0, len(ms))
	for _, m := range ms {
		refs = append(refs, memberRef{
			UserID:   m.UserID,
			Username: m.DisplayName,
			Role:     m.Role.String(),
		})
	}
	return c.JSON(http.StatusOK, refs)
}

// viewerFor is the note viewer for the session user. VisibilityRole folds a
// co-DM grant into the Owner tier, which is exactly who "shared with the GM"
// admits.
func viewerFor(c echo.Context, cc *campaigns.CampaignContext) permissions.Viewer {
	return permissions.RequestViewer(cc.VisibilityRole(), auth.GetUserID(c))
}

// viewableNote loads noteID and returns it only if the session user can view
// it in this campaign (Note.CanView, the one predicate the web and REST API
// routes share). Anything else is a 404, so a response never confirms that a
// note the caller cannot see exists.
func (h *Handler) viewableNote(c echo.Context, cc *campaigns.CampaignContext, noteID string) (*Note, error) {
	note, err := h.service.GetByID(c.Request().Context(), noteID)
	if err != nil {
		return nil, err
	}
	if !note.CanView(viewerFor(c, cc), cc.Campaign.ID) {
		return nil, apperror.NewNotFound("note not found")
	}
	return note, nil
}

// --- Attachment Handlers ---

// ListAttachments returns attachments for a note.
// GET /campaigns/:id/notes/:nid/attachments
func (h *Handler) ListAttachments(c echo.Context) error {
	if h.attService == nil {
		return c.JSON(http.StatusOK, []NoteAttachment{})
	}

	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")

	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	attachments, err := h.attService.ListAttachments(c.Request().Context(), noteID)
	if err != nil {
		return err
	}
	if attachments == nil {
		attachments = []NoteAttachment{}
	}
	return c.JSON(http.StatusOK, attachments)
}

// UploadAttachment uploads an audio file and attaches it to a note.
// POST /campaigns/:id/notes/:nid/attachments
func (h *Handler) UploadAttachment(c echo.Context) error {
	if h.attService == nil || h.mediaUploader == nil {
		return apperror.NewBadRequest("attachments not configured")
	}

	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("nid")

	// Attaching audio is editing the note: anyone who can view it in this
	// campaign, as for its body.
	if _, err := h.viewableNote(c, cc, noteID); err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}

	// Read file bytes.
	src, err := file.Open()
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(io.LimitReader(src, 100*1024*1024)) // 100MB limit
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}

	mimeType := file.Header.Get("Content-Type")

	// Upload via media service.
	filePath, err := h.mediaUploader.UploadRaw(c.Request().Context(),
		cc.Campaign.ID, userID, fileBytes, file.Filename, mimeType)
	if err != nil {
		return err
	}

	// Create attachment record.
	att := &NoteAttachment{
		NoteID:       noteID,
		CampaignID:   cc.Campaign.ID,
		FilePath:     filePath,
		OriginalName: file.Filename,
		MimeType:     mimeType,
		FileSize:     int64(len(fileBytes)),
	}
	if err := h.attService.CreateAttachment(c.Request().Context(), att); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, att)
}

// DeleteAttachment removes an attachment.
// DELETE /campaigns/:id/notes/:nid/attachments/:aid
func (h *Handler) DeleteAttachment(c echo.Context) error {
	if h.attService == nil {
		return apperror.NewBadRequest("attachments not configured")
	}

	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("nid")
	attID := c.Param("aid")

	note, err := h.viewableNote(c, cc, noteID)
	if err != nil {
		return err
	}
	// Only note owner or campaign owner can delete attachments.
	if note.UserID != userID && cc.MemberRole < campaigns.RoleOwner {
		return apperror.NewForbidden("only note owner or campaign owner can delete attachments")
	}

	// Verify attachment belongs to this note.
	att, err := h.attService.GetAttachment(c.Request().Context(), attID)
	if err != nil {
		return err
	}
	if att.NoteID != noteID {
		return apperror.NewNotFound("attachment not found")
	}

	if _, err := h.attService.DeleteAttachment(c.Request().Context(), attID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// UpdateTranscript saves or updates the transcript text for an attachment.
// PUT /campaigns/:id/notes/:nid/attachments/:aid/transcript
func (h *Handler) UpdateTranscript(c echo.Context) error {
	if h.attService == nil {
		return apperror.NewBadRequest("attachments not configured")
	}

	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	noteID := c.Param("nid")
	attID := c.Param("aid")

	note, err := h.viewableNote(c, cc, noteID)
	if err != nil {
		return err
	}
	if note.UserID != userID && cc.MemberRole < campaigns.RoleOwner {
		return apperror.NewForbidden("access denied")
	}

	// Verify attachment belongs to this note.
	att, err := h.attService.GetAttachment(c.Request().Context(), attID)
	if err != nil {
		return err
	}
	if att.NoteID != noteID {
		return apperror.NewNotFound("attachment not found")
	}

	var req struct {
		Transcript string `json:"transcript"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.attService.UpdateTranscript(c.Request().Context(), attID, req.Transcript); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
