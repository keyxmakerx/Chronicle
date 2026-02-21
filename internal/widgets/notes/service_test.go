package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockNoteRepo implements NoteRepository for testing.
type mockNoteRepo struct {
	createFn                 func(ctx context.Context, note *Note) error
	findByIDFn               func(ctx context.Context, id string) (*Note, error)
	updateFn                 func(ctx context.Context, note *Note) error
	deleteFn                 func(ctx context.Context, id string) error
	listByUserAndCampaignFn  func(ctx context.Context, userID, campaignID string) ([]Note, error)
	listByEntityFn           func(ctx context.Context, userID, campaignID, entityID string) ([]Note, error)
	listCampaignWideFn       func(ctx context.Context, userID, campaignID string) ([]Note, error)
}

func (m *mockNoteRepo) Create(ctx context.Context, note *Note) error {
	if m.createFn != nil {
		return m.createFn(ctx, note)
	}
	return nil
}

func (m *mockNoteRepo) FindByID(ctx context.Context, id string) (*Note, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("note not found")
}

func (m *mockNoteRepo) Update(ctx context.Context, note *Note) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, note)
	}
	return nil
}

func (m *mockNoteRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockNoteRepo) ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error) {
	if m.listByUserAndCampaignFn != nil {
		return m.listByUserAndCampaignFn(ctx, userID, campaignID)
	}
	return nil, nil
}

func (m *mockNoteRepo) ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error) {
	if m.listByEntityFn != nil {
		return m.listByEntityFn(ctx, userID, campaignID, entityID)
	}
	return nil, nil
}

func (m *mockNoteRepo) ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error) {
	if m.listCampaignWideFn != nil {
		return m.listCampaignWideFn(ctx, userID, campaignID)
	}
	return nil, nil
}

// --- Test Helpers ---

// assertAppError checks that err is an *apperror.AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d, got nil", expectedCode)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// sampleNote creates a note for testing.
func sampleNote() *Note {
	return &Note{
		ID:         "note-123",
		CampaignID: "camp-1",
		UserID:     "user-1",
		Title:      "Test Note",
		Content: []Block{
			{Type: "text", Value: "Hello world"},
			{Type: "checklist", Items: []ChecklistItem{
				{Text: "Item 1", Checked: false},
				{Text: "Item 2", Checked: true},
			}},
		},
		Color:     "#374151",
		Pinned:    false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	var createdNote *Note
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			createdNote = note
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return createdNote, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title:   "My Note",
		Content: []Block{{Type: "text", Value: "Hello"}},
		Color:   "#ff0000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "My Note" {
		t.Errorf("expected title My Note, got %s", note.Title)
	}
	if note.Color != "#ff0000" {
		t.Errorf("expected color #ff0000, got %s", note.Color)
	}
	if note.CampaignID != "camp-1" {
		t.Errorf("expected campaign camp-1, got %s", note.CampaignID)
	}
	if note.UserID != "user-1" {
		t.Errorf("expected user user-1, got %s", note.UserID)
	}
	if note.ID == "" {
		t.Error("expected ID to be generated")
	}
}

func TestCreate_EmptyTitleDefaultsToUntitled(t *testing.T) {
	var capturedTitle string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedTitle = note.Title
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Title: capturedTitle}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestCreate_WhitespaceOnlyTitleDefaultsToUntitled(t *testing.T) {
	var capturedTitle string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedTitle = note.Title
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Title: capturedTitle}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title: "   ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestCreate_TitleTooLong(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	longTitle := string(make([]byte, 201))
	for i := range longTitle {
		longTitle = longTitle[:i] + "a" + longTitle[i+1:]
	}
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title: longTitle,
	})
	assertAppError(t, err, 400)
}

func TestCreate_DefaultColor(t *testing.T) {
	var capturedColor string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedColor = note.Color
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Color: capturedColor}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title: "Test",
		Color: "", // Empty should default.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Color != "#374151" {
		t.Errorf("expected default color #374151, got %s", note.Color)
	}
}

func TestCreate_NilContentDefaultsToEmpty(t *testing.T) {
	var capturedContent []Block
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedContent = note.Content
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Content: capturedContent}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title:   "Test",
		Content: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Content == nil {
		t.Error("expected non-nil content slice")
	}
	if len(note.Content) != 0 {
		t.Errorf("expected empty content, got %d blocks", len(note.Content))
	}
}

func TestCreate_WithEntityScope(t *testing.T) {
	var capturedEntityID *string
	entityID := "entity-42"
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedEntityID = note.EntityID
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, EntityID: capturedEntityID}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title:    "Test",
		EntityID: &entityID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.EntityID == nil || *note.EntityID != "entity-42" {
		t.Errorf("expected entity ID entity-42, got %v", note.EntityID)
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			return errors.New("db error")
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateNoteRequest{
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newTitle := "Updated Title"
	pinned := true
	note, err := svc.Update(context.Background(), "note-123", UpdateNoteRequest{
		Title:  &newTitle,
		Pinned: &pinned,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Updated Title" {
		t.Errorf("expected Updated Title, got %s", note.Title)
	}
	if !note.Pinned {
		t.Error("expected pinned to be true")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	newTitle := "Test"
	_, err := svc.Update(context.Background(), "nonexistent", UpdateNoteRequest{
		Title: &newTitle,
	})
	assertAppError(t, err, 404)
}

func TestUpdate_TitleTooLong(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	longTitle := string(make([]byte, 201))
	for i := range longTitle {
		longTitle = longTitle[:i] + "x" + longTitle[i+1:]
	}
	_, err := svc.Update(context.Background(), "note-123", UpdateNoteRequest{
		Title: &longTitle,
	})
	assertAppError(t, err, 400)
}

func TestUpdate_EmptyTitleBecomesUntitled(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	emptyTitle := ""
	note, err := svc.Update(context.Background(), "note-123", UpdateNoteRequest{
		Title: &emptyTitle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestUpdate_ContentReplacement(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newContent := []Block{{Type: "text", Value: "Replaced"}}
	note, err := svc.Update(context.Background(), "note-123", UpdateNoteRequest{
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(note.Content) != 1 || note.Content[0].Value != "Replaced" {
		t.Errorf("expected replaced content, got %v", note.Content)
	}
}

func TestUpdate_ColorChange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newColor := "#ff5733"
	note, err := svc.Update(context.Background(), "note-123", UpdateNoteRequest{
		Color: &newColor,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Color != "#ff5733" {
		t.Errorf("expected #ff5733, got %s", note.Color)
	}
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	var deletedID string
	repo := &mockNoteRepo{
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	svc := NewNoteService(repo)
	err := svc.Delete(context.Background(), "note-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != "note-123" {
		t.Errorf("expected note-123, got %s", deletedID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockNoteRepo{
		deleteFn: func(ctx context.Context, id string) error {
			return apperror.NewNotFound("not found")
		},
	}

	svc := NewNoteService(repo)
	err := svc.Delete(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- ToggleCheck Tests ---

func TestToggleCheck_Success(t *testing.T) {
	existing := sampleNote()
	// Item 0 starts as unchecked.
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1, // The checklist block.
		ItemIndex:  0, // First item (unchecked -> checked).
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !note.Content[1].Items[0].Checked {
		t.Error("expected item 0 to be toggled to checked")
	}
}

func TestToggleCheck_ToggleCheckedToUnchecked(t *testing.T) {
	existing := sampleNote()
	// Item 1 starts as checked.
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1,
		ItemIndex:  1, // Second item (checked -> unchecked).
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Content[1].Items[1].Checked {
		t.Error("expected item 1 to be toggled to unchecked")
	}
}

func TestToggleCheck_BlockIndexOutOfRange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 99,
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NegativeBlockIndex(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: -1,
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NotChecklist(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 0, // Text block, not checklist.
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_ItemIndexOutOfRange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1,
		ItemIndex:  99,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NoteNotFound(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	_, err := svc.ToggleCheck(context.Background(), "nonexistent", ToggleCheckRequest{
		BlockIndex: 0,
		ItemIndex:  0,
	})
	assertAppError(t, err, 404)
}

// --- List Tests ---

func TestListByUserAndCampaign(t *testing.T) {
	repo := &mockNoteRepo{
		listByUserAndCampaignFn: func(ctx context.Context, userID, campaignID string) ([]Note, error) {
			return []Note{
				{ID: "1", Title: "Note 1"},
				{ID: "2", Title: "Note 2"},
			}, nil
		},
	}

	svc := NewNoteService(repo)
	notes, err := svc.ListByUserAndCampaign(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
}

func TestListByEntity(t *testing.T) {
	repo := &mockNoteRepo{
		listByEntityFn: func(ctx context.Context, userID, campaignID, entityID string) ([]Note, error) {
			if entityID != "entity-42" {
				t.Errorf("expected entity-42, got %s", entityID)
			}
			return []Note{{ID: "1"}}, nil
		},
	}

	svc := NewNoteService(repo)
	notes, err := svc.ListByEntity(context.Background(), "user-1", "camp-1", "entity-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}

func TestListCampaignWide(t *testing.T) {
	repo := &mockNoteRepo{
		listCampaignWideFn: func(ctx context.Context, userID, campaignID string) ([]Note, error) {
			return []Note{{ID: "1"}, {ID: "2"}, {ID: "3"}}, nil
		},
	}

	svc := NewNoteService(repo)
	notes, err := svc.ListCampaignWide(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 3 {
		t.Errorf("expected 3 notes, got %d", len(notes))
	}
}

// --- generateID Tests ---

func TestGenerateID_Format(t *testing.T) {
	id := generateID()
	// Should be UUID-like format: 8-4-4-4-12 = 36 chars.
	if len(id) != 36 {
		t.Errorf("expected 36-char ID, got %d chars: %s", len(id), id)
	}
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("expected UUID-like format, got %s", id)
	}
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID()
		if seen[id] {
			t.Fatalf("ID collision after %d iterations", i)
		}
		seen[id] = true
	}
}
