package posts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockPostRepo struct {
	createFn    func(ctx context.Context, post *Post) error
	findByIDFn  func(ctx context.Context, id string) (*Post, error)
	listFn      func(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error)
	updateFn    func(ctx context.Context, post *Post) error
	deleteFn    func(ctx context.Context, id string) error
	reorderFn   func(ctx context.Context, entityID string, postIDs []string) error
}

func (m *mockPostRepo) Create(ctx context.Context, post *Post) error {
	if m.createFn != nil {
		return m.createFn(ctx, post)
	}
	return nil
}

func (m *mockPostRepo) FindByID(ctx context.Context, id string) (*Post, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return &Post{ID: id, CampaignID: "camp-1", EntityID: "ent-1", Name: "Test Post"}, nil
}

func (m *mockPostRepo) ListByEntity(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error) {
	if m.listFn != nil {
		return m.listFn(ctx, entityID, includeDMOnly)
	}
	return nil, nil
}

func (m *mockPostRepo) Update(ctx context.Context, post *Post) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, post)
	}
	return nil
}

func (m *mockPostRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockPostRepo) Reorder(ctx context.Context, entityID string, postIDs []string) error {
	if m.reorderFn != nil {
		return m.reorderFn(ctx, entityID, postIDs)
	}
	return nil
}

// --- Test Helpers ---

func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	post, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", "Test Post", CreatePostRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Name != "Test Post" {
		t.Errorf("expected name 'Test Post', got %q", post.Name)
	}
	if post.EntityID != "ent-1" {
		t.Errorf("expected entity ID 'ent-1', got %q", post.EntityID)
	}
	if post.ID == "" {
		t.Error("expected post ID to be set")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", "", CreatePostRequest{})
	assertAppError(t, err, 400)
}

func TestCreate_WhitespaceOnlyName(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", "   ", CreatePostRequest{})
	assertAppError(t, err, 400)
}

func TestCreate_LongName(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	longName := ""
	for i := 0; i < 201; i++ {
		longName += "a"
	}

	_, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", longName, CreatePostRequest{})
	assertAppError(t, err, 400)
}

func TestCreate_SetsNextSortOrder(t *testing.T) {
	repo := &mockPostRepo{
		listFn: func(_ context.Context, _ string, _ bool) ([]Post, error) {
			return []Post{{ID: "p-1"}, {ID: "p-2"}}, nil
		},
	}
	svc := NewPostService(repo)

	post, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", "Third Post", CreatePostRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.SortOrder != 2 {
		t.Errorf("expected sort order 2, got %d", post.SortOrder)
	}
}

func TestCreate_WithContent(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	entry := json.RawMessage(`{"type":"doc","content":[]}`)
	html := "<p>Hello</p>"

	post, err := svc.Create(context.Background(), "camp-1", "ent-1", "user-1", "With Content", CreatePostRequest{
		Entry:     entry,
		EntryHTML: &html,
		IsPrivate: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !post.IsPrivate {
		t.Error("expected post to be private")
	}
	if post.EntryHTML == nil || *post.EntryHTML != html {
		t.Error("expected entry HTML to be set")
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	name := "Updated Name"
	post, err := svc.Update(context.Background(), "post-1", UpdatePostRequest{Name: &name})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", post.Name)
	}
}

func TestUpdate_EmptyName(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	empty := ""
	_, err := svc.Update(context.Background(), "post-1", UpdatePostRequest{Name: &empty})
	assertAppError(t, err, 400)
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockPostRepo{
		findByIDFn: func(_ context.Context, _ string) (*Post, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewPostService(repo)

	name := "Test"
	_, err := svc.Update(context.Background(), "missing", UpdatePostRequest{Name: &name})
	assertAppError(t, err, 404)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	err := svc.Delete(context.Background(), "post-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockPostRepo{
		findByIDFn: func(_ context.Context, _ string) (*Post, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewPostService(repo)

	err := svc.Delete(context.Background(), "missing")
	assertAppError(t, err, 404)
}

// --- Reorder Tests ---

func TestReorder_Success(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	err := svc.Reorder(context.Background(), "ent-1", []string{"p-1", "p-2", "p-3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReorder_EmptyIDs(t *testing.T) {
	repo := &mockPostRepo{}
	svc := NewPostService(repo)

	err := svc.Reorder(context.Background(), "ent-1", []string{})
	assertAppError(t, err, 400)
}
