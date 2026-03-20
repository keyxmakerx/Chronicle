package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// NoteService defines the business logic contract for notes.
type NoteService interface {
	Create(ctx context.Context, campaignID, userID string, req CreateNoteRequest) (*Note, error)
	GetByID(ctx context.Context, id string) (*Note, error)
	Update(ctx context.Context, id, userID string, req UpdateNoteRequest) (*Note, error)
	Delete(ctx context.Context, id string) error
	ToggleCheck(ctx context.Context, id string, req ToggleCheckRequest) (*Note, error)

	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error)
	ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error)
	ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error)

	// Locking
	AcquireLock(ctx context.Context, noteID, userID string) (*Note, error)
	ReleaseLock(ctx context.Context, noteID, userID string) error
	ForceReleaseLock(ctx context.Context, noteID string) error
	Heartbeat(ctx context.Context, noteID, userID string) error

	// Versions
	ListVersions(ctx context.Context, noteID string) ([]NoteVersion, error)
	GetVersion(ctx context.Context, versionID string) (*NoteVersion, error)
	RestoreVersion(ctx context.Context, noteID, versionID, userID string) (*Note, error)
}

// AttachmentService defines the business logic for note attachments.
type AttachmentService interface {
	ListAttachments(ctx context.Context, noteID string) ([]NoteAttachment, error)
	GetAttachment(ctx context.Context, id string) (*NoteAttachment, error)
	CreateAttachment(ctx context.Context, a *NoteAttachment) error
	DeleteAttachment(ctx context.Context, id string) (filePath string, err error)
	UpdateTranscript(ctx context.Context, id, transcript string) error
}

// NoteEventPublisher emits domain events when notes change.
// Implemented by the WebSocket EventBus adapter in app/routes.go.
type NoteEventPublisher interface {
	PublishNoteEvent(eventType, campaignID, noteID string, note *Note)
}

// NoopNoteEventPublisher is a no-op implementation for tests.
type NoopNoteEventPublisher struct{}

func (NoopNoteEventPublisher) PublishNoteEvent(string, string, string, *Note) {}

// noteService implements NoteService and AttachmentService.
type noteService struct {
	repo    NoteRepository
	attRepo AttachmentRepository
	events  NoteEventPublisher
}

// NewNoteService creates a new note service.
func NewNoteService(repo NoteRepository) NoteService {
	return &noteService{repo: repo, events: NoopNoteEventPublisher{}}
}

// NewNoteServiceWithAttachments creates a note service with attachment support.
func NewNoteServiceWithAttachments(repo NoteRepository, attRepo AttachmentRepository) *noteService {
	return &noteService{repo: repo, attRepo: attRepo, events: NoopNoteEventPublisher{}}
}

// SetEventPublisher sets the event publisher for real-time sync.
func (s *noteService) SetEventPublisher(pub NoteEventPublisher) {
	s.events = pub
}

// Create validates and persists a new note.
func (s *noteService) Create(ctx context.Context, campaignID, userID string, req CreateNoteRequest) (*Note, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled"
	}
	if len(title) > 200 {
		return nil, apperror.NewBadRequest("title must be 200 characters or less")
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#374151"
	}

	content := req.Content
	if content == nil {
		content = []Block{}
	}

	note := &Note{
		ID:           generateID(),
		CampaignID:   campaignID,
		UserID:       userID,
		EntityID:     req.EntityID,
		ParentID:     req.ParentID,
		IsFolder:     req.IsFolder,
		Title:        title,
		Content:      content,
		Color:        color,
		IsShared:     req.IsShared,
		SharedWith:   req.SharedWith,
		LastEditedBy: &userID,
	}

	if err := s.repo.Create(ctx, note); err != nil {
		return nil, err
	}

	created, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.events.PublishNoteEvent("created", campaignID, created.ID, created)
	return created, nil
}

// GetByID retrieves a note by ID.
func (s *noteService) GetByID(ctx context.Context, id string) (*Note, error) {
	return s.repo.FindByID(ctx, id)
}

// Update applies partial updates to a note and records a version snapshot.
func (s *noteService) Update(ctx context.Context, id, userID string, req UpdateNoteRequest) (*Note, error) {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Snapshot the current state before applying changes.
	s.createVersionSnapshot(ctx, note, userID)

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if len(title) > 200 {
			return nil, apperror.NewBadRequest("title must be 200 characters or less")
		}
		if title == "" {
			title = "Untitled"
		}
		note.Title = title
	}
	if req.Content != nil {
		note.Content = *req.Content
	}
	if req.Entry != nil {
		note.Entry = req.Entry
	}
	if req.EntryHTML != nil {
		sanitized := sanitize.HTML(*req.EntryHTML)
		note.EntryHTML = &sanitized
	}
	if req.Color != nil {
		note.Color = *req.Color
	}
	if req.Pinned != nil {
		note.Pinned = *req.Pinned
	}
	if req.IsShared != nil {
		note.IsShared = *req.IsShared
	}
	if req.SharedWith != nil {
		note.SharedWith = req.SharedWith
	}
	if req.ParentID != nil {
		// Empty string means move to top-level.
		if *req.ParentID == "" {
			note.ParentID = nil
		} else {
			note.ParentID = req.ParentID
		}
	}

	note.LastEditedBy = &userID

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	updated, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.events.PublishNoteEvent("updated", note.CampaignID, note.ID, updated)
	return updated, nil
}

// Delete removes a note.
func (s *noteService) Delete(ctx context.Context, id string) error {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.events.PublishNoteEvent("deleted", note.CampaignID, note.ID, note)
	return nil
}

// ToggleCheck flips a checklist item's checked state within a note.
func (s *noteService) ToggleCheck(ctx context.Context, id string, req ToggleCheckRequest) (*Note, error) {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.BlockIndex < 0 || req.BlockIndex >= len(note.Content) {
		return nil, apperror.NewBadRequest("block index out of range")
	}

	block := &note.Content[req.BlockIndex]
	if block.Type != "checklist" {
		return nil, apperror.NewBadRequest("block is not a checklist")
	}

	if req.ItemIndex < 0 || req.ItemIndex >= len(block.Items) {
		return nil, apperror.NewBadRequest("item index out of range")
	}

	block.Items[req.ItemIndex].Checked = !block.Items[req.ItemIndex].Checked

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	return note, nil
}

// ListByUserAndCampaign returns own + shared notes for a user in a campaign.
func (s *noteService) ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error) {
	return s.repo.ListByUserAndCampaign(ctx, userID, campaignID)
}

// ListByEntity returns notes scoped to a specific entity.
func (s *noteService) ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error) {
	return s.repo.ListByEntity(ctx, userID, campaignID, entityID)
}

// ListCampaignWide returns campaign-wide notes (not entity-scoped).
func (s *noteService) ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error) {
	return s.repo.ListCampaignWide(ctx, userID, campaignID)
}

// AcquireLock attempts to take a pessimistic edit lock on the note. Returns
// the refreshed note on success or a conflict error if another user holds it.
func (s *noteService) AcquireLock(ctx context.Context, noteID, userID string) (*Note, error) {
	acquired, err := s.repo.AcquireLock(ctx, noteID, userID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		note, _ := s.repo.FindByID(ctx, noteID)
		if note != nil && note.LockedBy != nil {
			return nil, apperror.NewConflict("note is currently being edited by another user")
		}
		return nil, apperror.NewConflict("could not acquire lock")
	}
	return s.repo.FindByID(ctx, noteID)
}

// ReleaseLock releases the edit lock (only if held by the requesting user).
func (s *noteService) ReleaseLock(ctx context.Context, noteID, userID string) error {
	return s.repo.ReleaseLock(ctx, noteID, userID)
}

// ForceReleaseLock releases the lock regardless of holder (owner override).
func (s *noteService) ForceReleaseLock(ctx context.Context, noteID string) error {
	return s.repo.ForceReleaseLock(ctx, noteID)
}

// Heartbeat keeps the edit lock alive by refreshing locked_at.
func (s *noteService) Heartbeat(ctx context.Context, noteID, userID string) error {
	return s.repo.RefreshLock(ctx, noteID, userID)
}

// ListVersions returns the version history for a note.
func (s *noteService) ListVersions(ctx context.Context, noteID string) ([]NoteVersion, error) {
	return s.repo.ListVersions(ctx, noteID, MaxVersionsPerNote)
}

// GetVersion retrieves a specific version.
func (s *noteService) GetVersion(ctx context.Context, versionID string) (*NoteVersion, error) {
	return s.repo.FindVersionByID(ctx, versionID)
}

// RestoreVersion reverts a note to a previous version's content. A new version
// is created to preserve the current state before the restore.
func (s *noteService) RestoreVersion(ctx context.Context, noteID, versionID, userID string) (*Note, error) {
	note, err := s.repo.FindByID(ctx, noteID)
	if err != nil {
		return nil, err
	}

	version, err := s.repo.FindVersionByID(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.NoteID != noteID {
		return nil, apperror.NewBadRequest("version does not belong to this note")
	}

	// Snapshot current state before restoring.
	s.createVersionSnapshot(ctx, note, userID)

	// Apply the version's content. Sanitize restored HTML in case the version
	// was created before HTML sanitization was enforced.
	note.Title = version.Title
	note.Content = version.Content
	note.Entry = version.Entry
	if version.EntryHTML != nil {
		sanitized := sanitize.HTML(*version.EntryHTML)
		note.EntryHTML = &sanitized
	} else {
		note.EntryHTML = nil
	}
	note.LastEditedBy = &userID

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, note.ID)
}

// createVersionSnapshot saves the current note state as a version record.
// Errors are swallowed — version tracking is non-critical.
func (s *noteService) createVersionSnapshot(ctx context.Context, note *Note, userID string) {
	v := &NoteVersion{
		ID:        generateID(),
		NoteID:    note.ID,
		UserID:    userID,
		Title:     note.Title,
		Content:   note.Content,
		Entry:     note.Entry,
		EntryHTML: note.EntryHTML,
	}
	_ = s.repo.CreateVersion(ctx, v)
	_ = s.repo.PruneVersions(ctx, note.ID, MaxVersionsPerNote)
}

// --- Attachment Service Methods ---

// ListAttachments returns all attachments for a note.
func (s *noteService) ListAttachments(ctx context.Context, noteID string) ([]NoteAttachment, error) {
	if s.attRepo == nil {
		return nil, apperror.NewInternal(nil)
	}
	return s.attRepo.ListByNote(ctx, noteID)
}

// GetAttachment retrieves a single attachment by ID.
func (s *noteService) GetAttachment(ctx context.Context, id string) (*NoteAttachment, error) {
	if s.attRepo == nil {
		return nil, apperror.NewInternal(nil)
	}
	return s.attRepo.FindAttachmentByID(ctx, id)
}

// CreateAttachment validates and persists a new attachment record.
func (s *noteService) CreateAttachment(ctx context.Context, a *NoteAttachment) error {
	if s.attRepo == nil {
		return apperror.NewInternal(nil)
	}
	if a.ID == "" {
		a.ID = generateID()
	}
	return s.attRepo.CreateAttachment(ctx, a)
}

// DeleteAttachment removes an attachment record and returns the file path for cleanup.
func (s *noteService) DeleteAttachment(ctx context.Context, id string) (string, error) {
	if s.attRepo == nil {
		return "", apperror.NewInternal(nil)
	}
	att, err := s.attRepo.FindAttachmentByID(ctx, id)
	if err != nil {
		return "", err
	}
	if err := s.attRepo.DeleteAttachment(ctx, id); err != nil {
		return "", err
	}
	return att.FilePath, nil
}

// UpdateTranscript sets the transcript text for an attachment.
func (s *noteService) UpdateTranscript(ctx context.Context, id, transcript string) error {
	if s.attRepo == nil {
		return apperror.NewInternal(nil)
	}
	return s.attRepo.UpdateTranscript(ctx, id, transcript)
}

// generateID creates a random 36-char hex string formatted as a UUID-like ID.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
