package posts

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PostService defines the business logic interface for entity posts.
type PostService interface {
	// Create creates a new post attached to an entity.
	Create(ctx context.Context, campaignID, entityID, userID, name string, req CreatePostRequest) (*Post, error)
	// GetByID returns a post by ID.
	GetByID(ctx context.Context, id string) (*Post, error)
	// ListByEntity returns all posts for an entity, filtering DM-only based on role.
	ListByEntity(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error)
	// Update modifies a post's name, content, or visibility.
	Update(ctx context.Context, id string, req UpdatePostRequest) (*Post, error)
	// Delete removes a post.
	Delete(ctx context.Context, id string) error
	// Reorder updates the sort order for posts within an entity.
	Reorder(ctx context.Context, entityID string, postIDs []string) error
}

type postService struct {
	repo PostRepository
}

// NewPostService creates a new post service backed by the given repository.
func NewPostService(repo PostRepository) PostService {
	return &postService{repo: repo}
}

func (s *postService) Create(ctx context.Context, campaignID, entityID, userID, name string, req CreatePostRequest) (*Post, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewBadRequest("post name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("post name must be 200 characters or less")
	}

	// Determine next sort order by counting existing posts.
	existing, err := s.repo.ListByEntity(ctx, entityID, true)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	post := &Post{
		ID:         uuid.New().String(),
		EntityID:   entityID,
		CampaignID: campaignID,
		Name:       name,
		Entry:      req.Entry,
		EntryHTML:  req.EntryHTML,
		IsPrivate:  req.IsPrivate,
		SortOrder:  len(existing),
		CreatedBy:  userID,
	}

	if err := s.repo.Create(ctx, post); err != nil {
		return nil, apperror.NewInternal(err)
	}

	slog.Info("entity post created", "post_id", post.ID, "entity_id", entityID, "name", name)
	return post, nil
}

func (s *postService) GetByID(ctx context.Context, id string) (*Post, error) {
	post, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, apperror.NewNotFound("post not found")
	}
	return post, nil
}

func (s *postService) ListByEntity(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error) {
	posts, err := s.repo.ListByEntity(ctx, entityID, includeDMOnly)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return posts, nil
}

func (s *postService) Update(ctx context.Context, id string, req UpdatePostRequest) (*Post, error) {
	post, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, apperror.NewNotFound("post not found")
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, apperror.NewBadRequest("post name is required")
		}
		if len(name) > 200 {
			return nil, apperror.NewBadRequest("post name must be 200 characters or less")
		}
		post.Name = name
	}
	if req.Entry != nil {
		post.Entry = req.Entry
	}
	if req.EntryHTML != nil {
		post.EntryHTML = req.EntryHTML
	}
	if req.IsPrivate != nil {
		post.IsPrivate = *req.IsPrivate
	}

	if err := s.repo.Update(ctx, post); err != nil {
		return nil, apperror.NewInternal(err)
	}

	return post, nil
}

func (s *postService) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return apperror.NewNotFound("post not found")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.NewInternal(err)
	}

	slog.Info("entity post deleted", "post_id", id)
	return nil
}

func (s *postService) Reorder(ctx context.Context, entityID string, postIDs []string) error {
	if len(postIDs) == 0 {
		return apperror.NewBadRequest("post IDs are required")
	}

	if err := s.repo.Reorder(ctx, entityID, postIDs); err != nil {
		return apperror.NewInternal(err)
	}

	return nil
}
