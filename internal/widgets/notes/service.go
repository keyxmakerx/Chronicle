package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// NoteService defines the business logic contract for notes.
type NoteService interface {
	Create(ctx context.Context, campaignID, userID string, req CreateNoteRequest) (*Note, error)
	GetByID(ctx context.Context, id string) (*Note, error)
	Update(ctx context.Context, id string, req UpdateNoteRequest) (*Note, error)
	Delete(ctx context.Context, id string) error
	ToggleCheck(ctx context.Context, id string, req ToggleCheckRequest) (*Note, error)

	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error)
	ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error)
	ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error)
}

// noteService implements NoteService.
type noteService struct {
	repo NoteRepository
}

// NewNoteService creates a new note service.
func NewNoteService(repo NoteRepository) NoteService {
	return &noteService{repo: repo}
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
		ID:         generateID(),
		CampaignID: campaignID,
		UserID:     userID,
		EntityID:   req.EntityID,
		Title:      title,
		Content:    content,
		Color:      color,
	}

	if err := s.repo.Create(ctx, note); err != nil {
		return nil, err
	}

	return s.repo.FindByID(ctx, note.ID)
}

// GetByID retrieves a note by ID.
func (s *noteService) GetByID(ctx context.Context, id string) (*Note, error) {
	return s.repo.FindByID(ctx, id)
}

// Update applies partial updates to a note.
func (s *noteService) Update(ctx context.Context, id string, req UpdateNoteRequest) (*Note, error) {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

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
	if req.Color != nil {
		note.Color = *req.Color
	}
	if req.Pinned != nil {
		note.Pinned = *req.Pinned
	}

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, note.ID)
}

// Delete removes a note.
func (s *noteService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
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

// ListByUserAndCampaign returns all notes for a user in a campaign.
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

// generateID creates a random 36-char hex string formatted as a UUID-like ID.
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
